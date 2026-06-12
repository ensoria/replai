package engine

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ensoria/replai/internal/codegen"
	"github.com/ensoria/replai/internal/envelope"
	"github.com/ensoria/replai/internal/errmap"
	"github.com/ensoria/replai/internal/meta"
	"github.com/ensoria/replai/internal/parse"
	"github.com/ensoria/replai/internal/project"
	"github.com/ensoria/replai/internal/runner"
	"github.com/ensoria/replai/internal/session"
	"github.com/ensoria/replai/internal/suggest"
)

// Exit codes per spec §4.2.
const (
	ExitOK      = 0
	ExitEval    = 1
	ExitUsage   = 2
	ExitTimeout = 124
)

const buildTimeout = 5 * time.Minute

// voidValueMarker is the compiler wording for calling a void function in
// value position; it triggers the transparent statement retry.
const voidValueMarker = "used as value"

// Compiler wordings for an import not satisfied by go.mod; they trigger the
// sibling-workspace retry.
const (
	missingModuleMarker    = "no required module provides package"
	cannotFindModuleMarker = "cannot find module providing package"
)

// Options carries evaluation limits, mirrored from CLI flags.
type Options struct {
	Timeout   time.Duration
	Depth     int
	MaxItems  int
	MaxStr    int
	MaxOutput int
	MaxMem    string
	Restrict  bool
}

// Imports blocked under --restrict. Static and best-effort by design: true
// sandboxing is out of scope per spec §8.
var restrictedImports = []string{"os", "os/exec", "syscall", "net"}

// Engine evaluates snippets against a project, optionally continuing a
// session state.
type Engine struct {
	Project *project.Project
	Opts    *Options
}

// New returns an engine for the project. A previously generated sibling
// workspace is exported to the process environment so import resolution
// (goimports, go/packages) sees the same module graph as the build.
func New(p *project.Project, opts *Options) *Engine {
	applyWorkEnv(p)
	return &Engine{Project: p, Opts: opts}
}

func missingModule(msg string) bool {
	return strings.Contains(msg, missingModuleMarker) || strings.Contains(msg, cannotFindModuleMarker)
}

func applyWorkEnv(p *project.Project) {
	for _, kv := range p.WorkEnv() {
		if i := strings.IndexByte(kv, '='); i > 0 {
			_ = os.Setenv(kv[:i], kv[i+1:])
		}
	}
}

// Outcome is the printable result of one evaluation.
type Outcome struct {
	Output   []byte
	ExitCode int
}

// Eval classifies and evaluates one snippet. st may be nil for one-shot
// evaluation; when non-nil it is mutated (entries appended) on success.
func (e *Engine) Eval(st *session.State, input string) *Outcome {
	vars := map[string]bool{}
	if st != nil {
		vars = st.VarNames()
	}
	sn, err := parse.Classify(input, vars)
	if err != nil {
		return e.parseFailure(err)
	}
	if sn.Kind == parse.KindMeta {
		return e.runMeta(st, sn.Meta)
	}
	env, exit := e.evalSnippet(st, sn, false)
	if exit == ExitOK && st != nil {
		persist(st, sn, env)
		// Resolved auto-imports become session imports so replays and later
		// suggestions see a stable import set.
		st.MergeImports(env.FinalImports)
	}
	return &Outcome{Output: env.Marshal(e.Opts.MaxOutput), ExitCode: exit}
}

func (e *Engine) runMeta(st *session.State, command string) *Outcome {
	out, exit := meta.Run(e.Project, st, command)
	out = envelope.MarshalRaw(out, e.Opts.MaxOutput)
	return &Outcome{Output: out, ExitCode: exit}
}

func (e *Engine) parseFailure(err error) *Outcome {
	env := envelope.NewError(envelope.KindCompile, err.Error())
	if pe, ok := err.(*parse.Error); ok {
		env.Error.Message = pe.Msg
		env.Error.Position = &envelope.Position{Line: pe.Line, Column: pe.Column}
	}
	return &Outcome{Output: env.Marshal(e.Opts.MaxOutput), ExitCode: ExitEval}
}

