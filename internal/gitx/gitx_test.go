package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseRemoteHead(t *testing.T) {
	ref, branch, ok := ParseRemoteHead("refs/remotes/origin/main", "origin")
	if !ok || ref != "origin/main" || branch != "main" {
		t.Fatalf("got %q %q %v", ref, branch, ok)
	}
	ref, branch, ok = ParseRemoteHead("refs/remotes/upstream/trunk", "upstream")
	if !ok || ref != "upstream/trunk" || branch != "trunk" {
		t.Fatalf("got %q %q %v", ref, branch, ok)
	}
	if _, _, ok := ParseRemoteHead("garbage", "origin"); ok {
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

// runGit runs a git command in dir with a deterministic identity, failing
// the test on error. Used for test-repo setup (commits, pushes) that the
// package's own git() helper cannot do without an author identity.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s", args, out)
	}
	return strings.TrimSpace(string(out))
}

func initRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(dir, "README.md"), []byte("hi\n"), 0o644)
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", "chore: init")
	return dir
}

func commitFile(t *testing.T, dir, name, content, msg string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-q", "-m", msg)
	return runGit(t, dir, "rev-parse", "HEAD")
}

// TestPushSuccess pushes a local branch to a local bare repo (no network)
// and asserts the branch lands there.
func TestPushSuccess(t *testing.T) {
	src := initRepo(t)
	runGit(t, src, "checkout", "-q", "-b", "feature/x")
	commitFile(t, src, "feature.txt", "hi\n", "feat: add feature file")

	bare := filepath.Join(t.TempDir(), "bare.git")
	runGit(t, t.TempDir(), "init", "-q", "--bare", bare)

	if err := Push(src, bare, "feature/x", ""); err != nil {
		t.Fatalf("Push failed: %v", err)
	}

	r, err := git(bare, "show-ref", "--verify", "--quiet", "refs/heads/feature/x")
	if err != nil || r.ExitCode != 0 {
		t.Fatalf("branch feature/x missing in bare repo: err=%v exitCode=%d stderr=%s", err, r.ExitCode, r.Stderr)
	}
}

// TestPushRejectedNonFastForward diverges the bare repo's branch from the
// local one (by pushing an unrelated commit from a second clone) and asserts
// Push reports an error for the resulting non-fast-forward push.
func TestPushRejectedNonFastForward(t *testing.T) {
	src := initRepo(t)
	bare := filepath.Join(t.TempDir(), "bare.git")
	runGit(t, t.TempDir(), "init", "-q", "--bare", bare)

	if err := Push(src, bare, "main", ""); err != nil {
		t.Fatalf("initial push failed: %v", err)
	}

	// Diverge the bare repo's main from src's main via a second clone.
	other := filepath.Join(t.TempDir(), "other")
	runGit(t, t.TempDir(), "clone", "-q", bare, other)
	commitFile(t, other, "other.txt", "x\n", "chore: other change")
	runGit(t, other, "push", "-q", "origin", "main")

	// src's main is now behind bare's main; a new, divergent local commit
	// makes this a non-fast-forward push.
	commitFile(t, src, "srconly.txt", "y\n", "chore: src change")

	if err := Push(src, bare, "main", ""); err == nil {
		t.Fatal("want error for non-fast-forward push, got nil")
	}
}

// TestPushRedactsToken forces a push failure against a URL that embeds a
// fake token and asserts the token never appears in the returned error.
func TestPushRedactsToken(t *testing.T) {
	src := initRepo(t)
	const token = "sekret-token-value"
	badURL := filepath.Join(t.TempDir(), token, "does-not-exist.git")

	err := Push(src, badURL, "main", token)
	if err == nil {
		t.Fatal("want error pushing to a nonexistent repository")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("token leaked in error: %q", err.Error())
	}
}

// TestFetchBundle creates a bundle from a temp repo and fetches it into a
// clone that already has the baseline commit, mirroring collect()'s use.
func TestFetchBundle(t *testing.T) {
	src := initRepo(t)
	b, err := Baseline(src)
	if err != nil {
		t.Fatal(err)
	}
	commitFile(t, src, "extra.txt", "extra\n", "feat: add extra file")

	bundle := filepath.Join(t.TempDir(), "test.bundle")
	runGit(t, src, "bundle", "create", bundle, b.SHA+"..main")

	dest := filepath.Join(t.TempDir(), "dest")
	if err := CloneNoCheckout(src, dest, b.SHA); err != nil {
		t.Fatal(err)
	}

	if err := FetchBundle(dest, bundle, "main"); err != nil {
		t.Fatalf("FetchBundle failed: %v", err)
	}
	n, err := CountCommits(dest, b.SHA, "main")
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("got %d commits after fetch, want 1", n)
	}
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
	if b.Branch != "main" || b.Ref != "main" || len(b.SHA) != 40 || b.Source != "local-branch" {
		t.Fatalf("got %+v", b)
	}
}

