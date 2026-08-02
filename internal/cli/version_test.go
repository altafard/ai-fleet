package cli

import (
	"runtime/debug"
	"testing"
)

func bi(version string, settings map[string]string) *debug.BuildInfo {
	b := &debug.BuildInfo{}
	b.Main.Version = version
	for k, v := range settings {
		b.Settings = append(b.Settings, debug.BuildSetting{Key: k, Value: v})
	}
	return b
}

func TestFormatVersion(t *testing.T) {
	vcs := map[string]string{
		"vcs.revision": "e288299abcdef0123456789abcdef0123456789a",
		"vcs.time":     "2026-08-02T10:15:30Z",
		"vcs.modified": "true",
	}
	cases := []struct {
		name string
		bi   *debug.BuildInfo
		want string
	}{
		{"nil build info", nil, "ai-fleet (unknown)"},
		{"real tag", bi("v0.2.0", nil), "ai-fleet v0.2.0"},
		{"pseudo-version", bi("v0.0.0-20260802101530-e288299abcde", nil), "ai-fleet v0.0.0-20260802101530-e288299abcde"},
		{"devel with vcs, dirty", bi("(devel)", vcs), "ai-fleet devel (e288299 2026-08-02, dirty)"},
		{"devel with vcs, clean", bi("(devel)", map[string]string{
			"vcs.revision": "e288299abcdef0123456789abcdef0123456789a",
			"vcs.time":     "2026-08-02T10:15:30Z",
			"vcs.modified": "false",
		}), "ai-fleet devel (e288299 2026-08-02)"},
		{"devel revision only", bi("(devel)", map[string]string{
			"vcs.revision": "e288299abcdef0123456789abcdef0123456789a",
		}), "ai-fleet devel (e288299)"},
		{"devel dirty only", bi("(devel)", map[string]string{
			"vcs.modified": "true",
		}), "ai-fleet devel (dirty)"},
		{"devel no settings", bi("(devel)", nil), "ai-fleet devel"},
		{"empty version string", bi("", nil), "ai-fleet (unknown)"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := formatVersion(c.bi); got != c.want {
				t.Errorf("formatVersion() = %q, want %q", got, c.want)
			}
		})
	}
}

func TestVersionLineNeverEmpty(t *testing.T) {
	if versionLine() == "" {
		t.Error("versionLine() returned empty string")
	}
}
