package config

import (
	"strings"
	"testing"
)

func TestValidateKey(t *testing.T) {
	for _, k := range Keys() {
		if err := ValidateKey(k); err != nil {
			t.Errorf("ValidateKey(%q) = %v, want nil", k, err)
		}
	}
	for _, k := range []string{"git.tokn", "project.name", "agent", "", "git.app"} {
		if err := ValidateKey(k); err == nil {
			t.Errorf("ValidateKey(%q) = nil, want error", k)
		}
	}
}

func TestValidateValue(t *testing.T) {
	cases := []struct {
		key, value string
		ok         bool
	}{
		{"agent.model", "opus[1m]", true},
		{"agent.model", "bad model", false},
		{"agent.effort", "high", true},
		{"agent.effort", "ultra", false},
		{"git.provider", "github", true},
		{"git.provider", "gitlab", false},
		{"git.repository", "owner/repo", true},
		{"git.repository", "", false},
		{"git.type", "user", true},
		{"git.type", "bot", true},
		{"git.type", "robot", false},
		{"git.author.name", "Bot", true},
		{"git.author.email", "b@e.c", true},
		{"git.author.email", "", false},
		{"git.token", "ghp_x", true},
		{"git.app.id", "123456", true},
		{"git.app.id", "has space", false},
		{"git.app.private-key", ".ai-fleet/bot.pem", true},
		{"git.app.installation-id", "42", true},
		{"git.app.installation-id", "abc", false},
	}
	for _, c := range cases {
		err := ValidateValue(c.key, c.value)
		if (err == nil) != c.ok {
			t.Errorf("ValidateValue(%q, %q) = %v, want ok=%v", c.key, c.value, err, c.ok)
		}
	}
	if err := ValidateValue("git.tokn", "x"); err == nil || !strings.Contains(err.Error(), "unknown") {
		t.Errorf("unknown key must error: %v", err)
	}
}
