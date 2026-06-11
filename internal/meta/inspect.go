package meta

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/token"
	"go/types"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/ensoria/replai/internal/envelope"
	"github.com/ensoria/replai/internal/project"
)

var loadMode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
	packages.NeedTypes | packages.NeedTypesInfo | packages.NeedSyntax | packages.NeedImports

func load(p *project.Project, pattern string) (*packages.Package, []byte, int) {
	cfg := &packages.Config{Mode: loadMode, Dir: p.Root, Fset: token.NewFileSet()}
	pkgs, err := packages.Load(cfg, pattern)
	if err != nil {
		data, exit := errOutput(envelope.KindInternal, fmt.Sprintf("failed to load %q: %v", pattern, err), "", exitError)
		return nil, data, exit
	}
	if len(pkgs) == 0 || pkgs[0].Types == nil {
		data, exit := errOutput(envelope.KindInternal, fmt.Sprintf("package %q not found from %s", pattern, p.Root),
			"use a full import path (e.g. github.com/ensoria/rest) or a relative one (./pkg/foo)", exitError)
		return nil, data, exit
	}
	pkg := pkgs[0]
	for _, e := range pkg.Errors {
		// Dependency packages may carry ignorable errors; only fail on hard
		// load failures of the requested package itself.
		if e.Kind == packages.ListError {
			data, exit := errOutput(envelope.KindInternal, e.Msg, "", exitError)
			return nil, data, exit
		}
	}
	return pkg, nil, exitOK
}

// fullQualifier renders type names with complete package paths.
func fullQualifier(p *types.Package) string { return p.Path() }

// shortQualifier renders type names with package short names for signature
// readability.
func shortQualifier(p *types.Package) string { return p.Name() }

type funcInfo struct {
	Name      string `json:"name"`
	Signature string `json:"signature"`
	Doc       string `json:"doc,omitempty"`
}

type funcsOut struct {
	OK        bool        `json:"ok"`
	Package   string      `json:"package"`
	Functions []*funcInfo `json:"functions"`
}

func runFuncs(p *project.Project, arg string) ([]byte, int) {
	if arg == "" {
		return errOutput(envelope.KindInternal, "usage: :funcs <pkg>", "", exitUsage)
	}
	pkg, errData, exit := load(p, arg)
	if errData != nil {
		return errData, exit
	}
	docs := collectDocs(pkg)
	scope := pkg.Types.Scope()
	funcs := []*funcInfo{}
	for _, name := range scope.Names() {
		if !token.IsExported(name) {
			continue
		}
		switch obj := scope.Lookup(name).(type) {
		case *types.Func:
			funcs = append(funcs, &funcInfo{
				Name:      name,
				Signature: types.ObjectString(obj, shortQualifier),
				Doc:       docs[name],
			})
		case *types.TypeName:
			named, ok := obj.Type().(*types.Named)
			if !ok {
				continue
			}
			for i := 0; i < named.NumMethods(); i++ {
				m := named.Method(i)
				if !m.Exported() {
					continue
				}
				key := name + "." + m.Name()
				funcs = append(funcs, &funcInfo{
					Name:      key,
					Signature: types.ObjectString(m, shortQualifier),
					Doc:       docs[key],
				})
			}
		}
	}
	sort.Slice(funcs, func(i, j int) bool { return funcs[i].Name < funcs[j].Name })
	return marshal(&funcsOut{OK: true, Package: pkg.PkgPath, Functions: funcs}, exitOK)
}

// collectDocs maps "Func" and "Type.Method" to the first line of their doc
// comments.
func collectDocs(pkg *packages.Package) map[string]string {
	out := map[string]string{}
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Doc == nil {
				continue
			}
			key := fd.Name.Name
			if fd.Recv != nil && len(fd.Recv.List) > 0 {
				if recv := recvTypeName(fd.Recv.List[0].Type); recv != "" {
					key = recv + "." + fd.Name.Name
				}
			}
			out[key] = firstLine(fd.Doc.Text())
		}
	}
	return out
}

func recvTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return recvTypeName(t.X)
	case *ast.IndexExpr:
		return recvTypeName(t.X)
	case *ast.IndexListExpr:
		return recvTypeName(t.X)
	}
	return ""
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

type fieldInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
	Tag  string `json:"tag,omitempty"`
}

type fieldsOut struct {
	OK     bool         `json:"ok"`
	Type   string       `json:"type"`
	Fields []*fieldInfo `json:"fields"`
}

