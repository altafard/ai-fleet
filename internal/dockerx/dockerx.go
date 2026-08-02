// Package dockerx shells out to the docker CLI — never the SDK, so Docker
// contexts and credential helpers keep working on Docker Desktop.
package dockerx

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

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

// ImageTag derives the image tag from the Dockerfile content —
// "ai-fleet:" plus the first 12 hex characters of its SHA-256 — so an
// unchanged Dockerfile reuses the cached image across runs.
func ImageTag(dockerfileContent []byte) string {
	sum := sha256.Sum256(dockerfileContent)
	return "ai-fleet:" + hex.EncodeToString(sum[:])[:12]
}

func BuildArgs(dockerfile, contextDir, tag string) []string {
	return []string{"build", "--progress=plain", "-f", dockerfile, "-t", tag, contextDir}
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
