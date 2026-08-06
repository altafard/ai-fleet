package execx

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestRunCaptures(t *testing.T) {
	r, err := Run("", "git", "--version")
	if err != nil {
		t.Fatal(err)
	}
	if r.ExitCode != 0 || !strings.HasPrefix(r.Stdout, "git version") {
		t.Fatalf("unexpected result: %+v", r)
	}
}

func TestRunNonZeroIsNotError(t *testing.T) {
	r, err := Run("", "git", "merge-base")
	if err != nil {
		t.Fatal(err)
	}
	if r.ExitCode == 0 {
		t.Fatal("want non-zero exit code")
	}
}

func TestRunCtxCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RunCtx(ctx, "", nil, "git", "--version"); err == nil {
		t.Fatal("want error for already-cancelled context")
	}
}

func TestRunCtxEnvPassthrough(t *testing.T) {
	r, err := RunCtx(context.Background(), "", []string{"GIT_AUTHOR_NAME=ctx-test"}, "git", "var", "GIT_AUTHOR_IDENT")
	if err != nil || r.ExitCode != 0 {
		t.Fatalf("r=%+v err=%v", r, err)
	}
	if !strings.HasPrefix(r.Stdout, "ctx-test ") {
		t.Fatalf("env not passed through: %q", r.Stdout)
	}
}

// TestStreamTooLongLineDoesNotHang: a line beyond the cap must fail fast
// with an error, never deadlock cmd.Wait (the historical failure mode).
func TestStreamTooLongLineDoesNotHang(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	old := maxStreamLine
	maxStreamLine = 1024
	t.Cleanup(func() { maxStreamLine = old })
	start := time.Now()
	_, err := Stream(context.Background(), "", nil, func(string) {},
		"bash", "-c", "printf 'a%.0s' {1..100000}; echo; echo done")
	if err == nil {
		t.Fatal("want error for over-long line")
	}
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("stream took %v — reader stopped draining", d)
	}
}

// TestRunCtxBoundedWithLingeringChild: a descendant that inherits the pipe
// and outlives the command must not keep RunCtx blocked past WaitDelay.
func TestRunCtxBoundedWithLingeringChild(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	start := time.Now()
	RunCtx(ctx, "", nil, "bash", "-c", "sleep 30 & sleep 30")
	if d := time.Since(start); d > 10*time.Second {
		t.Fatalf("RunCtx not bounded: took %v", d)
	}
}

func TestCause(t *testing.T) {
	if got := Cause(Result{Stderr: "fatal: not a repo", ExitCode: 128}, nil); got != "fatal: not a repo" {
		t.Fatalf("ran-and-failed cause = %q, want stderr", got)
	}
	if got := Cause(Result{}, errors.New(`exec: "git": executable file not found in $PATH`)); !strings.Contains(got, "executable file not found") {
		t.Fatalf("could-not-run cause lost: %q", got)
	}
}

func TestStreamLinesAndExit(t *testing.T) {
	var lines []string
	code, err := Stream(context.Background(), "", nil,
		func(l string) { lines = append(lines, l) }, "git", "--version")
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "git version") {
		t.Fatalf("lines=%v", lines)
	}
}
