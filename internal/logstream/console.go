// Package logstream renders interactive console progress and parses the
// text streams a run produces: docker build step lines and Claude Code
// stream-json events.
package logstream

import (
	"fmt"
	"io"
	"sync"
)

// Frames is the snake spinner.
var Frames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Console renders interactive progress: checkmark lines for completed
// steps and one in-place spinner line for the currently running phase.
// In non-TTY mode every state change is a plain appended line.
type Console struct {
	mu    sync.Mutex
	w     io.Writer
	tty   bool
	frame int
	spin  string // active spinner text; "" = none
}

// NewConsole returns a Console writing to w — with in-place spinner
// rendering when tty is true, plain appended lines otherwise.
func NewConsole(w io.Writer, tty bool) *Console { return &Console{w: w, tty: tty} }

// Check prints a completed-step line (clearing any active spinner).
func (c *Console) Check(msg string) { c.line("✓ " + msg) }

// Fail prints a failed-step line (clearing any active spinner).
func (c *Console) Fail(msg string) { c.line("✗ " + msg) }

// Done finishes the active spinner as a completed step.
func (c *Console) Done(msg string) { c.line("✓ " + msg) }

// Spin starts or updates the in-place progress line.
func (c *Console) Spin(text string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tty {
		c.spin = text
		fmt.Fprintf(c.w, "\r\033[K%s %s", Frames[c.frame], text)
		return
	}
	if text != c.spin {
		c.spin = text
		fmt.Fprintf(c.w, "… %s\n", text)
	}
}

// Tick advances the spinner; call it on a ~100 ms ticker in TTY mode.
func (c *Console) Tick() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.tty || c.spin == "" {
		return
	}
	c.frame = (c.frame + 1) % len(Frames)
	fmt.Fprintf(c.w, "\r\033[K%s %s", Frames[c.frame], c.spin)
}

func (c *Console) line(s string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.tty && c.spin != "" {
		fmt.Fprint(c.w, "\r\033[K")
	}
	c.spin = ""
	fmt.Fprintln(c.w, s)
}
