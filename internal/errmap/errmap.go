package errmap

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/ensoria/replai/internal/codegen"
)

// Origin of a mapped build error.
const (
	OriginSnippet = "snippet"
	OriginEntry   = "entry"
	OriginHarness = "harness"
)

var (
	buildErrRe = regexp.MustCompile(`(?m)^(.*\.go):(\d+)(?::(\d+))?: (.+)$`)

	panicHeadRe = regexp.MustCompile(`(?m)^(panic: .*|fatal error: .*)$`)
)

// BuildErr is one compiler diagnostic mapped back to user coordinates.
type BuildErr struct {
	Origin string
	Entry  int // session entry index when Origin == OriginEntry
	Line   int // line within the originating snippet
	Column int
	Msg    string
}

// MapBuildErrors parses `go build` stderr and maps each diagnostic in the
// generated main file back to snippet/entry coordinates.
func MapBuildErrors(stderr, mainPath string, lineMap []*codegen.LineRange) []*BuildErr {
	var out []*BuildErr
	for _, m := range buildErrRe.FindAllStringSubmatch(stderr, -1) {
		file, msg := m[1], m[4]
		line, _ := strconv.Atoi(m[2])
		col := 0
		if m[3] != "" {
			col, _ = strconv.Atoi(m[3])
		}
		if !strings.HasSuffix(mainPath, file) && file != mainPath {
			out = append(out, &BuildErr{Origin: OriginHarness, Line: line, Column: col, Msg: msg})
			continue
		}
		be := &BuildErr{Origin: OriginHarness, Line: line, Column: col, Msg: msg}
		for _, lr := range lineMap {
			if line >= lr.Start && line <= lr.End {
				be.Line = lr.SrcStart + (line - lr.Start)
				switch lr.Entry {
				case codegen.EntryCurrent:
					be.Origin = OriginSnippet
				case codegen.EntryHarness:
					be.Origin = OriginHarness
				default:
					be.Origin = OriginEntry
					be.Entry = lr.Entry
				}
				break
			}
		}
		out = append(out, be)
	}
	return out
}

// Primary picks the diagnostic to surface: the first snippet error, else the
// first entry error, else the first one at all.
func Primary(errs []*BuildErr) *BuildErr {
	for _, e := range errs {
		if e.Origin == OriginSnippet {
			return e
		}
	}
	for _, e := range errs {
		if e.Origin == OriginEntry {
			return e
		}
	}
	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// CleanCrash extracts a concise message and a cleaned stack from the raw
// stderr of a child that died without writing its envelope (fatal error,
// unrecovered goroutine panic, os.Exit). References to the generated file
// are remapped to snippet/entry coordinates via the line map.
func CleanCrash(diag, mainPath string, lineMap []*codegen.LineRange) (string, []string) {
	msg := strings.TrimSpace(diag)
	if m := panicHeadRe.FindString(diag); m != "" {
		msg = m
	} else if idx := strings.IndexByte(msg, '\n'); idx > 0 {
		msg = msg[:idx]
	}
	var stack []string
	for _, line := range strings.Split(diag, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "goroutine ") {
			continue
		}
		if strings.HasPrefix(trimmed, "panic: ") || strings.HasPrefix(trimmed, "fatal error: ") {
			continue
		}
		if strings.Contains(trimmed, "zz_replai_rt_") || strings.HasPrefix(trimmed, "runtime.") {
			continue
		}
		stack = append(stack, remapGenFileRefs(trimmed, mainPath, lineMap))
		if len(stack) >= maxCrashStackLines {
			break
		}
	}
	return msg, stack
}

var genLineRefRe = regexp.MustCompile(`:(\d+)`)

// remapGenFileRefs rewrites "<mainPath>:NN" references into snippet/entry
// coordinates.
func remapGenFileRefs(line, mainPath string, lineMap []*codegen.LineRange) string {
	if mainPath == "" || !strings.Contains(line, mainPath) {
		return line
	}
	idx := strings.Index(line, mainPath)
	rest := line[idx+len(mainPath):]
	m := genLineRefRe.FindStringSubmatch(rest)
	if m == nil {
		return line
	}
	genLine, _ := strconv.Atoi(m[1])
	for _, lr := range lineMap {
		if genLine >= lr.Start && genLine <= lr.End {
			src := lr.SrcStart + (genLine - lr.Start)
			loc := fmt.Sprintf("snippet:%d", src)
			if lr.Entry >= 0 {
				loc = fmt.Sprintf("session entry[%d]:%d", lr.Entry, src)
			}
			return line[:idx] + loc
		}
	}
	return line
}

const maxCrashStackLines = 20
