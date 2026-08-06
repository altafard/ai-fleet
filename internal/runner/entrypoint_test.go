//go:build !windows

package runner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

const stubClaude = `#!/usr/bin/env bash
printf '%s\n' "$@" > "${FLEET_OUT_DIR}/claude-args.txt"
echo '{"type":"system","subtype":"init"}'
case "${STUB_MODE:-commit}" in
  commit) echo hello > stub.txt; git add stub.txt; git commit -q -m "feat: add stub file";;
  dirty)  echo hello > stub.txt;;
  none)   ;;
  commit_sleep)
    echo hello > stub.txt; git add stub.txt; git commit -q -m "feat: add stub file"
    sleep 30
    ;;
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
		"FLEET_MODEL=opus", "FLEET_EFFORT=high",
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

// TestEntrypointPassesModelAndEffort: the host chose the model and effort at
// preflight; the entrypoint must hand both to claude verbatim.
func TestEntrypointPassesModelAndEffort(t *testing.T) {
	run, out, _ := harness(t, "commit", "0")
	stdout, code := run()
	if code != 0 {
		t.Fatalf("exit=%d\n%s", code, stdout)
	}
	args, err := os.ReadFile(filepath.Join(out, "claude-args.txt"))
	if err != nil {
		t.Fatalf("stub did not record args: %v", err)
	}
	for _, want := range []string{"--model\nopus\n", "--effort\nhigh\n"} {
		if !strings.Contains(string(args), want) {
			t.Fatalf("claude args missing %q:\n%s", want, args)
		}
	}
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

// TestEntrypointBundleWriteFailureExitsDistinct: commits that cannot be
// bundled must never masquerade as a clean no-change run — the entrypoint
// exits 86 so the host reports a failure instead of "no changes".
func TestEntrypointBundleWriteFailureExitsDistinct(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test permissions as root")
	}
	run, out, _ := harness(t, "commit", "0")
	os.Chmod(out, 0o555)
	t.Cleanup(func() { os.Chmod(out, 0o755) })
	stdout, code := run()
	if code != 86 {
		t.Fatalf("exit=%d want 86 (commits made, bundle unwritable)\n%s", code, stdout)
	}
	if !strings.Contains(stdout, "failed to write") {
		t.Fatalf("want bundle-failure notice on stdout\n%s", stdout)
	}
	if _, err := os.Stat(filepath.Join(out, "run.bundle")); err == nil {
		t.Fatal("bundle must not exist with read-only out dir")
	}
}

// TestEntrypointBundleWriteFailureKeepsClaudeExit: when claude itself failed,
// its exit code wins over the bundle-failure code.
func TestEntrypointBundleWriteFailureKeepsClaudeExit(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("cannot test permissions as root")
	}
	run, out, _ := harness(t, "commit", "7")
	os.Chmod(out, 0o555)
	t.Cleanup(func() { os.Chmod(out, 0o755) })
	stdout, code := run()
	if code != 7 {
		t.Fatalf("exit=%d want 7 (claude failure stays visible)\n%s", code, stdout)
	}
}

// TestEntrypointSigtermSalvagesBeforeSigkill is the regression test for C1:
// docker stop only signals the entrypoint's PID 1 (bash). A plain foreground
// child would defer the TERM trap until claude exits, so the 10s grace period
// would elapse and Docker would SIGKILL everything before any salvage could
// run. With claude backed by `& wait`, the trap fires immediately, forwards
// the signal to claude, and the EXIT trap salvages a bundle from whatever was
// already committed.
func TestEntrypointSigtermSalvagesBeforeSigkill(t *testing.T) {
	for _, bin := range []string{"bash", "git"} {
		if _, err := exec.LookPath(bin); err != nil {
			t.Skipf("%s not available", bin)
		}
	}
	tmp := t.TempDir()
	src := filepath.Join(tmp, "source")
	outDir := filepath.Join(tmp, "out")
	wsDir := filepath.Join(tmp, "workspace")
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

	cmd := exec.Command("bash", entry)
	cmd.Env = entrypointEnv(bin, src, wsDir, outDir, sha, "commit_sleep", "0")

	// Hand the child an *os.File for stdout rather than an io.Writer:
	// exec.Cmd would otherwise spawn a copy goroutine and cmd.Wait() would
	// block until pipe EOF — which the stub's orphaned `sleep 30` (which
	// inherits the fd; SIGTERM only reaches PID 1, matching docker-stop
	// semantics) postpones for the rest of its sleep.
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdout = pw

	if err := cmd.Start(); err != nil {
		t.Fatalf("start entrypoint: %v", err)
	}
	pw.Close()
	defer pr.Close()

	// Poll for the stub's commit to land before signaling, so the salvage
	// has something to bundle.
	deadline := time.Now().Add(10 * time.Second)
	committed := false
	for time.Now().Before(deadline) {
		out, err := exec.Command("git", "-C", wsDir, "log", "--oneline", "feature/test").CombinedOutput()
		if err == nil && strings.Contains(string(out), "add stub file") {
			committed = true
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !committed {
		cmd.Process.Kill()
		t.Fatal("stub claude never committed")
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("signal entrypoint: %v", err)
	}

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()
	select {
	case <-waitDone:
	case <-time.After(5 * time.Second):
		cmd.Process.Kill()
		t.Fatal("entrypoint did not exit within 5s of SIGTERM")
	}

	if code := cmd.ProcessState.ExitCode(); code != 130 {
		t.Fatalf("exit code = %d, want 130", code)
	}
	if _, err := os.Stat(filepath.Join(outDir, "run.bundle")); err != nil {
		t.Fatal("run.bundle missing after SIGTERM salvage")
	}
	bundleHasBranch(t, filepath.Join(outDir, "run.bundle"))
}
