package initx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/altafard/ai-fleet/internal/dockerx"
	"github.com/altafard/ai-fleet/internal/execx"
	"github.com/altafard/ai-fleet/internal/gitx"
	"github.com/altafard/ai-fleet/internal/logstream"
	"golang.org/x/term"
)

// Process exit codes returned by [Execute].
const (
	ExitOK      = 0 // initialized, image built
	ExitFailure = 1 // analysis, write or build failure
	ExitUsage   = 2 // toolchain or repository preflight failure
)

// Test seams: every external effect goes through a package variable so the
// orchestrator's stage ordering is testable without claude or docker.
var (
	gitVersion = func() (string, error) {
		r, err := execx.Run("", "git", "--version")
		if err != nil || r.ExitCode != 0 {
			return "", errors.New("git CLI not found in PATH")
		}
		return strings.TrimPrefix(r.Stdout, "git version "), nil
	}
	claudeVersion = func() (string, error) {
		r, err := execx.Run("", "claude", "--version")
		if err != nil || r.ExitCode != 0 {
			return "", errors.New("claude CLI not found in PATH")
		}
		return r.Stdout, nil
	}
	dockerVersion = dockerx.Version
	analyze       = Analyze
	buildImage    = dockerx.Build
	pruneRepo     = dockerx.PruneRepo
)

// Execute performs one `ai-fleet init` through all of its stages —
// toolchain checks, repository resolution, inventory analysis, file
// generation, image prebuild and prune — and returns the process exit code.
// Stage order is a guarantee: nothing is written to disk before the
// analysis has succeeded, so early failures leave the project untouched.
func Execute(cwd string) int {
	console := logstream.NewConsole(os.Stdout, term.IsTerminal(int(os.Stdout.Fd())))
	fail := func(code int, err error) int {
		console.Fail(err.Error())
		fmt.Fprintln(os.Stderr, "error:", err)
		return code
	}

	gv, err := gitVersion()
	if err != nil {
		return fail(ExitUsage, err)
	}
	console.Check("git " + gv)
	cv, err := claudeVersion()
	if err != nil {
		return fail(ExitUsage, err)
	}
	console.Check("claude " + cv)
	dv, err := dockerVersion()
	if err != nil {
		return fail(ExitUsage, err)
	}
	console.Check("docker " + dv)

	root, err := gitx.RepoRoot(cwd)
	if err != nil {
		return fail(ExitUsage, err)
	}
	console.Check("repository root: " + root)
	if !sameDir(cwd, root) {
		console.Warn("running from a subdirectory — initializing at " + filepath.Join(root, ".ai-fleet"))
	}

	if _, err := os.Stat(filepath.Join(root, ".ai-fleet", "ai-fleet.ini")); err == nil {
		return fail(ExitUsage, errors.New("project already initialized (.ai-fleet/ai-fleet.ini exists)"))
	}

	stopTick := startTicker(console)
	defer stopTick()

	console.Spin("analyzing project inventory (claude)")
	inv, err := analyze(root)
	if err != nil {
		return fail(ExitFailure, fmt.Errorf("inventory analysis failed: %w", err))
	}
	console.Done(fmt.Sprintf("inventory: %s, %d packages, %d env vars",
		inv.BaseImage, len(inv.Packages), len(inv.Env)))

	name, hash := ProjectName(root), ProjectHash(root)
	df, err := RenderDockerfile(inv)
	if err != nil {
		return fail(ExitFailure, err)
	}
	keptDockerignore := fileExists(filepath.Join(root, ".dockerignore"))
	dfPath, err := WriteFiles(root, Config{Global: false, Name: name, Hash: hash}, df)
	if err != nil {
		return fail(ExitFailure, err)
	}
	wrote := "wrote .ai-fleet/.gitignore, ai-fleet.ini, " + DockerfileName(name)
	if keptDockerignore {
		console.Warn("kept the existing .dockerignore — add .git and .ai-fleet to keep the build context small")
	} else {
		wrote += " and .dockerignore"
	}
	console.Check(wrote)

	repo := ImageRepo(name, hash)
	tag := repo + ":" + dockerx.ContentTag(df)
	console.Spin("building " + tag)
	buildStart := time.Now()
	var tail []string
	const tailMax = 10
	_, err = buildImage(context.Background(), dfPath, root, tag, func(line string) {
		if step, total, instr, ok := logstream.ParseBuildStep(line); ok {
			console.Spin(fmt.Sprintf("building image — step %d/%d: %s", step, total, instr))
		}
		tail = append(tail, line)
		if len(tail) > tailMax {
			tail = tail[len(tail)-tailMax:]
		}
	})
	if err != nil {
		console.Fail("docker build failed: " + err.Error())
		fmt.Fprintln(os.Stderr)
		for _, l := range tail {
			fmt.Fprintln(os.Stderr, "  "+l)
		}
		fmt.Fprintf(os.Stderr, "The generated files were kept — edit %s and `ai-fleet deploy unit` will rebuild the image.\n", dfPath)
		return ExitFailure
	}

	removed, warns, err := pruneRepo(repo, dockerx.ContentTag(df))
	for _, w := range warns {
		console.Warn(w)
	}
	if err != nil {
		console.Warn("image prune failed: " + err.Error())
	}
	console.Done(fmt.Sprintf("image built in %s, %d old image(s) pruned",
		time.Since(buildStart).Round(time.Second), removed))
	return ExitOK
}

// sameDir reports whether cwd and root are the same directory, resolving
// symlinks so macOS /tmp vs /private/tmp never triggers a bogus warning.
func sameDir(cwd, root string) bool {
	a, err1 := filepath.EvalSymlinks(cwd)
	b, err2 := filepath.EvalSymlinks(root)
	if err1 != nil || err2 != nil {
		return filepath.Clean(cwd) == filepath.Clean(root)
	}
	return a == b
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
