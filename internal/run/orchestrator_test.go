package run

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/altafard/ai-fleet/internal/gitx"
	"github.com/altafard/ai-fleet/internal/logstream"
)

// collectFixture builds a real project repo with a baseline commit and one
// feature commit on branch feature/x, plus a bundle of that commit — the
// raw material for exercising collect()'s exit-code matrix without Docker.
type collectFixture struct {
	root    string
	baseSHA string
	bundle  string // path to a valid run.bundle for feature/x
}

func newCollectFixture(t *testing.T) collectFixture {
	t.Helper()
	root := t.TempDir()
	mustGit := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
		return strings.TrimSpace(string(out))
	}
	mustGit("init", "-q", "-b", "main")
	os.WriteFile(filepath.Join(root, "README.md"), []byte("hi\n"), 0o644)
	mustGit("add", ".")
	mustGit("commit", "-q", "-m", "chore: init")
	baseSHA := mustGit("rev-parse", "HEAD")

	mustGit("checkout", "-q", "-b", "feature/x")
	os.WriteFile(filepath.Join(root, "new.txt"), []byte("work\n"), 0o644)
	mustGit("add", ".")
	mustGit("commit", "-q", "-m", "feat: add work")
	bundle := filepath.Join(t.TempDir(), "run.bundle")
	mustGit("bundle", "create", bundle, baseSHA+"..feature/x")
	mustGit("checkout", "-q", "main")

	return collectFixture{root: root, baseSHA: baseSHA, bundle: bundle}
}

// newCollectState prepares a run dir with a baseline worktree clone, ready
// for collect(). withBundle copies the fixture bundle into out/.
func newCollectState(t *testing.T, fx collectFixture, withBundle bool) (*state, *bytes.Buffer) {
	t.Helper()
	dir, err := CreateRunDir(fx.root, NewRunID(time.Now()))
	if err != nil {
		t.Fatal(err)
	}
	if err := gitx.CloneNoCheckout(fx.root, dir.Worktree(), fx.baseSHA); err != nil {
		t.Fatal(err)
	}
	if withBundle {
		b, err := os.ReadFile(fx.bundle)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(dir.BundleFile(), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	s := &state{
		o:       Options{Branch: "feature/x"},
		root:    fx.root,
		base:    gitx.Base{Ref: "main", Branch: "main", SHA: fx.baseSHA},
		dir:     dir,
		console: logstream.NewConsole(&buf, false),
	}
	return s, &buf
}

func TestCollectExitCodeMatrix(t *testing.T) {
	fx := newCollectFixture(t)
	cases := []struct {
		name          string
		withBundle    bool
		containerExit int
		wantCode      int
		wantConsole   string
	}{
		{"bundle + success", true, 0, ExitOK, "bundle verified: 1 commits"},
		{"bundle + claude failure (salvage)", true, 7, ExitFailure, "salvaged into the bundle"},
		{"no bundle + success", false, 0, ExitNoChanges, "no changes"},
		{"no bundle + claude failure", false, 7, ExitFailure, "produced no bundle"},
		{"no bundle + bundle-write failure", false, bundleFailedExit, ExitFailure, "could not write out/run.bundle"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, buf := newCollectState(t, fx, c.withBundle)
			got := collect(s, c.containerExit)
			if got != c.wantCode {
				t.Fatalf("exit = %d, want %d\nconsole:\n%s", got, c.wantCode, buf.String())
			}
			if !strings.Contains(buf.String(), c.wantConsole) {
				t.Fatalf("console missing %q:\n%s", c.wantConsole, buf.String())
			}
			if _, err := os.Stat(s.dir.Worktree()); err == nil {
				t.Fatal("worktree must be deleted after collect")
			}
		})
	}
}

// TestCollectBundleWriteFailureNeverNoChanges pins the C1 regression: exit
// 86 with no bundle must NEVER be reported as a clean no-change run.
func TestCollectBundleWriteFailureNeverNoChanges(t *testing.T) {
	fx := newCollectFixture(t)
	s, buf := newCollectState(t, fx, false)
	if got := collect(s, bundleFailedExit); got == ExitNoChanges || got == ExitOK {
		t.Fatalf("bundle-write failure reported as success (exit %d)\nconsole:\n%s", got, buf.String())
	}
}

func TestCollectSalvageCountsCommits(t *testing.T) {
	fx := newCollectFixture(t)
	s, _ := newCollectState(t, fx, true)
	if got := collect(s, 130); got != ExitFailure {
		t.Fatalf("exit = %d, want %d", got, ExitFailure)
	}
	if s.commits != 1 {
		t.Fatalf("commits = %d, want 1", s.commits)
	}
}
