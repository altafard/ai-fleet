package run

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/altafard/ai-fleet/internal/dockerx"
	"github.com/altafard/ai-fleet/internal/execx"
	"github.com/altafard/ai-fleet/internal/gitx"
	"github.com/altafard/ai-fleet/internal/logstream"
	"github.com/altafard/ai-fleet/internal/provider"
	"github.com/altafard/ai-fleet/internal/runner"
	"golang.org/x/term"
)

// Process exit codes returned by [Execute].
const (
	ExitOK          = 0   // success, bundle produced
	ExitFailure     = 1   // run or publish failure
	ExitUsage       = 2   // preflight or usage error
	ExitNoChanges   = 3   // success, but the session made no commits
	ExitInterrupted = 130 // interrupted by the user
)

// state carries everything phases hand to each other.
type state struct {
	o         Options
	root      string
	base      gitx.Base
	dir       Dir
	id        string
	console   *logstream.Console
	logFile   *os.File // out/log.jsonl: the container's stdout, verbatim
	imageID   string
	commits   int
	turns     int // claude assistant turns, for the progress spinner
	stopped   atomic.Bool
	prURL     string
	startedAt time.Time
}

// Execute performs one deploy-unit run through all of its phases —
// preflight, image build, run snapshot, container execution, collect, and
// (in PR mode) publish — and returns the process exit code (see the Exit
// constants).
func Execute(o Options) int {
	if err := o.Validate(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return ExitUsage
	}
	s := &state{o: o}

	// --- Phase 1: preflight — each check renders the moment it completes,
	// so a failure never hides how far preflight got ---
	s.console = logstream.NewConsole(os.Stdout, term.IsTerminal(int(os.Stdout.Fd())))
	fail := func(err error) int {
		s.console.Fail(err.Error())
		fmt.Fprintln(os.Stderr, "error:", err)
		return ExitUsage
	}
	gv, err := gitVersion()
	if err != nil {
		return fail(err)
	}
	s.console.Check("git " + gv)
	dv, err := dockerx.Version()
	if err != nil {
		return fail(err)
	}
	s.console.Check("docker " + dv)
	if os.Getenv("CLAUDE_CODE_OAUTH_TOKEN") == "" {
		return fail(fmt.Errorf("CLAUDE_CODE_OAUTH_TOKEN is not set; run `claude setup-token` and export it"))
	}
	s.console.Check("claude token present")
	if _, err := os.Stat(o.Dockerfile); err != nil {
		return fail(fmt.Errorf("dockerfile not found: %s", o.Dockerfile))
	}
	s.root, err = gitx.RepoRoot(o.Project)
	if err != nil {
		return fail(err)
	}
	s.console.Check(fmt.Sprintf("project %s is a git repository", filepath.Base(s.root)))
	// Baseline resolution is a preflight concern too: an unresolvable default
	// branch is an environment problem (exit 2), and nothing has been created
	// on disk yet.
	s.base, err = gitx.Baseline(s.root)
	if err != nil {
		return fail(err)
	}
	s.console.Check(fmt.Sprintf("baseline %s (%.7s, %s)", s.base.Ref, s.base.SHA, s.base.Source))

	s.id = NewRunID(time.Now())
	if s.o.Branch == "" {
		s.o.Branch = "feature/" + s.id
	}
	stopTick := startTicker(s.console)
	defer stopTick()

	s.startedAt = time.Now().UTC()
	code := runPhases(s)
	// The run dir exists iff a container run was actually prepared; earlier
	// failures (including a failed image build) leave nothing on disk.
	if s.dir.Root() != "" {
		writeFinalStatus(s, code, s.startedAt)
	}
	return code
}

