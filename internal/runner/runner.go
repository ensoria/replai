package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ensoria/replai/internal/evalrt"
)

const (
	goCommand     = "go"
	buildFlagsEnv = "GOFLAGS=-mod=readonly"

	// Exit code reported when the child was killed by the timeout.
	ExitTimeout = 124
)

// BuildError carries the raw `go build` stderr for error mapping.
type BuildError struct {
	Stderr string
}

func (e *BuildError) Error() string { return e.Stderr }

// Build compiles the generated runner in file-list mode. files must be
// absolute paths inside the project tree so `internal` imports resolve.
func Build(ctx context.Context, projectRoot string, files []string, out string, extraEnv []string) error {
	args := append([]string{"build", "-o", out}, files...)
	cmd := exec.CommandContext(ctx, goCommand, args...)
	cmd.Dir = projectRoot
	cmd.Env = append(append(os.Environ(), buildFlagsEnv), extraEnv...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	cmd.Stdout = io.Discard
	if err := cmd.Run(); err != nil {
		if stderr.Len() > 0 {
			return &BuildError{Stderr: stderr.String()}
		}
		return fmt.Errorf("go build failed: %w", err)
	}
	return nil
}

// Result is the outcome of one child execution.
type Result struct {
	// Wire is the child-reported result; nil when the child crashed or was
	// killed before writing its envelope.
	Wire *evalrt.ReplaiResult
	// Stdout/Stderr is output produced by the current snippet only.
	Stdout string
	Stderr string
	// Diag is the suppressed stderr stream (crash diagnostics).
	Diag     string
	ExitCode int
	TimedOut bool
}

// Run executes the built runner with the replai fd protocol: fd3 envelope,
// fd4 snippet stdout, fd5 snippet stderr, fd6 diagnostics. The child is
// placed in its own process group and the whole group is SIGKILLed on
// timeout, so a partial result is still returned.
func Run(projectRoot, bin string, extraEnv []string, timeout time.Duration) (*Result, error) {
	type pipe struct{ r, w *os.File }
	pipes := make([]*pipe, 4)
	for i := range pipes {
		r, w, err := os.Pipe()
		if err != nil {
			return nil, err
		}
		pipes[i] = &pipe{r: r, w: w}
	}

	cmd := exec.Command(bin)
	cmd.Dir = projectRoot
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdin = nil
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.ExtraFiles = []*os.File{pipes[0].w, pipes[1].w, pipes[2].w, pipes[3].w}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		for _, p := range pipes {
			p.r.Close()
			p.w.Close()
		}
		return nil, err
	}
	// The child owns the write ends now.
	for _, p := range pipes {
		p.w.Close()
	}

	outputs := make([]string, 4)
	var wg sync.WaitGroup
	for i, p := range pipes {
		wg.Add(1)
		go func(i int, r *os.File) {
			defer wg.Done()
			data, _ := io.ReadAll(r)
			outputs[i] = string(data)
			r.Close()
		}(i, p.r)
	}

	var timedOut atomic.Bool
	timer := time.AfterFunc(timeout, func() {
		timedOut.Store(true)
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	})
	waitErr := cmd.Wait()
	timer.Stop()
	wg.Wait()

	res := &Result{
		Stdout:   outputs[1],
		Stderr:   outputs[2],
		Diag:     outputs[3],
		TimedOut: timedOut.Load(),
	}
	if res.TimedOut {
		res.ExitCode = ExitTimeout
	} else if cmd.ProcessState != nil {
		res.ExitCode = cmd.ProcessState.ExitCode()
	}
	if waitErr != nil && cmd.ProcessState == nil {
		return nil, waitErr
	}
	if env := strings.TrimSpace(outputs[0]); env != "" {
		wire := &evalrt.ReplaiResult{}
		if err := json.Unmarshal([]byte(env), wire); err == nil {
			res.Wire = wire
		}
	}
	return res, nil
}
