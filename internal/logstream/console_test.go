package logstream

import (
	"bytes"
	"strings"
	"testing"
)

func TestConsoleNonTTY(t *testing.T) {
	var buf bytes.Buffer
	c := NewConsole(&buf, false)
	c.Check("git 2.44.0")
	c.Spin("building image")
	c.Spin("building image — step 2/6: RUN npm ci")
	c.Done("image built in 2m 14s")
	c.Spin("claude — turn 1: Bash(npm test)")
	c.Fail("boom")
	out := buf.String()
	for _, want := range []string{
		"✓ git 2.44.0",
		"… building image",
		"… building image — step 2/6: RUN npm ci",
		"✓ image built in 2m 14s",
		"… claude — turn 1: Bash(npm test)",
		"✗ boom",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "\r") {
		t.Error("non-TTY output must not use carriage returns")
	}
}

func TestConsoleNonTTYDedupesSpin(t *testing.T) {
	var buf bytes.Buffer
	c := NewConsole(&buf, false)
	c.Spin("same text")
	c.Spin("same text")
	if got := strings.Count(buf.String(), "same text"); got != 1 {
		t.Fatalf("repeated identical spin text must print once, got %d", got)
	}
}

func TestConsoleWarn(t *testing.T) {
	var buf bytes.Buffer
	c := NewConsole(&buf, false)
	c.Warn("subdirectory detected")
	if got := buf.String(); got != "! subdirectory detected\n" {
		t.Errorf("Warn output = %q", got)
	}
}

func TestConsoleTTYSpinAndTick(t *testing.T) {
	var buf bytes.Buffer
	c := NewConsole(&buf, true)
	c.Spin("building")
	if got := buf.String(); got != "\r\033[K⠋ building" {
		t.Fatalf("first spin = %q, want an in-place repaint with frame 0", got)
	}
	buf.Reset()
	c.Tick()
	if got := buf.String(); got != "\r\033[K⠙ building" {
		t.Fatalf("tick = %q, want the next frame repainting the same text", got)
	}
	// The frame counter must wrap: 9 more ticks land back on frame 0.
	for i := 0; i < 9; i++ {
		c.Tick()
	}
	buf.Reset()
	c.Spin("building")
	if got := buf.String(); got != "\r\033[K⠋ building" {
		t.Fatalf("after a full cycle = %q, want frame 0 again", got)
	}
}

func TestConsoleTTYLineReplacesLiveSpinner(t *testing.T) {
	var buf bytes.Buffer
	c := NewConsole(&buf, true)
	c.Spin("building")
	buf.Reset()
	c.Done("image built")
	if got := buf.String(); got != "\r\033[K✓ image built\n" {
		t.Fatalf("done over a live spinner = %q, want clear-then-line", got)
	}
	// With no live spinner there is nothing to clear.
	buf.Reset()
	c.Check("next step")
	if got := buf.String(); got != "✓ next step\n" {
		t.Fatalf("line without spinner = %q", got)
	}
}

func TestConsoleTickIsSilentWithoutLiveSpinner(t *testing.T) {
	var tty bytes.Buffer
	c := NewConsole(&tty, true)
	c.Tick() // never spun
	c.Spin("x")
	c.Done("done") // line ended the spinner
	tty.Reset()
	c.Tick()
	if tty.Len() != 0 {
		t.Fatalf("tick painted %q with no live spinner", tty.String())
	}

	var plain bytes.Buffer
	n := NewConsole(&plain, false)
	n.Spin("x")
	plain.Reset()
	n.Tick()
	if plain.Len() != 0 {
		t.Fatalf("non-TTY tick painted %q", plain.String())
	}
}

func TestFrames(t *testing.T) {
	if len(Frames) != 10 || Frames[0] != "⠋" {
		t.Fatalf("frames=%v", Frames)
	}
}
