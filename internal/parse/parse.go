package parse

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"sort"
	"strconv"
	"strings"
)

const (
	metaPrefix = ":"

	filePrefix = "package replai\n"
	stmtPrefix = "package replai\nfunc replaiTmp() {\n"
	stmtSuffix = "\n}"

	// Line offsets introduced by the synthetic wrappers above.
	filePrefixLines = 1
	stmtPrefixLines = 2
)

// Kind classifies a snippet.
type Kind int

const (
	KindExpr Kind = iota
	KindStmts
	KindDecls
	KindImportOnly
	KindMeta
)

// String returns the session-entry kind name.
func (k Kind) String() string {
	switch k {
	case KindExpr:
		return "expr"
	case KindStmts:
		return "stmt"
	case KindDecls:
		return "decl"
	case KindImportOnly:
		return "import"
	case KindMeta:
		return "meta"
	}
	return "unknown"
}

// Import is a single import requested by a snippet.
type Import struct {
	Alias string `json:"alias,omitempty"`
	Path  string `json:"path"`
}

// Snippet is the classification result for one piece of input.
type Snippet struct {
	Kind      Kind
	Source    string    // body source after leading imports are stripped and := rewrites applied
	Imports   []*Import // imports declared by the snippet
	Defined   []string  // variables defined or updated by statement code
	DeclNames []string  // names declared at file level (funcs, types, vars, consts)
	Meta      string    // raw meta command including the leading colon

	// TailExpr is a trailing expression statement split off from statement
	// code so its value can be captured (REPL semantics: the last expression
	// of a block is the result). TailLine is its 1-based starting line within
	// the original statement source.
	TailExpr string
	TailLine int
}

// Error is a parse failure with a position in the original snippet.
type Error struct {
	Line, Column int
	Msg          string
}

func (e *Error) Error() string {
	return fmt.Sprintf("%d:%d: %s", e.Line, e.Column, e.Msg)
}

// Classify determines how a snippet must be evaluated. sessionVars holds the
// variables already defined by the session and drives the := redefinition
// rewrite.
func Classify(src string, sessionVars map[string]bool) (*Snippet, error) {
	trimmed := strings.TrimSpace(src)
	if trimmed == "" {
		return nil, &Error{Line: 1, Column: 1, Msg: "empty input"}
	}
	if strings.HasPrefix(trimmed, metaPrefix) {
		return &Snippet{Kind: KindMeta, Meta: trimmed}, nil
	}

	imports, rest := splitLeadingImports(src)
	if strings.TrimSpace(rest) == "" {
		if len(imports) == 0 {
			return nil, &Error{Line: 1, Column: 1, Msg: "empty input"}
		}
		return &Snippet{Kind: KindImportOnly, Imports: imports}, nil
	}

	// File-level declarations (funcs, types) are detected first so they can be
	// hoisted to the top level of the generated program; var/const-only
	// snippets fall through to statement handling so they may reference
	// session variables.
	if sn, ok := classifyDecls(rest, imports); ok {
		return sn, nil
	}
	if expr := strings.TrimSpace(rest); isExpression(expr) {
		return &Snippet{Kind: KindExpr, Source: expr, Imports: imports}, nil
	}
	return classifyStmts(rest, imports, sessionVars)
}

// AsStmts forcibly classifies a snippet as statements without splitting a
// trailing expression; used when an expression-wrapped build fails because
// the call has no value.
func AsStmts(src string, sessionVars map[string]bool) (*Snippet, error) {
	imports, rest := splitLeadingImports(src)
	sn, err := classifyStmts(rest, imports, sessionVars)
	if err != nil {
		return nil, err
	}
	if sn.TailExpr != "" {
		sn.Source = strings.TrimSpace(sn.Source + "\n" + sn.TailExpr)
		sn.TailExpr = ""
		sn.TailLine = 0
	}
	return sn, nil
}

func isExpression(src string) bool {
	expr, err := parser.ParseExpr(src)
	if err != nil {
		return false
	}
	// A bare identifier list etc. cannot reach here; reject weird cases where
	// the parser succeeded on a prefix only.
	_ = expr
	return true
}

// splitLeadingImports extracts import declarations from the start of the
// snippet and returns them together with the remaining source.
func splitLeadingImports(src string) ([]*Import, string) {
	fset := token.NewFileSet()
	file, _ := parser.ParseFile(fset, "snippet.go", filePrefix+src, parser.ImportsOnly)
	if file == nil || len(file.Imports) == 0 {
		return nil, src
	}
	var imports []*Import
	end := 0
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		declEnd := fset.File(file.Pos()).Offset(gd.End()) - len(filePrefix)
		if declEnd > end {
			end = declEnd
		}
	}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		imp := &Import{Path: path}
		if spec.Name != nil {
			imp.Alias = spec.Name.Name
		}
		imports = append(imports, imp)
	}
	if end <= 0 || end > len(src) {
		return imports, src
	}
	return imports, src[end:]
}

