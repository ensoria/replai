package errmap_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/ensoria/replai/internal/codegen"
	"github.com/ensoria/replai/internal/errmap"
)

var _ = Describe("MapBuildErrors", func() {
	const mainPath = "/proj/.replai/run/main.go"

	lineMap := []*codegen.LineRange{
		{Start: 8, End: 9, Entry: 0, SrcStart: 1},
		{Start: 12, End: 14, Entry: codegen.EntryCurrent, SrcStart: 1},
	}

	It("maps a snippet error to snippet coordinates", func() {
		stderr := "# command-line-arguments\n/proj/.replai/run/main.go:13:5: undefined: foo\n"
		errs := errmap.MapBuildErrors(stderr, mainPath, lineMap)
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Origin).To(Equal(errmap.OriginSnippet))
		Expect(errs[0].Line).To(Equal(2))
		Expect(errs[0].Column).To(Equal(5))
		Expect(errs[0].Msg).To(Equal("undefined: foo"))
	})

	It("maps an error in a re-executed entry to that entry", func() {
		stderr := "/proj/.replai/run/main.go:9:1: undefined: bar\n"
		errs := errmap.MapBuildErrors(stderr, mainPath, lineMap)
		Expect(errs).To(HaveLen(1))
		Expect(errs[0].Origin).To(Equal(errmap.OriginEntry))
		Expect(errs[0].Entry).To(Equal(0))
		Expect(errs[0].Line).To(Equal(2))
	})

	It("marks unmapped generated lines as harness", func() {
		stderr := "/proj/.replai/run/main.go:3:1: some harness problem\n"
		errs := errmap.MapBuildErrors(stderr, mainPath, lineMap)
		Expect(errs[0].Origin).To(Equal(errmap.OriginHarness))
	})
})

var _ = Describe("Primary", func() {
	It("prefers snippet errors over entry errors", func() {
		errs := []*errmap.BuildErr{
			{Origin: errmap.OriginEntry, Msg: "entry"},
			{Origin: errmap.OriginSnippet, Msg: "snippet"},
		}
		Expect(errmap.Primary(errs).Msg).To(Equal("snippet"))
	})

	It("returns nil for no errors", func() {
		Expect(errmap.Primary(nil)).To(BeNil())
	})
})

var _ = Describe("CleanCrash", func() {
	const mainPath = "/proj/.replai/run/main.go"
	lineMap := []*codegen.LineRange{{Start: 10, End: 10, Entry: codegen.EntryCurrent, SrcStart: 1}}

	It("extracts the panic head as the message", func() {
		diag := "panic: bg\n\ngoroutine 7 [running]:\nmain.replaiBody.func1()\n\t/proj/.replai/run/main.go:10 +0x2c\n"
		msg, stack := errmap.CleanCrash(diag, mainPath, lineMap)
		Expect(msg).To(Equal("panic: bg"))
		Expect(stack).To(ContainElement("snippet:1"))
	})

	It("extracts fatal errors", func() {
		msg, _ := errmap.CleanCrash("fatal error: all goroutines are asleep - deadlock!\n", mainPath, lineMap)
		Expect(msg).To(Equal("fatal error: all goroutines are asleep - deadlock!"))
	})

	It("drops replai runtime frames", func() {
		diag := "panic: x\n\t/proj/.replai/run/zz_replai_rt_main.go:70 +0x1d\nruntime.main()\n"
		_, stack := errmap.CleanCrash(diag, mainPath, lineMap)
		Expect(stack).To(BeEmpty())
	})
})
