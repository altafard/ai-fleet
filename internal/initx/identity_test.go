package initx

import (
	"strings"
	"testing"
)

func TestProjectName(t *testing.T) {
	cases := []struct{ root, want string }{
		{"/Users/x/Projects/ai-fleet", "ai-fleet"},
		{"/home/x/My App", "my-app"},
		{"/home/x/CamelCase", "camelcase"},
		{"/home/x/weird__name..", "weird-name"},
		{"/home/x/---", "project"},
		{"/home/x/размер", "project"},
		// Docker's reference grammar allows only ".", "_", "__" or "-"+ between
		// alphanumeric runs, so every other separator run has to collapse.
		{"/home/x/foo. bar", "foo-bar"},
		{"/home/x/a..b", "a-b"},
		{"/home/x/a___b", "a-b"},
	}
	for _, c := range cases {
		if got := ProjectName(c.root); got != c.want {
			t.Errorf("ProjectName(%q) = %q, want %q", c.root, got, c.want)
		}
	}
}

func TestProjectHashIsStableAndShort(t *testing.T) {
	a := ProjectHash("/Users/x/Projects/ai-fleet")
	b := ProjectHash("/Users/x/Projects/ai-fleet")
	c := ProjectHash("/Users/y/Projects/ai-fleet")
	if a != b {
		t.Errorf("hash not stable: %q vs %q", a, b)
	}
	if a == c {
		t.Error("different paths produced the same hash")
	}
	if len(a) != 4 || strings.ToLower(a) != a {
		t.Errorf("hash %q is not 4 lowercase hex chars", a)
	}
}

func TestImageRepo(t *testing.T) {
	if got := ImageRepo("ai-fleet", "a1b2c3d4"); got != "ai-fleet/ai-fleet-a1b2c3d4" {
		t.Errorf("ImageRepo = %q", got)
	}
}