// runPhases installs one signal watcher covering every phase (build, clone,
// container run, collect) and runs them in sequence. On the first Ctrl-C /
// SIGTERM: the build context is cancelled (aborting dockerx.Build), and the
// container — if one has been started — is asked to stop gracefully so the
// entrypoint's trap can salvage a bundle. Regardless of which phase caught
// the signal or what that phase's own return code was, runPhases reports
// ExitInterrupted once a signal has been observed.
func runPhases(s *state) (code int) {
	// worktree/ is a disposable template; delete it on every exit path
	// (success or failure) regardless of which phase returned. Phases that
	// already delete it earlier (collect, publish ordering) make this a
	// harmless no-op — os.RemoveAll on a missing path returns nil. Guarded:
	// before the run dir exists, s.dir is the zero value and its Worktree()
	// would be a relative path.
	defer func() {
		if s.dir.Root() != "" {
			_ = os.RemoveAll(s.dir.Worktree())
		}
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// done unblocks the watcher goroutine on normal completion: signal.Stop
	// only stops future relaying, it does not close sigCh.
	done := make(chan struct{})
	defer close(done)

	var containerName atomic.Value // string; unset until the container starts

	go func() {
		select {
		case <-sigCh:
		case <-done:
			return
		}
		s.stopped.Store(true)
		cancel()
		if name, ok := containerName.Load().(string); ok && name != "" {
			if err := dockerx.Stop(name); err != nil {
				fmt.Fprintln(os.Stderr, "ai-fleet: docker stop failed:", err)
			}
		}
		// Second Ctrl-C: immediate kill, no salvage. Keep watching sigCh so a
		// repeated signal is never silently dropped into the buffered channel
		// while nobody is listening (which would otherwise make the process
		// unkillable short of SIGKILL).
		select {
		case <-sigCh:
			fmt.Fprintln(os.Stderr, "ai-fleet: second interrupt received, exiting immediately")
			os.Exit(ExitInterrupted)
		case <-done:
			return
		}
	}()

	defer func() {
		if s.stopped.Load() {
			code = ExitInterrupted
		}
	}()

	// --- Phase 2: image build (console-only: docker output drives the
	// spinner and, on failure, the printed tail — it is not logged). Runs
	// before anything is created on disk: a failed build leaves no run dir. ---
	df, err := os.ReadFile(s.o.Dockerfile)
	if err != nil {
		code = fatal(s, err)
		return
	}
	tag := dockerx.ImageTag(df)
	s.console.Spin("building image")
	buildStart := time.Now()
	var buildTail []string
	const buildTailMax = 10
	s.imageID, err = dockerx.Build(ctx, s.o.Dockerfile, filepath.Dir(s.o.Dockerfile), tag,
		func(line string) {
			if step, total, instr, ok := logstream.ParseBuildStep(line); ok {
				s.console.Spin(fmt.Sprintf("building image — step %d/%d: %s", step, total, instr))
			}
			buildTail = append(buildTail, line)
			if len(buildTail) > buildTailMax {
				buildTail = buildTail[len(buildTail)-buildTailMax:]
			}
		})
	if err != nil {
		s.console.Fail(err.Error())
		fmt.Fprintln(os.Stderr)
		for _, l := range buildTail {
			fmt.Fprintln(os.Stderr, "  "+l)
		}
		code = ExitFailure
		return
	}
	s.console.Done(fmt.Sprintf("image built in %s", time.Since(buildStart).Round(time.Second)))

	// --- Phase 3: run snapshot — the run dir exists only from here on ---
	s.dir, err = CreateRunDir(s.root, s.id)
	if err != nil {
		code = fatal(s, err)
		return
	}
	if err := gitx.CloneNoCheckout(s.root, s.dir.Worktree(), s.base.SHA); err != nil {
		code = fatal(s, err)
		return
	}
	s.console.Check("baseline clone created")
	task := s.o.Prompt
	if s.o.PromptFile != "" {
		b, err := os.ReadFile(s.o.PromptFile)
		if err != nil {
			code = fatal(s, err)
			return
		}
		task = string(b)
	}
	prompt, err := runner.RenderPrompt(s.o.Branch, task)
	if err != nil {
		code = fatal(s, err)
		return
	}
	if err := os.WriteFile(s.dir.PromptFile(), []byte(prompt), 0o644); err != nil {
		code = fatal(s, err)
		return
	}
	if err := os.WriteFile(s.dir.EntrypointFile(), runner.EntrypointScript(), 0o755); err != nil {
		code = fatal(s, err)
		return
	}

	// --- Phase 4+5: container run (entrypoint acts inside; its stdout —
	// claude's stream-json — is written verbatim to out/log.jsonl) ---
	s.logFile, err = os.Create(s.dir.LogFile())
	if err != nil {
		code = fatal(s, err)
		return
	}
	defer s.logFile.Close()

	name := "ai-fleet-" + s.id
	mounts := []dockerx.Mount{
		{Source: s.dir.Worktree(), Target: "/source/clone", RO: true},
		{Source: s.dir.PromptFile(), Target: "/source/prompt.md", RO: true},
		{Source: s.dir.EntrypointFile(), Target: "/source/entrypoint.sh", RO: true},
		{Source: s.dir.Out(), Target: "/out"},
	}
	envKeys := []string{"CLAUDE_CODE_OAUTH_TOKEN", "GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL",
		"FLEET_BRANCH", "FLEET_BASELINE_SHA"}
	env := []string{
		"GIT_AUTHOR_NAME=" + s.o.GitAuthorName, "GIT_AUTHOR_EMAIL=" + s.o.GitAuthorEmail,
		"FLEET_BRANCH=" + s.o.Branch, "FLEET_BASELINE_SHA=" + s.base.SHA,
	}
	args := dockerx.RunArgs(tag, name, mounts, envKeys, []string{"bash", "/source/entrypoint.sh"})
	// Set before starting the container so a signal arriving during startup
	// can never race the watcher goroutine into skipping the stop.
	containerName.Store(name)
	s.console.Spin("container running")

	exit, err := dockerx.RunContainer(context.Background(), args, env, func(line string) {
		s.logFile.WriteString(line + "\n")
		s.spinClaude(line)
	})
	if err != nil {
		code = fatal(s, err)
		return
	}
	if exit == 0 {
		s.console.Done("container exited")
	} else {
		s.console.Fail(fmt.Sprintf("container exited with code %d", exit))
	}

	// --- Phase 6: collect (salvage semantics preserved: collect still runs
	// even when the container was stopped by a signal, so a bundle salvaged
	// by the entrypoint's trap is still verified and reported) ---
	code = collect(s, exit)
	return
}

// spinClaude derives spinner progress from one line of the container's
// stdout when it is a claude stream-json assistant event; every other line
// is left alone.
func (s *state) spinClaude(line string) {
	var ev map[string]any
	if json.Unmarshal([]byte(line), &ev) != nil {
		return
	}
	if t, _ := ev["type"].(string); t != "assistant" {
		return
	}
	s.turns++
	msg, _ := logstream.ClaudeSummary(ev)
	s.console.Spin(fmt.Sprintf("claude — turn %d: %s", s.turns, msg))
}

// gitVersion returns e.g. "2.44.0".
func gitVersion() (string, error) {
	r, err := execx.Run("", "git", "--version")
	if err != nil || r.ExitCode != 0 {
		return "", fmt.Errorf("git CLI not found in PATH")
	}
	return strings.TrimPrefix(r.Stdout, "git version "), nil
}

// collect verifies the bundle, counts commits and prints merge instructions.
// containerExit is the entrypoint's (claude's) exit code.
func collect(s *state, containerExit int) int {
	deleteWorktree := func() { os.RemoveAll(s.dir.Worktree()) }

	if _, err := os.Stat(s.dir.BundleFile()); err != nil {
		// No bundle: either a clean no-change run or a failure without salvage.
		deleteWorktree()
		if containerExit == 0 {
			s.console.Check("run finished: no changes")
			return ExitNoChanges
		}
		return fatal(s, fmt.Errorf("container failed with exit code %d and produced no bundle", containerExit))
	}
	if err := gitx.VerifyBundle(s.dir.Worktree(), s.dir.BundleFile()); err != nil {
		deleteWorktree()
		return fatal(s, err)
	}
	if err := gitx.FetchBundle(s.dir.Worktree(), s.dir.BundleFile(), s.o.Branch); err != nil {
		deleteWorktree()
		return fatal(s, err)
	}
	n, err := gitx.CountCommits(s.dir.Worktree(), s.base.SHA, s.o.Branch)
	if err != nil {
		deleteWorktree()
		return fatal(s, err)
	}
	s.commits = n
	s.console.Check(fmt.Sprintf("bundle verified: %d commits", n))

	code := ExitOK
	if containerExit != 0 {
		// Salvaged bundle from a failed run: keep it, report failure, no PR.
		code = ExitFailure
		s.console.Fail(fmt.Sprintf("run failed (exit %d) but %d commits were salvaged into the bundle", containerExit, n))
	} else if s.o.PRMode() {
		if err := publish(s); err != nil {
			deleteWorktree()
			return fatal(s, err)
		}
	}
	deleteWorktree()
	if code == ExitOK {
		fmt.Print(MergeInstructions(s.dir.Rel(s.root), s.o.Branch, s.base.Branch, n))
	}
	return code
}

// publish runs only for successful runs with commits (enforced by collect).
// Order matters: validate pull-request.md BEFORE pushing, so a composition
// failure leaves nothing on the remote. No fallback composition (spec).
func publish(s *state) error {
	title, body, err := ParsePRFile(s.dir.PRFile())
	if err != nil {
		return err
	}
	s.console.Check("PR composed: " + title)

	p, err := provider.New(s.o.GitProvider)
	if err != nil {
		return err
	}
	pushURL, err := p.PushURL(s.o.GitRepository, s.o.GitToken)
	if err != nil {
		return err
	}
	// The bundle was already fetched into worktree/ by collect.
	if err := gitx.Push(s.dir.Worktree(), pushURL, s.o.Branch, s.o.GitToken); err != nil {
		return err
	}
	s.console.Check("pushed " + s.o.Branch)

	client := &http.Client{Timeout: 30 * time.Second}
	url, err := p.CreatePR(client, s.o.GitRepository, s.o.GitToken, provider.PR{
		Title: title, Body: body, Head: s.o.Branch, Base: s.base.Branch,
	})
	if err != nil {
		return err
	}
	s.prURL = url
	s.console.Check("pull request created: " + url)
	fmt.Println("Pull request:", url)
	return nil
}

// fatal reports a run failure. The log-file pointer only makes sense once
// the container's stream exists.
func fatal(s *state, err error) int {
	s.console.Fail(err.Error())
	if s.logFile != nil {
		fmt.Fprintln(os.Stderr, "See", s.dir.LogFile())
	}
	return ExitFailure
}

func startTicker(c *logstream.Console) func() {
	t := time.NewTicker(100 * time.Millisecond)
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-t.C:
				c.Tick()
			case <-done:
				return
			}
		}
	}()
	return func() { t.Stop(); close(done) }
}

// writeFinalStatus persists status.json for any outcome.
func writeFinalStatus(s *state, code int, started time.Time) {
	st := Status{
		RunID: s.id, BaselineRef: s.base.Ref, BaselineSHA: s.base.SHA,
		Branch: s.o.Branch, ImageID: s.imageID, ExitCode: code, CommitCount: s.commits,
		PRURL: s.prURL, StartedAt: started.Format(time.RFC3339),
		FinishedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if code != ExitOK && code != ExitNoChanges {
		st.Error = fmt.Sprintf("run failed with exit code %d", code)
	}
	WriteStatus(s.dir.StatusFile(), st)
}
