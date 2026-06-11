package evalrt

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("stack cleaning (rt_main.go)", func() {
	const genFile = "/proj/.replai/run/main.go"

	lineMap := []replaiLineRange{
		{Start: 10, End: 12, Entry: 0, SrcStart: 1},
		{Start: 14, End: 14, Entry: replaiCurrentSnippet, SrcStart: 1},
	}

	It("keeps user frames, remaps generated lines, and drops harness frames", func() {
		stack := "goroutine 1 [running]:\n" +
			"runtime/debug.Stack()\n\t/goroot/src/runtime/debug/stack.go:26 +0x64\n" +
			"main.replaiRun.func1()\n\t/proj/.replai/run/zz_replai_rt_main.go:70 +0x1d\n" +
			"panic({0x102, 0x14000})\n\t/goroot/src/runtime/panic.go:792 +0x124\n" +
			"example.com/fixture/pkg/widget.Boom(...)\n\t/proj/pkg/widget/widget.go:50 +0x2c\n" +
			"main.replaiBody(0x1400)\n\t/proj/.replai/run/main.go:14 +0x20\n" +
			"main.main()\n\t/proj/.replai/run/main.go:30 +0x1c\n"
		frames := replaiCleanStack(stack, lineMap, genFile)
		Expect(frames).To(Equal([]string{
			"example.com/fixture/pkg/widget.Boom (/proj/pkg/widget/widget.go:50)",
			"snippet:1",
		}))
	})

	It("labels frames from re-executed session entries", func() {
		stack := "goroutine 1 [running]:\n" +
			"main.replaiBody(0x1400)\n\t/proj/.replai/run/main.go:11 +0x20\n"
		frames := replaiCleanStack(stack, lineMap, genFile)
		Expect(frames).To(Equal([]string{"session entry[0]:2"}))
	})
})

var _ = Describe("location parsing (rt_main.go)", func() {
	It("splits file and line from a traceback location", func() {
		file, line := replaiSplitLoc("/a/b/main.go:42 +0x1d")
		Expect(file).To(Equal("/a/b/main.go"))
		Expect(line).To(Equal(42))
	})
})

var _ = Describe("line mapping (rt_main.go)", func() {
	lineMap := []replaiLineRange{{Start: 5, End: 7, Entry: replaiCurrentSnippet, SrcStart: 1}}

	It("maps generated lines back to snippet lines", func() {
		entry, src, ok := replaiMapLine(lineMap, 6)
		Expect(ok).To(BeTrue())
		Expect(entry).To(Equal(replaiCurrentSnippet))
		Expect(src).To(Equal(2))
	})

	It("reports unmapped lines", func() {
		_, _, ok := replaiMapLine(lineMap, 99)
		Expect(ok).To(BeFalse())
	})
})
