package suggest_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/replai/internal/suggest"
)

var _ = Describe("Levenshtein", func() {
	It("computes edit distances", func() {
		Expect(suggest.Levenshtein("abc", "abc")).To(Equal(0))
		Expect(suggest.Levenshtein("Nwe", "New")).To(Equal(2))
		Expect(suggest.Levenshtein("", "ab")).To(Equal(2))
		Expect(suggest.Levenshtein("kitten", "sitting")).To(Equal(3))
	})
})

var _ = Describe("Similar", func() {
	It("ranks close names first", func() {
		got := suggest.Similar("Nwe", []string{"New", "NewService", "Delete"})
		Expect(got).NotTo(BeEmpty())
		Expect(got[0]).To(Equal("New"))
	})

	It("includes prefix matches", func() {
		got := suggest.Similar("New", []string{"NewServiceWithDB", "Old"})
		Expect(got).To(ContainElement("NewServiceWithDB"))
	})

	It("excludes the target itself and unrelated names", func() {
		got := suggest.Similar("Foo", []string{"Foo", "Bar"})
		Expect(got).To(BeEmpty())
	})
})

var _ = Describe("ForBuildError", func() {
	const root = "/proj"

	It("turns a missing module into a go get instruction", func() {
		msg := "no required module provides package github.com/x/y; to add it:"
		out := suggest.ForBuildError(root, msg, nil, nil)
		Expect(out).To(ContainSubstring("go get github.com/x/y"))
	})

	It("suggests similar session names for a bare undefined identifier", func() {
		out := suggest.ForBuildError(root, "undefined: Widgt", nil, []string{"Widget"})
		Expect(out).To(ContainSubstring("did you mean Widget?"))
	})

	It("hints at failed auto-import for a lowercase undefined identifier", func() {
		out := suggest.ForBuildError(root, "undefined: widget", nil, nil)
		Expect(out).To(ContainSubstring("auto-import failed"))
	})

	It("points to :fields for missing struct members", func() {
		out := suggest.ForBuildError(root, "w.Nmae undefined (type *widget.Widget has no field or method Nmae)", nil, nil)
		Expect(out).To(ContainSubstring(":fields"))
	})

	It("returns empty for unrecognized messages", func() {
		Expect(suggest.ForBuildError(root, "something else entirely", nil, nil)).To(BeEmpty())
	})
})
