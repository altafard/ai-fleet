package cli

import (
	"runtime/debug"
	"strings"
)

// formatVersion renders the one-line version string from Go build info.
// A tagged install reports the module version; a source build reports the
// VCS revision, commit date, and dirty state, omitting whatever is absent.
func formatVersion(bi *debug.BuildInfo) string {
	if bi == nil || bi.Main.Version == "" {
		return "ai-fleet (unknown)"
	}
	if bi.Main.Version != "(devel)" {
		return "ai-fleet " + bi.Main.Version
	}

	var rev, date string
	dirty := false
	for _, s := range bi.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
			if len(rev) > 7 {
				rev = rev[:7]
			}
		case "vcs.time":
			date, _, _ = strings.Cut(s.Value, "T")
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}

	detail := strings.TrimSpace(rev + " " + date)
	if dirty {
		if detail == "" {
			detail = "dirty"
		} else {
			detail += ", dirty"
		}
	}
	if detail == "" {
		return "ai-fleet devel"
	}
	return "ai-fleet devel (" + detail + ")"
}

// versionLine reads the running binary's build info; it never fails.
func versionLine() string {
	bi, ok := debug.ReadBuildInfo()
	if !ok {
		bi = nil
	}
	return formatVersion(bi)
}
