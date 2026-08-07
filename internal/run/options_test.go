package run

import (
	"strings"
	"testing"
)

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

func TestExportedValidators(t *testing.T) {
	cases := []struct {
		model, effort string
		wantM, wantE  bool
	}{
		{"opus", "high", true, true},
		{"opus[1m]", "max", true, true},
		{"claude-opus-5", "low", true, true},
		{"bad model", "medium", false, true},
		{"-leading", "xhigh", false, true},
		{"opus", "ultra", true, false},
		{"opus", "", true, false},
	}
	for _, c := range cases {
		if got := ValidModel(c.model); got != c.wantM {
			t.Errorf("ValidModel(%q) = %v, want %v", c.model, got, c.wantM)
		}
		if got := ValidEffort(c.effort); got != c.wantE {
			t.Errorf("ValidEffort(%q) = %v, want %v", c.effort, got, c.wantE)
		}
	}
}

func TestValidateBotCredentials(t *testing.T) {
	base := Options{Prompt: "p", GitAuthorName: "a", GitAuthorEmail: "e",
		Model: "opus", Effort: "high", GitProvider: "github", GitRepository: "o/r"}

	bot := base
	bot.GitEntityType, bot.GitAppID, bot.GitAppPrivateKey = "bot", "123", "/k.pem"
	if err := bot.Validate(); err != nil {
		t.Errorf("bot with app credentials: %v", err)
	}
	if !bot.PRMode() || !bot.BotMode() {
		t.Error("PRMode/BotMode must be true")
	}

	cases := []struct {
		name   string
		mutate func(*Options)
		want   string
	}{
		{"bot missing app creds", func(o *Options) { o.GitEntityType = "bot" }, "git.app.id and git.app.private-key"},
		{"bot with token", func(o *Options) {
			o.GitEntityType, o.GitAppID, o.GitAppPrivateKey, o.GitToken = "bot", "123", "/k.pem", "tok"
		}, "git.token must not be set"},
		{"user with app creds", func(o *Options) { o.GitAppID = "123" }, "require git.type"},
		{"bad type", func(o *Options) { o.GitEntityType = "robot" }, "git.type"},
		{"user missing token", func(o *Options) {}, "needs a token"},
		{"provider without repository", func(o *Options) { o.GitRepository = ""; o.GitToken = "tok" }, "both"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			o := base
			c.mutate(&o)
			err := o.Validate()
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("err = %v, want containing %q", err, c.want)
			}
		})
	}
}

func TestValidateCredentialsWithoutPRModeAreIgnored(t *testing.T) {
	// A config-stored token (or bot identity) must not break non-PR runs.
	o := Options{Prompt: "p", GitAuthorName: "a", GitAuthorEmail: "e",
		Model: "opus", Effort: "high", GitToken: "tok",
		GitEntityType: "bot", GitAppID: "1", GitAppPrivateKey: "/k.pem"}
	if err := o.Validate(); err != nil {
		t.Fatalf("credentials without provider/repository must be ignored: %v", err)
	}
	if o.PRMode() {
		t.Fatal("PRMode must require provider and repository")
	}
}
