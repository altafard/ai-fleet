package run

import (
	"os"
	"path/filepath"
)

// Dir is the on-disk layout of one run under .ai-fleet/runs/<run-id>/.
type Dir struct{ root string }

// CreateRunDir creates .ai-fleet/runs/<id>/ (including its out/
// subdirectory) under projectRoot and returns the layout.
func CreateRunDir(projectRoot, id string) (Dir, error) {
	d := Dir{root: filepath.Join(projectRoot, ".ai-fleet", "runs", id)}
	if err := os.MkdirAll(d.Out(), 0o755); err != nil {
		return Dir{}, err
	}
	return d, nil
}

// Root returns the run directory itself; it is empty for the zero Dir.
func (d Dir) Root() string { return d.root }

// Worktree returns the baseline clone directory (deleted after the run).
func (d Dir) Worktree() string { return filepath.Join(d.root, "worktree") }

// Out returns the directory mounted read-write into the container at /out.
func (d Dir) Out() string { return filepath.Join(d.root, "out") }

// PromptFile returns the rendered prompt, mounted at /source/prompt.md.
func (d Dir) PromptFile() string { return filepath.Join(d.root, "prompt.md") }

// EntrypointFile returns the entrypoint script, mounted at /source/entrypoint.sh.
func (d Dir) EntrypointFile() string { return filepath.Join(d.root, "entrypoint.sh") }

// StatusFile returns the run's status.json path.
func (d Dir) StatusFile() string { return filepath.Join(d.root, "status.json") }

// BundleFile returns the result bundle path (out/run.bundle).
func (d Dir) BundleFile() string { return filepath.Join(d.Out(), "run.bundle") }

// LogFile returns the container-stream log path (out/log.jsonl).
func (d Dir) LogFile() string { return filepath.Join(d.Out(), "log.jsonl") }

// PRFile returns the session-written PR description path (out/pull-request.md).
func (d Dir) PRFile() string { return filepath.Join(d.Out(), "pull-request.md") }

// Rel returns the run dir relative to the project root with forward slashes,
// for user-facing messages on every OS.
func (d Dir) Rel(projectRoot string) string {
	r, err := filepath.Rel(projectRoot, d.root)
	if err != nil {
		return d.root
	}
	return filepath.ToSlash(r)
}
