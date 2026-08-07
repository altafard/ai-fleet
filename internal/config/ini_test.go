package config

import (
	"strings"
	"testing"
)

const sampleINI = `# machine-local registration
[project]
name = ai-fleet
hash = 4c92
this project line is not config's business

[agent]
model = opus
effort = high

[git]
; identity
author.name = Bot
author.email = bot@example.com
`

func TestParseOwned(t *testing.T) {
	got, err := ParseOwned(sampleINI)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"agent.model": "opus", "agent.effort": "high",
		"git.author.name": "Bot", "git.author.email": "bot@example.com",
	}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestParseOwnedEmptyAndMissingSections(t *testing.T) {
	got, err := ParseOwned("")
	if err != nil || len(got) != 0 {
		t.Fatalf("empty input: %v %v", got, err)
	}
	got, err = ParseOwned("[project]\nname = x\nhash = y\n")
	if err != nil || len(got) != 0 {
		t.Fatalf("project-only input: %v %v", got, err)
	}
}

func TestParseOwnedMalformedLineInOwnedSection(t *testing.T) {
	_, err := ParseOwned("[agent]\nmodel opus\n")
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("want line-numbered error, got %v", err)
	}
}
