package run

import "testing"

func opts() Options {
	return Options{
		Prompt: "do things", Dockerfile: "Dockerfile",
		GitAuthorName: "Bot", GitAuthorEmail: "bot@example.com",
		Model: "claude-opus-5", Effort: "high",
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
		{"no author name", func(o *Options) { o.GitAuthorName = "" }},
		{"no author email", func(o *Options) { o.GitAuthorEmail = "" }},
		{"bad branch", func(o *Options) { o.Branch = "feat ure" }},
		{"partial PR flags", func(o *Options) { o.GitProvider = "github" }},
		{"unknown provider", func(o *Options) {
			o.GitProvider, o.GitRepository, o.GitToken = "svn", "o/r", "t"
		}},
		{"no model", func(o *Options) { o.Model = "" }},
		{"no effort", func(o *Options) { o.Effort = "" }},
		{"unknown effort", func(o *Options) { o.Effort = "hihg" }},
		{"effort not a flag value", func(o *Options) { o.Effort = "ultracode" }},
		{"model with space", func(o *Options) { o.Model = "opus 5" }},
		{"model with shell metachar", func(o *Options) { o.Model = "opus;rm" }},
		{"model with dollar", func(o *Options) { o.Model = "$MODEL" }},
		{"model leading dash", func(o *Options) { o.Model = "-opus" }},
	}
	for _, c := range cases {
		o := opts()
		c.mut(&o)
		if err := o.Validate(); err == nil {
			t.Errorf("%s: want error, got nil", c.name)
		}
	}
}

func TestValidateDockerfileOptional(t *testing.T) {
	o := Options{Prompt: "do it", GitAuthorName: "a", GitAuthorEmail: "a@b",
		Model: "opus", Effort: "high"}
	if err := o.Validate(); err != nil {
		t.Errorf("Validate with empty Dockerfile = %v, want nil", err)
	}
}

func TestValidateModelAndEffortAccepted(t *testing.T) {
	models := []string{"opus", "claude-opus-5", "opus[1m]", "claude-3.5_test"}
	efforts := []string{"low", "medium", "high", "xhigh", "max"}
	for _, m := range models {
		for _, e := range efforts {
			o := opts()
			o.Model, o.Effort = m, e
			if err := o.Validate(); err != nil {
				t.Errorf("model %q effort %q: want ok, got %v", m, e, err)
			}
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
