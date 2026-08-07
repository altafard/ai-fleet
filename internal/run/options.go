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

	GitEntityType        string // "user" (default when empty) | "bot"
	GitAppID             string // bot: GitHub App ID (or Client ID)
	GitAppPrivateKey     string // bot: path to the RSA PEM (already scope-resolved)
	GitAppInstallationID string // bot: optional; discovered from the repository when empty
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
// Dockerfile, git-identity and model/effort flags, a safe branch name, and —
// only once provider+repository opt into PR mode — a supported provider and
// complete credentials (user token, or bot git.app.* fields). Credentials
// present without provider+repository are ignored, not an error, so a
// config-stored token never breaks a non-PR run. It reports the first
// violation found.
func (o *Options) Validate() error {
	if (o.Prompt == "") == (o.PromptFile == "") {
		return errors.New("exactly one of --prompt or --prompt-file is required")
	}
	if o.GitAuthorName == "" || o.GitAuthorEmail == "" {
		return errors.New("--git-author-name and --git-author-email are required — pass the flags or set git.author.name / git.author.email via `ai-fleet config set`")
	}
	if o.Model == "" || o.Effort == "" {
		return errors.New("--model and --effort are required — pass the flags or set agent.model / agent.effort via `ai-fleet config set`")
	}
	if !ValidModel(o.Model) {
		return fmt.Errorf("invalid --model %q: allowed characters are A-Z a-z 0-9 . _ [ ] -", o.Model)
	}
	if !ValidEffort(o.Effort) {
		return fmt.Errorf("invalid --effort %q: must be one of low, medium, high, xhigh, max", o.Effort)
	}
	if o.Branch != "" && !branchRe.MatchString(o.Branch) {
		return fmt.Errorf("invalid --branch %q: allowed characters are A-Z a-z 0-9 . _ / -", o.Branch)
	}
	// PR mode is opted into by provider+repository. Everything below —
	// git.type validity, the bot/user credential-shape contradictions, and
	// credential completeness — is credential detail that only matters once
	// PR mode is opted into: these fields arrive from persistent config, and
	// a stored token or bot identity must not break non-PR runs.
	if (o.GitProvider == "") != (o.GitRepository == "") {
		return errors.New("PR mode needs both --git-provider and --git-repository")
	}
	if o.PRMode() {
		if o.GitProvider != "github" {
			return fmt.Errorf("unsupported --git-provider %q: v1 supports \"github\"", o.GitProvider)
		}
		if o.GitEntityType != "" && o.GitEntityType != "user" && o.GitEntityType != "bot" {
			return fmt.Errorf("invalid git.type %q: must be \"user\" or \"bot\"", o.GitEntityType)
		}
		if o.BotMode() && o.GitToken != "" {
			return errors.New("git.type is \"bot\": git.token must not be set (bot auth uses git.app.id and git.app.private-key)")
		}
		if !o.BotMode() && (o.GitAppID != "" || o.GitAppPrivateKey != "" || o.GitAppInstallationID != "") {
			return errors.New("git.app.* settings require git.type = \"bot\"")
		}
		if o.BotMode() {
			if o.GitAppID == "" || o.GitAppPrivateKey == "" {
				return errors.New("PR mode with git.type = \"bot\" needs git.app.id and git.app.private-key")
			}
		} else if o.GitToken == "" {
			return errors.New("PR mode needs a token: --git-token, AI_FLEET_GIT_TOKEN, or `ai-fleet config set git.token`")
		}
	}
	return nil
}

// PRMode reports whether this run publishes a pull request. Credential
// completeness is Validate's job; presence of provider+repository is the
// opt-in.
func (o *Options) PRMode() bool {
	return o.GitProvider != "" && o.GitRepository != ""
}

// BotMode reports whether PR credentials are a GitHub App, not a user token.
func (o *Options) BotMode() bool { return o.GitEntityType == "bot" }

// ValidModel reports whether s has the shape of a claude model reference.
// Shape only — model validity is claude's concern (see modelRe).
func ValidModel(s string) bool { return modelRe.MatchString(s) }

// ValidEffort reports whether s is one of the claude CLI's effort levels.
func ValidEffort(s string) bool { return effortLevels[s] }