func (e *Engine) evalSnippet(st *session.State, sn *parse.Snippet, retried bool) (*envelope.Envelope, int) {
	res, err := codegen.Generate(e.Project, st, sn)
	if ce, ok := err.(*codegen.CompileError); ok && missingModule(ce.Msg) {
		// A sibling monorepo module may provide the import: generate
		// .replai/go.work once and regenerate with workspace resolution.
		if created, werr := e.Project.EnsureSiblingWork(); werr == nil && created {
			applyWorkEnv(e.Project)
			res, err = codegen.Generate(e.Project, st, sn)
		}
	}
	if err != nil {
		if ce, ok := err.(*codegen.CompileError); ok && res != nil {
			return e.buildFailure(st, sn, res, ce.Msg), ExitEval
		}
		return envelope.NewError(envelope.KindInternal, err.Error()), ExitUsage
	}

	if e.Opts.Restrict {
		if blocked := e.restrictedImport(res.Imports); blocked != "" {
			env := envelope.NewError(envelope.KindInternal,
				fmt.Sprintf("import %q is blocked by --restrict (static check; blocked: %s)", blocked, strings.Join(restrictedImports, ", ")))
			env.Error.Suggestion = "drop --restrict to allow file/network/exec access"
			return env, ExitUsage
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), buildTimeout)
	defer cancel()
	buildErr := runner.Build(ctx, e.Project.Root, res.FilePaths, e.Project.RunnerBin(), e.Project.WorkEnv())
	if be, ok := buildErr.(*runner.BuildError); ok && missingModule(be.Stderr) {
		// A sibling monorepo module may provide the package: generate
		// .replai/go.work once and retry. Later builds pick it up via WorkEnv.
		if created, werr := e.Project.EnsureSiblingWork(); werr == nil && created {
			applyWorkEnv(e.Project)
			buildErr = runner.Build(ctx, e.Project.Root, res.FilePaths, e.Project.RunnerBin(), e.Project.WorkEnv())
		}
	}
	if err := buildErr; err != nil {
		be, ok := err.(*runner.BuildError)
		if !ok {
			return envelope.NewError(envelope.KindInternal, err.Error()), ExitUsage
		}
		// Transparent retry: a void call cannot be captured as a value, so
		// re-evaluate the snippet as statements.
		captured := sn.Kind == parse.KindExpr || sn.TailExpr != ""
		if !retried && captured && strings.Contains(be.Stderr, voidValueMarker) {
			vars := map[string]bool{}
			if st != nil {
				vars = st.VarNames()
			}
			if stmtSn, perr := parse.AsStmts(fullSource(sn), vars); perr == nil {
				stmtSn.Imports = sn.Imports
				env, exit := e.evalSnippet(st, stmtSn, true)
				if exit == ExitOK {
					*sn = *stmtSn
				}
				return env, exit
			}
		}
		return e.buildFailure(st, sn, res, be.Stderr), ExitEval
	}

	run, err := runner.Run(e.Project.Root, e.Project.RunnerBin(), e.runEnv(), e.Opts.Timeout)
	if err != nil {
		return envelope.NewError(envelope.KindInternal, err.Error()), ExitUsage
	}
	return e.assemble(sn, res, run)
}

func (e *Engine) restrictedImport(imports map[string]string) string {
	for _, path := range imports {
		for _, blocked := range restrictedImports {
			if path == blocked || strings.HasPrefix(path, blocked+"/") {
				return path
			}
		}
	}
	return ""
}

func (e *Engine) runEnv() []string {
	env := []string{
		fmt.Sprintf("%s=%d", "REPLAI_DEPTH", e.Opts.Depth),
		fmt.Sprintf("%s=%d", "REPLAI_MAX_ITEMS", e.Opts.MaxItems),
		fmt.Sprintf("%s=%d", "REPLAI_MAX_STR", e.Opts.MaxStr),
	}
	if e.Opts.MaxMem != "" {
		env = append(env, "GOMEMLIMIT="+e.Opts.MaxMem)
	}
	return env
}

func (e *Engine) buildFailure(st *session.State, sn *parse.Snippet, res *codegen.Result, stderr string) *envelope.Envelope {
	errs := errmap.MapBuildErrors(stderr, res.MainPath, res.LineMap)
	primary := errmap.Primary(errs)
	if primary == nil {
		return envelope.NewError(envelope.KindCompile, strings.TrimSpace(stderr))
	}

	env := envelope.NewError(envelope.KindCompile, primary.Msg)
	switch primary.Origin {
	case errmap.OriginSnippet:
		env.Error.Position = &envelope.Position{Line: primary.Line, Column: primary.Column}
	case errmap.OriginEntry:
		env.Error.Message = fmt.Sprintf("in session entry %d (%s): %s",
			primary.Entry, entrySummary(st, primary.Entry), primary.Msg)
		env.Error.Suggestion = "a prior session entry no longer compiles; fix it by redefining, or use :reset to clear the session"
	}
	if env.Error.Suggestion == "" {
		env.Error.Suggestion = suggest.ForBuildError(e.Project.Root, primary.Msg, res.Imports, e.localNames(st, sn))
	}
	return env
}

