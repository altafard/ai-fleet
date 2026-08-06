package initx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
	gitVersion    = gitx.Version
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
// A generated Dockerfile already on disk is adopted verbatim and the
// analysis is skipped — the file is committed, its registration is not.
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

	name, hash := ProjectName(root), ProjectHash(root)
	dfPath := filepath.Join(root, ".ai-fleet", DockerfileName(name))

	stopTick := startTicker(console)
	defer stopTick()

	// The Dockerfile is committed while ai-fleet.ini is not, so a fresh clone
	// has the one but not the other. Analyzing again would overwrite a file
	// the user was invited to hand-edit; there is nothing to analyze here,
	// only a registration to restore.
	var df []byte
	if fileExists(dfPath) {
		df, err = os.ReadFile(dfPath)
		if err != nil {
			return fail(ExitFailure, err)
		}
		console.Check("found existing dockerfile, re-registering without re-analyzing: " + dfPath)
	} else {
		console.Spin("analyzing project inventory (claude)")
		inv, err := analyze(root)
		if err != nil {
			return fail(ExitFailure, fmt.Errorf("inventory analysis failed: %w", err))
		}
		console.Done(fmt.Sprintf("inventory: %s, %d packages, %d env vars",
			inv.BaseImage, len(inv.Packages), len(inv.Env)))
		rendered, err := RenderDockerfile(inv)
		if err != nil {
			return fail(ExitFailure, err)
		}
		df = rendered
	}

	keptDockerignore := fileExists(filepath.Join(root, ".dockerignore"))
	if _, err := WriteFiles(root, Config{Name: name, Hash: hash}, df); err != nil {
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
	built := fmt.Sprintf("image built in %s", time.Since(buildStart).Round(time.Second))
	// "0 old image(s) pruned" reads as "nothing needed pruning" even when the
	// prune itself failed, so the count is only reported when it happened.
	if err == nil && removed > 0 {
		built += fmt.Sprintf(", %d old image(s) pruned", removed)
	}
	console.Done(built)
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
