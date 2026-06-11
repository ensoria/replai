package session_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/replai/internal/parse"
	"github.com/ensoria/replai/internal/session"
)

var _ = Describe("State", func() {
	Describe("Vars", func() {
		It("aggregates variables with later entries winning", func() {
			st := &session.State{Entries: []*session.Entry{
				{Kind: "stmt", Defined: map[string]string{"x": "int"}},
				{Kind: "stmt", Defined: map[string]string{"x": "string", "y": "bool"}},
			}}
			vars := st.Vars()
			Expect(vars).To(HaveLen(2))
			Expect(vars[0].Name).To(Equal("x"))
			Expect(vars[0].Type).To(Equal("string"))
			Expect(vars[1].Name).To(Equal("y"))
		})
	})

	Describe("VarNames", func() {
		It("returns the set of defined names", func() {
			st := &session.State{Entries: []*session.Entry{
				{Kind: "stmt", Defined: map[string]string{"a": "int"}},
			}}
			Expect(st.VarNames()).To(Equal(map[string]bool{"a": true}))
		})
	})

	Describe("MergeImports", func() {
		It("adds new imports and skips duplicates", func() {
			st := &session.State{}
			st.MergeImports([]*parse.Import{{Path: "fmt"}})
			st.MergeImports([]*parse.Import{{Path: "fmt"}, {Path: "strings"}})
			Expect(st.Imports).To(HaveLen(2))
		})

		It("treats different aliases of the same path as distinct", func() {
			st := &session.State{}
			st.MergeImports([]*parse.Import{{Path: "fmt"}, {Alias: "f", Path: "fmt"}})
			Expect(st.Imports).To(HaveLen(2))
		})
	})

	Describe("Reset", func() {
		It("clears entries and imports", func() {
			st := &session.State{
				Imports: []*parse.Import{{Path: "fmt"}},
				Entries: []*session.Entry{{Kind: "stmt", Source: "x := 1"}},
			}
			st.Reset()
			Expect(st.Imports).To(BeEmpty())
			Expect(st.Entries).To(BeEmpty())
		})
	})
})
