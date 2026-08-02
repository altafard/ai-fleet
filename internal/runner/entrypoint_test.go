//go:build !windows

package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const stubClaude = `#!/usr/bin/env bash
echo '{"type":"system","subtype":"init"}'
case "${STUB_MODE:-commit}" in
  commit) echo hello > stub.txt; git add stub.txt; git commit -q -m "feat: add stub file";;
  dirty)  echo hello > stub.txt;;
  none)   ;;
esac
echo '{"type":"result","subtype":"success","num_turns":1}'
exit "${STUB_EXIT:-0}"
`

// harness prepares source repo, stub claude, and env; returns run func.
// The entrypoint is silent by design — the run outcome is asserted through
// the exit code, the bundle on disk, and git state, never through output.
func harness(t *testing.T, mode, stubExit string) (runEntry func() (string, int), outDir, wsDir string) {
	t.Helper()
	for _, bin := range []string{"bash", "git"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available", bin)
		}
	}
	tmp := t.TempDir()
	src := filepath.Join(tmp, "source")
	outDir = filepath.Join(tmp, "out")
	wsDir = filepath.Join(tmp, "workspace")
	bin := filepath.Join(tmp, "bin")
	for _, d := range []string{filepath.Join(src, "clone"), outDir, bin} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	sha := initCloneRepo(t, filepath.Join(src, "clone"))

	os.WriteFile(filepath.Join(bin, "claude"), []byte(stubClaude), 0o755)
	os.WriteFile(filepath.Join(src, "prompt.md"), []byte("do the thing"), 0o644)
	entry := filepath.Join(tmp, "entrypoint.sh")
	os.WriteFile(entry, EntrypointScript(), 0o755)

	runEntry = func() (string, int) {
		cmd := exec.Command("bash", entry)
		cmd.Env = entrypointEnv(bin, src, wsDir, outDir, sha, mode, stubExit)
		out, _ := cmd.CombinedOutput()
		return string(out), cmd.ProcessState.ExitCode()
	}
	return runEntry, outDir, wsDir
}

func initCloneRepo(t *testing.T, clone string) string {
	t.Helper()
	mustGit := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = clone
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	mustGit("init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(clone, "README.md"), []byte("hi\n"), 0o644)
	mustGit("add", ".")
	mustGit("commit", "-q", "-m", "chore: init")
	return mustGit("rev-parse", "HEAD")
}

func entrypointEnv(bin, src, wsDir, outDir, sha, mode, stubExit string) []string {
	return append(os.Environ(),
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
		"FLEET_SOURCE_DIR="+src, "FLEET_WORKSPACE_DIR="+wsDir, "FLEET_OUT_DIR="+outDir,
		"FLEET_BRANCH=feature/test", "FLEET_BASELINE_SHA="+sha,
		"GIT_AUTHOR_NAME=Bot", "GIT_AUTHOR_EMAIL=bot@example.com",
		"GIT_COMMITTER_NAME=Bot", "GIT_COMMITTER_EMAIL=bot@example.com",
		"STUB_MODE="+mode, "STUB_EXIT="+stubExit)
}

func bundleHasBranch(t *testing.T, bundle string) {
	t.Helper()
	heads, err := exec.Command("git", "bundle", "list-heads", bundle).CombinedOutput()
	if err != nil || !strings.Contains(string(heads), "refs/heads/feature/test") {
		t.Fatalf("bundle heads: %v %s", err, heads)
	}
}

func TestEntrypointHappyPath(t *testing.T) {
	run, out, _ := harness(t, "commit", "0")
	stdout, code := run()
	if code != 0 {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
	// claude's stream-json must pass through untouched.
	if !strings.Contains(stdout, `{"type":"system","subtype":"init"}`) {
		t.Fatalf("claude stdout not passed through:\n%s", stdout)
	}
	bundleHasBranch(t, filepath.Join(out, "run.bundle"))
}

func TestEntrypointDirtyTreeGetsSafetyCommit(t *testing.T) {
	run, out, _ := harness(t, "dirty", "0")
	stdout, code := run()
	if code != 0 {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
	// The stub never commits in dirty mode: run.bundle can only exist
	// because the safety commit captured the dirty tree.
	bundleHasBranch(t, filepath.Join(out, "run.bundle"))
}

func TestEntrypointNoChanges(t *testing.T) {
	run, out, _ := harness(t, "none", "0")
	stdout, code := run()
	if code != 0 {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
	if _, err := os.Stat(filepath.Join(out, "run.bundle")); err == nil {
		t.Fatal("bundle must not exist")
	}
}

func TestEntrypointClaudeFailureStillSalvages(t *testing.T) {
	run, out, _ := harness(t, "commit", "7")
	stdout, code := run()
	if code != 7 {
		t.Fatalf("exit=%d want 7\n%s", code, stdout)
	}
	bundleHasBranch(t, filepath.Join(out, "run.bundle"))
}

func TestEntrypointSalvageErrorPreservesExitCode(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test permissions as root")
	}
	run, out, _ := harness(t, "commit", "0")
	os.Chmod(out, 0o555)
	t.Cleanup(func() { os.Chmod(out, 0o755) })
	stdout, code := run()
	if code != 0 {
		t.Fatalf("exit=%d want 0 (claude success, not salvage error)\n%s", code, stdout)
	}
	if _, err := os.Stat(filepath.Join(out, "run.bundle")); err == nil {
		t.Fatal("bundle must not exist with read-only out dir")
	}
}

