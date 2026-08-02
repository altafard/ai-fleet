package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
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

func (g *GitHub) PushURL(repo, token string) (string, error) {
	owner, name, err := g.parseRepo(repo)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("https://x-access-token:%s@github.com/%s/%s.git", token, owner, name), nil
}

// CreatePR opens a pull request on repo and returns its HTML URL. A
// non-201 response becomes an error carrying the HTTP status code and the
// (bounded) response body; the token appears only in the Authorization
// header, never in errors.
func (g *GitHub) CreatePR(client *http.Client, repo, token string, pr PR) (string, error) {
	owner, name, err := g.parseRepo(repo)
	if err != nil {
		return "", err
	}
	payload, _ := json.Marshal(map[string]string{
		"title": pr.Title, "body": pr.Body, "head": pr.Head, "base": pr.Base,
	})
	req, err := http.NewRequest("POST",
		fmt.Sprintf("%s/repos/%s/%s/pulls", g.APIBase, owner, name), bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("github PR creation failed: %d %s", resp.StatusCode, body)
	}
	var out struct {
		HTMLURL string `json:"html_url"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.HTMLURL == "" {
		return "", fmt.Errorf("unexpected github response: %s", body)
	}
	return out.HTMLURL, nil
}
