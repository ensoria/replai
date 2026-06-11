package suggest

import (
	"fmt"
	"go/token"
	"regexp"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

const (
	maxCandidates = 5
	maxDistance   = 2
)

var (
	undefinedSelRe  = regexp.MustCompile(`undefined: (\w+)\.(\w+)`)
	undefinedRe     = regexp.MustCompile(`undefined: (\w+)$`)
	missingModuleRe = regexp.MustCompile(`no required module provides package (\S+?);?\s`)
	noFieldRe       = regexp.MustCompile(`type (\S+) has no field or method (\w+)`)
)

// ForBuildError produces a "next step" suggestion for a compiler diagnostic.
// imports maps package names/aliases used in the generated file to import
// paths; locals are session variable and declaration names.
func ForBuildError(projectRoot, msg string, imports map[string]string, locals []string) string {
	if m := missingModuleRe.FindStringSubmatch(msg + " "); m != nil {
		return fmt.Sprintf("the module providing %q is not in go.mod; run: go get %s (in %s)", m[1], m[1], projectRoot)
	}
	if m := undefinedSelRe.FindStringSubmatch(msg); m != nil {
		pkgName, sym := m[1], m[2]
		path, ok := imports[pkgName]
		if !ok {
			return fmt.Sprintf("package %q is not imported; add: import %q-style path for it", pkgName, pkgName)
		}
		exports := loadExports(projectRoot, path)
		if similar := Similar(sym, exports); len(similar) > 0 {
			return fmt.Sprintf("did you mean %s.%s? (similar symbols: %s); use `:funcs %s` to list the package API",
				pkgName, similar[0], strings.Join(similar, ", "), path)
		}
		return fmt.Sprintf("%s.%s does not exist; use `:funcs %s` to list the package API", pkgName, sym, path)
	}
	if m := undefinedRe.FindStringSubmatch(msg); m != nil {
		sym := m[1]
		candidates := append([]string{}, locals...)
		for name := range imports {
			candidates = append(candidates, name)
		}
		if similar := Similar(sym, candidates); len(similar) > 0 {
			return fmt.Sprintf("did you mean %s? (known names: %s); `:vars` lists session variables", similar[0], strings.Join(similar, ", "))
		}
		if strings.ToLower(sym) == sym {
			return fmt.Sprintf("%s is not defined in this session; if %s is a package, auto-import failed — the referenced symbol may not exist (check with `:funcs <import-path>`) or add an explicit import", sym, sym)
		}
		return fmt.Sprintf("%s is not defined in this session; `:vars` lists session variables", sym)
	}
	if m := noFieldRe.FindStringSubmatch(msg); m != nil {
		return fmt.Sprintf("use `:fields %s` or `:funcs` on the package to inspect the available API", strings.TrimPrefix(m[1], "*"))
	}
	if strings.Contains(msg, "no new variables on left side of :=") {
		return "all variables already exist; replai normally rewrites := to = — if this persists, use = or :reset the session"
	}
	return ""
}

// Similar ranks candidates by levenshtein distance (<= maxDistance) or prefix
// match against target.
func Similar(target string, candidates []string) []string {
	type scored struct {
		name string
		dist int
	}
	lower := strings.ToLower(target)
	var hits []scored
	seen := map[string]bool{}
	for _, c := range candidates {
		if c == "" || c == target || seen[c] {
			continue
		}
		seen[c] = true
		d := Levenshtein(lower, strings.ToLower(c))
		if d <= maxDistance || strings.HasPrefix(strings.ToLower(c), lower) {
			hits = append(hits, scored{name: c, dist: d})
		}
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].dist != hits[j].dist {
			return hits[i].dist < hits[j].dist
		}
		return hits[i].name < hits[j].name
	})
	if len(hits) > maxCandidates {
		hits = hits[:maxCandidates]
	}
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.name
	}
	return out
}

// Levenshtein computes the edit distance between two strings.
func Levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	ra, rb := []rune(a), []rune(b)
	prev := make([]int, len(rb)+1)
	cur := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		cur[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			cur[j] = min3(cur[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(rb)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}

// loadExports returns the exported names of a package, or nil when it cannot
// be loaded quickly.
func loadExports(projectRoot, path string) []string {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes,
		Dir:  projectRoot,
		Fset: token.NewFileSet(),
	}
	pkgs, err := packages.Load(cfg, path)
	if err != nil || len(pkgs) == 0 || pkgs[0].Types == nil {
		return nil
	}
	scope := pkgs[0].Types.Scope()
	var out []string
	for _, name := range scope.Names() {
		if token.IsExported(name) {
			out = append(out, name)
		}
	}
	return out
}
