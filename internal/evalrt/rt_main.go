package evalrt

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// File descriptors handed to the child by the parent via exec ExtraFiles.
const (
	replaiFDEnvelope = 3 // single JSON ReplaiResult on completion
	replaiFDStdout   = 4 // stdout produced by the current snippet only
	replaiFDStderr   = 5 // stderr produced by the current snippet only
	replaiFDDiag     = 6 // suppressed stderr, used by the parent for crash diagnosis
)

// Environment variables carrying formatter limits from the parent.
const (
	replaiEnvDepth    = "REPLAI_DEPTH"
	replaiEnvMaxItems = "REPLAI_MAX_ITEMS"
	replaiEnvMaxStr   = "REPLAI_MAX_STR"
)

const (
	replaiExitOK    = 0
	replaiExitPanic = 1
	replaiExitSetup = 2
)

// replaiCurrentSnippet marks a line range belonging to the snippet being
// evaluated right now (as opposed to a re-executed session entry).
const replaiCurrentSnippet = -1

// replaiLineRange maps a span of the generated file back to its origin so
// panic stacks and compile errors can be reported in snippet coordinates.
type replaiLineRange struct {
	Start    int // first line in the generated file (1-based, inclusive)
	End      int // last line in the generated file (inclusive)
	Entry    int // session entry index, or replaiCurrentSnippet
	SrcStart int // line in the original snippet corresponding to Start
}

// replaiGlobalCtx lets replaiCapture receive a multi-value expression as its
// sole argument (the only form Go spreads across a variadic parameter).
var replaiGlobalCtx *replaiCtx

// replaiCtx threads evaluation state through the generated body.
type replaiCtx struct {
	envOut  *os.File
	devnull *os.File
	result  *ReplaiResult
	started time.Time
	running bool
}

// replaiRun is the entry point called by the generated main function.
func replaiRun(body func(*replaiCtx), lineMap []replaiLineRange, genFile string) {
	replaiLoadLimits()
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		os.Exit(replaiExitSetup)
	}
	// Route stderr to the diagnostic pipe and silence stdout: output from
	// re-executed session entries must not pollute the snippet's streams.
	_ = syscall.Dup2(replaiFDDiag, 2)
	_ = syscall.Dup2(int(devnull.Fd()), 1)

	rc := &replaiCtx{
		envOut:  os.NewFile(replaiFDEnvelope, "replai-envelope"),
		devnull: devnull,
		result:  &ReplaiResult{Values: []*ReplaiValue{}, Defined: []*ReplaiDefined{}},
	}
	replaiGlobalCtx = rc
	defer func() {
		r := recover()
		rc.replaiEnd()
		if r != nil {
			rc.result.Panic = replaiNewPanic(r, debug.Stack(), lineMap, genFile)
		}
		if data, jerr := json.Marshal(rc.result); jerr == nil {
			_, _ = rc.envOut.Write(data)
		}
		_ = rc.envOut.Close()
		if r != nil {
			os.Exit(replaiExitPanic)
		}
		os.Exit(replaiExitOK)
	}()
	body(rc)
}

// replaiBegin switches the process streams to the snippet-output pipes and
// starts the duration clock. Called immediately before the current snippet.
func (rc *replaiCtx) replaiBegin() {
	_ = syscall.Dup2(replaiFDStdout, 1)
	_ = syscall.Dup2(replaiFDStderr, 2)
	rc.running = true
	rc.started = time.Now()
}

// replaiEnd stops the duration clock and re-silences the streams.
func (rc *replaiCtx) replaiEnd() {
	if !rc.running {
		return
	}
	rc.result.DurationMS = time.Since(rc.started).Milliseconds()
	rc.running = false
	_ = syscall.Dup2(int(rc.devnull.Fd()), 1)
	_ = syscall.Dup2(replaiFDDiag, 2)
}

func replaiLoadLimits() {
	if n, err := strconv.Atoi(os.Getenv(replaiEnvDepth)); err == nil && n > 0 {
		replaiMaxDepth = n
	}
	if n, err := strconv.Atoi(os.Getenv(replaiEnvMaxItems)); err == nil && n > 0 {
		replaiMaxItems = n
	}
	if n, err := strconv.Atoi(os.Getenv(replaiEnvMaxStr)); err == nil && n > 0 {
		replaiMaxStr = n
	}
}

// replaiNewPanic converts a recovered value and raw stack into a cleaned
// panic report: replai harness and runtime frames are dropped, and frames in
// the generated file are remapped to snippet/entry coordinates.
func replaiNewPanic(r interface{}, stack []byte, lineMap []replaiLineRange, genFile string) *ReplaiPanic {
	return &ReplaiPanic{
		Value: fmt.Sprintf("%v", r),
		Stack: replaiCleanStack(string(stack), lineMap, genFile),
	}
}

func replaiCleanStack(stack string, lineMap []replaiLineRange, genFile string) []string {
	var frames []string
	lines := strings.Split(stack, "\n")
	for i := 0; i < len(lines); i++ {
		fn := strings.TrimSpace(lines[i])
		if fn == "" || strings.HasPrefix(fn, "goroutine ") || strings.HasPrefix(lines[i], "\t") {
			continue
		}
		if i+1 >= len(lines) || !strings.HasPrefix(lines[i+1], "\t") {
			continue
		}
		loc := strings.TrimSpace(lines[i+1])
		i++
		name := fn
		if p := strings.LastIndex(name, "("); p > 0 {
			name = name[:p]
		}
		if strings.HasPrefix(name, "runtime.") || strings.HasPrefix(name, "runtime/") ||
			name == "panic" || name == "main.main" {
			continue
		}
		file, lineNo := replaiSplitLoc(loc)
		if strings.Contains(file, "zz_replai_rt_") {
			continue
		}
		if file == genFile {
			entry, srcLine, ok := replaiMapLine(lineMap, lineNo)
			if !ok {
				continue // harness line inside the generated file
			}
			if entry == replaiCurrentSnippet {
				frames = append(frames, fmt.Sprintf("snippet:%d", srcLine))
			} else {
				frames = append(frames, fmt.Sprintf("session entry[%d]:%d", entry, srcLine))
			}
			continue
		}
		if strings.HasPrefix(name, "main.replai") {
			continue
		}
		frames = append(frames, fmt.Sprintf("%s (%s:%d)", name, file, lineNo))
	}
	return frames
}

// replaiSplitLoc splits "/path/file.go:42 +0x1d" into path and line.
func replaiSplitLoc(loc string) (string, int) {
	if sp := strings.IndexByte(loc, ' '); sp >= 0 {
		loc = loc[:sp]
	}
	colon := strings.LastIndexByte(loc, ':')
	if colon < 0 {
		return loc, 0
	}
	n, err := strconv.Atoi(loc[colon+1:])
	if err != nil {
		return loc, 0
	}
	return loc[:colon], n
}

func replaiMapLine(lineMap []replaiLineRange, line int) (entry int, srcLine int, ok bool) {
	for _, lr := range lineMap {
		if line >= lr.Start && line <= lr.End {
			return lr.Entry, lr.SrcStart + (line - lr.Start), true
		}
	}
	return 0, 0, false
}
