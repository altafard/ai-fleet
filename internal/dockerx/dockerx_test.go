package dockerx

import (
	"reflect"
	"regexp"
	"testing"
)

func TestMountArg(t *testing.T) {
	cases := []struct {
		m    Mount
		want string
	}{
		{Mount{"/runs/x/worktree", "/source/clone", true}, "/runs/x/worktree:/source/clone:ro"},
		{Mount{"/runs/x/out", "/out", false}, "/runs/x/out:/out"},
		{Mount{`C:\work\wt`, "/source/clone", true}, `C:\work\wt:/source/clone:ro`},
	}
	for _, c := range cases {
		if got := c.m.Arg(); got != c.want {
			t.Errorf("got %q want %q", got, c.want)
		}
	}
}

func TestBuildArgs(t *testing.T) {
	got := BuildArgs("/p/Dockerfile", "/p", "ai-fleet:abc123def456")
	want := []string{"build", "--progress=plain", "-f", "/p/Dockerfile", "-t", "ai-fleet:abc123def456", "/p"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestRunArgsKeepsSecretsOutOfArgv(t *testing.T) {
	got := RunArgs("ai-fleet:abc", "ai-fleet-260802-101530-ab12",
		[]Mount{{"/w", "/source/clone", true}},
		[]string{"CLAUDE_CODE_OAUTH_TOKEN", "FLEET_BRANCH"},
		[]string{"bash", "/source/entrypoint.sh"})
	want := []string{"run", "--rm", "--name", "ai-fleet-260802-101530-ab12",
		"-v", "/w:/source/clone:ro",
		"-e", "CLAUDE_CODE_OAUTH_TOKEN", "-e", "FLEET_BRANCH",
		"ai-fleet:abc", "bash", "/source/entrypoint.sh"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	for _, a := range got {
		if len(a) > 40 {
			t.Errorf("suspiciously long arg (secret value?): %q", a)
		}
	}
}

func TestImageTag(t *testing.T) {
	tag := ImageTag([]byte("FROM debian\n"))
	if !regexp.MustCompile(`^ai-fleet:[0-9a-f]{12}$`).MatchString(tag) {
		t.Fatalf("tag=%q", tag)
	}
	if tag == ImageTag([]byte("FROM alpine\n")) {
		t.Fatal("different dockerfiles must produce different tags")
	}
}
