package initx

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DockerfileName is the committed Dockerfile's file name inside .ai-fleet/.
func DockerfileName(name string) string { return name + ".ai-fleet.dockerfile" }

// GitignoreContent is allowlist-style: ignore everything, keep the
// gitignore itself (plain "*" would ignore the file defining the rules)
// and the generated Dockerfile. ai-fleet.ini stays untracked on purpose —
// its hash is machine-specific.
func GitignoreContent(name string) string {
	return "*\n!.gitignore\n!" + DockerfileName(name) + "\n"
}

// DockerignoreContent trims the build context: the generated Dockerfile has
// no COPY or ADD, so tarring .git/ and .ai-fleet/runs/ (bundles and full
// session transcripts, which only grow) into every build buys nothing.
const DockerignoreContent = ".git\n.ai-fleet\n"

// WriteFiles creates .ai-fleet/ under root (tolerating a pre-existing
// folder — deploy runs create .ai-fleet/runs/ on their own) and writes the
// three generated files, plus a repo-root .dockerignore when none exists.
// Returns the Dockerfile's path.
func WriteFiles(root string, c Config, dockerfile []byte) (string, error) {
	dir := filepath.Join(root, ".ai-fleet")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte(GitignoreContent(c.Name)), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "ai-fleet.ini"), []byte(RenderINI(c)), 0o644); err != nil {
		return "", err
	}
	dfPath := filepath.Join(dir, DockerfileName(c.Name))
	if err := os.WriteFile(dfPath, dockerfile, 0o644); err != nil {
		return "", err
	}
	// Docker only honours a .dockerignore at the build context root, which
	// here is the repository root, not .ai-fleet/. An existing one belongs to
	// the project and its rules are none of our business.
	if di := filepath.Join(root, ".dockerignore"); !fileExists(di) {
		if err := os.WriteFile(di, []byte(DockerignoreContent), 0o644); err != nil {
			return "", err
		}
	}
	return dfPath, nil
}

// fileExists reports whether path is present; an unreadable path counts as
// present, since writing over it is the outcome to avoid.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil || !os.IsNotExist(err)
}

// ResolveProject loads the project config under root and returns the
// generated Dockerfile path and the project's image repository. This is
// what `deploy unit` calls when --dockerfile is omitted.
func ResolveProject(root string) (dockerfilePath, imageRepo string, err error) {
	data, err := os.ReadFile(filepath.Join(root, ".ai-fleet", "ai-fleet.ini"))
	if err != nil {
		return "", "", errors.New("project is not initialized — run `ai-fleet init` first")
	}
	c, err := ParseINI(string(data))
	if err != nil {
		return "", "", fmt.Errorf(".ai-fleet/ai-fleet.ini: %w", err)
	}
	return filepath.Join(root, ".ai-fleet", DockerfileName(c.Name)), ImageRepo(c.Name, c.Hash), nil
}
