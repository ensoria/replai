// Package widget is a test fixture exercising the replai evaluation paths.
package widget

import (
	"errors"
	"fmt"
	"os"
	"time"
)

// Widget is a sample struct with nested and special-cased field types.
type Widget struct {
	ID        int
	Name      string
	Tags      []string
	Timeout   time.Duration
	CreatedAt time.Time
	Parent    *Widget
}

// New creates a Widget with deterministic fields.
func New(name string) *Widget {
	return &Widget{
		ID:        42,
		Name:      name,
		Tags:      []string{"a", "b"},
		Timeout:   30 * time.Second,
		CreatedAt: time.Date(2026, 6, 11, 9, 30, 0, 0, time.UTC),
	}
}

// Rename updates the widget name and returns it for chaining.
func (w *Widget) Rename(name string) *Widget {
	w.Name = name
	return w
}

// Multi returns several values plus a nil error.
func Multi() (int, string, error) {
	return 7, "seven", nil
}

// Fail always returns a non-nil error.
func Fail() (int, error) {
	return 0, errors.New("widget exploded")
}

// Boom panics.
func Boom() {
	panic("boom from widget")
}

// Sleep blocks for d; used for timeout tests.
func Sleep(d time.Duration) {
	fmt.Println("sleeping")
	time.Sleep(d)
}

// PrintBoth writes to stdout and stderr.
func PrintBoth() {
	fmt.Println("to stdout")
	fmt.Fprintln(os.Stderr, "to stderr")
}

// Void returns nothing; exercises the statement-retry path.
func Void() {
	fmt.Println("void called")
}

// Cycle returns a self-referencing widget.
func Cycle() *Widget {
	w := New("cyclic")
	w.Parent = w
	return w
}
