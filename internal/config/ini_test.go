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

func TestSetLineUpdatesInPlacePreservingEverythingElse(t *testing.T) {
	got := SetLine(sampleINI, "agent.model", "sonnet")
	want := strings.Replace(sampleINI, "model = opus", "model = sonnet", 1)
	if got != want {
		t.Fatalf("surgical update changed more than one line:\n%q\nwant\n%q", got, want)
	}
}

func TestSetLineInsertsAfterSectionHeader(t *testing.T) {
	got := SetLine(sampleINI, "git.token", "ghp_x")
	if !strings.Contains(got, "[git]\ntoken = ghp_x\n; identity\n") {
		t.Fatalf("insert not after header:\n%q", got)
	}
	// nothing else moved
	if !strings.Contains(got, "name = ai-fleet") || !strings.Contains(got, "# machine-local registration") {
		t.Fatal("foreign content disturbed")
	}
}

func TestSetLineAppendsMissingSection(t *testing.T) {
	got := SetLine("[project]\nname = x\nhash = y\n", "agent.model", "opus")
	if !strings.HasSuffix(got, "\n[agent]\nmodel = opus\n") {
		t.Fatalf("missing section not appended:\n%q", got)
	}
}

func TestSetLineOnEmptyContent(t *testing.T) {
	got := SetLine("", "agent.model", "opus")
	if got != "[agent]\nmodel = opus\n" {
		t.Fatalf("got %q", got)
	}
}

func TestRemoveLine(t *testing.T) {
	got := RemoveLine(sampleINI, "agent.effort")
	want := strings.Replace(sampleINI, "effort = high\n", "", 1)
	if got != want {
		t.Fatalf("remove touched more than the key line:\n%q", got)
	}
	if RemoveLine(sampleINI, "git.token") != sampleINI {
		t.Fatal("removing an absent key must be a no-op")
	}
}
