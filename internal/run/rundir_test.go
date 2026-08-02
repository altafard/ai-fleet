package run

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateRunDir(t *testing.T) {
	root := t.TempDir()
	d, err := CreateRunDir(root, "260802-101530-ab12")
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(d.Out()); err != nil || !fi.IsDir() {
		t.Fatalf("out dir missing: %v", err)
	}
	want := filepath.Join(root, ".ai-fleet", "runs", "260802-101530-ab12")
	if d.Root() != want {
		t.Fatalf("root=%q want %q", d.Root(), want)
	}
	if filepath.Base(d.BundleFile()) != "run.bundle" ||
		filepath.Base(d.LogFile()) != "log.jsonl" ||
		filepath.Base(d.PRFile()) != "pull-request.md" ||
		filepath.Base(d.StatusFile()) != "status.json" {
		t.Fatal("unexpected artifact names")
	}
	if d.Rel(root) != ".ai-fleet/runs/260802-101530-ab12" {
		t.Fatalf("rel=%q", d.Rel(root))
	}
}
