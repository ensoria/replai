package parse_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/replai/internal/parse"
)

var _ = Describe("Classify", func() {
	var sessionVars map[string]bool

	BeforeEach(func() {
		sessionVars = map[string]bool{}
	})

	classify := func(src string) *parse.Snippet {
		sn, err := parse.Classify(src, sessionVars)
		Expect(err).NotTo(HaveOccurred())
		return sn
	}

	Describe("meta commands", func() {
		It("classifies a leading colon as meta", func() {
			sn := classify(":vars")
			Expect(sn.Kind).To(Equal(parse.KindMeta))
			Expect(sn.Meta).To(Equal(":vars"))
		})
	})

	Describe("expressions", func() {
		It("classifies a call expression", func() {
			sn := classify(`widget.New("x")`)
			Expect(sn.Kind).To(Equal(parse.KindExpr))
			Expect(sn.Source).To(Equal(`widget.New("x")`))
		})

		It("classifies an arithmetic expression", func() {
			Expect(classify("1 + 2").Kind).To(Equal(parse.KindExpr))
		})

		It("classifies a composite literal", func() {
			Expect(classify(`map[string]int{"a": 1}`).Kind).To(Equal(parse.KindExpr))
		})
	})

	Describe("statements", func() {
		It("classifies a short variable declaration and records the name", func() {
			sn := classify("x := 1")
			Expect(sn.Kind).To(Equal(parse.KindStmts))
			Expect(sn.Defined).To(Equal([]string{"x"}))
		})

		It("records multiple defined names", func() {
			sn := classify("a, b := 1, 2")
			Expect(sn.Defined).To(Equal([]string{"a", "b"}))
		})

		It("treats var-only declarations as statements so they can use session variables", func() {
			sn := classify("var x = 1")
			Expect(sn.Kind).To(Equal(parse.KindStmts))
			Expect(sn.Defined).To(Equal([]string{"x"}))
		})

		It("records updates of session variables", func() {
			sessionVars["x"] = true
			sn := classify("x = 5")
			Expect(sn.Kind).To(Equal(parse.KindStmts))
			Expect(sn.Defined).To(Equal([]string{"x"}))
		})

		It("ignores the blank identifier", func() {
			sn := classify("_, y := 1, 2")
			Expect(sn.Defined).To(Equal([]string{"y"}))
		})
	})

	Describe(":= redefinition rewrite", func() {
		It("rewrites := to = when every left-hand variable already exists", func() {
			sessionVars["x"] = true
			sn := classify("x := 5")
			Expect(sn.Source).To(Equal("x =  5"))
			Expect(sn.Defined).To(Equal([]string{"x"}))
		})

		It("keeps := when at least one variable is new", func() {
			sessionVars["x"] = true
			sn := classify("x, y := 1, 2")
			Expect(sn.Source).To(Equal("x, y := 1, 2"))
		})

		It("rewrites a redefinition within the same snippet", func() {
			sn := classify("x := 1\nx := 2")
			Expect(sn.Source).To(Equal("x := 1\nx =  2"))
		})
	})

	Describe("trailing expression split", func() {
		It("splits a trailing expression off statement code", func() {
			sn := classify("x := 1\ny := 2\nx + y")
			Expect(sn.Kind).To(Equal(parse.KindStmts))
			Expect(sn.Source).To(Equal("x := 1\ny := 2"))
			Expect(sn.TailExpr).To(Equal("x + y"))
			Expect(sn.TailLine).To(Equal(3))
		})

		It("does not split when the snippet is a single expression statement", func() {
			sn, err := parse.AsStmts("f()", sessionVars)
			Expect(err).NotTo(HaveOccurred())
			Expect(sn.Source).To(Equal("f()"))
			Expect(sn.TailExpr).To(BeEmpty())
		})
	})

	Describe("declarations", func() {
		It("classifies a function declaration", func() {
			sn := classify("func double(n int) int { return n * 2 }")
			Expect(sn.Kind).To(Equal(parse.KindDecls))
			Expect(sn.DeclNames).To(Equal([]string{"double"}))
		})

		It("classifies a type declaration", func() {
			sn := classify("type Pair struct{ A, B int }")
			Expect(sn.Kind).To(Equal(parse.KindDecls))
			Expect(sn.DeclNames).To(Equal([]string{"Pair"}))
		})

		It("classifies a method declaration", func() {
			sn := classify("func (p *Pair) Sum() int { return p.A + p.B }")
			Expect(sn.Kind).To(Equal(parse.KindDecls))
			Expect(sn.DeclNames).To(Equal([]string{"Sum"}))
		})
	})

	Describe("imports", func() {
		It("classifies a lone import", func() {
			sn := classify(`import "fmt"`)
			Expect(sn.Kind).To(Equal(parse.KindImportOnly))
			Expect(sn.Imports).To(HaveLen(1))
			Expect(sn.Imports[0].Path).To(Equal("fmt"))
		})

		It("keeps the alias of an aliased import", func() {
			sn := classify(`import f "fmt"`)
			Expect(sn.Imports[0].Alias).To(Equal("f"))
			Expect(sn.Imports[0].Path).To(Equal("fmt"))
		})

		It("splits leading imports from following statements", func() {
			sn := classify("import \"fmt\"\nx := 1")
			Expect(sn.Kind).To(Equal(parse.KindStmts))
			Expect(sn.Imports).To(HaveLen(1))
			Expect(sn.Defined).To(Equal([]string{"x"}))
		})

		It("handles grouped imports before an expression", func() {
			sn := classify("import (\n\t\"fmt\"\n\t\"strings\"\n)\nstrings.ToUpper(\"a\")")
			Expect(sn.Kind).To(Equal(parse.KindExpr))
			Expect(sn.Imports).To(HaveLen(2))
		})
	})

	Describe("errors", func() {
		It("rejects empty input", func() {
			_, err := parse.Classify("   ", sessionVars)
			Expect(err).To(HaveOccurred())
		})

		It("rejects comment-only input without panicking", func() {
			_, err := parse.Classify("// just a comment", sessionVars)
			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError(ContainSubstring("empty input")))
		})

		It("reports a position for a syntax error", func() {
			_, err := parse.Classify("x := = 1", sessionVars)
			Expect(err).To(HaveOccurred())
			pe, ok := err.(*parse.Error)
			Expect(ok).To(BeTrue())
			Expect(pe.Line).To(Equal(1))
		})
	})
})