func runFields(p *project.Project, arg string) ([]byte, int) {
	pkgPath, typeName, ok := splitSymbol(strings.TrimPrefix(arg, "*"))
	if !ok {
		return errOutput(envelope.KindInternal, "usage: :fields <pkg>.<Type>", "example: :fields github.com/ensoria/rest.Server", exitUsage)
	}
	pkg, errData, exit := load(p, pkgPath)
	if errData != nil {
		return errData, exit
	}
	obj := pkg.Types.Scope().Lookup(typeName)
	if obj == nil {
		return errOutput(envelope.KindInternal,
			fmt.Sprintf("type %s not found in %s", typeName, pkg.PkgPath),
			fmt.Sprintf("use `:funcs %s` to list the package API", pkg.PkgPath), exitError)
	}
	structType, ok := obj.Type().Underlying().(*types.Struct)
	if !ok {
		return errOutput(envelope.KindInternal,
			fmt.Sprintf("%s.%s is not a struct (underlying: %s)", pkg.PkgPath, typeName, obj.Type().Underlying().String()),
			"", exitError)
	}
	fields := make([]*fieldInfo, 0, structType.NumFields())
	for i := 0; i < structType.NumFields(); i++ {
		f := structType.Field(i)
		fields = append(fields, &fieldInfo{
			Name: f.Name(),
			Type: types.TypeString(f.Type(), shortQualifier),
			Tag:  structType.Tag(i),
		})
	}
	return marshal(&fieldsOut{OK: true, Type: pkg.PkgPath + "." + typeName, Fields: fields}, exitOK)
}

type docOut struct {
	OK     bool   `json:"ok"`
	Symbol string `json:"symbol"`
	Doc    string `json:"doc"`
}

func runDoc(p *project.Project, arg string) ([]byte, int) {
	if arg == "" {
		return errOutput(envelope.KindInternal, "usage: :doc <pkg>[.<Symbol>]", "", exitUsage)
	}
	// Try the whole argument as a package first, then as pkg.Symbol.
	if pkg, errData, _ := load(p, arg); errData == nil {
		text, err := packageDoc(pkg, "")
		if err == nil {
			return marshal(&docOut{OK: true, Symbol: pkg.PkgPath, Doc: text}, exitOK)
		}
	}
	pkgPath, symbol, ok := splitSymbol(arg)
	if !ok {
		return errOutput(envelope.KindInternal, fmt.Sprintf("package or symbol %q not found", arg), "", exitError)
	}
	pkg, errData, exit := load(p, pkgPath)
	if errData != nil {
		return errData, exit
	}
	text, err := packageDoc(pkg, symbol)
	if err != nil {
		return errOutput(envelope.KindInternal, err.Error(),
			fmt.Sprintf("use `:funcs %s` to list the package API", pkg.PkgPath), exitError)
	}
	return marshal(&docOut{OK: true, Symbol: pkgPath + "." + symbol, Doc: text}, exitOK)
}

func packageDoc(pkg *packages.Package, symbol string) (string, error) {
	dp, err := doc.NewFromFiles(pkg.Fset, pkg.Syntax, pkg.PkgPath)
	if err != nil {
		return "", err
	}
	if symbol == "" {
		return strings.TrimSpace(dp.Doc), nil
	}
	for _, f := range dp.Funcs {
		if f.Name == symbol {
			return strings.TrimSpace(f.Doc), nil
		}
	}
	for _, t := range dp.Types {
		if t.Name == symbol {
			return strings.TrimSpace(t.Doc), nil
		}
		for _, m := range append(t.Methods, t.Funcs...) {
			if t.Name+"."+m.Name == symbol || m.Name == symbol {
				return strings.TrimSpace(m.Doc), nil
			}
		}
	}
	for _, vals := range [][]*doc.Value{dp.Consts, dp.Vars} {
		for _, v := range vals {
			for _, name := range v.Names {
				if name == symbol {
					return strings.TrimSpace(v.Doc), nil
				}
			}
		}
	}
	return "", fmt.Errorf("symbol %s not found in %s", symbol, pkg.PkgPath)
}

// splitSymbol splits "pkg/path.Symbol" at the last dot after the last slash.
func splitSymbol(arg string) (string, string, bool) {
	lastSlash := strings.LastIndexByte(arg, '/')
	lastDot := strings.LastIndexByte(arg, '.')
	if lastDot <= lastSlash || lastDot < 0 || lastDot == len(arg)-1 {
		return "", "", false
	}
	return arg[:lastDot], arg[lastDot+1:], true
}
