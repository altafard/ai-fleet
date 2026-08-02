// Package gitx shells out to the host git CLI. No go-git: host git is
// already a hard requirement and behaves identically to what users run.
package gitx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/altafard/ai-fleet/internal/execx"
)

// baselineQueryTimeout bounds the single ls-remote metadata query so an
// unreachable remote can never hang a run.
const baselineQueryTimeout = 10 * time.Second

func git(dir string, args ...string) (execx.Result, error) {
	return execx.Run(dir, "git", args...)
}

// RepoRoot resolves the root of the git repository containing dir,
// erroring when dir is not inside a git work tree.
func RepoRoot(dir string) (string, error) {
	r, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	if r.ExitCode != 0 {
		return "", fmt.Errorf("%s is not inside a git repository", dir)
	}
	return r.Stdout, nil
}

// Base is the resolved baseline the run starts from.
type Base struct {
	Remote string // e.g. "origin"; empty when resolved from a local branch
	Ref    string // e.g. "origin/main" or "main"
	Branch string // e.g. "main" — the PR base branch
	SHA    string
	Source string // which rule resolved it: remote-head-local | remote-query | inferred-single | inferred-named | local-branch
}

// SelectRemote picks the remote the baseline comes from: "origin" when it
// exists (git's own convention — every clone creates it), else the sole
// remote, else the one named by the checkout.defaultRemote config. Several
// remotes with no way to choose is an error, not a guess.
func SelectRemote(root string) (string, bool, error) {
	r, err := git(root, "remote")
	if err != nil {
		return "", false, err
	}
	if r.ExitCode != 0 {
		return "", false, fmt.Errorf("git remote failed: %s", r.Stderr)
	}
	names := strings.Fields(r.Stdout)
	if len(names) == 0 {
		return "", false, nil
	}
	for _, n := range names {
		if n == "origin" {
			return "origin", true, nil
		}
	}
	if len(names) == 1 {
		return names[0], true, nil
	}
	if c, err := git(root, "config", "checkout.defaultRemote"); err == nil && c.ExitCode == 0 && c.Stdout != "" {
		for _, n := range names {
			if n == c.Stdout {
				return n, true, nil
			}
		}
	}
	return "", true, fmt.Errorf("multiple remotes (%s) and none is origin; set checkout.defaultRemote or add an origin remote",
		strings.Join(names, ", "))
}

// Baseline resolves the run's starting point. With a remote, the rules run
// cheapest-first so common cases never touch the network:
//  1. the local refs/remotes/<remote>/HEAD symref (present in cloned repos);
//  2. one non-interactive, timeout-bounded ls-remote metadata query — the
//     remote's authoritative default branch, for init-ed repos with working
//     credentials (no objects are fetched; the SHA still comes from the
//     local remote-tracking ref, as of the user's last fetch);
//  3. offline inference from local remote-tracking refs: the sole branch,
//     else the one named by init.defaultBranch, then main, then master.
//
// Without a remote, the current local branch is the baseline.
func Baseline(root string) (Base, error) {
	remote, hasRemote, err := SelectRemote(root)
	if err != nil {
		return Base{}, err
	}
	if !hasRemote {
		b, err := git(root, "symbolic-ref", "--short", "HEAD")
		if err != nil {
			return Base{}, err
		}
		if b.ExitCode != 0 {
			return Base{}, errors.New("HEAD is detached and there is no remote; check out a branch first")
		}
		s, err := git(root, "rev-parse", "HEAD")
		if err != nil || s.ExitCode != 0 {
			return Base{}, errors.New("repository has no commits")
		}
		return Base{Ref: b.Stdout, Branch: b.Stdout, SHA: s.Stdout, Source: "local-branch"}, nil
	}

	trackingSHA := func(branch string) (string, bool) {
		s, err := git(root, "rev-parse", "refs/remotes/"+remote+"/"+branch)
		return s.Stdout, err == nil && s.ExitCode == 0
	}

	// Rule 1: local <remote>/HEAD symref — cloned repos, zero network.
	if h, err := git(root, "symbolic-ref", "refs/remotes/"+remote+"/HEAD"); err == nil && h.ExitCode == 0 {
		if _, branch, ok := ParseRemoteHead(h.Stdout, remote); ok {
			if sha, ok := trackingSHA(branch); ok {
				return Base{Remote: remote, Ref: remote + "/" + branch, Branch: branch, SHA: sha,
					Source: "remote-head-local"}, nil
			}
		}
	}

	// Rule 2: ask the remote — authoritative, but strictly optional.
	if branch, ok := queryRemoteHead(root, remote); ok {
		sha, found := trackingSHA(branch)
		if !found {
			return Base{}, fmt.Errorf("remote %s default branch is %q but it has not been fetched; run: git fetch %s",
				remote, branch, remote)
		}
		return Base{Remote: remote, Ref: remote + "/" + branch, Branch: branch, SHA: sha, Source: "remote-query"}, nil
	}

	// Rule 3: offline inference from what was fetched previously.
	if branch, source, ok := inferDefaultBranch(root, remote); ok {
		if sha, found := trackingSHA(branch); found {
			return Base{Remote: remote, Ref: remote + "/" + branch, Branch: branch, SHA: sha, Source: source}, nil
		}
	}

	return Base{}, fmt.Errorf("cannot determine the default branch of remote %s; run: git fetch %s (or: git remote set-head %s -a)",
		remote, remote, remote)
}

// ParseRemoteHead maps "refs/remotes/<remote>/<branch>" to ("<remote>/<branch>", branch).
func ParseRemoteHead(sym, remote string) (string, string, bool) {
	p := "refs/remotes/" + remote + "/"
	if !strings.HasPrefix(sym, p) {
		return "", "", false
	}
	branch := strings.TrimPrefix(sym, p)
	return remote + "/" + branch, branch, branch != ""
}

