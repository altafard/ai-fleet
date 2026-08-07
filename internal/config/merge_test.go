package config

import (
	"path/filepath"
	"testing"
)

func TestMergeLocalWins(t *testing.T) {
	local := map[string]string{"agent.model": "opus"}
	global := map[string]string{"agent.model": "sonnet", "agent.effort": "high"}
	m := Merge(local, global, "/repo", "/home/u")
	if m["agent.model"] != "opus" || m["agent.effort"] != "high" {
		t.Fatalf("got %v", m)
	}
}

func TestMergeResolvesPrivateKeyAgainstItsScope(t *testing.T) {
	m := Merge(map[string]string{"git.app.private-key": ".ai-fleet/bot.pem"}, nil, "/repo", "/home/u")
	if m["git.app.private-key"] != filepath.Join("/repo", ".ai-fleet", "bot.pem") {
		t.Fatalf("local-relative: %q", m["git.app.private-key"])
	}
	m = Merge(nil, map[string]string{"git.app.private-key": "keys/bot.pem"}, "/repo", "/home/u")
	if m["git.app.private-key"] != filepath.Join("/home/u", "keys", "bot.pem") {
		t.Fatalf("global-relative: %q", m["git.app.private-key"])
	}
	abs := filepath.Join("/abs", "bot.pem")
	m = Merge(map[string]string{"git.app.private-key": abs}, nil, "/repo", "/home/u")
	if m["git.app.private-key"] != abs {
		t.Fatalf("absolute must pass through: %q", m["git.app.private-key"])
	}
}
