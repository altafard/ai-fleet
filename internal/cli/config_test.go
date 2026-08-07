package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altafard/ai-fleet/internal/run"
)

// runConfig executes the root command with args; returns stdout and code.
func runConfig(t *testing.T, args ...string) (string, int) {
	t.Helper()
	code := 0
	root := newRoot(&code)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		return out.String(), 2
	}
	return out.String(), code
}

// initProject creates a git repo with an initialized .ai-fleet and chdirs in.
func initProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	// gitx.RepoRoot reports the physical path (git resolves symlinks); resolve
	// macOS's /var -> /private/var here so callers can compare paths directly.
	if r, err := filepath.EvalSymlinks(root); err == nil {
		root = r
	}
	cmd := exec.Command("git", "init", "-q", "-b", "main")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %s", out)
	}
	if err := os.MkdirAll(filepath.Join(root, ".ai-fleet"), 0o755); err != nil {
		t.Fatal(err)
	}
	ini := "[project]\nname = p\nhash = abcd\n"
	if err := os.WriteFile(filepath.Join(root, ".ai-fleet", "ai-fleet.ini"), []byte(ini), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	return root
}

func TestConfigSetGetListLocal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	initProject(t)

	if _, code := runConfig(t, "config", "set", "agent.model", "opus"); code != 0 {
		t.Fatalf("set: code %d", code)
	}
	out, code := runConfig(t, "config", "get", "agent.model")
	if code != 0 || strings.TrimSpace(out) != "opus" {
		t.Fatalf("get: %q code %d", out, code)
	}
	out, code = runConfig(t, "config", "list")
	if code != 0 || !strings.Contains(out, "agent.model = opus") {
		t.Fatalf("list: %q code %d", out, code)
	}
}

func TestConfigGlobalScopeAndMergedGet(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	initProject(t)

	if _, code := runConfig(t, "config", "set", "--global", "agent.effort", "high"); code != 0 {
		t.Fatal("global set failed")
	}
	// merged get falls through to global…
	out, code := runConfig(t, "config", "get", "agent.effort")
	if code != 0 || strings.TrimSpace(out) != "high" {
		t.Fatalf("merged get: %q code %d", out, code)
	}
	// …and local wins once set.
	runConfig(t, "config", "set", "agent.effort", "low")
	out, _ = runConfig(t, "config", "get", "agent.effort")
	if strings.TrimSpace(out) != "low" {
		t.Fatalf("local must win: %q", out)
	}
	// --global still reads the global file only.
	out, _ = runConfig(t, "config", "get", "--global", "agent.effort")
	if strings.TrimSpace(out) != "high" {
		t.Fatalf("--global get: %q", out)
	}
}

func TestConfigGetUnsetIsSilentExit1(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	initProject(t)
	out, code := runConfig(t, "config", "get", "agent.model")
	if code != 1 || strings.TrimSpace(out) != "" {
		t.Fatalf("want silent exit 1, got %q code %d", out, code)
	}
}

func TestConfigSetWithoutValueRemoves(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	initProject(t)
	runConfig(t, "config", "set", "agent.model", "opus")
	if _, code := runConfig(t, "config", "set", "agent.model"); code != 0 {
		t.Fatalf("remove: code %d", code)
	}
	if _, code := runConfig(t, "config", "get", "agent.model"); code != 1 {
		t.Fatalf("key still set after removal")
	}
}

func TestConfigUsageErrors(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	initProject(t)
	if out, code := runConfig(t, "config", "set", "git.tokn", "x"); code != 2 || !strings.Contains(out, "unknown") {
		t.Fatalf("unknown key: %q code %d", out, code)
	}
	if _, code := runConfig(t, "config", "set", "agent.effort", "ultra"); code != 2 {
		t.Fatal("invalid value must be code 2")
	}
	if _, code := runConfig(t, "config", "set", "agent.model", "opus", "extra"); code != 2 {
		t.Fatal("wrong arg count must be code 2")
	}
}

func TestConfigOutsideProjectRequiresGlobal(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Chdir(t.TempDir()) // not a repo, not initialized
	out, code := runConfig(t, "config", "get", "agent.model")
	if code != 2 || !strings.Contains(out, "ai-fleet init") {
		t.Fatalf("want init hint with code 2, got %q code %d", out, code)
	}
	if _, code := runConfig(t, "config", "set", "--global", "agent.model", "opus"); code != 0 {
		t.Fatal("--global must work anywhere")
	}
}

func TestConfigSetConflictWithinScope(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	initProject(t)
	runConfig(t, "config", "set", "git.type", "bot")
	if out, code := runConfig(t, "config", "set", "git.token", "tok"); code != 2 || !strings.Contains(out, "bot") {
		t.Fatalf("token under type=bot must be code 2: %q code %d", out, code)
	}
	runConfig(t, "config", "set", "git.type", "user")
	if _, code := runConfig(t, "config", "set", "git.app.id", "123"); code != 2 {
		t.Fatal("app.* under type=user must be code 2")
	}
}

func TestApplyConfigPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := initProject(t)

	runConfig(t, "config", "set", "--global", "agent.model", "sonnet")
	runConfig(t, "config", "set", "agent.model", "opus") // local wins over global
	runConfig(t, "config", "set", "agent.effort", "high")
	runConfig(t, "config", "set", "git.token", "from-config")
	runConfig(t, "config", "set", "git.app.private-key", ".ai-fleet/bot.pem")

	o := run.Options{Project: root, GitToken: "from-env"} // env already bound, must win
	if err := applyConfig(&o); err != nil {
		t.Fatal(err)
	}
	if o.Model != "opus" || o.Effort != "high" {
		t.Fatalf("config not applied: %+v", o)
	}
	if o.GitToken != "from-env" {
		t.Fatalf("env must beat local config: %q", o.GitToken)
	}
	if o.GitAppPrivateKey != filepath.Join(root, ".ai-fleet", "bot.pem") {
		t.Fatalf("key path not resolved against the repo root: %q", o.GitAppPrivateKey)
	}
}

func TestApplyConfigOutsideRepoIsNoOpForLocal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := t.TempDir()
	t.Chdir(dir)
	// global still applies; the missing local scope is not an error here —
	// run.Execute's preflight owns reporting an unusable project.
	o := run.Options{Project: dir}
	if err := applyConfig(&o); err != nil {
		t.Fatal(err)
	}
}

func TestApplyConfigParseErrorIsReported(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := initProject(t)
	os.WriteFile(filepath.Join(root, ".ai-fleet", "ai-fleet.ini"),
		[]byte("[project]\nname = p\nhash = h\n[agent]\nmodel opus\n"), 0o644)
	o := run.Options{Project: root}
	if err := applyConfig(&o); err == nil || !strings.Contains(err.Error(), "line") {
		t.Fatalf("parse error must surface with a line number: %v", err)
	}
}
