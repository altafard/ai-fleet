package execx

import (
	"context"
	"strings"
	"testing"
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
