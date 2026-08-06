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

func TestFrames(t *testing.T) {
	if len(Frames) != 10 || Frames[0] != "⠋" {
		t.Fatalf("frames=%v", Frames)
	}
}
