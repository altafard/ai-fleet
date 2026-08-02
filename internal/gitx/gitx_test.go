package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseOriginHead(t *testing.T) {
	ref, branch, ok := ParseOriginHead("refs/remotes/origin/main")
	if !ok || ref != "origin/main" || branch != "main" {
		t.Fatalf("got %q %q %v", ref, branch, ok)
	}
	if _, _, ok := ParseOriginHead("garbage"); ok {
		t.Fatal("want !ok")
	}
}

func TestRedact(t *testing.T) {
	if got := Redact("push https://x:tok123@host failed", "tok123"); strings.Contains(got, "tok123") {
		t.Fatalf("token leaked: %q", got)
	}
	if got := Redact("clean", ""); got != "clean" {
		t.Fatalf("empty secret changed string: %q", got)
	}
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	env := append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir, cmd.Env = dir, env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	run("init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644)
	run("add", ".")
	run("commit", "-q", "-m", "chore: init")
	return dir
}

func TestRepoRootFromSubdir(t *testing.T) {
	root := initRepo(t)
	sub := filepath.Join(root, "a", "b")
	os.MkdirAll(sub, 0o755)
	got, err := RepoRoot(sub)
	if err != nil {
		t.Fatal(err)
	}
	// macOS tempdirs may traverse symlinks; compare resolved paths.
	r1, _ := filepath.EvalSymlinks(got)
	r2, _ := filepath.EvalSymlinks(root)
	if r1 != r2 {
		t.Fatalf("got %q want %q", got, root)
	}
}

func TestRepoRootOutsideRepo(t *testing.T) {
	if _, err := RepoRoot(t.TempDir()); err == nil {
		t.Fatal("want error outside a repo")
	}
}

func TestBaselineLocal(t *testing.T) {
	root := initRepo(t)
	b, err := Baseline(root)
	if err != nil {
		t.Fatal(err)
	}
	if b.Branch != "main" || b.Ref != "main" || len(b.SHA) != 40 {
		t.Fatalf("got %+v", b)
	}
}

func TestCloneNoCheckoutAndCount(t *testing.T) {
	root := initRepo(t)
	b, _ := Baseline(root)
	dest := filepath.Join(t.TempDir(), "wt")
	if err := CloneNoCheckout(root, dest, b.SHA); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, ".git")); err != nil {
		t.Fatal(".git missing in clone")
	}
	if _, err := os.Stat(filepath.Join(dest, "README.md")); err == nil {
		t.Fatal("worktree files must not be checked out")
	}
	n, err := CountCommits(dest, b.SHA, "HEAD")
	if err != nil || n != 0 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}
