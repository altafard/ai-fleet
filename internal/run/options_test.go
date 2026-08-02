package run

import "testing"

func opts() Options {
	return Options{
		Prompt: "do things", Dockerfile: "Dockerfile",
		GitAuthorName: "Bot", GitAuthorEmail: "bot@example.com",
	}
}

func TestValidateOK(t *testing.T) {
	o := opts()
	if err := o.Validate(); err != nil {
		t.Fatalf("want ok, got %v", err)
	}
}

func TestValidateRules(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Options)
	}{
		{"no prompt", func(o *Options) { o.Prompt = "" }},
		{"both prompts", func(o *Options) { o.PromptFile = "p.md" }},
		{"no dockerfile", func(o *Options) { o.Dockerfile = "" }},
		{"no author name", func(o *Options) { o.GitAuthorName = "" }},
		{"no author email", func(o *Options) { o.GitAuthorEmail = "" }},
		{"bad branch", func(o *Options) { o.Branch = "feat ure" }},
		{"partial PR flags", func(o *Options) { o.GitProvider = "github" }},
		{"unknown provider", func(o *Options) {
			o.GitProvider, o.GitRepository, o.GitToken = "svn", "o/r", "t"
		}},
	}
	for _, c := range cases {
		o := opts()
		c.mut(&o)
		if err := o.Validate(); err == nil {
			t.Errorf("%s: want error, got nil", c.name)
		}
	}
}

func TestPRMode(t *testing.T) {
	o := opts()
	if o.PRMode() {
		t.Fatal("want false without PR flags")
	}
	o.GitProvider, o.GitRepository, o.GitToken = "github", "owner/repo", "tok"
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	if !o.PRMode() {
		t.Fatal("want true with all PR flags")
	}
}
