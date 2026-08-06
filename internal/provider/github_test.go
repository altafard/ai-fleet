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
		{"owner/repo", "https://github.com/owner/repo.git"},
		{"https://github.com/owner/repo", "https://github.com/owner/repo.git"},
		{"https://github.com/owner/repo.git", "https://github.com/owner/repo.git"},
	}
	for _, c := range cases {
		got, err := gh.PushURL(c.repo, "tok")
		if err != nil || got != c.want {
			t.Errorf("%q: got %q err %v", c.repo, got, err)
		}
		if strings.Contains(got, "tok") {
			t.Errorf("%q: token must never be embedded in the push URL: %q", c.repo, got)
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
	url, existed, err := gh.CreatePR(srv.Client(), "owner/repo", "tok",
		PR{Title: "feat: x", Body: "## Summary\nb", Head: "feature/x", Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://github.com/owner/repo/pull/7" || existed {
		t.Fatalf("url=%q existed=%v", url, existed)
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
	_, _, err := gh.CreatePR(srv.Client(), "owner/repo", "tok", PR{Title: "t", Body: "b", Head: "h", Base: "m"})
	if err == nil || !strings.Contains(err.Error(), "422") {
		t.Fatalf("err=%v", err)
	}
}

const alreadyExistsBody = `{"message":"Validation Failed","errors":[` +
	`{"resource":"PullRequest","code":"custom","message":"A pull request already exists for owner:feature/x."}]}`

// A 422 whose body says the PR already exists is not a failure: the run's
// work is published, so the existing PR is looked up and adopted.
func TestGitHubCreatePRAdoptsExisting(t *testing.T) {
	var gotListQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.WriteHeader(422)
			w.Write([]byte(alreadyExistsBody))
			return
		}
		gotListQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode([]map[string]string{{"html_url": "https://github.com/owner/repo/pull/3"}})
	}))
	defer srv.Close()

	gh := &GitHub{APIBase: srv.URL}
	url, existed, err := gh.CreatePR(srv.Client(), "owner/repo", "tok",
		PR{Title: "t", Body: "b", Head: "feature/x", Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if url != "https://github.com/owner/repo/pull/3" || !existed {
		t.Fatalf("url=%q existed=%v", url, existed)
	}
	if !strings.Contains(gotListQuery, "head=owner%3Afeature%2Fx") || !strings.Contains(gotListQuery, "state=open") {
		t.Fatalf("lookup query=%q", gotListQuery)
	}
}

// GitHub claiming "already exists" while the lookup finds nothing must stay
// an error — success without a URL would hide a real inconsistency.
func TestGitHubCreatePRAdoptLookupFindsNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.WriteHeader(422)
			w.Write([]byte(alreadyExistsBody))
			return
		}
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()

	gh := &GitHub{APIBase: srv.URL}
	_, _, err := gh.CreatePR(srv.Client(), "owner/repo", "tok",
		PR{Title: "t", Body: "b", Head: "feature/x", Base: "main"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err=%v", err)
	}
}
