package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/altafard/ai-fleet/internal/provider"
)

// execDeps records every seam invocation so tests can assert on ordering
// and call counts, not just on outcomes.
type execDeps struct {
	gitCalls, dockerCalls, buildCalls, runCalls, pushCalls int
	runArgs                                                []string
	pushURL, pushToken                                     string
	prov                                                   *fakeProvider
}

// stubExec replaces every seam with a canned success; individual tests
// override what they need. Restore runs via t.Cleanup.
func stubExec(t *testing.T) *execDeps {
	t.Helper()
	d := &execDeps{prov: &fakeProvider{url: "https://github.com/o/r/pull/1"}}
	origGit, origDocker := gitVersion, dockerVersion
	origBuild, origRun := buildImage, runContainer
	origStop, origRemove, origPrune := stopContainer, removeContainer, pruneRepo
	origPush, origProv := pushBranch, newProvider
	t.Cleanup(func() {
		gitVersion, dockerVersion = origGit, origDocker
		buildImage, runContainer = origBuild, origRun
		stopContainer, removeContainer, pruneRepo = origStop, origRemove, origPrune
		pushBranch, newProvider = origPush, origProv
	})
	gitVersion = func() (string, error) { d.gitCalls++; return "2.47.0", nil }
	dockerVersion = func() (string, error) { d.dockerCalls++; return "28.0.0", nil }
	buildImage = func(ctx context.Context, dockerfile, contextDir, tag string, onLine func(string)) (string, error) {
		d.buildCalls++
		return "sha256:stub", nil
	}
	runContainer = func(ctx context.Context, args, env []string, onLine func(string)) (int, error) {
		d.runCalls++
		d.runArgs = args
		return 0, nil
	}
	stopContainer = func(name string) error { return nil }
	removeContainer = func(name string) error { return nil }
	pruneRepo = func(repo, keep string) (int, []string, error) { return 0, nil, nil }
	pushBranch = func(dir, url, branch, token string) error {
		d.pushCalls++
		d.pushURL, d.pushToken = url, token
		return nil
	}
	newProvider = func(name string) (provider.Provider, error) {
		if name != "github" {
			return nil, fmt.Errorf("unsupported git provider %q", name)
		}
		return d.prov, nil
	}
	t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "oauth-test-token")
	return d
}

type fakeProvider struct {
	url     string
	prErr   error
	created *provider.PR
}

func (f *fakeProvider) PushURL(repo, token string) (string, error) {
	return "https://github.com/" + repo + ".git", nil
}

func (f *fakeProvider) CreatePR(c *http.Client, repo, token string, pr provider.PR) (string, bool, error) {
	f.created = &pr
	if f.prErr != nil {
		return "", false, f.prErr
	}
	return f.url, false, nil
}

// validOptions builds a minimal non-PR Options against fx, with an explicit
// dockerfile so preflight does not require an initialized project.
func validOptions(t *testing.T, fx collectFixture) Options {
	t.Helper()
	df := filepath.Join(fx.root, "test.dockerfile")
	if err := os.WriteFile(df, []byte("FROM scratch\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return Options{
		Prompt: "do the task", Project: fx.root, Dockerfile: df,
		Branch: "feature/x", Model: "opus", Effort: "high",
		GitAuthorName: "T", GitAuthorEmail: "t@t",
	}
}

// captureOutput runs f with os.Stdout and os.Stderr redirected into pipes
// and returns everything written to them. Execute writes its console to
// os.Stdout directly, so assertions on user-visible output need this.
func captureOutput(t *testing.T, f func()) string {
	t.Helper()
	origOut, origErr := os.Stdout, os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout, os.Stderr = w, w
	defer func() { os.Stdout, os.Stderr = origOut, origErr }()
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	f()
	w.Close()
	return <-done
}

// readStatus loads the single run's status.json under fx.root.
func readStatus(t *testing.T, root string) map[string]any {
	t.Helper()
	matches, _ := filepath.Glob(filepath.Join(root, ".ai-fleet", "runs", "*", "status.json"))
	if len(matches) != 1 {
		t.Fatalf("want exactly one status.json, got %v", matches)
	}
	b, err := os.ReadFile(matches[0])
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("status.json is not valid JSON: %v\n%s", err, b)
	}
	return m
}

