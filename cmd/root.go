package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/ensoria/replai/internal/engine"
	"github.com/ensoria/replai/internal/envelope"
	"github.com/ensoria/replai/internal/project"
	"github.com/ensoria/replai/internal/session"
)

// Defaults per spec.
const (
	defaultTimeout   = 30 * time.Second
	defaultDepth     = 5
	defaultMaxItems  = 50
	defaultMaxStr    = 2000
	defaultMaxOutput = 16384
	defaultMaxMem    = "512MiB"
)

var (
	flagTimeout   time.Duration
	flagDepth     int
	flagMaxItems  int
	flagMaxStr    int
	flagMaxOutput int
	flagMaxMem    string
	flagRestrict  bool
	flagJSON      bool
)

// exitCode is set by command runners and returned from Execute.
var exitCode = engine.ExitOK

var rootCmd = &cobra.Command{
	Use:           "replai",
	Short:         "AI-agent-optimized Go REPL: evaluates Go code against the current project and prints exactly one JSON object per invocation",
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute runs the CLI and returns the process exit code.
func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		printEnvelope(envelope.NewError(envelope.KindInternal, err.Error()))
		return engine.ExitUsage
	}
	return exitCode
}

func init() {
	pf := rootCmd.PersistentFlags()
	pf.DurationVar(&flagTimeout, "timeout", defaultTimeout, "kill the evaluation after this duration (exit code 124)")
	pf.IntVar(&flagDepth, "depth", defaultDepth, "max nesting depth expanded in value repr")
	pf.IntVar(&flagMaxItems, "max-items", defaultMaxItems, "max slice/map items shown in value repr")
	pf.IntVar(&flagMaxStr, "max-str", defaultMaxStr, "max string length shown in value repr")
	pf.IntVar(&flagMaxOutput, "max-output", defaultMaxOutput, "max total envelope bytes; overflow is trimmed with markers")
	pf.StringVar(&flagMaxMem, "max-mem", defaultMaxMem, "GOMEMLIMIT for the evaluation process (best effort)")
	pf.BoolVar(&flagRestrict, "restrict", false, "reject snippets importing os, os/exec, net, syscall (static check)")
	pf.BoolVar(&flagJSON, "json", false, "with --help: print command help as JSON")

	defaultHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		if flagJSON {
			printHelpJSON(c)
			return
		}
		defaultHelp(c, args)
	})

	rootCmd.AddCommand(evalCmd, sessionCmd, replCmd, versionCmd)
}

func engineOptions() *engine.Options {
	return &engine.Options{
		Timeout:   flagTimeout,
		Depth:     flagDepth,
		MaxItems:  flagMaxItems,
		MaxStr:    flagMaxStr,
		MaxOutput: flagMaxOutput,
		MaxMem:    flagMaxMem,
		Restrict:  flagRestrict,
	}
}

// newEngine locates the project from the working directory and prepares the
// .replai layout.
func newEngine() (*engine.Engine, *project.Project, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, nil, err
	}
	p, err := project.Find(cwd)
	if err != nil {
		return nil, nil, err
	}
	if err := p.EnsureLayout(); err != nil {
		return nil, nil, err
	}
	return engine.New(p, engineOptions()), p, nil
}

func sessionStore(p *project.Project) *session.Store {
	return session.NewStore(p.SessionsDir())
}

// printOutcome writes the evaluation output and records the exit code.
func printOutcome(out *engine.Outcome) {
	fmt.Fprintln(os.Stdout, string(out.Output))
	exitCode = out.ExitCode
}

// printEnvelope writes an envelope built outside the engine.
func printEnvelope(env *envelope.Envelope) {
	fmt.Fprintln(os.Stdout, string(env.Marshal(flagMaxOutput)))
}

// failUsage reports a replai usage error (exit code 2).
func failUsage(message, suggestion string) {
	env := envelope.NewError(envelope.KindInternal, message)
	env.Error.Suggestion = suggestion
	printEnvelope(env)
	exitCode = engine.ExitUsage
}

// printJSON marshals any fixed-order struct to stdout.
func printJSON(v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		printEnvelope(envelope.NewError(envelope.KindInternal, err.Error()))
		exitCode = engine.ExitUsage
		return
	}
	fmt.Fprintln(os.Stdout, string(data))
}
