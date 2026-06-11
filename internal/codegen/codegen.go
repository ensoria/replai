package codegen

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/imports"

	"github.com/ensoria/replai/internal/evalrt"
	"github.com/ensoria/replai/internal/parse"
	"github.com/ensoria/replai/internal/project"
	"github.com/ensoria/replai/internal/session"
)

const (
	mainFileName = "main.go"
	rtFilePrefix = "zz_replai_"

	evalrtPackageClause = "package evalrt"
	mainPackageClause   = "package main"

	filePerm = 0o644
	dirPerm  = 0o755
)

// Line-map origin markers; non-negative values are session entry indices.
const (
	// EntryCurrent marks lines of the snippet being evaluated now.
	EntryCurrent = -1
	// EntryHarness marks generated harness lines with no user origin.
	EntryHarness = -2
)

// LineRange maps a generated-file line span back to its origin.
type LineRange struct {
	Start    int // first generated line (1-based, inclusive)
	End      int // last generated line (inclusive)
	Entry    int // session entry index, EntryCurrent, or EntryHarness
	SrcStart int // snippet line corresponding to Start
}

// Result describes the generated runner package.
type Result struct {
	Dir          string
	MainPath     string
	FilePaths    []string // all files to hand to `go build`, main.go first
	LineMap      []*LineRange
	AutoImports  []string          // import paths added by goimports resolution
	Imports      map[string]string // package name/alias -> import path (final set)
	FinalImports []*parse.Import   // the import block actually generated
}

// CompileError is a body error surfaced during the goimports pass, already
// expressed in generated-file coordinates compatible with the line map.
type CompileError struct {
	Msg string
}

func (e *CompileError) Error() string { return e.Msg }

// Generate assembles the runner package for the given session state and
// current snippet into <project>/.replai/run.
func Generate(p *project.Project, st *session.State, sn *parse.Snippet) (*Result, error) {
	explicit := mergeImports(st, sn)

	draft, draftMap := assemble(p, st, sn, explicit)
	processed, err := processInDir(p.Root, filepath.Join(p.RunDir(), mainFileName), []byte(draft))
	if err != nil {
		return &Result{MainPath: filepath.Join(p.RunDir(), mainFileName), LineMap: draftMap}, &CompileError{Msg: err.Error()}
	}

	final, autoImports := reconcileImports(explicit, processed)
	src, lineMap := draft, draftMap
	if !sameImports(explicit, final) {
		src, lineMap = assemble(p, st, sn, final)
	}

	res := &Result{
		Dir:          p.RunDir(),
		MainPath:     filepath.Join(p.RunDir(), mainFileName),
		LineMap:      lineMap,
		AutoImports:  autoImports,
		Imports:      importMap(final),
		FinalImports: final,
	}
	if err := writeRunner(p, src, res); err != nil {
		return nil, err
	}
	return res, nil
}

// processInDir runs goimports with the working directory set to the project
// root: the goimports module resolver derives the module context from the
// process cwd, which differs from the project when replai is used as a
// library (tests) or invoked from elsewhere.
func processInDir(dir, filename string, src []byte) ([]byte, error) {
	old, err := os.Getwd()
	if err == nil && old != dir {
		if chErr := os.Chdir(dir); chErr == nil {
			defer func() { _ = os.Chdir(old) }()
		}
	}
	return imports.Process(filename, src, nil)
}

func mergeImports(st *session.State, sn *parse.Snippet) []*parse.Import {
	var out []*parse.Import
	add := func(imp *parse.Import) {
		for _, have := range out {
			if have.Path == imp.Path && have.Alias == imp.Alias {
				return
			}
		}
		out = append(out, imp)
	}
	if st != nil {
		for _, imp := range st.Imports {
			add(imp)
		}
	}
	if sn != nil {
		for _, imp := range sn.Imports {
			add(imp)
		}
	}
	return out
}