// simulateSession makes runContainer behave like a real session: it locates
// the run dir the orchestrator just created, drops the fixture bundle (and
// optionally a pull-request.md) into out/, and emits one stream line.
func simulateSession(t *testing.T, d *execDeps, fx collectFixture, exit int, withPR bool) {
	t.Helper()
	runContainer = func(ctx context.Context, args, env []string, onLine func(string)) (int, error) {
		d.runCalls++
		d.runArgs = args
		matches, _ := filepath.Glob(filepath.Join(fx.root, ".ai-fleet", "runs", "*"))
		if len(matches) != 1 {
			return 0, fmt.Errorf("want exactly one run dir, got %v", matches)
		}
		out := filepath.Join(matches[0], "out")
		b, err := os.ReadFile(fx.bundle)
		if err != nil {
			return 0, err
		}
		if err := os.WriteFile(filepath.Join(out, "run.bundle"), b, 0o644); err != nil {
			return 0, err
		}
		if withPR {
			pr := "feat: add work\n\n## Summary\nadds work\n"
			if err := os.WriteFile(filepath.Join(out, "pull-request.md"), []byte(pr), 0o644); err != nil {
				return 0, err
			}
		}
		onLine(`{"type":"assistant","message":{"content":[{"type":"text","text":"hi"}]}}`)
		return exit, nil
	}
}

func TestExecutePreflightOrderAndUsageExits(t *testing.T) {
	fx := newCollectFixture(t)

	t.Run("git missing stops before docker", func(t *testing.T) {
		d := stubExec(t)
		gitVersion = func() (string, error) { return "", errors.New("git CLI not found in PATH") }
		var code int
		captureOutput(t, func() { code = Execute(validOptions(t, fx)) })
		if code != ExitUsage {
			t.Fatalf("exit = %d, want %d", code, ExitUsage)
		}
		if d.dockerCalls != 0 {
			t.Fatal("docker was checked even though the git check failed first")
		}
	})

	t.Run("docker missing", func(t *testing.T) {
		stubExec(t)
		dockerVersion = func() (string, error) { return "", errors.New("docker CLI not found in PATH") }
		var code int
		captureOutput(t, func() { code = Execute(validOptions(t, fx)) })
		if code != ExitUsage {
			t.Fatalf("exit = %d, want %d", code, ExitUsage)
		}
	})

	t.Run("missing claude token", func(t *testing.T) {
		stubExec(t)
		t.Setenv("CLAUDE_CODE_OAUTH_TOKEN", "")
		var code int
		out := captureOutput(t, func() { code = Execute(validOptions(t, fx)) })
		if code != ExitUsage {
			t.Fatalf("exit = %d, want %d", code, ExitUsage)
		}
		if !strings.Contains(out, "CLAUDE_CODE_OAUTH_TOKEN") {
			t.Fatalf("error does not name the missing variable:\n%s", out)
		}
	})

	t.Run("not a repository", func(t *testing.T) {
		stubExec(t)
		o := validOptions(t, fx)
		o.Project = t.TempDir()
		var code int
		captureOutput(t, func() { code = Execute(o) })
		if code != ExitUsage {
			t.Fatalf("exit = %d, want %d", code, ExitUsage)
		}
	})

	t.Run("not initialized without --dockerfile", func(t *testing.T) {
		stubExec(t)
		o := validOptions(t, fx)
		o.Dockerfile = ""
		var code int
		out := captureOutput(t, func() { code = Execute(o) })
		if code != ExitUsage {
			t.Fatalf("exit = %d, want %d", code, ExitUsage)
		}
		if !strings.Contains(out, "ai-fleet init") {
			t.Fatalf("error does not point at `ai-fleet init`:\n%s", out)
		}
	})
}

