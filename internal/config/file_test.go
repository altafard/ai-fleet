package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadMissingFileIsEmpty(t *testing.T) {
	m, err := Load(filepath.Join(t.TempDir(), "nope.ini"))
	if err != nil || len(m) != 0 {
		t.Fatalf("got %v, %v", m, err)
	}
}

func TestLoadParseErrorNamesFileAndLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ai-fleet.ini")
	os.WriteFile(p, []byte("[agent]\nmodel opus\n"), 0o644)
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), p) || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("want error naming file and line, got %v", err)
	}
}

func TestSetRoundTripPreservesForeignContent(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ai-fleet.ini")
	orig := "# note\n[project]\nname = x\nhash = y\n"
	os.WriteFile(p, []byte(orig), 0o644)

	if err := Set(p, "agent.model", "opus"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "# note\n[project]\nname = x\nhash = y\n") {
		t.Fatalf("foreign content disturbed:\n%s", b)
	}
	m, err := Load(p)
	if err != nil || m["agent.model"] != "opus" {
		t.Fatalf("got %v, %v", m, err)
	}
	if fi, _ := os.Stat(p); fi.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %o, want 0600 (the file may hold a token)", fi.Mode().Perm())
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %d entries", len(entries))
	}
}

func TestSetCreatesParentDirForGlobalFirstWrite(t *testing.T) {
	dir := filepath.Join(t.TempDir(), ".ai-fleet")
	p := filepath.Join(dir, "ai-fleet.ini")
	if err := Set(p, "agent.effort", "high"); err != nil {
		t.Fatal(err)
	}
	if fi, _ := os.Stat(dir); fi.Mode().Perm() != 0o700 {
		t.Fatalf("dir perm = %o, want 0700", fi.Mode().Perm())
	}
	m, _ := Load(p)
	if m["agent.effort"] != "high" {
		t.Fatalf("got %v", m)
	}
}

func TestRemoveKey(t *testing.T) {
	p := filepath.Join(t.TempDir(), "ai-fleet.ini")
	os.WriteFile(p, []byte("[agent]\nmodel = opus\neffort = high\n"), 0o644)
	if err := Remove(p, "agent.model"); err != nil {
		t.Fatal(err)
	}
	m, _ := Load(p)
	if _, ok := m["agent.model"]; ok || m["agent.effort"] != "high" {
		t.Fatalf("got %v", m)
	}
}

func TestPaths(t *testing.T) {
	if got := LocalPath("/repo"); got != filepath.Join("/repo", ".ai-fleet", "ai-fleet.ini") {
		t.Fatalf("LocalPath = %q", got)
	}
	t.Setenv("HOME", "/home/u")
	gp, err := GlobalPath()
	if err != nil || gp != filepath.Join("/home/u", ".ai-fleet", "ai-fleet.ini") {
		t.Fatalf("GlobalPath = %q, %v", gp, err)
	}
}