func (e *Engine) localNames(st *session.State, sn *parse.Snippet) []string {
	var names []string
	if st != nil {
		for name := range st.VarNames() {
			names = append(names, name)
		}
		for _, entry := range st.Entries {
			names = append(names, entry.DeclNames...)
		}
	}
	if sn != nil {
		names = append(names, sn.Defined...)
		names = append(names, sn.DeclNames...)
	}
	return names
}

func entrySummary(st *session.State, idx int) string {
	if st == nil || idx < 0 || idx >= len(st.Entries) {
		return "?"
	}
	src := st.Entries[idx].Source
	if i := strings.IndexByte(src, '\n'); i > 0 {
		src = src[:i] + " ..."
	}
	return "`" + src + "`"
}

func (e *Engine) assemble(sn *parse.Snippet, res *codegen.Result, run *runner.Result) (*envelope.Envelope, int) {
	env := envelope.New()
	env.Stdout = run.Stdout
	env.Stderr = run.Stderr
	env.AutoImports = res.AutoImports
	env.FinalImports = res.FinalImports

	if run.TimedOut {
		env.Error = &envelope.Error{
			Kind:       envelope.KindTimeout,
			Message:    fmt.Sprintf("evaluation timed out after %s and the process group was killed", e.Opts.Timeout),
			Suggestion: "raise --timeout, or check the snippet for blocking calls (locks, channel reads, infinite loops); partial stdout/stderr is included",
		}
		return env, ExitTimeout
	}

	if run.Wire == nil {
		msg, stack := errmap.CleanCrash(run.Diag+run.Stderr, res.MainPath, res.LineMap)
		if msg == "" {
			msg = fmt.Sprintf("process exited with code %d before reporting a result", run.ExitCode)
		}
		env.Error = &envelope.Error{Kind: envelope.KindRuntime, Message: msg, Stack: stack}
		return env, ExitEval
	}

	env.DurationMS = run.Wire.DurationMS
	env.DefinedTypes = run.Wire.Defined
	for _, d := range run.Wire.Defined {
		env.Defined = append(env.Defined, d.Name)
	}
	env.Defined = append(env.Defined, sn.DeclNames...)

	if run.Wire.Panic != nil {
		env.Error = &envelope.Error{
			Kind:    envelope.KindPanic,
			Message: "panic: " + run.Wire.Panic.Value,
			Stack:   run.Wire.Panic.Stack,
		}
		return env, ExitEval
	}

	env.OK = true
	switch len(run.Wire.Values) {
	case 0:
	case 1:
		env.Value = run.Wire.Values[0]
	default:
		env.Values = run.Wire.Values
	}
	return env, ExitOK
}

// fullSource reassembles the snippet source including a split-off tail
// expression.
func fullSource(sn *parse.Snippet) string {
	if sn.TailExpr == "" {
		return sn.Source
	}
	return sn.Source + "\n" + sn.TailExpr
}

// persist appends the evaluated snippet to the session so later evals replay
// it. Every successful snippet is kept (including expressions, replayed via
// replaiDiscard) so heap mutations remain visible to subsequent evals.
func persist(st *session.State, sn *parse.Snippet, env *envelope.Envelope) {
	st.MergeImports(sn.Imports)
	if sn.Kind == parse.KindImportOnly {
		return
	}
	entry := &session.Entry{Kind: sn.Kind.String(), Source: sn.Source, DeclNames: sn.DeclNames}
	if len(sn.Defined) > 0 {
		entry.Defined = map[string]string{}
		typeOf := map[string]string{}
		for _, v := range env.DefinedTypes {
			typeOf[v.Name] = v.Type
		}
		for _, name := range sn.Defined {
			entry.Defined[name] = typeOf[name]
		}
	}
	st.Entries = append(st.Entries, entry)
	// A split-off tail expression is replayed separately for its side effects.
	if sn.TailExpr != "" {
		st.Entries = append(st.Entries, &session.Entry{Kind: parse.KindExpr.String(), Source: sn.TailExpr})
	}
}