func TestExecuteBuildFailureLeavesNoRunDir(t *testing.T) {
	fx := newCollectFixture(t)
	d := stubExec(t)
	buildImage = func(ctx context.Context, dockerfile, contextDir, tag string, onLine func(string)) (string, error) {
		d.buildCalls++
		onLine("some builder output")
		return "", errors.New("docker build failed with exit code 1")
	}
	var code int
	captureOutput(t, func() { code = Execute(validOptions(t, fx)) })
	if code != ExitFailure {
		t.Fatalf("exit = %d, want %d", code, ExitFailure)
	}
	if d.buildCalls != 1 {
		t.Fatalf("build attempted %d times, want exactly 1 (no retry)", d.buildCalls)
	}
	if _, err := os.Stat(filepath.Join(fx.root, ".ai-fleet")); !os.IsNotExist(err) {
		t.Fatal("a failed build must leave nothing on disk")
	}
	if d.runCalls != 0 {
		t.Fatal("container ran after a failed build")
	}
}

func TestExecuteNoChangesRun(t *testing.T) {
	fx := newCollectFixture(t)
	d := stubExec(t)
	var code int
	captureOutput(t, func() { code = Execute(validOptions(t, fx)) })
	if code != ExitNoChanges {
		t.Fatalf("exit = %d, want %d", code, ExitNoChanges)
	}
	if d.runCalls != 1 {
		t.Fatalf("container ran %d times, want exactly 1", d.runCalls)
	}
	// The entrypoint contract: the container command is bash, explicitly.
	if !strings.Contains(strings.Join(d.runArgs, " "), "bash") {
		t.Fatalf("container not launched with bash: %v", d.runArgs)
	}
	st := readStatus(t, fx.root)
	if st["exit_code"] != float64(ExitNoChanges) || st["model"] != "opus" || st["effort"] != "high" {
		t.Fatalf("status.json: %v", st)
	}
	if _, exists := st["error"]; exists {
		t.Fatal("a no-change run is a success; status.json must carry no error")
	}
	matches, _ := filepath.Glob(filepath.Join(fx.root, ".ai-fleet", "runs", "*"))
	if len(matches) != 1 {
		t.Fatalf("run dirs: %v", matches)
	}
	for _, f := range []string{"prompt.md", "entrypoint.sh", "out/log.jsonl"} {
		if _, err := os.Stat(filepath.Join(matches[0], f)); err != nil {
			t.Errorf("%s missing from the run dir", f)
		}
	}
	if _, err := os.Stat(filepath.Join(matches[0], "worktree")); err == nil {
		t.Error("worktree must be deleted after the run")
	}
}

func TestExecuteRunWithCommits(t *testing.T) {
	fx := newCollectFixture(t)
	d := stubExec(t)
	simulateSession(t, d, fx, 0, false)
	var code int
	out := captureOutput(t, func() { code = Execute(validOptions(t, fx)) })
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitOK, out)
	}
	st := readStatus(t, fx.root)
	if st["commit_count"] != float64(1) || st["exit_code"] != float64(ExitOK) {
		t.Fatalf("status.json: %v", st)
	}
	if !strings.Contains(out, "git fetch") {
		t.Fatalf("merge instructions not printed:\n%s", out)
	}
	// The container stream must land verbatim in out/log.jsonl.
	logs, _ := filepath.Glob(filepath.Join(fx.root, ".ai-fleet", "runs", "*", "out", "log.jsonl"))
	b, err := os.ReadFile(logs[0])
	if err != nil || !strings.Contains(string(b), `"type":"assistant"`) {
		t.Fatalf("log.jsonl: %s (%v)", b, err)
	}
}

func TestExecuteContainerFailureSalvages(t *testing.T) {
	fx := newCollectFixture(t)
	d := stubExec(t)
	simulateSession(t, d, fx, 7, false)
	var code int
	out := captureOutput(t, func() { code = Execute(validOptions(t, fx)) })
	if code != ExitFailure {
		t.Fatalf("exit = %d, want %d", code, ExitFailure)
	}
	if !strings.Contains(out, "salvaged") {
		t.Fatalf("salvage not reported:\n%s", out)
	}
	st := readStatus(t, fx.root)
	if st["commit_count"] != float64(1) {
		t.Fatalf("salvaged commits not recorded: %v", st)
	}
	if st["error"] == nil || st["error"] == "" {
		t.Fatalf("failed run must record an error: %v", st)
	}
}

