//go:build integration

package dockerx

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestProjectImageBuildAndPrune exercises the real naming and prune flow:
// two builds of the same project repo with different content, then prune
// keeps only the current tag. Uses busybox — tiny, and no Claude-install
// layer (slow, network-bound, and irrelevant to naming/prune behavior).
func TestProjectImageBuildAndPrune(t *testing.T) {
	if _, err := Version(); err != nil {
		t.Skip("docker unavailable:", err)
	}
	const repo = "ai-fleet/inttest-prune-00000000"
	t.Cleanup(func() {
		// Remove everything in the test repo, whatever the test left behind.
		if _, _, err := PruneRepo(repo, ""); err != nil {
			t.Log("cleanup:", err)
		}
	})

	dir := t.TempDir()
	build := func(content string) string {
		t.Helper()
		df := []byte(content)
		p := filepath.Join(dir, "Dockerfile")
		if err := os.WriteFile(p, df, 0o644); err != nil {
			t.Fatal(err)
		}
		tag := ContentTag(df)
		if _, err := Build(context.Background(), p, dir, repo+":"+tag, func(string) {}); err != nil {
			t.Fatal(err)
		}
		return tag
	}

	old := build("FROM busybox:latest\nLABEL ai-fleet-inttest=1\n")
	cur := build("FROM busybox:latest\nLABEL ai-fleet-inttest=2\n")
	if old == cur {
		t.Fatal("test needs two distinct tags")
	}

	tags, err := ListTags(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 {
		t.Fatalf("ListTags = %v, want 2 tags", tags)
	}

	removed, warns, err := PruneRepo(repo, cur)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 || len(warns) != 0 {
		t.Errorf("PruneRepo removed=%d warns=%v, want 1 and none", removed, warns)
	}
	tags, err = ListTags(repo)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 1 || tags[0] != cur {
		t.Errorf("after prune ListTags = %v, want only %q", tags, cur)
	}
}
