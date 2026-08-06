package initx

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/altafard/ai-fleet/internal/dockerx"
	"github.com/altafard/ai-fleet/internal/execx"
)

// gitInit creates a real repository — real git is the codebase's test
// convention (see gitx tests).
func gitInit(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	cmd := exec.Command("git", "init", "-q", "-b", "main")
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init: %s", out)
	}
	return root
}

// stubOK replaces every external seam with a canned success and returns a
// pointer to the tag the build stub received.
func stubOK(t *testing.T) *string {
	t.Helper()
	var builtTag string
	origGit, origClaude, origDocker := gitVersion, claudeVersion, dockerVersion
	origAnalyze, origBuild, origPrune := analyze, buildImage, pruneRepo
	t.Cleanup(func() {
		gitVersion, claudeVersion, dockerVersion = origGit, origClaude, origDocker
		analyze, buildImage, pruneRepo = origAnalyze, origBuild, origPrune
	})
	gitVersion = func() (string, error) { return "2.47.0", nil }
	claudeVersion = func() (string, error) { return "2.1.14", nil }
	dockerVersion = func() (string, error) { return "28.3.2", nil }
	analyze = func(root string) (Inventory, error) {
		return Inventory{BaseImage: "debian:bookworm-slim"}, nil
	}
	buildImage = func(ctx context.Context, dockerfile, contextDir, tag string, onLine func(string)) (string, error) {
		builtTag = tag
		return "sha256:fake", nil
	}
	pruneRepo = func(repo, keep string) (int, []string, error) { return 0, nil, nil }
	return &builtTag
}

func TestExecuteHappyPath(t *testing.T) {
	builtTag := stubOK(t)
	root := gitInit(t)

	if code := Execute(root); code != ExitOK {
		t.Fatalf("Execute = %d, want %d", code, ExitOK)
	}
	ini := filepath.Join(root, ".ai-fleet", "ai-fleet.ini")
	if _, err := os.Stat(ini); err != nil {
		t.Errorf("ai-fleet.ini not written: %v", err)
	}
	name, hash := ProjectName(root), ProjectHash(root)
	df := filepath.Join(root, ".ai-fleet", DockerfileName(name))
	content, err := os.ReadFile(df)
	if err != nil {
		t.Fatalf("dockerfile not written: %v", err)
	}
	want := "ai-fleet/" + name + "-" + hash + ":" + dockerx.ContentTag(content)
	if *builtTag != want {
		t.Errorf("built tag = %q, want %q", *builtTag, want)
	}
}

func TestExecuteFromSubdirectoryInitializesAtRoot(t *testing.T) {
	stubOK(t)
	root := gitInit(t)
	sub := filepath.Join(root, "sub", "dir")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if code := Execute(sub); code != ExitOK {
		t.Fatalf("Execute = %d, want %d", code, ExitOK)
	}
	if _, err := os.Stat(filepath.Join(root, ".ai-fleet", "ai-fleet.ini")); err != nil {
		t.Errorf("ini not at repo root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sub, ".ai-fleet")); !os.IsNotExist(err) {
		t.Error(".ai-fleet was created in the subdirectory")
	}
}

func TestExecuteNotARepo(t *testing.T) {
	stubOK(t)
	dir := t.TempDir()
	if code := Execute(dir); code != ExitUsage {
		t.Fatalf("Execute = %d, want %d", code, ExitUsage)
	}
	if _, err := os.Stat(filepath.Join(dir, ".ai-fleet")); !os.IsNotExist(err) {
		t.Error(".ai-fleet was created outside a repository")
	}
}

func TestExecuteAlreadyInitialized(t *testing.T) {
	stubOK(t)
	root := gitInit(t)
	if _, err := WriteFiles(root, Config{Name: "p", Hash: "abcd1234"}, []byte("FROM x\n")); err != nil {
		t.Fatal(err)
	}
	if code := Execute(root); code != ExitUsage {
		t.Fatalf("Execute = %d, want %d", code, ExitUsage)
	}
}

func TestExecuteRunsDirDoesNotCountAsInitialized(t *testing.T) {
	stubOK(t)
	root := gitInit(t)
	if err := os.MkdirAll(filepath.Join(root, ".ai-fleet", "runs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code := Execute(root); code != ExitOK {
		t.Fatalf("Execute = %d, want %d", code, ExitOK)
	}
}

func TestExecuteAnalysisFailureWritesNothing(t *testing.T) {
	stubOK(t)
	analyze = func(root string) (Inventory, error) { return Inventory{}, errors.New("boom") }
	root := gitInit(t)
	if code := Execute(root); code != ExitFailure {
		t.Fatalf("Execute = %d, want %d", code, ExitFailure)
	}
	if _, err := os.Stat(filepath.Join(root, ".ai-fleet")); !os.IsNotExist(err) {
		t.Error("analysis failure left files on disk")
	}
}

func TestExecuteBuildFailureKeepsFiles(t *testing.T) {
	stubOK(t)
	buildImage = func(ctx context.Context, dockerfile, contextDir, tag string, onLine func(string)) (string, error) {
		return "", errors.New("docker build failed with exit code 1")
	}
	root := gitInit(t)
	if code := Execute(root); code != ExitFailure {
		t.Fatalf("Execute = %d, want %d", code, ExitFailure)
	}
	if _, err := os.Stat(filepath.Join(root, ".ai-fleet", "ai-fleet.ini")); err != nil {
		t.Errorf("build failure removed generated files: %v", err)
	}
}

func TestExecuteToolCheckFailure(t *testing.T) {
	stubOK(t)
	claudeVersion = func() (string, error) { return "", errors.New("claude CLI not found in PATH") }
	root := gitInit(t)
	if code := Execute(root); code != ExitUsage {
		t.Fatalf("Execute = %d, want %d", code, ExitUsage)
	}
}

func TestAnalyzeNonZeroExit(t *testing.T) {
	orig := runClaude
	t.Cleanup(func() { runClaude = orig })
	runClaude = func(ctx context.Context, dir string) (execx.Result, error) {
		return execx.Result{ExitCode: 1, Stderr: "not logged in"}, nil
	}
	if _, err := Analyze(t.TempDir()); err == nil || !strings.Contains(err.Error(), "exited with code 1") {
		t.Errorf("error = %v", err)
	}
}
