package evalrt

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func fileTopLevelNames(file *ast.File) map[string]bool {
	names := map[string]bool{}
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Recv == nil {
				names[d.Name.Name] = true
			}
		case *ast.GenDecl:
			for _, spec := range d.Specs {
				switch s := spec.(type) {
				case *ast.TypeSpec:
					names[s.Name.Name] = true
				case *ast.ValueSpec:
					for _, n := range s.Names {
						names[n.Name] = true
					}
				}
			}
		}
	}
	return names
}

var _ = Describe("Sources (embed.go)", func() {
	It("embeds every runtime file", func() {
		sources, err := Sources()
		Expect(err).NotTo(HaveOccurred())
		Expect(sources).To(HaveKey("rt_main.go"))
		Expect(sources).To(HaveKey("rt_capture.go"))
		Expect(sources).To(HaveKey("rt_format.go"))
		Expect(sources).To(HaveKey("rt_envelope.go"))
	})

	It("contains only standard-library imports so the generated runner stays dependency-free", func() {
		sources, err := Sources()
		Expect(err).NotTo(HaveOccurred())
		fset := token.NewFileSet()
		for name, src := range sources {
			file, err := parser.ParseFile(fset, name, src, parser.ImportsOnly)
			Expect(err).NotTo(HaveOccurred(), name)
			for _, imp := range file.Imports {
				path, err := strconv.Unquote(imp.Path.Value)
				Expect(err).NotTo(HaveOccurred())
				Expect(strings.Contains(path, ".")).To(BeFalse(),
					"%s imports non-stdlib package %s", name, path)
			}
		}
	})

	It("declares only replai-prefixed top-level identifiers to avoid user collisions", func() {
		sources, err := Sources()
		Expect(err).NotTo(HaveOccurred())
		fset := token.NewFileSet()
		for name, src := range sources {
			file, err := parser.ParseFile(fset, name, src, 0)
			Expect(err).NotTo(HaveOccurred(), name)
			for ident := range fileTopLevelNames(file) {
				lower := strings.ToLower(ident)
				Expect(strings.HasPrefix(lower, "replai")).To(BeTrue(),
					"%s declares top-level identifier %q without a replai prefix", name, ident)
			}
		}
	})

	It("does not embed test files", func() {
		sources, err := Sources()
		Expect(err).NotTo(HaveOccurred())
		for name := range sources {
			Expect(strings.HasSuffix(name, "_test.go")).To(BeFalse(), name)
		}
	})
})