func classifyDecls(src string, imports []*Import) (*Snippet, bool) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "snippet.go", filePrefix+src, 0)
	if err != nil {
		return nil, false
	}
	hoistable := false
	var names []string
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			hoistable = true
			names = append(names, d.Name.Name)
		case *ast.GenDecl:
			switch d.Tok {
			case token.IMPORT:
				continue
			case token.TYPE:
				hoistable = true
				for _, s := range d.Specs {
					if ts, ok := s.(*ast.TypeSpec); ok {
						names = append(names, ts.Name.Name)
					}
				}
			case token.VAR, token.CONST:
				for _, s := range d.Specs {
					if vs, ok := s.(*ast.ValueSpec); ok {
						for _, n := range vs.Names {
							names = append(names, n.Name)
						}
					}
				}
			}
		default:
			return nil, false
		}
	}
	// var/const-only snippets are evaluated as statements so they can use
	// session variables; only funcs and types require top-level hoisting.
	if !hoistable {
		return nil, false
	}
	return &Snippet{Kind: KindDecls, Source: strings.TrimSpace(src), Imports: imports, DeclNames: names}, true
}

func classifyStmts(src string, imports []*Import, sessionVars map[string]bool) (*Snippet, error) {
	fset := token.NewFileSet()
	wrapped := stmtPrefix + src + stmtSuffix
	file, err := parser.ParseFile(fset, "snippet.go", wrapped, 0)
	if err != nil {
		return nil, asParseError(err, stmtPrefixLines)
	}
	fn, ok := file.Decls[len(file.Decls)-1].(*ast.FuncDecl)
	if !ok || fn.Body == nil {
		return nil, &Error{Line: 1, Column: 1, Msg: "could not parse input as statements"}
	}

	known := make(map[string]bool, len(sessionVars))
	for k := range sessionVars {
		known[k] = true
	}
	var defined []string
	seen := map[string]bool{}
	addDefined := func(name string) {
		if name == "_" || seen[name] {
			return
		}
		seen[name] = true
		defined = append(defined, name)
	}

	// Offsets (in the original snippet) of ":=" tokens to rewrite to "=".
	var rewrites []int
	offsetOf := func(pos token.Pos) int {
		return fset.File(file.Pos()).Offset(pos) - len(stmtPrefix)
	}

	for _, stmt := range fn.Body.List {
		switch s := stmt.(type) {
		case *ast.AssignStmt:
			if s.Tok == token.DEFINE {
				allKnown := true
				for _, lhs := range s.Lhs {
					id, ok := lhs.(*ast.Ident)
					if !ok {
						allKnown = false
						continue
					}
					if id.Name != "_" && !known[id.Name] {
						allKnown = false
					}
				}
				if allKnown {
					rewrites = append(rewrites, offsetOf(s.TokPos))
				}
			}
			for _, lhs := range s.Lhs {
				if id, ok := lhs.(*ast.Ident); ok {
					addDefined(id.Name)
					known[id.Name] = true
				}
			}
		case *ast.IncDecStmt:
			if id, ok := s.X.(*ast.Ident); ok {
				addDefined(id.Name)
			}
		case *ast.DeclStmt:
			gd, ok := s.Decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range gd.Specs {
				if vs, ok := spec.(*ast.ValueSpec); ok {
					for _, n := range vs.Names {
						addDefined(n.Name)
						known[n.Name] = true
					}
				}
			}
		}
	}

	rewritten := applyRewrites(src, rewrites)
	sn := &Snippet{
		Kind:    KindStmts,
		Source:  strings.TrimSpace(rewritten),
		Imports: imports,
		Defined: defined,
	}

	// REPL semantics: a trailing expression statement is split off so its
	// value becomes the evaluation result instead of a compile error.
	if last, ok := fn.Body.List[len(fn.Body.List)-1].(*ast.ExprStmt); ok && len(fn.Body.List) > 1 {
		start := offsetOf(last.Pos())
		if start > 0 && start < len(rewritten) {
			sn.Source = strings.TrimSpace(rewritten[:start])
			sn.TailExpr = strings.TrimSpace(rewritten[start:])
			sn.TailLine = 1 + strings.Count(src[:start], "\n")
		}
	}
	return sn, nil
}

// applyRewrites replaces ":=" with "= " at the given byte offsets, keeping
// the source length (and thus line/column positions) unchanged.
func applyRewrites(src string, offsets []int) string {
	if len(offsets) == 0 {
		return src
	}
	sort.Sort(sort.Reverse(sort.IntSlice(offsets)))
	b := []byte(src)
	for _, off := range offsets {
		if off >= 0 && off+1 < len(b) && b[off] == ':' && b[off+1] == '=' {
			b[off] = '='
			b[off+1] = ' '
		}
	}
	return string(b)
}

func asParseError(err error, lineOffset int) error {
	if list, ok := err.(scanner.ErrorList); ok && len(list) > 0 {
		first := list[0]
		line := first.Pos.Line - lineOffset
		if line < 1 {
			line = 1
		}
		return &Error{Line: line, Column: first.Pos.Column, Msg: first.Msg}
	}
	return &Error{Line: 1, Column: 1, Msg: err.Error()}
}
