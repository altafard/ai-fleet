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
	Dockerfile     string // empty → the generated .ai-fleet Dockerfile, resolved in preflight
	Project        string // any dir inside the repo; root resolved in preflight
	Branch         string // empty → feature/<run-id>, applied in preflight
	GitAuthorName  string
	GitAuthorEmail string
	Model          string // claude model alias or full ID; validity is claude's concern
	Effort         string // one of effortLevels, passed to claude --effort
	GitProvider    string // PR mode; "github" in v1
	GitRepository  string
	GitToken       string
}

// branchRe restricts branch names to a conservative subset of what git
// permits, so the name is always safe to interpolate into shell commands
// and refspecs.
var branchRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// modelRe deliberately checks shape, not validity: model names change with
// every Claude release, so ai-fleet only rejects strings that could not be a
// model reference (whitespace, shell metacharacters). Brackets are allowed
// because claude's alias syntax uses them (e.g. "opus[1m]").
var modelRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._\[\]-]*$`)

// effortLevels are the claude CLI's --effort values. ultracode is excluded:
// it is a session-only multi-agent mode, not a reasoning-effort level.
var effortLevels = map[string]bool{
	"low": true, "medium": true, "high": true, "xhigh": true, "max": true,
}

// Validate checks flag coherence: exactly one prompt source, the required
// Dockerfile, git-identity and model/effort flags, a safe branch name, and
// all-or-none PR-mode flags naming a supported provider. It reports the first
// violation found.
func (o *Options) Validate() error {
	if (o.Prompt == "") == (o.PromptFile == "") {
		return errors.New("exactly one of --prompt or --prompt-file is required")
	}
	if o.GitAuthorName == "" || o.GitAuthorEmail == "" {
		return errors.New("--git-author-name and --git-author-email are required")
	}
	if o.Model == "" || o.Effort == "" {
		return errors.New("--model and --effort are required")
	}
	if !modelRe.MatchString(o.Model) {
		return fmt.Errorf("invalid --model %q: allowed characters are A-Z a-z 0-9 . _ [ ] -", o.Model)
	}
	if !effortLevels[o.Effort] {
		return fmt.Errorf("invalid --effort %q: must be one of low, medium, high, xhigh, max", o.Effort)
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
