// Package run implements the deploy-unit run: preflight checks, baseline
// resolution, image build, workspace snapshot, container execution, and
// collection of the resulting git bundle (plus optional pull-request
// publication).
package run

import (
	"errors"
	"fmt"
	"regexp"
)

// Options are the fully-resolved inputs of one `deploy unit` invocation.
type Options struct {
	Prompt         string
	PromptFile     string
	Dockerfile     string
	Project        string // any dir inside the repo; root resolved in preflight
	Branch         string // empty → feature/<run-id>, applied in preflight
	GitAuthorName  string
	GitAuthorEmail string
	GitProvider    string // PR mode; "github" in v1
	GitRepository  string
	GitToken       string
}

// branchRe restricts branch names to a conservative subset of what git
// permits, so the name is always safe to interpolate into shell commands
// and refspecs.
var branchRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// Validate checks flag coherence: exactly one prompt source, the required
// Dockerfile and git-identity flags, a safe branch name, and all-or-none
// PR-mode flags naming a supported provider. It reports the first
// violation found.
func (o *Options) Validate() error {
	if (o.Prompt == "") == (o.PromptFile == "") {
		return errors.New("exactly one of --prompt or --prompt-file is required")
	}
	if o.Dockerfile == "" {
		return errors.New("--dockerfile is required")
	}
	if o.GitAuthorName == "" || o.GitAuthorEmail == "" {
		return errors.New("--git-author-name and --git-author-email are required")
	}
	if o.Branch != "" && !branchRe.MatchString(o.Branch) {
		return fmt.Errorf("invalid --branch %q: allowed characters are A-Z a-z 0-9 . _ / -", o.Branch)
	}
	n := 0
	for _, v := range []string{o.GitProvider, o.GitRepository, o.GitToken} {
		if v != "" {
			n++
		}
	}
	if n != 0 && n != 3 {
		return errors.New("PR mode needs all of --git-provider, --git-repository and --git-token (or AI_FLEET_GIT_TOKEN)")
	}
	if n == 3 && o.GitProvider != "github" {
		return fmt.Errorf("unsupported --git-provider %q: v1 supports \"github\"", o.GitProvider)
	}
	return nil
}

// PRMode reports whether all three PR-mode inputs (provider, repository,
// token) are present, which makes a successful run publish a pull request
// after collect.
func (o *Options) PRMode() bool {
	return o.GitProvider != "" && o.GitRepository != "" && o.GitToken != ""
}
