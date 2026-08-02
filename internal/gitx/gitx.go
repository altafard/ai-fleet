// Package gitx shells out to the host git CLI. No go-git: host git is
// already a hard requirement and behaves identically to what users run.
package gitx

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/altafard/ai-fleet/internal/execx"
)

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
	Ref    string // e.g. "origin/main" or "main"
	Branch string // e.g. "main" — the PR base branch
	SHA    string
}

func Baseline(root string) (Base, error) {
	if r, err := git(root, "remote", "get-url", "origin"); err == nil && r.ExitCode == 0 {
		h, err := git(root, "symbolic-ref", "refs/remotes/origin/HEAD")
		if err != nil {
			return Base{}, err
		}
		if h.ExitCode != 0 {
			return Base{}, errors.New("origin has no default branch; run: git remote set-head origin -a")
		}
		ref, branch, ok := ParseOriginHead(h.Stdout)
		if !ok {
			return Base{}, fmt.Errorf("cannot parse origin HEAD %q", h.Stdout)
		}
		s, err := git(root, "rev-parse", ref)
		if err != nil || s.ExitCode != 0 {
			return Base{}, fmt.Errorf("cannot resolve %s: %s", ref, s.Stderr)
		}
		return Base{Ref: ref, Branch: branch, SHA: s.Stdout}, nil
	}
	b, err := git(root, "symbolic-ref", "--short", "HEAD")
	if err != nil {
		return Base{}, err
	}
	if b.ExitCode != 0 {
		return Base{}, errors.New("HEAD is detached and there is no origin remote; check out a branch first")
	}
	s, err := git(root, "rev-parse", "HEAD")
	if err != nil || s.ExitCode != 0 {
		return Base{}, errors.New("repository has no commits")
	}
	return Base{Ref: b.Stdout, Branch: b.Stdout, SHA: s.Stdout}, nil
}

// ParseOriginHead maps "refs/remotes/origin/<branch>" to ("origin/<branch>", branch).
func ParseOriginHead(sym string) (string, string, bool) {
	const p = "refs/remotes/origin/"
	if !strings.HasPrefix(sym, p) {
		return "", "", false
	}
	branch := strings.TrimPrefix(sym, p)
	return "origin/" + branch, branch, branch != ""
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

// Push pushes branch to url. url may embed a token — the caller passes the
// token so errors can be redacted here, the only place that sees git output.
func Push(dir, url, branch, token string) error {
	if r, err := git(dir, "push", url, branch+":"+branch); err != nil || r.ExitCode != 0 {
		return errors.New(Redact("push failed: "+r.Stderr, token))
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
