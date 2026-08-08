package provider

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// LoadRSAPrivateKey reads a GitHub App private key: PEM, PKCS#1 (GitHub's
// download format) or PKCS#8.
func LoadRSAPrivateKey(path string) (*rsa.PrivateKey, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, fmt.Errorf("%s: not a PEM file", path)
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	any, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%s: not an RSA private key: %w", path, err)
	}
	k, ok := any.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New(path + ": not an RSA private key")
	}
	return k, nil
}

// appJWT signs the app-authentication JWT: RS256, issuer = app ID, iat
// backdated 60s for clock skew, expiry 9 minutes (GitHub caps at 10).
// Hand-rolled with stdlib crypto — a JWT library for one fixed header and
// three claims is not worth a dependency.
func appJWT(key *rsa.PrivateKey, appID string, now time.Time) (string, error) {
	b64 := base64.RawURLEncoding.EncodeToString
	header := b64([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(map[string]any{
		"iss": appID,
		"iat": now.Add(-60 * time.Second).Unix(),
		"exp": now.Add(9 * time.Minute).Unix(),
	})
	if err != nil {
		return "", err
	}
	signing := header + "." + b64(claims)
	digest := sha256.Sum256([]byte(signing))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signing + "." + b64(sig), nil
}

// InstallationToken mints a GitHub App installation access token for repo:
// app JWT → installation lookup (unless installationID is given) → token
// exchange. Called at publish time so the token's fixed 1-hour lifetime
// cannot expire mid-run. The token and the JWT must never reach argv,
// errors, or logs.
func (g *GitHub) InstallationToken(client *http.Client, repo, appID, pemPath, installationID string) (string, error) {
	// installationID can arrive from a hand-edited config file — schema
	// validation only runs at `config set` time, not on Load/Apply — so it
	// must be checked here, before it reaches a URL, not just at the CLI edge.
	if installationID != "" && !isDigits(installationID) {
		return "", fmt.Errorf("invalid installation id %q: must be a number", installationID)
	}
	owner, name, err := g.parseRepo(repo)
	if err != nil {
		return "", err
	}
	key, err := LoadRSAPrivateKey(pemPath)
	if err != nil {
		return "", err
	}
	jwt, err := appJWT(key, appID, time.Now())
	if err != nil {
		return "", err
	}
	do := func(method, url string, body []byte) (int, []byte, error) {
		req, err := http.NewRequest(method, url, bytes.NewReader(body))
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Authorization", "Bearer "+jwt)
		req.Header.Set("Accept", "application/vnd.github+json")
		resp, err := client.Do(req)
		if err != nil {
			return 0, nil, err
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		return resp.StatusCode, b, nil
	}

	if installationID == "" {
		code, body, err := do("GET", fmt.Sprintf("%s/repos/%s/%s/installation", g.APIBase, owner, name), nil)
		if err != nil {
			return "", err
		}
		if code == http.StatusNotFound {
			return "", fmt.Errorf("github app %s is not installed on %s/%s", appID, owner, name)
		}
		if code != http.StatusOK {
			return "", fmt.Errorf("installation lookup failed: %d %s", code, body)
		}
		var inst struct {
			ID int64 `json:"id"`
		}
		if err := json.Unmarshal(body, &inst); err != nil || inst.ID == 0 {
			return "", fmt.Errorf("unexpected installation response: %s", body)
		}
		installationID = fmt.Sprint(inst.ID)
	}

	payload := []byte(`{"permissions":{"contents":"write","pull_requests":"write"}}`)
	code, body, err := do("POST", fmt.Sprintf("%s/app/installations/%s/access_tokens", g.APIBase, installationID), payload)
	if err != nil {
		return "", err
	}
	if code == http.StatusForbidden {
		return "", fmt.Errorf("installation %s lacks a required permission (contents: write, pull_requests: write): %s", installationID, body)
	}
	if code != http.StatusCreated {
		return "", fmt.Errorf("installation token exchange failed: %d %s", code, body)
	}
	var out struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(body, &out); err != nil || out.Token == "" {
		return "", errors.New("unexpected token response from github")
	}
	return out.Token, nil
}

// isDigits reports whether s is non-empty and contains only ASCII digits.
func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
