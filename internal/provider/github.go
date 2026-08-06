package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// GitHub implements [Provider] against the GitHub REST API.
type GitHub struct {
	APIBase string // https://api.github.com; overridable in tests
}

var ghRepoRe = regexp.MustCompile(`^(?:https://github\.com/)?([\w.-]+)/([\w.-]+?)(?:\.git)?$`)

func (g *GitHub) parseRepo(repo string) (string, string, error) {
	m := ghRepoRe.FindStringSubmatch(repo)
	if m == nil {
		return "", "", fmt.Errorf("cannot parse github repository %q (want owner/name or URL)", repo)
	}
	return m[1], m[2], nil
}

// PushURL returns a credential-free push URL. The token is deliberately NOT
// embedded: it would end up in `git push` argv, visible to any local user
// via ps. Authentication happens in gitx.Push through an environment-backed
// credential helper instead.
func (g *GitHub) PushURL(repo, token string) (string, error) {
	owner, name, err := g.parseRepo(repo)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://github.com/%s/%s.git", owner, name), nil
}

// CreatePR opens a pull request on repo and returns its HTML URL. A 422
// reporting that a PR for the head already exists adopts that PR (its URL,
// existed=true). Any other non-201 response becomes an error carrying the
// HTTP status code and the (bounded) response body; the token appears only
// in the Authorization header, never in errors.
func (g *GitHub) CreatePR(client *http.Client, repo, token string, pr PR) (string, bool, error) {
	owner, name, err := g.parseRepo(repo)
	if err != nil {
		return "", false, err
	}
	payload, _ := json.Marshal(map[string]string{
		"title": pr.Title, "body": pr.Body, "head": pr.Head, "base": pr.Base,
	})
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/repos/%s/%s/pulls", g.APIBase, owner, name), bytes.NewReader(payload))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", false, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusCreated {
		// "A pull request already exists for <head>" is a 422, but not a
		// failure: the push already updated the branch, so the run's work is
		// published — adopt the existing PR instead of discarding the run.
		if resp.StatusCode == http.StatusUnprocessableEntity && prAlreadyExists(body) {
			url, lerr := g.findOpenPR(client, owner, name, token, pr.Head)
			if lerr != nil {
				return "", false, fmt.Errorf("github says a PR already exists for %s but it could not be adopted: %w", pr.Head, lerr)
			}
			return url, true, nil
		}
		return "", false, fmt.Errorf("github PR creation failed: %d %s", resp.StatusCode, body)
	}
	var out struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.HTMLURL == "" {
		return "", false, fmt.Errorf("unexpected github response: %s", body)
	}
	return out.HTMLURL, false, nil
}

// prAlreadyExists reports whether a 422 body carries GitHub's
// "A pull request already exists" validation error. Any other 422
// (missing base branch, bad payload) must stay a failure.
func prAlreadyExists(body []byte) bool {
	var v struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if json.Unmarshal(body, &v) != nil {
		return false
	}
	for _, e := range v.Errors {
		if strings.Contains(e.Message, "pull request already exists") {
			return true
		}
	}
	return false
}

// findOpenPR returns the HTML URL of the open pull request whose head is
// owner:head. Head is owner-qualified: ai-fleet always pushes to the same
// repository the PR targets.
func (g *GitHub) findOpenPR(client *http.Client, owner, name, token, head string) (string, error) {
	q := url.Values{"head": {owner + ":" + head}, "state": {"open"}}
	req, err := http.NewRequest("GET",
		fmt.Sprintf("%s/repos/%s/%s/pulls?%s", g.APIBase, owner, name, q.Encode()), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github PR lookup failed: %d %s", resp.StatusCode, body)
	}
	var prs []struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &prs); err != nil {
		return "", fmt.Errorf("unexpected github response: %s", body)
	}
	if len(prs) == 0 || prs[0].HTMLURL == "" {
		return "", fmt.Errorf("no open pull request found for head %s:%s", owner, head)
	}
	return prs[0].HTMLURL, nil
}
