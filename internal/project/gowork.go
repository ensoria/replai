package project

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	workFileName     = "go.work"
	defaultGoVersion = "1.24"
)

// GoWorkPath is the generated workspace file enabling imports of sibling
// monorepo modules at their local sources. The user's files are never touched.
func (p *Project) GoWorkPath() string {
	return filepath.Join(p.ReplaiDir(), workFileName)
}

// WorkEnv returns the GOWORK environment override to apply to builds: the
// generated workspace if present, none when the project has its own go.work.
func (p *Project) WorkEnv() []string {
	if p.HasGoWork {
		return nil
	}
	if _, err := os.Stat(p.GoWorkPath()); err == nil {
		return []string{"GOWORK=" + p.GoWorkPath()}
	}
	return nil
}

// EnsureSiblingWork generates .replai/go.work with `use` directives for the
// project and every sibling directory containing a go.mod, so monorepo
// modules not yet required in go.mod become importable. Returns false when
// there is nothing to gain (project has its own go.work, or no siblings).
func (p *Project) EnsureSiblingWork() (bool, error) {
	if p.HasGoWork {
		return false, nil
	}
	parent := filepath.Dir(p.Root)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return false, err
	}
	var uses []string
	maxVersion := defaultGoVersion
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(parent, e.Name())
		modFile := filepath.Join(dir, goModFile)
		if _, err := os.Stat(modFile); err != nil {
			continue
		}
		if v := goDirective(modFile); compareGoVersions(v, maxVersion) > 0 {
			maxVersion = v
		}
		// Absolute paths: `use` directives resolve relative to the go.work
		// file location (.replai/), not the project root.
		uses = append(uses, "\t"+dir)
	}
	if len(uses) <= 1 {
		return false, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "go %s\n\nuse (\n%s\n)\n", maxVersion, strings.Join(uses, "\n"))
	return true, os.WriteFile(p.GoWorkPath(), []byte(b.String()), 0o644)
}

func goDirective(goModPath string) string {
	f, err := os.Open(goModPath)
	if err != nil {
		return defaultGoVersion
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "go ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "go "))
		}
	}
	return defaultGoVersion
}

func compareGoVersions(a, b string) int {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		na, nb := 0, 0
		if i < len(pa) {
			na, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			nb, _ = strconv.Atoi(pb[i])
		}
		if na != nb {
			return na - nb
		}
	}
	return 0
}
