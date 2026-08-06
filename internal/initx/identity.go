// Package initx implements `ai-fleet init`: toolchain checks, project
// identity, Claude-driven inventory analysis, generated files, and the
// prebuilt project image.
package initx

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"regexp"
	"strings"
)

// invalidComponent matches every character not allowed in a Docker
// repository name component; separatorRun matches separator sequences the
// reference grammar rejects. Docker only accepts ".", "_", "__" or a run of
// "-" between alphanumeric runs, so anything longer than one character
// collapses to a single "-", which is always valid.
var (
	invalidComponent = regexp.MustCompile(`[^a-z0-9._-]`)
	separatorRun     = regexp.MustCompile(`[._-]{2,}`)
)

// ProjectName derives the project name from the repo-root path: the
// directory basename, lowercased and sanitized to a valid Docker repository
// component. A basename with no valid characters falls back to "project".
func ProjectName(root string) string {
	name := strings.ToLower(filepath.Base(root))
	name = invalidComponent.ReplaceAllString(name, "-")
	name = strings.Trim(name, "-._")
	name = separatorRun.ReplaceAllString(name, "-")
	if name == "" {
		return "project"
	}
	return name
}

// ProjectHash disambiguates same-named projects on one machine: the first
// 8 hex chars of SHA-256 of the absolute repo-root path.
func ProjectHash(root string) string {
	sum := sha256.Sum256([]byte(root))
	return hex.EncodeToString(sum[:])[:8]
}

// ImageRepo is the Docker repository holding a project's images.
func ImageRepo(name, hash string) string {
	return "ai-fleet/" + name + "-" + hash
}
