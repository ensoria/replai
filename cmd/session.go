package cmd

import (
	"encoding/json"
	"time"

	"github.com/spf13/cobra"

	"github.com/ensoria/replai/internal/engine"
	"github.com/ensoria/replai/internal/session"
)

const reExecutionNote = "session entries are re-executed before every eval with output suppressed; side effects (DB writes, HTTP calls) repeat each time"

var sessionCmd = &cobra.Command{
	Use:   "session",
	Short: "Stateful evaluation: variables and imports persist across separate replai invocations",
}

var sessionStartCmd = &cobra.Command{
	Use:   "start",
	Short: "Create a session and print its id",
	Args:  cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		_, p, err := newEngine()
		if err != nil {
			failUsage(err.Error(), "")
			return
		}
		st, err := sessionStore(p).Create(p.Root)
		if err != nil {
			failUsage(err.Error(), "")
			return
		}
		printJSON(&sessionStartOut{OK: true, SessionID: st.ID, Project: p.Root, Note: reExecutionNote})
	},
}

type sessionStartOut struct {
	OK        bool   `json:"ok"`
	SessionID string `json:"session_id"`
	Project   string `json:"project"`
	Note      string `json:"note"`
}

var sessionEvalCmd = &cobra.Command{
	Use:   "eval <session-id> [code]",
	Short: "Evaluate a snippet with the session state (variables, imports) applied",
	Args:  cobra.RangeArgs(1, 2),
	Run: func(cmd *cobra.Command, args []string) {
		input, ok := resolveInput(args[1:])
		if !ok {
			return
		}
		eng, p, err := newEngine()
		if err != nil {
			failUsage(err.Error(), "")
			return
		}
		store := sessionStore(p)
		id := args[0]
		var out *engine.Outcome
		err = store.WithLockAndLog(id, func(st *session.State) (*session.LogRecord, error) {
			out = eng.Eval(st, input)
			return &session.LogRecord{
				Time:   time.Now().UTC(),
				Input:  input,
				Output: json.RawMessage(out.Output),
			}, nil
		})
		if err != nil {
			failUsage(err.Error(), "")
			return
		}
		printOutcome(out)
	},
}

var sessionVarsCmd = &cobra.Command{
	Use:   "vars <session-id>",
	Short: "List session variables with their types",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		eng, p, err := newEngine()
		if err != nil {
			failUsage(err.Error(), "")
			return
		}
		store := sessionStore(p)
		st, err := store.Load(args[0])
		if err != nil {
			failUsage(err.Error(), "")
			return
		}
		printOutcome(eng.Eval(st, ":vars"))
	},
}

var sessionLogCmd = &cobra.Command{
	Use:   "log <session-id>",
	Short: "Replay the session log (every input with its output envelope)",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		_, p, err := newEngine()
		if err != nil {
			failUsage(err.Error(), "")
			return
		}
		store := sessionStore(p)
		if _, err := store.Load(args[0]); err != nil {
			failUsage(err.Error(), "")
			return
		}
		records, err := store.ReadLog(args[0])
		if err != nil {
			failUsage(err.Error(), "")
			return
		}
		printJSON(&sessionLogOut{OK: true, SessionID: args[0], Log: records})
	},
}

type sessionLogOut struct {
	OK        bool              `json:"ok"`
	SessionID string            `json:"session_id"`
	Log       []json.RawMessage `json:"log"`
}

var sessionEndCmd = &cobra.Command{
	Use:   "end <session-id>",
	Short: "Delete a session and its log",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		_, p, err := newEngine()
		if err != nil {
			failUsage(err.Error(), "")
			return
		}
		if err := sessionStore(p).Delete(args[0]); err != nil {
			failUsage(err.Error(), "")
			return
		}
		printJSON(&sessionEndOut{OK: true, Ended: args[0]})
	},
}

type sessionEndOut struct {
	OK    bool   `json:"ok"`
	Ended string `json:"ended"`
}

func init() {
	sessionCmd.AddCommand(sessionStartCmd, sessionEvalCmd, sessionVarsCmd, sessionLogCmd, sessionEndCmd)
}