func TestExecutePublishSuccess(t *testing.T) {
	fx := newCollectFixture(t)
	d := stubExec(t)
	simulateSession(t, d, fx, 0, true)
	o := validOptions(t, fx)
	o.GitProvider, o.GitRepository, o.GitToken = "github", "o/r", "sekret-token-value"
	var code int
	out := captureOutput(t, func() { code = Execute(o) })
	if code != ExitOK {
		t.Fatalf("exit = %d, want %d\n%s", code, ExitOK, out)
	}
	if d.pushCalls != 1 {
		t.Fatalf("push called %d times, want 1", d.pushCalls)
	}
	if d.prov.created == nil || d.prov.created.Title != "feat: add work" ||
		d.prov.created.Head != "feature/x" || d.prov.created.Base != "main" {
		t.Fatalf("PR composed wrong: %+v", d.prov.created)
	}
	st := readStatus(t, fx.root)
	if st["pr_url"] != "https://github.com/o/r/pull/1" {
		t.Fatalf("pr_url not recorded: %v", st)
	}
	if !strings.Contains(out, "https://github.com/o/r/pull/1") {
		t.Fatalf("PR URL not shown to the user:\n%s", out)
	}
	// Secret hygiene: neither token may appear on any user-visible surface
	// or in the persisted status file.
	stBytes, _ := json.Marshal(st)
	for _, surface := range []string{out, string(stBytes)} {
		if strings.Contains(surface, "sekret-token-value") || strings.Contains(surface, "oauth-test-token") {
			t.Fatal("a token leaked into user-visible output or status.json")
		}
	}
}

func TestExecutePublishFailureIsRunFailure(t *testing.T) {
	fx := newCollectFixture(t)
	d := stubExec(t)
	simulateSession(t, d, fx, 0, true)
	d.prov.prErr = errors.New("github PR creation failed: 500 oops")
	o := validOptions(t, fx)
	o.GitProvider, o.GitRepository, o.GitToken = "github", "o/r", "sekret-token-value"
	var code int
	out := captureOutput(t, func() { code = Execute(o) })
	if code != ExitFailure {
		t.Fatalf("exit = %d, want %d", code, ExitFailure)
	}
	if strings.Contains(out, "sekret-token-value") {
		t.Fatal("the git token leaked into failure output")
	}
	st := readStatus(t, fx.root)
	if st["error"] == nil {
		t.Fatalf("publish failure must be recorded in status.json: %v", st)
	}
	if _, exists := st["pr_url"]; exists {
		t.Fatalf("no PR was created; pr_url must be absent: %v", st)
	}
}

func TestExecutePublishBadPRFileFailsBeforePush(t *testing.T) {
	fx := newCollectFixture(t)
	d := stubExec(t)
	simulateSession(t, d, fx, 0, false) // bundle, but no pull-request.md
	o := validOptions(t, fx)
	o.GitProvider, o.GitRepository, o.GitToken = "github", "o/r", "tok"
	var code int
	captureOutput(t, func() { code = Execute(o) })
	if code != ExitFailure {
		t.Fatalf("exit = %d, want %d", code, ExitFailure)
	}
	if d.pushCalls != 0 {
		t.Fatal("a bad pull-request.md must fail before anything reaches the remote")
	}
}

// A signal that lands before the container exists must prevent the launch:
// the build stub raises a real SIGINT against the test process and waits
// for the watcher to cancel the build context, proving the full
// Notify → watcher → stopped → pre-launch re-check chain.
func TestExecuteSignalBeforeLaunchSkipsContainer(t *testing.T) {
	fx := newCollectFixture(t)
	d := stubExec(t)
	buildImage = func(ctx context.Context, dockerfile, contextDir, tag string, onLine func(string)) (string, error) {
		if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
			return "", err
		}
		select {
		case <-ctx.Done():
		case <-time.After(5 * time.Second):
			return "", errors.New("signal watcher never cancelled the build context")
		}
		return "sha256:stub", nil
	}
	var code int
	captureOutput(t, func() { code = Execute(validOptions(t, fx)) })
	if code != ExitInterrupted {
		t.Fatalf("exit = %d, want %d", code, ExitInterrupted)
	}
	if d.runCalls != 0 {
		t.Fatal("container was launched for an operator who already interrupted")
	}
	st := readStatus(t, fx.root)
	if st["exit_code"] != float64(ExitInterrupted) {
		t.Fatalf("status.json: %v", st)
	}
}
