package cmd

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("describeCommand (helpjson.go)", func() {
	It("describes the full command tree with flags", func() {
		root := describeCommand(rootCmd)
		Expect(root.Name).To(Equal("replai"))

		var names []string
		for _, sub := range root.Subcommands {
			names = append(names, sub.Name)
		}
		Expect(names).To(ContainElements("eval", "session", "repl", "version"))

		var flagNames []string
		for _, f := range root.Flags {
			flagNames = append(flagNames, f.Name)
		}
		Expect(flagNames).To(ContainElements("timeout", "depth", "max-items", "max-str", "max-output", "restrict"))
	})

	It("documents defaults so agents can self-discover limits", func() {
		root := describeCommand(rootCmd)
		for _, f := range root.Flags {
			if f.Name == "timeout" {
				Expect(f.Default).To(Equal("30s"))
			}
			if f.Name == "max-output" {
				Expect(f.Default).To(Equal("16384"))
			}
		}
	})

	It("excludes cobra's auto-generated help and completion commands", func() {
		root := describeCommand(rootCmd)
		for _, sub := range root.Subcommands {
			Expect(sub.Name).NotTo(Equal("help"))
			Expect(sub.Name).NotTo(Equal("completion"))
		}
	})
})