// queryRemoteHead runs the single metadata query. It must never prompt or
// hang: terminal prompts and askpass are disabled, SSH runs in BatchMode
// (unless the user configured their own transport), and the whole call is
// timeout-bounded. Every failure — offline, no credentials, timeout — is
// expected and reported as !ok, never as a fatal error.
func queryRemoteHead(root, remote string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), baselineQueryTimeout)
	defer cancel()
	env := []string{"GIT_TERMINAL_PROMPT=0", "GIT_ASKPASS=true", "SSH_ASKPASS=true"}
	if os.Getenv("GIT_SSH_COMMAND") == "" && os.Getenv("GIT_SSH") == "" {
		env = append(env, "GIT_SSH_COMMAND=ssh -oBatchMode=yes")
	}
	// -c credential.helper= disables configured helpers (a GUI helper could
	// otherwise prompt and hang despite GIT_TERMINAL_PROMPT=0).
	r, err := execx.RunCtx(ctx, root, env, "git", "-c", "credential.helper=", "ls-remote", "--symref", remote, "HEAD")
	if err != nil || r.ExitCode != 0 {
		return "", false
	}
	for _, line := range strings.Split(r.Stdout, "\n") {
		if strings.HasPrefix(line, "ref: refs/heads/") {
			f := strings.Fields(line) // ["ref:", "refs/heads/<branch>", "HEAD"]
			if len(f) >= 2 {
				branch := strings.TrimPrefix(f[1], "refs/heads/")
				return branch, branch != ""
			}
		}
	}
	return "", false
}

// inferDefaultBranch guesses the default branch from the local
// remote-tracking refs when the remote cannot be asked: unambiguous when
// only one branch exists, otherwise the conventional names win.
func inferDefaultBranch(root, remote string) (string, string, bool) {
	r, err := git(root, "for-each-ref", "--format=%(refname:strip=3)", "refs/remotes/"+remote)
	if err != nil || r.ExitCode != 0 || r.Stdout == "" {
		return "", "", false
	}
	var branches []string
	for _, b := range strings.Split(r.Stdout, "\n") {
		if b != "" && b != "HEAD" {
			branches = append(branches, b)
		}
	}
	if len(branches) == 1 {
		return branches[0], "inferred-single", true
	}
	var candidates []string
	if c, err := git(root, "config", "init.defaultBranch"); err == nil && c.ExitCode == 0 && c.Stdout != "" {
		candidates = append(candidates, c.Stdout)
	}
	candidates = append(candidates, "main", "master")
	for _, cand := range candidates {
		for _, b := range branches {
			if b == cand {
				return b, "inferred-named", true
			}
		}
	}
	return "", "", false
}

// CloneNoCheckout makes the baseline template: full local clone without a
// working tree, HEAD detached at sha.
func CloneNoCheckout(root, dest, sha string) error {
	if r, err := git("", "clone", "--quiet", "--no-checkout", root, dest); err != nil || r.ExitCode != 0 {
		return fmt.Errorf("clone failed: %s", r.Stderr)
	}
	if r, err := git(dest, "update-ref", "--no-deref", "HEAD", sha); err != nil || r.ExitCode != 0 {
		return fmt.Errorf("cannot set baseline HEAD: %s", r.Stderr)
	}
	return nil
}

// VerifyBundle checks that the bundle file can be applied to the
// repository at dir — that is, all of its prerequisite commits exist there.
func VerifyBundle(dir, bundle string) error {
	if r, err := git(dir, "bundle", "verify", bundle); err != nil || r.ExitCode != 0 {
		return fmt.Errorf("bundle verify failed: %s", r.Stderr)
	}
	return nil
}

// FetchBundle imports branch from the bundle file into the repository at
// dir as a local branch of the same name.
func FetchBundle(dir, bundle, branch string) error {
	if r, err := git(dir, "fetch", bundle, branch+":"+branch); err != nil || r.ExitCode != 0 {
		return fmt.Errorf("fetch from bundle failed: %s", r.Stderr)
	}
	return nil
}

// CountCommits returns the number of commits reachable from to but not
// from from (git rev-list --count from..to) in the repository at dir.
func CountCommits(dir, from, to string) (int, error) {
	r, err := git(dir, "rev-list", "--count", from+".."+to)
	if err != nil || r.ExitCode != 0 {
		return 0, fmt.Errorf("rev-list failed: %s", r.Stderr)
	}
	return strconv.Atoi(r.Stdout)
}

// Push pushes branch to url, authenticating via an inline credential helper
// that reads the token from the child's environment — the token never
// appears in process argv (visible in `ps`) or in the URL. Errors are still
// redacted defensively.
func Push(dir, url, branch, token string) error {
	const helper = `credential.helper=!f() { echo "username=x-access-token"; echo "password=${AI_FLEET_PUSH_TOKEN}"; }; f`
	env := []string{"AI_FLEET_PUSH_TOKEN=" + token, "GIT_TERMINAL_PROMPT=0"}
	r, err := execx.RunCtx(context.Background(), dir, env,
		"git", "-c", "credential.helper=", "-c", helper, "push", url, branch+":"+branch)
	if err != nil || r.ExitCode != 0 {
		msg := r.Stderr
		if msg == "" && err != nil {
			msg = err.Error()
		}
		return errors.New(Redact("push failed: "+msg, token))
	}
	return nil
}

// Redact replaces every occurrence of secret in s with "****". An empty
// secret leaves s unchanged.
func Redact(s, secret string) string {
	if secret == "" {
		return s
	}
	return strings.ReplaceAll(s, secret, "****")
}
