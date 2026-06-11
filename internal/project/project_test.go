package project_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/replai/internal/project"
)

func writeFile(path, content string) {
	Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
	Expect(os.WriteFile(path, []byte(content), 0o644)).To(Succeed())
}

var _ = Describe("Find", func() {
	var root string

	BeforeEach(func() {
		root = GinkgoT().TempDir()
		writeFile(filepath.Join(root, "go.mod"), "module example.com/proj\n\ngo 1.24.2\n")
	})

	It("finds the module from the root directory", func() {
		p, err := project.Find(root)
		Expect(err).NotTo(HaveOccurred())
		Expect(p.ModulePath).To(Equal("example.com/proj"))
	})

	It("walks up from a nested directory", func() {
		nested := filepath.Join(root, "a", "b")
		Expect(os.MkdirAll(nested, 0o755)).To(Succeed())
		p, err := project.Find(nested)
		Expect(err).NotTo(HaveOccurred())
		Expect(p.Root).To(Equal(root))
	})

	It("reports a missing project", func() {
		_, err := project.Find(GinkgoT().TempDir())
		Expect(err).To(MatchError(ContainSubstring("no go.mod")))
	})

	It("detects a project-level go.work", func() {
		writeFile(filepath.Join(root, "go.work"), "go 1.24.2\n\nuse .\n")
		p, err := project.Find(root)
		Expect(err).NotTo(HaveOccurred())
		Expect(p.HasGoWork).To(BeTrue())
	})
})

var _ = Describe("EnsureLayout", func() {
	It("creates the .replai tree with a self-ignoring gitignore", func() {
		root := GinkgoT().TempDir()
		writeFile(filepath.Join(root, "go.mod"), "module example.com/proj\n")
		p, err := project.Find(root)
		Expect(err).NotTo(HaveOccurred())
		Expect(p.EnsureLayout()).To(Succeed())

		Expect(p.RunDir()).To(BeADirectory())
		Expect(p.CacheDir()).To(BeADirectory())
		Expect(p.SessionsDir()).To(BeADirectory())
		data, err := os.ReadFile(filepath.Join(p.ReplaiDir(), ".gitignore"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("*\n"))
	})
})
