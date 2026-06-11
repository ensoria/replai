package session

import (
	"time"

	"github.com/ensoria/replai/internal/parse"
)

// State is the persisted state of one session: the imports and the source
// entries that are re-executed (with output suppressed) before each new
// snippet. Re-execution means side effects of past entries repeat on every
// eval; this is documented behavior.
type State struct {
	ID         string          `json:"id"`
	CreatedAt  time.Time       `json:"created_at"`
	ProjectDir string          `json:"project_dir"`
	Imports    []*parse.Import `json:"imports"`
	Entries    []*Entry        `json:"entries"`
}

// Entry is one successfully evaluated snippet retained for re-execution.
type Entry struct {
	Kind      string            `json:"kind"`                 // expr | stmt | decl
	Source    string            `json:"source"`               // post-rewrite snippet source
	Defined   map[string]string `json:"defined,omitempty"`    // var name -> full type
	DeclNames []string          `json:"decl_names,omitempty"` // hoisted decl names
}

// Vars returns the session variables (later entries win) sorted by name order
// of first definition.
func (s *State) Vars() []*Var {
	idx := map[string]int{}
	var vars []*Var
	for _, e := range s.Entries {
		for name, typ := range e.Defined {
			if i, ok := idx[name]; ok {
				vars[i].Type = typ
				continue
			}
			idx[name] = len(vars)
			vars = append(vars, &Var{Name: name, Type: typ})
		}
	}
	return vars
}

// VarNames returns the set of defined variable names.
func (s *State) VarNames() map[string]bool {
	out := map[string]bool{}
	for _, e := range s.Entries {
		for name := range e.Defined {
			out[name] = true
		}
	}
	return out
}

// Var is a session variable with its last reported type.
type Var struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// MergeImports adds imports not yet present (by alias+path identity).
func (s *State) MergeImports(imports []*parse.Import) {
	for _, imp := range imports {
		found := false
		for _, have := range s.Imports {
			if have.Path == imp.Path && have.Alias == imp.Alias {
				found = true
				break
			}
		}
		if !found {
			s.Imports = append(s.Imports, imp)
		}
	}
}

// Reset drops all entries and imports.
func (s *State) Reset() {
	s.Imports = nil
	s.Entries = nil
}
