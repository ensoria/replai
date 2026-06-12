package envelope

import (
	"encoding/json"
	"fmt"

	"github.com/ensoria/replai/internal/evalrt"
	"github.com/ensoria/replai/internal/parse"
)

// Error kinds per spec §4.2.
const (
	KindCompile  = "compile"
	KindRuntime  = "runtime"
	KindPanic    = "panic"
	KindTimeout  = "timeout"
	KindInternal = "internal"
)

// Truncation priorities: what gets dropped first when the envelope exceeds
// the --max-output budget.
const (
	fieldValueJSON = "value.json"
	fieldStdout    = "stdout"
	fieldStderr    = "stderr"
	fieldValueRepr = "value.repr"
	fieldErrorMsg  = "error.message"
	fieldErrorSug  = "error.suggestion"
	fieldStack     = "error.stack"
	fieldMeta      = "meta"
	fieldEnvelope  = "envelope"

	truncMarker = "...(+%d chars, truncated by --max-output)"
)

// Envelope is the single JSON object replai prints for every evaluation.
// Struct field order fixes the JSON key order per spec §7.
type Envelope struct {
	OK              bool                  `json:"ok"`
	Error           *Error                `json:"error,omitempty"`
	Value           *evalrt.ReplaiValue   `json:"value,omitempty"`
	Values          []*evalrt.ReplaiValue `json:"values,omitempty"`
	Stdout          string                `json:"stdout"`
	Stderr          string                `json:"stderr"`
	Defined         []string              `json:"defined"`
	AutoImports     []string              `json:"auto_imports,omitempty"`
	DurationMS      int64                 `json:"duration_ms"`
	Truncated       bool                  `json:"truncated"`
	TruncatedFields []string              `json:"truncated_fields,omitempty"`

	// DefinedTypes carries the child-reported variable types for session
	// persistence; it is not part of the printed envelope.
	DefinedTypes []*evalrt.ReplaiDefined `json:"-"`
	// FinalImports carries the generated import block for session
	// persistence; it is not part of the printed envelope.
	FinalImports []*parse.Import `json:"-"`
}

// Error describes why an evaluation failed, with enough context for the
// caller to take the next step.
type Error struct {
	Kind       string    `json:"kind"`
	Message    string    `json:"message"`
	Position   *Position `json:"position,omitempty"`
	Suggestion string    `json:"suggestion,omitempty"`
	Stack      []string  `json:"stack,omitempty"`
}

// Position locates an error inside the user snippet.
type Position struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// New returns an envelope with non-nil slices so JSON output always carries
// the fixed key set.
func New() *Envelope {
	return &Envelope{Defined: []string{}}
}

// NewError builds a failure envelope.
func NewError(kind, message string) *Envelope {
	e := New()
	e.Error = &Error{Kind: kind, Message: message}
	return e
}

// Marshal renders the envelope as compact JSON, enforcing the maxOutput
// budget. Trimmed content is always marked, never silently dropped.
func (e *Envelope) Marshal(maxOutput int) []byte {
	data, err := json.Marshal(e)
	if err != nil {
		fallback := NewError(KindInternal, "failed to marshal envelope: "+err.Error())
		data, _ = json.Marshal(fallback)
		return data
	}
	if maxOutput <= 0 || len(data) <= maxOutput {
		return data
	}

	// Keep per-field budgets proportional to the total so trimming converges
	// even for small --max-output values.
	keep := maxOutput / 4
	if keep < minTrimKeepBytes {
		keep = minTrimKeepBytes
	}
	steps := []func(*Envelope) bool{
		dropValueJSON,
		func(e *Envelope) bool { return trimStdout(e, keep) },
		func(e *Envelope) bool { return trimStderr(e, keep) },
		func(e *Envelope) bool { return trimRepr(e, keep) },
		func(e *Envelope) bool { return trimErrorMessage(e, keep) },
		func(e *Envelope) bool { return trimErrorSuggestion(e, keep) },
		trimStack,
	}
	for _, step := range steps {
		if !step(e) {
			continue
		}
		e.Truncated = true
		data, err = json.Marshal(e)
		if err == nil && len(data) <= maxOutput {
			return data
		}
	}
	fallback := NewError(KindInternal, "output exceeded --max-output after trimming")
	fallback.Truncated = true
	fallback.TruncatedFields = append(append([]string{}, e.TruncatedFields...), fieldEnvelope)
	data, err = json.Marshal(fallback)
	if err == nil && (maxOutput <= 0 || len(data) <= maxOutput) {
		return data
	}
	fallback.Error.Message = "output exceeded --max-output"
	fallback.TruncatedFields = []string{fieldEnvelope}
	data, _ = json.Marshal(fallback)
	return data
}

