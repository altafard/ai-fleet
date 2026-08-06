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

// WriteFiles creates .ai-fleet/ under root (tolerating a pre-existing
// folder — deploy runs create .ai-fleet/runs/ on their own) and writes the
// three generated files. Returns the Dockerfile's path.
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
	return dfPath, nil
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
