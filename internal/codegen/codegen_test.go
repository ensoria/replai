package codegen_test

import (
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/replai/internal/codegen"
	"github.com/ensoria/replai/internal/parse"
	"github.com/ensoria/replai/internal/project"
	"github.com/ensoria/replai/internal/session"
)

var _ = Describe("Generate", func() {
	var p *project.Project

	BeforeEach(func() {
		root := GinkgoT().TempDir()
		Expect(os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/proj\n\ngo 1.24.2\n"), 0o644)).To(Succeed())
		var err error
		p, err = project.Find(root)
		Expect(err).NotTo(HaveOccurred())
		Expect(p.EnsureLayout()).To(Succeed())
	})

	classify := func(src string, vars map[string]bool) *parse.Snippet {
		sn, err := parse.Classify(src, vars)
		Expect(err).NotTo(HaveOccurred())
		return sn
	}

	readMain := func(res *codegen.Result) string {
		data, err := os.ReadFile(res.MainPath)
		Expect(err).NotTo(HaveOccurred())
		return string(data)
	}

	It("wraps an expression in replaiCapture and writes the runtime files", func() {
		res, err := codegen.Generate(p, nil, classify("1 + 2", nil))
		Expect(err).NotTo(HaveOccurred())
		src := readMain(res)
		Expect(src).To(ContainSubstring("replaiCapture("))
		Expect(src).To(ContainSubstring("1 + 2,"))
		Expect(res.FilePaths[0]).To(Equal(res.MainPath))
		Expect(len(res.FilePaths)).To(BeNumerically(">", 1))
		for _, f := range res.FilePaths[1:] {
			Expect(filepath.Base(f)).To(HavePrefix("zz_replai_"))
			data, err := os.ReadFile(f)
			Expect(err).NotTo(HaveOccurred())
			Expect(string(data)).To(ContainSubstring("package main"))
			Expect(string(data)).NotTo(ContainSubstring("package evalrt"))
		}
	})

	It("maps the snippet to exact generated lines", func() {
		res, err := codegen.Generate(p, nil, classify("1 + 2", nil))
		Expect(err).NotTo(HaveOccurred())
		var snippetRange *codegen.LineRange
		for _, lr := range res.LineMap {
			if lr.Entry == codegen.EntryCurrent {
				snippetRange = lr
			}
		}
		Expect(snippetRange).NotTo(BeNil())
		lines := strings.Split(readMain(res), "\n")
		Expect(lines[snippetRange.Start-1]).To(Equal("1 + 2,"))
	})

	It("auto-imports referenced stdlib packages and reports them", func() {
		res, err := codegen.Generate(p, nil, classify(`strings.ToUpper("a")`, nil))
		Expect(err).NotTo(HaveOccurred())
		Expect(res.AutoImports).To(ContainElement("strings"))
		Expect(readMain(res)).To(ContainSubstring(`"strings"`))
	})

	It("preserves an explicitly requested but unused import as a blank import", func() {
		st := &session.State{Imports: []*parse.Import{{Path: "fmt"}}}
		res, err := codegen.Generate(p, st, classify("1 + 2", nil))
		Expect(err).NotTo(HaveOccurred())
		Expect(readMain(res)).To(ContainSubstring(`_ "fmt"`))
	})

	It("replays prior entries before the snippet and keeps variables used", func() {
		st := &session.State{Entries: []*session.Entry{
			{Kind: "stmt", Source: "x := 1", Defined: map[string]string{"x": "int"}},
			{Kind: "expr", Source: "x + 1"},
		}}
		res, err := codegen.Generate(p, st, classify("x * 2", st.VarNames()))
		Expect(err).NotTo(HaveOccurred())
		src := readMain(res)
		Expect(src).To(ContainSubstring("x := 1"))
		Expect(src).To(ContainSubstring("replaiDiscard("))
		Expect(src).To(ContainSubstring("replaiUse(x)"))
		idxEntry := strings.Index(src, "x := 1")
		idxBegin := strings.Index(src, "rc.replaiBegin()")
		idxSnippet := strings.Index(src, "x * 2")
		Expect(idxEntry).To(BeNumerically("<", idxBegin))
		Expect(idxBegin).To(BeNumerically("<", idxSnippet))
	})

	It("hoists declarations above the body and reports defined statement variables", func() {
		st := &session.State{Entries: []*session.Entry{
			{Kind: "decl", Source: "func double(n int) int { return n * 2 }", DeclNames: []string{"double"}},
		}}
		res, err := codegen.Generate(p, st, classify("y := double(2)", st.VarNames()))
		Expect(err).NotTo(HaveOccurred())
		src := readMain(res)
		idxDecl := strings.Index(src, "func double")
		idxBody := strings.Index(src, "func replaiBody")
		Expect(idxDecl).To(BeNumerically("<", idxBody))
		Expect(src).To(ContainSubstring(`rc.replaiDefined("y", y)`))
	})

	It("captures a split-off trailing expression", func() {
		res, err := codegen.Generate(p, nil, classify("x := 1\nx + 1", nil))
		Expect(err).NotTo(HaveOccurred())
		src := readMain(res)
		Expect(src).To(ContainSubstring("x := 1"))
		Expect(src).To(ContainSubstring("replaiCapture("))
		Expect(src).To(ContainSubstring("x + 1,"))
	})
})