// reconcileImports diffs the goimports result against the explicit set:
// additions become auto-imports; explicitly requested imports that goimports
// dropped (unused) are preserved as blank imports for their init side effects.
func reconcileImports(explicit []*parse.Import, processed []byte) ([]*parse.Import, []string) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, mainFileName, processed, parser.ImportsOnly)
	if err != nil || file == nil {
		return explicit, nil
	}
	var final []*parse.Import
	have := map[string]bool{}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		imp := &parse.Import{Path: path}
		if spec.Name != nil {
			imp.Alias = spec.Name.Name
		}
		final = append(final, imp)
		have[path] = true
	}
	explicitPaths := map[string]bool{}
	for _, imp := range explicit {
		explicitPaths[imp.Path] = true
		if !have[imp.Path] {
			final = append(final, &parse.Import{Alias: "_", Path: imp.Path})
			have[imp.Path] = true
		}
	}
	var auto []string
	for _, imp := range final {
		if !explicitPaths[imp.Path] {
			auto = append(auto, imp.Path)
		}
	}
	sort.Strings(auto)
	return final, auto
}

func sameImports(a, b []*parse.Import) bool {
	if len(a) != len(b) {
		return false
	}
	key := func(imps []*parse.Import) string {
		parts := make([]string, len(imps))
		for i, imp := range imps {
			parts[i] = imp.Alias + " " + imp.Path
		}
		sort.Strings(parts)
		return strings.Join(parts, "\n")
	}
	return key(a) == key(b)
}

func importMap(imps []*parse.Import) map[string]string {
	out := map[string]string{}
	for _, imp := range imps {
		name := imp.Alias
		if name == "" {
			name = filepath.Base(imp.Path)
		}
		if name != "_" {
			out[name] = imp.Path
		}
	}
	return out
}

// lineWriter builds the generated source while tracking line numbers.
type lineWriter struct {
	b    strings.Builder
	line int // line number of the next line to be written (1-based)
}

func newLineWriter() *lineWriter { return &lineWriter{line: 1} }

func (w *lineWriter) writeLine(s string) {
	w.b.WriteString(s)
	w.b.WriteByte('\n')
	w.line++
}

// writeBlock writes user source verbatim (no added indentation, so columns in
// compiler errors stay exact) and returns its generated line span.
func (w *lineWriter) writeBlock(src string, suffix string) (start, end int) {
	lines := strings.Split(strings.TrimRight(src, "\n"), "\n")
	start = w.line
	for i, line := range lines {
		if i == len(lines)-1 && suffix != "" {
			line += suffix
		}
		w.writeLine(line)
	}
	return start, w.line - 1
}

