package engine_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/replai/internal/engine"
	"github.com/ensoria/replai/internal/project"
	"github.com/ensoria/replai/internal/session"
)

const (
	e2eTimeout   = 30 * time.Second
	e2eMaxOutput = 65536
)

// envelope mirrors the printed JSON for assertions.
type envelopeOut struct {
	OK    bool `json:"ok"`
	Error *struct {
		Kind       string `json:"kind"`
		Message    string `json:"message"`
		Suggestion string `json:"suggestion"`
		Position   *struct {
			Line   int `json:"line"`
			Column int `json:"column"`
		} `json:"position"`
		Stack []string `json:"stack"`
	} `json:"error"`
	Value *struct {
		Repr string `json:"repr"`
		Type string `json:"type"`
		Err  *struct {
			Nil     bool   `json:"nil"`
			Message string `json:"message"`
		} `json:"err"`
	} `json:"value"`
	Values []*struct {
		Repr string `json:"repr"`
		Err  *struct {
			Nil bool `json:"nil"`
		} `json:"err"`
	} `json:"values"`
	Stdout      string   `json:"stdout"`
	Stderr      string   `json:"stderr"`
	Defined     []string `json:"defined"`
	AutoImports []string `json:"auto_imports"`
}

var _ = Describe("Engine (end-to-end against the fixture project)", Label("e2e"), Ordered, func() {
	var eng *engine.Engine

	newEngine := func(timeout time.Duration) *engine.Engine {
		wd, err := os.Getwd()
		Expect(err).NotTo(HaveOccurred())
		p, err := project.Find(filepath.Join(wd, "testdata", "fixtureproj"))
		Expect(err).NotTo(HaveOccurred())
		Expect(p.EnsureLayout()).To(Succeed())
		return engine.New(p, &engine.Options{
			Timeout:   timeout,
			Depth:     5,
			MaxItems:  50,
			MaxStr:    2000,
			MaxOutput: e2eMaxOutput,
		})
	}

	BeforeAll(func() {
		eng = newEngine(e2eTimeout)
	})

	eval := func(st *session.State, input string) (*envelopeOut, int) {
		out := eng.Eval(st, input)
		env := &envelopeOut{}
		Expect(json.Unmarshal(out.Output, env)).To(Succeed(), string(out.Output))
		return env, out.ExitCode
	}

	It("evaluates an expression with auto-import", func() {
		env, exit := eval(nil, `widget.New("demo")`)
		Expect(exit).To(Equal(engine.ExitOK))
		Expect(env.OK).To(BeTrue())
		Expect(env.Value.Type).To(Equal("*example.com/fixture/pkg/widget.Widget"))
		Expect(env.Value.Repr).To(ContainSubstring(`Name: "demo"`))
		Expect(env.AutoImports).To(ContainElement("example.com/fixture/pkg/widget"))
	})

	It("splits multi-value results and a trailing nil error", func() {
		env, exit := eval(nil, "widget.Multi()")
		Expect(exit).To(Equal(engine.ExitOK))
		Expect(env.Values).To(HaveLen(2))
		Expect(env.Values[1].Err).NotTo(BeNil())
		Expect(env.Values[1].Err.Nil).To(BeTrue())
	})

	It("reports a non-nil returned error in value.err without failing", func() {
		env, exit := eval(nil, "widget.Fail()")
		Expect(exit).To(Equal(engine.ExitOK))
		Expect(env.OK).To(BeTrue())
		Expect(env.Value.Err.Nil).To(BeFalse())
		Expect(env.Value.Err.Message).To(Equal("widget exploded"))
	})

	It("separates snippet stdout and stderr from the envelope", func() {
		env, _ := eval(nil, "widget.PrintBoth()")
		Expect(env.Stdout).To(Equal("to stdout\n"))
		Expect(env.Stderr).To(Equal("to stderr\n"))
	})

	It("imports internal packages of the target project", func() {
		env, exit := eval(nil, "secret.Token()")
		Expect(exit).To(Equal(engine.ExitOK))
		Expect(env.Value.Repr).To(Equal(`"internal-token"`))
	})

	It("retries void calls as statements transparently", func() {
		env, exit := eval(nil, "widget.Void()")
		Expect(exit).To(Equal(engine.ExitOK))
		Expect(env.Stdout).To(Equal("void called\n"))
	})

	It("captures the trailing expression of a statement block", func() {
		env, exit := eval(nil, "x := 2\ny := 3\nx * y")
		Expect(exit).To(Equal(engine.ExitOK))
		Expect(env.Value.Repr).To(Equal("6"))
		Expect(env.Defined).To(Equal([]string{"x", "y"}))
	})

	It("reports panics with a cleaned, remapped stack", func() {
		env, exit := eval(nil, "widget.Boom()")
		Expect(exit).To(Equal(engine.ExitEval))
		Expect(env.Error.Kind).To(Equal("panic"))
		Expect(env.Error.Message).To(Equal("panic: boom from widget"))
		Expect(env.Error.Stack).To(ContainElement("snippet:1"))
		for _, frame := range env.Error.Stack {
			Expect(frame).NotTo(ContainSubstring("zz_replai_rt_"))
		}
	})

	It("maps compile errors to snippet positions with suggestions", func() {
		st := &session.State{}
		_, exit := eval(st, `w := widget.New("a")`)
		Expect(exit).To(Equal(engine.ExitOK))
		env, exit := eval(st, `widget.Nwe("x")`)
		Expect(exit).To(Equal(engine.ExitEval))
		Expect(env.Error.Kind).To(Equal("compile"))
		Expect(env.Error.Position.Line).To(Equal(1))
		Expect(env.Error.Suggestion).To(ContainSubstring("did you mean widget.New?"))
	})

	It("kills runaway evaluations with exit code 124 and keeps partial output", func() {
		fast := newEngine(1500 * time.Millisecond)
		out := fast.Eval(nil, "widget.Sleep(30 * time.Second)")
		Expect(out.ExitCode).To(Equal(engine.ExitTimeout))
		env := &envelopeOut{}
		Expect(json.Unmarshal(out.Output, env)).To(Succeed())
		Expect(env.Error.Kind).To(Equal("timeout"))
		Expect(env.Stdout).To(Equal("sleeping\n"))
	})

	It("persists state across separate engine invocations (session continuity)", func() {
		st := &session.State{}
		_, exit := eval(st, `w := widget.New("first")`)
		Expect(exit).To(Equal(engine.ExitOK))
		Expect(st.Entries).To(HaveLen(1))
		Expect(st.Entries[0].Defined).To(HaveKeyWithValue("w", "*example.com/fixture/pkg/widget.Widget"))

		// A fresh engine sees the same state, as a separate process would
		// after loading it from disk.
		eng2 := newEngine(e2eTimeout)
		out := eng2.Eval(st, "w.Name")
		env := &envelopeOut{}
		Expect(json.Unmarshal(out.Output, env)).To(Succeed())
		Expect(env.Value.Repr).To(Equal(`"first"`))
	})

	It("replays heap mutations made by expression entries", func() {
		st := &session.State{}
		eval(st, `w := widget.New("a")`)
		eval(st, `w.Rename("mutated")`)
		env, _ := eval(st, "w.Name")
		Expect(env.Value.Repr).To(Equal(`"mutated"`))
	})

	It("blocks restricted imports under --restrict", func() {
		restricted := newEngine(e2eTimeout)
		restricted.Opts.Restrict = true
		out := restricted.Eval(nil, `import "os/exec"
exec.Command("ls")`)
		Expect(out.ExitCode).To(Equal(engine.ExitUsage))
		env := &envelopeOut{}
		Expect(json.Unmarshal(out.Output, env)).To(Succeed())
		Expect(env.Error.Message).To(ContainSubstring("--restrict"))
	})
})
