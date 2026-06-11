package cmd

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/replai/internal/engine"
)

var _ = Describe("resolveInput (eval.go)", func() {
	BeforeEach(func() {
		flagEvalFile = ""
		exitCode = engine.ExitOK
	})

	It("returns the positional argument", func() {
		input, ok := resolveInput([]string{"1 + 2"})
		Expect(ok).To(BeTrue())
		Expect(input).To(Equal("1 + 2"))
	})

	It("reads from a file with -f", func() {
		path := filepath.Join(GinkgoT().TempDir(), "snippet.go.txt")
		Expect(os.WriteFile(path, []byte("x := 1"), 0o644)).To(Succeed())
		flagEvalFile = path
		input, ok := resolveInput(nil)
		Expect(ok).To(BeTrue())
		Expect(input).To(Equal("x := 1"))
	})

	It("fails with exit code 2 when no code is given", func() {
		_, ok := resolveInput(nil)
		Expect(ok).To(BeFalse())
		Expect(exitCode).To(Equal(engine.ExitUsage))
	})

	It("fails with exit code 2 for an unreadable file", func() {
		flagEvalFile = "/nonexistent/snippet"
		_, ok := resolveInput(nil)
		Expect(ok).To(BeFalse())
		Expect(exitCode).To(Equal(engine.ExitUsage))
	})
})

var _ = Describe("balanced (repl.go)", func() {
	It("treats complete lines as balanced", func() {
		Expect(balanced("x := 1")).To(BeTrue())
		Expect(balanced(`fmt.Println("}")`)).To(BeTrue())
	})

	It("waits for closing braces", func() {
		Expect(balanced("func f() {")).To(BeFalse())
		Expect(balanced("func f() {\n}")).To(BeTrue())
	})

	It("waits inside raw string literals", func() {
		Expect(balanced("s := `multi")).To(BeFalse())
		Expect(balanced("s := `multi\nline`")).To(BeTrue())
	})

	It("ignores brackets in line comments", func() {
		Expect(balanced("x := 1 // {")).To(BeTrue())
	})
})