// MarshalRaw renders non-envelope command output under the same output budget.
// Oversized raw outputs are replaced by a compact truncation envelope instead
// of silently ignoring --max-output.
func MarshalRaw(data []byte, maxOutput int) []byte {
	if maxOutput <= 0 || len(data) <= maxOutput {
		return data
	}
	env := NewError(KindInternal, "meta output exceeded --max-output")
	env.Error.Suggestion = "raise --max-output to retrieve the full meta output"
	env.Truncated = true
	env.TruncatedFields = []string{fieldMeta}
	return env.Marshal(maxOutput)
}

func (e *Envelope) allValues() []*evalrt.ReplaiValue {
	if e.Value != nil {
		return []*evalrt.ReplaiValue{e.Value}
	}
	return e.Values
}

func (e *Envelope) markTruncated(field string) {
	for _, f := range e.TruncatedFields {
		if f == field {
			return
		}
	}
	e.TruncatedFields = append(e.TruncatedFields, field)
}

func dropValueJSON(e *Envelope) bool {
	changed := false
	for _, v := range e.allValues() {
		if len(v.JSON) > 0 {
			v.JSON = nil
			changed = true
		}
	}
	if changed {
		e.markTruncated(fieldValueJSON)
	}
	return changed
}

func cutString(s string, keep int) (string, bool) {
	if len(s) <= keep {
		return s, false
	}
	return s[:keep] + fmt.Sprintf(truncMarker, len(s)-keep), true
}

func trimStdout(e *Envelope, keep int) bool {
	out, changed := cutString(e.Stdout, keep)
	if changed {
		e.Stdout = out
		e.markTruncated(fieldStdout)
	}
	return changed
}

func trimStderr(e *Envelope, keep int) bool {
	out, changed := cutString(e.Stderr, keep)
	if changed {
		e.Stderr = out
		e.markTruncated(fieldStderr)
	}
	return changed
}

func trimRepr(e *Envelope, keep int) bool {
	changed := false
	for _, v := range e.allValues() {
		repr, c := cutString(v.Repr, keep)
		if c {
			v.Repr = repr
			changed = true
		}
	}
	if changed {
		e.markTruncated(fieldValueRepr)
	}
	return changed
}

func trimErrorMessage(e *Envelope, keep int) bool {
	if e.Error == nil {
		return false
	}
	msg, changed := cutString(e.Error.Message, keep)
	if changed {
		e.Error.Message = msg
		e.markTruncated(fieldErrorMsg)
	}
	return changed
}

func trimErrorSuggestion(e *Envelope, keep int) bool {
	if e.Error == nil {
		return false
	}
	suggestion, changed := cutString(e.Error.Suggestion, keep)
	if changed {
		e.Error.Suggestion = suggestion
		e.markTruncated(fieldErrorSug)
	}
	return changed
}

func trimStack(e *Envelope) bool {
	if e.Error == nil || len(e.Error.Stack) <= maxStackFrames {
		return false
	}
	e.Error.Stack = e.Error.Stack[:maxStackFrames]
	e.markTruncated(fieldStack)
	return true
}

const (
	// minTrimKeepBytes floors how much of an oversized field survives a trim.
	minTrimKeepBytes = 256
	maxStackFrames   = 20
)
