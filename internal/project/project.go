package project

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	goModFile  = "go.mod"
	goWorkFile = "go.work"

	replaiDirName   = ".replai"
	runDirName      = "run"
	cacheDirName    = "cache"
	sessionsDirName = "sessions"
	runnerBinName   = "runner"

	gitignoreName    = ".gitignore"
	gitignoreContent = "*\n"

	dirPerm = 0o755
)

// ErrNotFound is returned when no go.mod/go.work is found upwards from the
// starting directory.
var ErrNotFound = errors.New("no go.mod or go.work found in this directory or any parent; run replai inside a Go project")

// Project is the Go module (or workspace) replai operates on.
type Project struct {
	// Root is the directory containing go.mod (or go.work).
	Root string
	// ModulePath is the module path from go.mod; empty for a bare workspace root.
	ModulePath string
	// HasGoWork reports whether the project root has its own go.work.
	HasGoWork bool
}

// Find walks upwards from start looking for go.mod or go.work.
func Find(start string) (*Project, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return nil, err
	}
	for {
		modPath := filepath.Join(dir, goModFile)
		workPath := filepath.Join(dir, goWorkFile)
		_, modErr := os.Stat(modPath)
		_, workErr := os.Stat(workPath)
		if modErr == nil || workErr == nil {
			p := &Project{Root: dir, HasGoWork: workErr == nil}
			if modErr == nil {
				mp, err := modulePath(modPath)
				if err != nil {
					return nil, err
				}
				p.ModulePath = mp
			}
			return p, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, ErrNotFound
		}
		dir = parent
	}
}

func modulePath(goModPath string) (string, error) {
	f, err := os.Open(goModPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if strings.HasPrefix(line, "module ") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "module ")), `"`), nil
		}
	}
	return "", fmt.Errorf("no module directive in %s", goModPath)
}

// EnsureLayout creates the .replai working directories and the self-ignoring
// .gitignore. It never touches files outside .replai.
func (p *Project) EnsureLayout() error {
	for _, dir := range []string{p.RunDir(), p.CacheDir(), p.SessionsDir()} {
		if err := os.MkdirAll(dir, dirPerm); err != nil {
			return err
		}
	}
	gi := filepath.Join(p.ReplaiDir(), gitignoreName)
	if _, err := os.Stat(gi); os.IsNotExist(err) {
		return os.WriteFile(gi, []byte(gitignoreContent), 0o644)
	}
	return nil
}

func (p *Project) ReplaiDir() string   { return filepath.Join(p.Root, replaiDirName) }
func (p *Project) RunDir() string      { return filepath.Join(p.ReplaiDir(), runDirName) }
func (p *Project) CacheDir() string    { return filepath.Join(p.ReplaiDir(), cacheDirName) }
func (p *Project) SessionsDir() string { return filepath.Join(p.ReplaiDir(), sessionsDirName) }
func (p *Project) RunnerBin() string   { return filepath.Join(p.CacheDir(), runnerBinName) }
