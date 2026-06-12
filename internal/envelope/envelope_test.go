package envelope_test

import (
	"encoding/json"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/replai/internal/envelope"
	"github.com/ensoria/replai/internal/evalrt"
)

var _ = Describe("Envelope", func() {
	Describe("Marshal", func() {
		It("fixes the JSON key order per spec", func() {
			env := envelope.New()
			env.OK = true
			env.Value = &evalrt.ReplaiValue{Repr: "42", Type: "int"}
			data := env.Marshal(0)
			s := string(data)
			Expect(strings.Index(s, `"ok"`)).To(BeNumerically("<", strings.Index(s, `"value"`)))
			Expect(strings.Index(s, `"value"`)).To(BeNumerically("<", strings.Index(s, `"stdout"`)))
			Expect(strings.Index(s, `"stdout"`)).To(BeNumerically("<", strings.Index(s, `"stderr"`)))
			Expect(strings.Index(s, `"stderr"`)).To(BeNumerically("<", strings.Index(s, `"defined"`)))
			Expect(strings.Index(s, `"defined"`)).To(BeNumerically("<", strings.Index(s, `"duration_ms"`)))
			Expect(strings.Index(s, `"duration_ms"`)).To(BeNumerically("<", strings.Index(s, `"truncated"`)))
		})

		It("always carries stdout, stderr, defined, and truncated keys", func() {
			data := envelope.New().Marshal(0)
			var m map[string]any
			Expect(json.Unmarshal(data, &m)).To(Succeed())
			Expect(m).To(HaveKey("stdout"))
			Expect(m).To(HaveKey("stderr"))
			Expect(m).To(HaveKey("defined"))
			Expect(m).To(HaveKey("truncated"))
		})

		It("returns compact output below the budget unchanged", func() {
			env := envelope.New()
			env.OK = true
			env.Stdout = "small"
			data := env.Marshal(16384)
			Expect(len(data)).To(BeNumerically("<", 200))
			Expect(env.Truncated).To(BeFalse())
		})
	})

	Describe("budget trimming", func() {
		It("drops value.json first and records it in truncated_fields", func() {
			env := envelope.New()
			env.OK = true
			env.Value = &evalrt.ReplaiValue{
				Repr: "short",
				Type: "string",
				JSON: json.RawMessage(`"` + strings.Repeat("j", 5000) + `"`),
			}
			data := env.Marshal(1000)
			var m map[string]any
			Expect(json.Unmarshal(data, &m)).To(Succeed())
			Expect(m["truncated"]).To(BeTrue())
			Expect(m["truncated_fields"]).To(ContainElement("value.json"))
			value := m["value"].(map[string]any)
			Expect(value).NotTo(HaveKey("json"))
			Expect(value["repr"]).To(Equal("short"))
		})

		It("trims stdout with an explicit marker, never silently", func() {
			env := envelope.New()
			env.OK = true
			env.Stdout = strings.Repeat("x", 50000)
			data := env.Marshal(2000)
			Expect(len(data)).To(BeNumerically("<=", 2000))
			var m map[string]any
			Expect(json.Unmarshal(data, &m)).To(Succeed())
			Expect(m["stdout"].(string)).To(ContainSubstring("truncated by --max-output"))
			Expect(m["truncated_fields"]).To(ContainElement("stdout"))
		})

		It("trims in priority order: json, stdout, stderr, repr", func() {
			env := envelope.New()
			env.OK = true
			env.Stdout = strings.Repeat("o", 3000)
			env.Stderr = strings.Repeat("e", 3000)
			env.Value = &evalrt.ReplaiValue{Repr: strings.Repeat("r", 3000), Type: "string"}
			data := env.Marshal(2500)
			var m map[string]any
			Expect(json.Unmarshal(data, &m)).To(Succeed())
			fields := m["truncated_fields"].([]any)
			Expect(fields).To(Equal([]any{"stdout", "stderr", "value.repr"}))
		})

		It("trims long error messages and suggestions", func() {
			env := envelope.NewError(envelope.KindCompile, "undefined: "+strings.Repeat("a", 2000))
			env.Error.Suggestion = strings.Repeat("b", 2000)
			data := env.Marshal(1000)
			Expect(len(data)).To(BeNumerically("<=", 1000))
			var m map[string]any
			Expect(json.Unmarshal(data, &m)).To(Succeed())
			Expect(m["truncated"]).To(BeTrue())
			Expect(m["truncated_fields"]).To(ContainElement("error.message"))
			Expect(m["truncated_fields"]).To(ContainElement("error.suggestion"))
		})

		It("replaces oversized raw meta output with a truncation envelope", func() {
			data := envelope.MarshalRaw([]byte(`{"ok":true,"doc":"`+strings.Repeat("x", 5000)+`"}`), 1000)
			Expect(len(data)).To(BeNumerically("<=", 1000))
			var m map[string]any
			Expect(json.Unmarshal(data, &m)).To(Succeed())
			Expect(m["ok"]).To(BeFalse())
			Expect(m["truncated"]).To(BeTrue())
			Expect(m["truncated_fields"]).To(ContainElement("meta"))
		})
	})
})
