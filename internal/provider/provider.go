// Package provider abstracts PR-capable git hosting APIs. v1 implements
// GitHub; GitLab/Gitea implement the same interface later.
package provider

import (
	"fmt"
	"net/http"
)

// PR describes the pull request to open: its title and body, the head
// (working) branch, and the base (baseline) branch.
type PR struct {
	Title string
	Body  string
	Head  string // working branch
	Base  string // baseline branch
}

// Provider adapts one git-hosting API: a push URL for a repository and
// pull-request creation.
type Provider interface {
	// PushURL returns the token-authenticated push URL for the repository.
	PushURL(repo, token string) (string, error)
	// CreatePR opens the pull request and returns its URL.
	CreatePR(client *http.Client, repo, token string, pr PR) (string, error)
}

// New returns the Provider named by name. Only "github" is supported;
// anything else is an error.
func New(name string) (Provider, error) {
	switch name {
	case "github":
		return &GitHub{APIBase: "https://api.github.com"}, nil
	}
	return nil, fmt.Errorf("unsupported git provider %q", name)
}
