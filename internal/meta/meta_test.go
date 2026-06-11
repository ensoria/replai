package meta_test

import (
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/replai/internal/meta"
	"github.com/ensoria/replai/internal/parse"
	"github.com/ensoria/replai/internal/session"
)

func mustJSON(data []byte) map[string]any {
	var m map[string]any
	ExpectWithOffset(1, json.Unmarshal(data, &m)).To(Succeed(), string(data))
	return m
}

var _ = Describe("Run", func() {
	It("rejects unknown commands with exit code 2 and a pointer to :help", func() {
		out, exit := meta.Run(nil, nil, ":nope")
		Expect(exit).To(Equal(2))
		m := mustJSON(out)
		Expect(m["ok"]).To(BeFalse())
		Expect(m["error"].(map[string]any)["suggestion"]).To(ContainSubstring(":help"))
	})

	Describe(":help", func() {
		It("lists every meta command as machine-readable JSON", func() {
			out, exit := meta.Run(nil, nil, ":help")
			Expect(exit).To(Equal(0))
			m := mustJSON(out)
			Expect(m["ok"]).To(BeTrue())
			commands := m["commands"].([]any)
			Expect(len(commands)).To(Equal(8))
		})
	})

	Describe(":vars", func() {
		It("returns an empty list without a session", func() {
			out, exit := meta.Run(nil, nil, ":vars")
			Expect(exit).To(Equal(0))
			Expect(mustJSON(out)["vars"]).To(BeEmpty())
		})

		It("lists session variables with types", func() {
			st := &session.State{Entries: []*session.Entry{
				{Kind: "stmt", Defined: map[string]string{"x": "int"}},
			}}
			out, _ := meta.Run(nil, st, ":vars")
			vars := mustJSON(out)["vars"].([]any)
			Expect(vars).To(HaveLen(1))
			Expect(vars[0].(map[string]any)["name"]).To(Equal("x"))
			Expect(vars[0].(map[string]any)["type"]).To(Equal("int"))
		})
	})

	Describe(":imports", func() {
		It("lists session imports", func() {
			st := &session.State{Imports: []*parse.Import{{Alias: "f", Path: "fmt"}}}
			out, _ := meta.Run(nil, st, ":imports")
			imports := mustJSON(out)["imports"].([]any)
			Expect(imports).To(HaveLen(1))
			Expect(imports[0].(map[string]any)["path"]).To(Equal("fmt"))
			Expect(imports[0].(map[string]any)["alias"]).To(Equal("f"))
		})
	})

	Describe(":reset", func() {
		It("clears the session state in place", func() {
			st := &session.State{
				Imports: []*parse.Import{{Path: "fmt"}},
				Entries: []*session.Entry{{Kind: "stmt", Source: "x := 1"}},
			}
			out, exit := meta.Run(nil, st, ":reset")
			Expect(exit).To(Equal(0))
			Expect(mustJSON(out)["reset"]).To(BeTrue())
			Expect(st.Entries).To(BeEmpty())
			Expect(st.Imports).To(BeEmpty())
		})
	})
})
