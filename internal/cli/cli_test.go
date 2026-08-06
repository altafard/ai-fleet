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
