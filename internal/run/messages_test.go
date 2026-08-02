package run

import (
	"strings"
	"testing"
)

func TestMergeInstructions(t *testing.T) {
	got := MergeInstructions(".ai-fleet/runs/260802-101530-ab12", "feature/x", "main", 4)
	for _, want := range []string{
		"branch feature/x (4 commits)",
		"git fetch .ai-fleet/runs/260802-101530-ab12/out/run.bundle feature/x:feature/x",
		"git log -p main..feature/x",
		"git merge feature/x",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}
