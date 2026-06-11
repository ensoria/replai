package cmd

import (
	"io"
	"os"

	"github.com/spf13/cobra"
)

const stdinMarker = "-"

var flagEvalFile string

var evalCmd = &cobra.Command{
	Use:   "eval [code]",
	Short: "Evaluate one snippet (expression, statements, or declarations) and print the result envelope",
	Args:  cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		input, ok := resolveInput(args)
		if !ok {
			return
		}
		eng, _, err := newEngine()
		if err != nil {
			failUsage(err.Error(), "")
			return
		}
		printOutcome(eng.Eval(nil, input))
	},
}

func init() {
	evalCmd.Flags().StringVarP(&flagEvalFile, "file", "f", "", "read the snippet from a file")
}

// resolveInput picks the snippet source: -f FILE, "-" for stdin, or the
// positional argument.
func resolveInput(args []string) (string, bool) {
	if flagEvalFile != "" {
		data, err := os.ReadFile(flagEvalFile)
		if err != nil {
			failUsage(err.Error(), "")
			return "", false
		}
		return string(data), true
	}
	if len(args) == 0 {
		failUsage("no code given", "pass code as an argument, use -f FILE, or pipe code with `replai eval -`")
		return "", false
	}
	if args[0] == stdinMarker {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			failUsage(err.Error(), "")
			return "", false
		}
		return string(data), true
	}
	return args[0], true
}
