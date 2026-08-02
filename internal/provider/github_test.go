package provider

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNew(t *testing.T) {
	if _, err := New("github"); err != nil {
		t.Fatal(err)
	}
	if _, err := New("svn"); err == nil {
		t.Fatal("want error for unknown provider")
	}
}

func TestGitHubPushURL(t *testing.T) {
	gh := &GitHub{}
	cases := []struct{ repo, want string }{
		{"owner/repo", "https://x-access-token:tok@github.com/owner/repo.git"},
		{"https://github.com/owner/repo", "https://x-access-token:tok@github.com/owner/repo.git"},
		{"https://github.com/owner/repo.git", "https://x-access-token:tok@github.com/owner/repo.git"},
	}
	for _, c := range cases {
		got, err := gh.PushURL(c.repo, "tok")
		if err != nil || got != c.want {
			t.Errorf("%q: got %q err %v", c.repo, got, err)
		}
	}
	if _, err := gh.PushURL("not a repo", "tok"); err == nil {
		t.Fatal("want error for unparseable repo")
	}
}

func TestGitHubCreatePR(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(map[string]string{"html_url": "https://github.com/owner/repo/pull/7"})
	}))
	defer srv.Close()

	gh := &GitHub{APIBase: srv.URL}
	url, err := gh.CreatePR(srv.Client(), "owner/repo", "tok",
		PR{Title: "feat: x", Body: "## Summary\nb", Head: "feature/x", Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://github.com/owner/repo/pull/7" {
		t.Fatalf("url=%q", url)
	}
	if gotPath != "/repos/owner/repo/pulls" || gotAuth != "Bearer tok" {
		t.Fatalf("path=%q auth=%q", gotPath, gotAuth)
	}
	if gotBody["title"] != "feat: x" || gotBody["head"] != "feature/x" || gotBody["base"] != "main" {
		t.Fatalf("body=%v", gotBody)
	}
}

func TestGitHubCreatePRAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		w.Write([]byte(`{"message":"Validation Failed"}`))
	}))
	defer srv.Close()
	gh := &GitHub{APIBase: srv.URL}
	_, err := gh.CreatePR(srv.Client(), "owner/repo", "tok", PR{Title: "t", Body: "b", Head: "h", Base: "m"})
	if err == nil || !strings.Contains(err.Error(), "422") {
		t.Fatalf("err=%v", err)
	}
}
