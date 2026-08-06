package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestVersionFlagPrintsSingleVersionLine(t *testing.T) {
	var code int
	root := newRoot(&code)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--version"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error: %v", err)
	}
	got := out.String()
	if !strings.HasPrefix(got, "ai-fleet ") {
		t.Errorf("output %q does not start with %q", got, "ai-fleet ")
	}
	if strings.Count(got, "\n") != 1 || !strings.HasSuffix(got, "\n") {
		t.Errorf("output %q is not exactly one line", got)
	}
}

func TestInitCommandRegistered(t *testing.T) {
	var code int
	root := newRoot(&code)
	cmd, _, err := root.Find([]string{"init"})
	if err != nil || cmd.Use != "init" {
		t.Fatalf("init command not registered: %v", err)
	}
	if cmd.Short == "" {
		t.Error("init command has no Short description")
	}
}
