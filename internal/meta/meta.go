package meta

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/ensoria/replai/internal/envelope"
	"github.com/ensoria/replai/internal/project"
	"github.com/ensoria/replai/internal/session"
)

// Meta command names.
const (
	cmdType    = ":type"
	cmdDoc     = ":doc"
	cmdFuncs   = ":funcs"
	cmdFields  = ":fields"
	cmdVars    = ":vars"
	cmdImports = ":imports"
	cmdReset   = ":reset"
	cmdHelp    = ":help"
)

const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

// Run executes a meta command. st may be nil (one-shot mode); :reset mutates
// st in place and relies on the caller to persist it.
func Run(p *project.Project, st *session.State, command string) ([]byte, int) {
	name, arg := splitCommand(command)
	switch name {
	case cmdHelp:
		return marshal(helpOutput(), exitOK)
	case cmdVars:
		return runVars(st)
	case cmdImports:
		return runImports(st)
	case cmdReset:
		return runReset(st)
	case cmdType:
		return runType(p, st, arg)
	case cmdDoc:
		return runDoc(p, arg)
	case cmdFuncs:
		return runFuncs(p, arg)
	case cmdFields:
		return runFields(p, arg)
	default:
		return errOutput(envelope.KindInternal,
			fmt.Sprintf("unknown meta command %q", name),
			"use :help to list available meta commands", exitUsage)
	}
}

func splitCommand(command string) (string, string) {
	trimmed := strings.TrimSpace(command)
	if i := strings.IndexAny(trimmed, " \t"); i > 0 {
		return trimmed[:i], strings.TrimSpace(trimmed[i:])
	}
	return trimmed, ""
}

func marshal(v interface{}, exit int) ([]byte, int) {
	data, err := json.Marshal(v)
	if err != nil {
		return errOutput(envelope.KindInternal, err.Error(), "", exitUsage)
	}
	return data, exit
}

type metaError struct {
	OK    bool            `json:"ok"`
	Error *envelope.Error `json:"error"`
}

func errOutput(kind, message, suggestion string, exit int) ([]byte, int) {
	data, _ := json.Marshal(&metaError{OK: false, Error: &envelope.Error{
		Kind: kind, Message: message, Suggestion: suggestion,
	}})
	return data, exit
}

type helpCommand struct {
	Command     string `json:"command"`
	Usage       string `json:"usage"`
	Description string `json:"description"`
}

type helpOut struct {
	OK       bool           `json:"ok"`
	Commands []*helpCommand `json:"commands"`
}

func helpOutput() *helpOut {
	return &helpOut{OK: true, Commands: []*helpCommand{
		{cmdType, ":type <expr>", "show the static type of an expression without evaluating it"},
		{cmdDoc, ":doc <pkg>[.<Symbol>]", "show the doc comment of a package, function, or type"},
		{cmdFuncs, ":funcs <pkg>", "list exported functions and methods of a package with signatures"},
		{cmdFields, ":fields <pkg>.<Type>", "list struct fields with types and tags"},
		{cmdVars, ":vars", "list variables defined in the session with their types"},
		{cmdImports, ":imports", "list imports active in the session"},
		{cmdReset, ":reset", "clear all session entries and imports"},
		{cmdHelp, ":help", "list meta commands"},
	}}
}

type varsOut struct {
	OK   bool           `json:"ok"`
	Vars []*session.Var `json:"vars"`
}

func runVars(st *session.State) ([]byte, int) {
	out := &varsOut{OK: true, Vars: []*session.Var{}}
	if st != nil {
		if vars := st.Vars(); vars != nil {
			out.Vars = vars
		}
	}
	return marshal(out, exitOK)
}

type importOut struct {
	Alias string `json:"alias,omitempty"`
	Path  string `json:"path"`
}

type importsOut struct {
	OK      bool         `json:"ok"`
	Imports []*importOut `json:"imports"`
}

func runImports(st *session.State) ([]byte, int) {
	out := &importsOut{OK: true, Imports: []*importOut{}}
	if st != nil {
		for _, imp := range st.Imports {
			out.Imports = append(out.Imports, &importOut{Alias: imp.Alias, Path: imp.Path})
		}
	}
	return marshal(out, exitOK)
}

type resetOut struct {
	OK    bool `json:"ok"`
	Reset bool `json:"reset"`
}

func runReset(st *session.State) ([]byte, int) {
	if st != nil {
		st.Reset()
	}
	return marshal(&resetOut{OK: true, Reset: true}, exitOK)
}
