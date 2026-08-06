// Package dockerx shells out to the docker CLI — never the SDK, so Docker
// contexts and credential helpers keep working on Docker Desktop.
package dockerx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	"github.com/altafard/ai-fleet/internal/execx"
)

// Version returns the docker client version, erroring when the CLI is
// missing from PATH or the daemon is unreachable.
func Version() (string, error) {
	r, err := execx.Run("", "docker", "version", "--format", "{{.Client.Version}}")
	if err != nil {
		return "", errors.New("docker CLI not found in PATH")
	}
	if r.ExitCode != 0 {
		return "", fmt.Errorf("docker is not reachable: %s", r.Stderr)
	}
	return r.Stdout, nil
}

// Mount describes one bind mount for docker run.
type Mount struct {
	Source string
	Target string
	RO     bool
}

// Arg renders the mount as a docker -v argument ("source:target" with a
// ":ro" suffix for read-only mounts).
func (m Mount) Arg() string {
	s := m.Source + ":" + m.Target
	if m.RO {
		s += ":ro"
	}
	return s
}

// ContentTag derives an image tag from Dockerfile content — the first 12
// hex characters of its SHA-256 — so an unchanged Dockerfile reuses the
// cached image across runs.
func ContentTag(dockerfileContent []byte) string {
	sum := sha256.Sum256(dockerfileContent)
	return hex.EncodeToString(sum[:])[:12]
}

// ImageTag is the legacy single-namespace tag used when the user supplies
// an explicit --dockerfile: intent unknown, so these images are never
// pruned and stay out of the per-project repositories.
func ImageTag(dockerfileContent []byte) string {
	return "ai-fleet:" + ContentTag(dockerfileContent)
}

// BuildArgs deliberately passes no --progress flag: the legacy builder
// rejects it (exit 125), and BuildKit auto-selects plain progress when its
// output is piped — which it always is here. Both builders therefore
// stream parseable plain text on every docker install.
func BuildArgs(dockerfile, contextDir, tag string) []string {
	return []string{"build", "-f", dockerfile, "-t", tag, contextDir}
}

// RunArgs passes env as bare "-e KEY": docker reads the value from its own
// process environment, keeping secrets out of argv.
func RunArgs(image, name string, mounts []Mount, envKeys []string, cmd []string) []string {
	args := []string{"run", "--rm", "--name", name}
	for _, m := range mounts {
		args = append(args, "-v", m.Arg())
	}
	for _, k := range envKeys {
		args = append(args, "-e", k)
	}
	args = append(args, image)
	return append(args, cmd...)
}

// Build runs docker build, streaming every output line to onLine, and
// returns the built image ID. Cancelling ctx aborts the build.
func Build(ctx context.Context, dockerfile, contextDir, tag string, onLine func(string)) (string, error) {
	code, err := execx.Stream(ctx, "", nil, onLine, "docker", BuildArgs(dockerfile, contextDir, tag)...)
	if err != nil {
		return "", err
	}
	if code != 0 {
		return "", fmt.Errorf("docker build failed with exit code %d", code)
	}
	r, err := execx.Run("", "docker", "inspect", "--format", "{{.Id}}", tag)
	if err != nil || r.ExitCode != 0 {
		return "", fmt.Errorf("cannot inspect built image: %s", r.Stderr)
	}
	return r.Stdout, nil
}

// RunContainer streams merged container output; returns the container's exit code.
func RunContainer(ctx context.Context, args []string, env []string, onLine func(string)) (int, error) {
	return execx.Stream(ctx, "", env, onLine, "docker", args...)
}

// Stop gracefully stops the named container (docker stop), giving its
// PID 1 the grace period to clean up before Docker escalates to SIGKILL.
func Stop(name string) error {
	r, err := execx.Run("", "docker", "stop", name)
	if err != nil || r.ExitCode != 0 {
		return fmt.Errorf("docker stop failed: %s", r.Stderr)
	}
	return nil
}

// ListTags returns the tags currently present for one image repository.
func ListTags(repo string) ([]string, error) {
	r, err := execx.Run("", "docker", "images", repo, "--format", "{{.Tag}}")
	if err != nil || r.ExitCode != 0 {
		return nil, fmt.Errorf("docker images failed: %s", r.Stderr)
	}
	if r.Stdout == "" {
		return nil, nil
	}
	return strings.Fields(r.Stdout), nil
}

// PruneRepo removes every image in repo whose tag differs from keep.
// Individual removal failures (an image still used by a container) are
// warnings, not errors — pruning is housekeeping, never a reason to fail
// the surrounding command.
func PruneRepo(repo, keep string) (removed int, warnings []string, err error) {
	tags, err := ListTags(repo)
	if err != nil {
		return 0, nil, err
	}
	for _, t := range tags {
		if t == keep || t == "<none>" {
			continue
		}
		r, err := execx.Run("", "docker", "rmi", repo+":"+t)
		if err != nil || r.ExitCode != 0 {
			warnings = append(warnings, fmt.Sprintf("could not remove %s:%s: %s", repo, t, r.Stderr))
			continue
		}
		removed++
	}
	return removed, warnings, nil
}
