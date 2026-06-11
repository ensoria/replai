package evalrt

import (
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type sample struct {
	Name string
	N    int
	Next *sample
}

var _ = Describe("replaiFormat (rt_format.go)", func() {
	BeforeEach(func() {
		replaiMaxDepth = 5
		replaiMaxItems = 50
		replaiMaxStr = 2000
	})

	It("renders nil", func() {
		Expect(replaiFormat(nil)).To(Equal("nil"))
	})

	It("renders scalars", func() {
		Expect(replaiFormat(42)).To(Equal("42"))
		Expect(replaiFormat(true)).To(Equal("true"))
		Expect(replaiFormat("hi")).To(Equal(`"hi"`))
	})

	It("dereferences pointers instead of printing addresses", func() {
		s := &sample{Name: "a", N: 1}
		out := replaiFormat(s)
		Expect(out).To(HavePrefix("&evalrt.sample{"))
		Expect(out).To(ContainSubstring(`Name: "a"`))
		Expect(out).NotTo(ContainSubstring("0x"))
	})

	It("renders nil pointers with their type", func() {
		var s *sample
		Expect(replaiFormat(s)).To(Equal("(*evalrt.sample)(nil)"))
	})

	It("cuts cycles with a marker", func() {
		s := &sample{Name: "loop"}
		s.Next = s
		Expect(replaiFormat(s)).To(ContainSubstring("<cycle: *evalrt.sample>"))
	})

	It("limits depth with an explicit marker naming the flag", func() {
		replaiMaxDepth = 1
		nested := &sample{Name: "outer", Next: &sample{Name: "inner"}}
		out := replaiFormat(nested)
		Expect(out).To(ContainSubstring("...(depth limit, use --depth="))
		Expect(out).NotTo(ContainSubstring("inner"))
	})

	It("limits slice items with a count and the lifting flag", func() {
		replaiMaxItems = 3
		out := replaiFormat([]int{1, 2, 3, 4, 5})
		Expect(out).To(Equal("[]int{1, 2, 3, ...(+2 items, use --max-items=5)}"))
	})

	It("truncates long strings with the remaining count", func() {
		replaiMaxStr = 4
		out := replaiFormat("abcdefgh")
		Expect(out).To(Equal(`"abcd"...(+4 chars, use --max-str=8)`))
	})

	It("renders maps with deterministically sorted keys", func() {
		m := map[string]int{"b": 2, "a": 1, "c": 3}
		Expect(replaiFormat(m)).To(Equal(`map[string]int{"a": 1, "b": 2, "c": 3}`))
	})

	It("renders time.Time as RFC 3339", func() {
		ts := time.Date(2026, 6, 11, 9, 30, 0, 0, time.UTC)
		Expect(replaiFormat(ts)).To(Equal(`time.Time("2026-06-11T09:30:00Z")`))
	})

	It("renders time.Duration with both human and nanosecond forms", func() {
		Expect(replaiFormat(30 * time.Second)).To(Equal("30s (30000000000ns)"))
	})

	It("renders valid UTF-8 byte slices as strings", func() {
		Expect(replaiFormat([]byte("hello"))).To(Equal(`[]byte("hello")`))
	})

	It("renders invalid UTF-8 byte slices as hex", func() {
		Expect(replaiFormat([]byte{0xff, 0xfe})).To(Equal("[]byte(0xfffe)"))
	})
})

var _ = Describe("replaiFullTypeOf (rt_format.go)", func() {
	It("qualifies named types with the full package path", func() {
		Expect(replaiFullTypeOf(&sample{})).To(Equal("*github.com/ensoria/replai/internal/evalrt.sample"))
	})

	It("handles containers", func() {
		Expect(replaiFullTypeOf([]string{})).To(Equal("[]string"))
		Expect(replaiFullTypeOf(map[string]int{})).To(Equal("map[string]int"))
	})

	It("renders nil as nil", func() {
		Expect(replaiFullTypeOf(nil)).To(Equal("nil"))
	})
})

var _ = Describe("limit loading (rt_main.go)", func() {
	It("reads limits from the environment", func() {
		GinkgoT().Setenv("REPLAI_DEPTH", "9")
		GinkgoT().Setenv("REPLAI_MAX_ITEMS", "7")
		GinkgoT().Setenv("REPLAI_MAX_STR", "11")
		replaiLoadLimits()
		Expect(replaiMaxDepth).To(Equal(9))
		Expect(replaiMaxItems).To(Equal(7))
		Expect(replaiMaxStr).To(Equal(11))
		replaiMaxDepth, replaiMaxItems, replaiMaxStr = 5, 50, 2000
	})
})

var _ = Describe("string truncation marker", func() {
	It("never silently drops content", func() {
		replaiMaxStr = 10
		long := strings.Repeat("x", 100)
		out := replaiFormat(long)
		Expect(out).To(ContainSubstring("+90 chars"))
		replaiMaxStr = 2000
	})
})