func TestSelectRemote(t *testing.T) {
	root := initRepo(t)
	if _, has, err := SelectRemote(root); has || err != nil {
		t.Fatalf("no remotes: has=%v err=%v", has, err)
	}
	runGit(t, root, "remote", "add", "upstream", "/nowhere/a")
	if name, has, err := SelectRemote(root); !has || err != nil || name != "upstream" {
		t.Fatalf("sole remote: %q %v %v", name, has, err)
	}
	runGit(t, root, "remote", "add", "fork", "/nowhere/b")
	if _, _, err := SelectRemote(root); err == nil {
		t.Fatal("ambiguous remotes must error")
	}
	runGit(t, root, "config", "checkout.defaultRemote", "fork")
	if name, _, err := SelectRemote(root); err != nil || name != "fork" {
		t.Fatalf("checkout.defaultRemote: %q %v", name, err)
	}
	runGit(t, root, "remote", "add", "origin", "/nowhere/c")
	if name, _, err := SelectRemote(root); err != nil || name != "origin" {
		t.Fatalf("origin must win: %q %v", name, err)
	}
}

// TestBaselineRemoteHeadLocal covers rule 1: a cloned repo carries the
// refs/remotes/origin/HEAD symref, so resolution is local-only.
func TestBaselineRemoteHeadLocal(t *testing.T) {
	upstream := initRepo(t)
	clone := filepath.Join(t.TempDir(), "clone")
	runGit(t, t.TempDir(), "clone", "-q", upstream, clone)
	b, err := Baseline(clone)
	if err != nil {
		t.Fatal(err)
	}
	if b.Ref != "origin/main" || b.Branch != "main" || b.Source != "remote-head-local" {
		t.Fatalf("got %+v", b)
	}
}

// TestBaselineRemoteQuery covers rule 2: remote added by hand (no
// origin/HEAD symref), remote reachable — a local-path remote stands in
// for the network, so ls-remote works without credentials.
func TestBaselineRemoteQuery(t *testing.T) {
	upstream := initRepo(t) // default branch "main"
	root := initRepo(t)
	runGit(t, root, "remote", "add", "origin", upstream)
	runGit(t, root, "fetch", "-q", "origin")
	// Recent git may set origin/HEAD on first fetch; remove it so this test
	// deterministically exercises the query rule, not the symref rule.
	cmd := exec.Command("git", "symbolic-ref", "--delete", "refs/remotes/origin/HEAD")
	cmd.Dir = root
	cmd.Run() // ignore error: symref may legitimately not exist on older git
	b, err := Baseline(root)
	if err != nil {
		t.Fatal(err)
	}
	if b.Ref != "origin/main" || b.Source != "remote-query" {
		t.Fatalf("got %+v", b)
	}
}

// TestBaselineRemoteQueryUnfetched: the query names the default branch but
// no remote-tracking ref exists locally — actionable error, no guessing.
func TestBaselineRemoteQueryUnfetched(t *testing.T) {
	upstream := initRepo(t)
	root := initRepo(t)
	runGit(t, root, "remote", "add", "origin", upstream)
	_, err := Baseline(root)
	if err == nil || !strings.Contains(err.Error(), "git fetch origin") {
		t.Fatalf("want fetch hint, got %v", err)
	}
}

// TestBaselineInferred covers rule 3: remote unreachable, resolution falls
// back to local remote-tracking refs.
func TestBaselineInferred(t *testing.T) {
	root := initRepo(t)
	sha := runGit(t, root, "rev-parse", "HEAD")
	runGit(t, root, "remote", "add", "origin", filepath.Join(t.TempDir(), "gone"))

	runGit(t, root, "update-ref", "refs/remotes/origin/trunk", sha)
	b, err := Baseline(root)
	if err != nil {
		t.Fatal(err)
	}
	if b.Ref != "origin/trunk" || b.Source != "inferred-single" {
		t.Fatalf("single: got %+v", b)
	}

	runGit(t, root, "config", "init.defaultBranch", "main")
	runGit(t, root, "update-ref", "refs/remotes/origin/main", sha)
	b, err = Baseline(root)
	if err != nil {
		t.Fatal(err)
	}
	if b.Ref != "origin/main" || b.Source != "inferred-named" {
		t.Fatalf("named: got %+v", b)
	}
}

// TestBaselineNoInformation: remote unreachable and nothing fetched — the
// error carries both remedies.
func TestBaselineNoInformation(t *testing.T) {
	root := initRepo(t)
	runGit(t, root, "remote", "add", "origin", filepath.Join(t.TempDir(), "gone"))
	_, err := Baseline(root)
	if err == nil || !strings.Contains(err.Error(), "git fetch origin") {
		t.Fatalf("want error with fetch hint, got %v", err)
	}
}

func TestVersion(t *testing.T) {
	v, err := Version()
	if err != nil {
		t.Fatal(err)
	}
	if v == "" || strings.HasPrefix(v, "git version") {
		t.Fatalf("Version() = %q, want the bare number", v)
	}
}

// When git cannot run at all (not on PATH), the wrapper errors must carry
// that cause instead of formatting an empty stderr into "…failed: ".
func TestErrorsKeepCauseWhenGitCannotRun(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	cases := []struct {
		name string
		call func() error
	}{
		{"CloneNoCheckout", func() error { return CloneNoCheckout(t.TempDir(), filepath.Join(t.TempDir(), "wt"), "deadbeef") }},
		{"VerifyBundle", func() error { return VerifyBundle(t.TempDir(), "run.bundle") }},
		{"FetchBundle", func() error { return FetchBundle(t.TempDir(), "run.bundle", "feature/x") }},
		{"CountCommits", func() error { _, err := CountCommits(t.TempDir(), "a", "b"); return err }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatal("want error when git is missing")
			}
			if !strings.Contains(err.Error(), "executable file not found") {
				t.Errorf("error lost its cause: %q", err.Error())
			}
		})
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
