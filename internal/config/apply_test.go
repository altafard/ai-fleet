package config

import (
	"testing"

	"github.com/altafard/ai-fleet/internal/run"
)

func TestApplyFillsOnlyEmptyFields(t *testing.T) {
	o := run.Options{Model: "haiku", GitToken: "from-env"}
	Apply(&o, map[string]string{
		"agent.model": "opus", "agent.effort": "high",
		"git.provider": "github", "git.repository": "o/r",
		"git.author.name": "Bot", "git.author.email": "b@e.c",
		"git.token": "from-config", "git.type": "bot",
		"git.app.id": "123", "git.app.private-key": "/k.pem", "git.app.installation-id": "42",
	})
	if o.Model != "haiku" {
		t.Errorf("flag value overridden: %q", o.Model)
	}
	if o.GitToken != "from-env" {
		t.Errorf("env value overridden: %q", o.GitToken)
	}
	if o.Effort != "high" || o.GitProvider != "github" || o.GitRepository != "o/r" ||
		o.GitAuthorName != "Bot" || o.GitAuthorEmail != "b@e.c" ||
		o.GitEntityType != "bot" || o.GitAppID != "123" ||
		o.GitAppPrivateKey != "/k.pem" || o.GitAppInstallationID != "42" {
		t.Errorf("config values not applied: %+v", o)
	}
}

func TestApplyEmptyConfigIsNoOp(t *testing.T) {
	o := run.Options{Model: "opus"}
	Apply(&o, map[string]string{})
	if o.Model != "opus" || o.Effort != "" {
		t.Errorf("got %+v", o)
	}
}
