package evalrt

import "encoding/json"

// The rt_*.go files in this package are embedded into the replai binary and
// written into the generated runner directory rewritten as `package main`.
// They therefore obey two hard constraints:
//   - import the standard library only
//   - every top-level identifier carries a "Replai"/"replai" prefix so it
//     cannot collide with user-hoisted declarations
//
// ReplaiResult is the wire payload the child process writes to the envelope
// file descriptor; the parent unmarshals it via this very package.

// ReplaiResult is the child-side evaluation result sent over the envelope fd.
type ReplaiResult struct {
	Values     []*ReplaiValue   `json:"values"`
	Defined    []*ReplaiDefined `json:"defined"`
	DurationMS int64            `json:"duration_ms"`
	Panic      *ReplaiPanic     `json:"panic,omitempty"`
}

// ReplaiValue is one captured evaluation result.
type ReplaiValue struct {
	Repr string          `json:"repr,omitempty"`
	Type string          `json:"type,omitempty"`
	JSON json.RawMessage `json:"json,omitempty"`
	Err  *ReplaiErrValue `json:"err,omitempty"`
}

// ReplaiErrValue is a trailing `error` return value, split out per spec §4.1.
type ReplaiErrValue struct {
	Nil     bool   `json:"nil"`
	Message string `json:"message,omitempty"`
	Type    string `json:"type,omitempty"`
}

// ReplaiDefined reports a variable defined or updated by the snippet.
type ReplaiDefined struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// ReplaiPanic carries a recovered panic with a cleaned, remapped stack.
type ReplaiPanic struct {
	Value string   `json:"value"`
	Stack []string `json:"stack"`
}
