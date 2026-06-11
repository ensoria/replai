package meta

import (
	"go/ast"
	"go/types"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/ensoria/replai/internal/codegen"
	"github.com/ensoria/replai/internal/envelope"
	"github.com/ensoria/replai/internal/parse"
	"github.com/ensoria/replai/internal/project"
	"github.com/ensoria/replai/internal/session"
)

const captureFuncName = "replaiCapture"

type typeOut struct {
	OK   bool   `json:"ok"`
	Expr string `json:"expr"`
	Type string `json:"type"`
}

// runType reports the static type of an expression in the session context
// without executing anything: the usual runner package is generated, then
// type-checked in-process via go/packages.
func runType(p *project.Project, st *session.State, arg string) ([]byte, int) {
	if arg == "" {
		return errOutput(envelope.KindInternal, "usage: :type <expr>", "", exitUsage)
	}
	vars := map[string]bool{}
	if st != nil {
		vars = st.VarNames()
	}
	sn, err := parse.Classify(arg, vars)
	if err != nil {
		return errOutput(envelope.KindCompile, err.Error(), "", exitError)
	}
	if sn.Kind != parse.KindExpr {
		return errOutput(envelope.KindInternal, "argument of :type must be a single expression",
			"statements and declarations have no type; evaluate them instead", exitUsage)
	}

	res, err := codegen.Generate(p, st, sn)
	if err != nil {
		return errOutput(envelope.KindCompile, err.Error(), "", exitError)
	}
	pkg, errData, exit := load(p, "file="+res.MainPath)
	if errData != nil {
		return errData, exit
	}

	if msg := snippetTypeError(pkg, res); msg != "" {
		return errOutput(envelope.KindCompile, msg, "", exitError)
	}
	typ := captureArgType(pkg)
	if typ == "" {
		return errOutput(envelope.KindInternal, "could not determine the expression type", "", exitError)
	}
	return marshal(&typeOut{OK: true, Expr: sn.Source, Type: typ}, exitOK)
}

// snippetTypeError surfaces type errors located in the generated main file.
func snippetTypeError(pkg *packages.Package, res *codegen.Result) string {
	for _, e := range pkg.Errors {
		if e.Kind != packages.TypeError && e.Kind != packages.ParseError {
			continue
		}
		if strings.Contains(e.Pos, res.MainPath) || strings.Contains(e.Pos, "main.go") {
			return e.Msg
		}
	}
	return ""
}

// captureArgType finds the replaiCapture call and reports the type of its
// value argument(s) with full package qualification.
func captureArgType(pkg *packages.Package) string {
	if pkg.TypesInfo == nil {
		return ""
	}
	for _, file := range pkg.Syntax {
		var result string
		ast.Inspect(file, func(n ast.Node) bool {
			if result != "" {
				return false
			}
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			ident, ok := call.Fun.(*ast.Ident)
			if !ok || ident.Name != captureFuncName || len(call.Args) == 0 {
				return true
			}
			var parts []string
			for _, arg := range call.Args {
				tv, ok := pkg.TypesInfo.Types[arg]
				if !ok || tv.Type == nil {
					continue
				}
				parts = append(parts, typeString(tv.Type))
			}
			result = strings.Join(parts, ", ")
			return false
		})
		if result != "" {
			return result
		}
	}
	return ""
}

func typeString(t types.Type) string {
	if tuple, ok := t.(*types.Tuple); ok {
		parts := make([]string, tuple.Len())
		for i := 0; i < tuple.Len(); i++ {
			parts[i] = types.TypeString(tuple.At(i).Type(), fullQualifier)
		}
		return "(" + strings.Join(parts, ", ") + ")"
	}
	return types.TypeString(t, fullQualifier)
}