// assemble renders the generated main.go and its line map.
func assemble(p *project.Project, st *session.State, sn *parse.Snippet, imps []*parse.Import) (string, []*LineRange) {
	w := newLineWriter()
	var lineMap []*LineRange
	mark := func(start, end, entry, srcStart int) {
		if end >= start {
			lineMap = append(lineMap, &LineRange{Start: start, End: end, Entry: entry, SrcStart: srcStart})
		}
	}

	w.writeLine("// Code generated by replai. DO NOT EDIT.")
	w.writeLine(mainPackageClause)
	w.writeLine("")
	if len(imps) > 0 {
		w.writeLine("import (")
		for _, imp := range imps {
			if imp.Alias != "" {
				w.writeLine("\t" + imp.Alias + " " + strconv.Quote(imp.Path))
			} else {
				w.writeLine("\t" + strconv.Quote(imp.Path))
			}
		}
		w.writeLine(")")
		w.writeLine("")
	}

	var entries []*session.Entry
	if st != nil {
		entries = st.Entries
	}

	// Hoisted declarations from session entries, then from the current snippet.
	for i, e := range entries {
		if e.Kind == parse.KindDecls.String() {
			start, end := w.writeBlock(e.Source, "")
			mark(start, end, i, 1)
			w.writeLine("")
		}
	}
	if sn != nil && sn.Kind == parse.KindDecls {
		start, end := w.writeBlock(sn.Source, "")
		mark(start, end, EntryCurrent, 1)
		w.writeLine("")
	}

	w.writeLine("func replaiBody(rc *replaiCtx) {")
	var useVars []string
	seenVar := map[string]bool{}
	addVars := func(names []string) {
		for _, n := range names {
			if !seenVar[n] {
				seenVar[n] = true
				useVars = append(useVars, n)
			}
		}
	}
	for i, e := range entries {
		switch e.Kind {
		case parse.KindStmts.String():
			start, end := w.writeBlock(e.Source, "")
			mark(start, end, i, 1)
			addVars(sortedKeys(e.Defined))
		case parse.KindExpr.String():
			w.writeLine("replaiDiscard(")
			start, end := w.writeBlock(e.Source, ",")
			mark(start, end, i, 1)
			w.writeLine(")")
		}
	}

	w.writeLine("rc.replaiBegin()")
	if sn != nil {
		switch sn.Kind {
		case parse.KindExpr:
			w.writeLine("replaiCapture(")
			start, end := w.writeBlock(sn.Source, ",")
			mark(start, end, EntryCurrent, 1)
			w.writeLine(")")
		case parse.KindStmts:
			start, end := w.writeBlock(sn.Source, "")
			mark(start, end, EntryCurrent, 1)
			if sn.TailExpr != "" {
				w.writeLine("replaiCapture(")
				ts, te := w.writeBlock(sn.TailExpr, ",")
				mark(ts, te, EntryCurrent, sn.TailLine)
				w.writeLine(")")
			}
		}
	}
	w.writeLine("rc.replaiEnd()")
	if sn != nil && sn.Kind == parse.KindStmts {
		for _, name := range sn.Defined {
			w.writeLine(fmt.Sprintf("rc.replaiDefined(%q, %s)", name, name))
		}
		addVars(sn.Defined)
	}
	if len(useVars) > 0 {
		w.writeLine("replaiUse(" + strings.Join(useVars, ", ") + ")")
	}
	w.writeLine("}")
	w.writeLine("")

	mainPath := filepath.Join(p.RunDir(), mainFileName)
	w.writeLine("const replaiGenFile = " + strconv.Quote(mainPath))
	w.writeLine("")
	w.writeLine("func main() { replaiRun(replaiBody, replaiGenLineMap, replaiGenFile) }")
	w.writeLine("")
	w.writeLine("var replaiGenLineMap = []replaiLineRange{")
	for _, lr := range lineMap {
		if lr.Entry == EntryHarness {
			continue
		}
		w.writeLine(fmt.Sprintf("\t{Start: %d, End: %d, Entry: %d, SrcStart: %d},", lr.Start, lr.End, lr.Entry, lr.SrcStart))
	}
	w.writeLine("}")

	return w.b.String(), lineMap
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// writeRunner clears the run directory and writes main.go plus the rewritten
// evalrt runtime files.
func writeRunner(p *project.Project, mainSrc string, res *Result) error {
	if err := os.RemoveAll(res.Dir); err != nil {
		return err
	}
	if err := os.MkdirAll(res.Dir, dirPerm); err != nil {
		return err
	}
	if err := os.WriteFile(res.MainPath, []byte(mainSrc), filePerm); err != nil {
		return err
	}
	res.FilePaths = []string{res.MainPath}

	sources, err := evalrt.Sources()
	if err != nil {
		return err
	}
	names := make([]string, 0, len(sources))
	for name := range sources {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		content := strings.Replace(string(sources[name]), evalrtPackageClause, mainPackageClause, 1)
		path := filepath.Join(res.Dir, rtFilePrefix+name)
		if err := os.WriteFile(path, []byte(content), filePerm); err != nil {
			return err
		}
		res.FilePaths = append(res.FilePaths, path)
	}
	return nil
}
