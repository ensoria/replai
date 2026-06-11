package cmd

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/ensoria/replai/internal/session"
)

const replBufferSize = 1024 * 1024

var replCmd = &cobra.Command{
	Use:   "repl",
	Short: "Interactive loop (human fallback): reads snippets from stdin, prints one envelope per snippet",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		eng, p, err := newEngine()
		if err != nil {
			failUsage(err.Error(), "")
			return
		}
		// In-memory session: state survives within the loop, not across runs.
		st := &session.State{ID: "repl", CreatedAt: time.Now().UTC(), ProjectDir: p.Root, Entries: []*session.Entry{}}

		scanner := bufio.NewScanner(os.Stdin)
		scanner.Buffer(make([]byte, 0, replBufferSize), replBufferSize)
		var buf []string
		for scanner.Scan() {
			buf = append(buf, scanner.Text())
			input := strings.Join(buf, "\n")
			if strings.TrimSpace(input) == "" || !balanced(input) {
				continue
			}
			buf = nil
			out := eng.Eval(st, input)
			fmt.Fprintln(os.Stdout, string(out.Output))
		}
	},
}

// balanced reports whether all braces, brackets, and parens outside string
// and rune literals are closed, signaling a complete snippet.
func balanced(src string) bool {
	depth := 0
	var quote byte
	escaped := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		if quote != 0 {
			switch {
			case escaped:
				escaped = false
			case c == '\\' && quote != '`':
				escaped = true
			case c == quote:
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			quote = c
		case '{', '(', '[':
			depth++
		case '}', ')', ']':
			depth--
		case '/':
			if i+1 < len(src) && src[i+1] == '/' {
				// Skip line comments.
				for i < len(src) && src[i] != '\n' {
					i++
				}
			}
		}
	}
	return depth <= 0 && quote != '`'
}
