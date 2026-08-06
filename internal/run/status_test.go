package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteStatus(t *testing.T) {
	p := filepath.Join(t.TempDir(), "status.json")
	err := WriteStatus(p, Status{
		RunID: "260802-101530-ab12", BaselineRef: "origin/main", BaselineSHA: "abc",
		Branch: "feature/x", Model: "claude-opus-5", Effort: "high",
		ImageID: "sha256:1", ExitCode: 0, CommitCount: 4,
		StartedAt: "2026-08-02T10:15:30Z", FinishedAt: "2026-08-02T10:22:01Z",
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	if m["run_id"] != "260802-101530-ab12" || m["commit_count"] != float64(4) {
		t.Fatalf("got %s", b)
	}
	if m["model"] != "claude-opus-5" || m["effort"] != "high" {
		t.Fatalf("model/effort not recorded: %s", b)
	}
	if _, exists := m["pr_url"]; exists {
		t.Fatal("empty pr_url must be omitted")
	}
	if _, exists := m["error"]; exists {
		t.Fatal("empty error must be omitted")
	}
}
