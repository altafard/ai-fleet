package runner

import (
	"os"
	"strings"
	"testing"
)

func TestRenderPromptGolden(t *testing.T) {
	got, err := RenderPrompt("feature/260802-101530-ab12", "Add a health endpoint.")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/prompt_golden.md")
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("rendered prompt differs from golden file:\n%s", got)
	}
}

func TestRenderPromptContainsParts(t *testing.T) {
	got, _ := RenderPrompt("feature/x", "THE-TASK-SENTINEL")
	for _, want := range []string{
		"branch feature/x",
		"<task>\nTHE-TASK-SENTINEL\n</task>",
		"Conventional Commits",
		"/out/pull-request.md",
		"## Summary",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q", want)
		}
	}
}
