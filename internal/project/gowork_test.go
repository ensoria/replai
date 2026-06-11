package project_test

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/replai/internal/project"
)

var _ = Describe("EnsureSiblingWork", func() {
	var parent, root string

	BeforeEach(func() {
		parent = GinkgoT().TempDir()
		root = filepath.Join(parent, "app")
		writeFile(filepath.Join(root, "go.mod"), "module example.com/app\n\ngo 1.24.2\n")
	})

	It("generates a workspace listing the project and its siblings with absolute paths", func() {
		writeFile(filepath.Join(parent, "lib", "go.mod"), "module example.com/lib\n\ngo 1.25.0\n")
		p, err := project.Find(root)
		Expect(err).NotTo(HaveOccurred())
		Expect(p.EnsureLayout()).To(Succeed())

		created, err := p.EnsureSiblingWork()
		Expect(err).NotTo(HaveOccurred())
		Expect(created).To(BeTrue())

		data, err := os.ReadFile(p.GoWorkPath())
		Expect(err).NotTo(HaveOccurred())
		content := string(data)
		Expect(content).To(ContainSubstring("go 1.25.0"))
		Expect(content).To(ContainSubstring(root))
		Expect(content).To(ContainSubstring(filepath.Join(parent, "lib")))
	})

	It("does nothing when there are no sibling modules", func() {
		p, err := project.Find(root)
		Expect(err).NotTo(HaveOccurred())
		Expect(p.EnsureLayout()).To(Succeed())

		created, err := p.EnsureSiblingWork()
		Expect(err).NotTo(HaveOccurred())
		Expect(created).To(BeFalse())
	})

	It("does nothing when the project has its own go.work", func() {
		writeFile(filepath.Join(root, "go.work"), "go 1.24.2\n\nuse .\n")
		writeFile(filepath.Join(parent, "lib", "go.mod"), "module example.com/lib\n")
		p, err := project.Find(root)
		Expect(err).NotTo(HaveOccurred())
		created, err := p.EnsureSiblingWork()
		Expect(err).NotTo(HaveOccurred())
		Expect(created).To(BeFalse())
	})
})

var _ = Describe("WorkEnv", func() {
	It("returns GOWORK only when the generated workspace exists", func() {
		parent := GinkgoT().TempDir()
		root := filepath.Join(parent, "app")
		writeFile(filepath.Join(root, "go.mod"), "module example.com/app\n")
		p, err := project.Find(root)
		Expect(err).NotTo(HaveOccurred())
		Expect(p.WorkEnv()).To(BeEmpty())

		Expect(p.EnsureLayout()).To(Succeed())
		writeFile(p.GoWorkPath(), "go 1.24.2\n")
		Expect(p.WorkEnv()).To(Equal([]string{"GOWORK=" + p.GoWorkPath()}))
	})
})
