package evalrt

import (
	"errors"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type nilErr struct{}

func (*nilErr) Error() string { return "should not be called" }

var _ = Describe("replaiCapture (rt_capture.go)", func() {
	var rc *replaiCtx

	BeforeEach(func() {
		rc = &replaiCtx{result: &ReplaiResult{Values: []*ReplaiValue{}, Defined: []*ReplaiDefined{}}}
		replaiGlobalCtx = rc
	})

	It("captures a single value with repr, type, and JSON", func() {
		replaiCapture(42)
		Expect(rc.result.Values).To(HaveLen(1))
		v := rc.result.Values[0]
		Expect(v.Repr).To(Equal("42"))
		Expect(v.Type).To(Equal("int"))
		Expect(string(v.JSON)).To(Equal("42"))
		Expect(v.Err).To(BeNil())
	})

	It("splits a trailing non-nil error into value.err", func() {
		replaiCapture(7, errors.New("kaboom"))
		Expect(rc.result.Values).To(HaveLen(1))
		v := rc.result.Values[0]
		Expect(v.Repr).To(Equal("7"))
		Expect(v.Err).NotTo(BeNil())
		Expect(v.Err.Nil).To(BeFalse())
		Expect(v.Err.Message).To(Equal("kaboom"))
	})

	It("treats a trailing untyped nil in a multi-value result as a nil error", func() {
		replaiCapture(7, "seven", nil)
		Expect(rc.result.Values).To(HaveLen(2))
		Expect(rc.result.Values[1].Err).NotTo(BeNil())
		Expect(rc.result.Values[1].Err.Nil).To(BeTrue())
	})

	It("reports a lone error as a value holding only err", func() {
		replaiCapture(errors.New("only"))
		Expect(rc.result.Values).To(HaveLen(1))
		v := rc.result.Values[0]
		Expect(v.Repr).To(BeEmpty())
		Expect(v.Err.Nil).To(BeFalse())
		Expect(v.Err.Message).To(Equal("only"))
	})

	It("does not call Error on a typed nil error pointer", func() {
		var e *nilErr
		replaiCapture(1, error(e))
		v := rc.result.Values[0]
		Expect(v.Err).NotTo(BeNil())
		Expect(v.Err.Nil).To(BeTrue())
	})

	It("keeps a single nil value as nil, not as an error", func() {
		replaiCapture(nil)
		Expect(rc.result.Values).To(HaveLen(1))
		Expect(rc.result.Values[0].Repr).To(Equal("nil"))
		Expect(rc.result.Values[0].Err).To(BeNil())
	})

	It("records defined variables with full types", func() {
		rc.replaiDefined("x", 42)
		Expect(rc.result.Defined).To(HaveLen(1))
		Expect(rc.result.Defined[0].Name).To(Equal("x"))
		Expect(rc.result.Defined[0].Type).To(Equal("int"))
	})

	It("omits JSON for unmarshalable values", func() {
		replaiCapture(func() {})
		Expect(rc.result.Values[0].JSON).To(BeNil())
	})
})
