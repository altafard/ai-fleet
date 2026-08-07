package config

import (
	"fmt"
	"strings"
)

// owned are the sections config reads and writes. Everything else —
// [project] today, anything future — is invisible: sections are ownership
// boundaries, and a malformed line in a foreign section is the owner's
// problem, not config's.
var owned = map[string]bool{"agent": true, "git": true}

// ParseOwned extracts section-qualified keys from the owned sections.
func ParseOwned(data string) (map[string]string, error) {
	out := map[string]string{}
	section := ""
	for i, raw := range strings.Split(data, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = line[1 : len(line)-1]
			continue
		}
		if !owned[section] {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("line %d: not a key = value pair: %q", i+1, line)
		}
		out[section+"."+strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out, nil
}

// splitKey breaks "git.author.name" into ("git", "author.name").
func splitKey(key string) (string, string) {
	section, rest, _ := strings.Cut(key, ".")
	return section, rest
}

// lineKey returns the key of a `k = v` line, or "" for anything else.
func lineKey(line string) string {
	t := strings.TrimSpace(line)
	if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, ";") || strings.HasPrefix(t, "[") {
		return ""
	}
	k, _, ok := strings.Cut(t, "=")
	if !ok {
		return ""
	}
	return strings.TrimSpace(k)
}

// SetLine surgically sets key in content: only the key's own line is
// touched (or inserted, right after its section header so the position is
// deterministic); comments, [project], and unknown keys stay byte-for-byte.
func SetLine(content, key, value string) string {
	section, k := splitKey(key)
	header := "[" + section + "]"
	lines := strings.Split(content, "\n")
	cur := ""
	headerAt := -1
	for i, raw := range lines {
		t := strings.TrimSpace(raw)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			cur = t[1 : len(t)-1]
			if cur == section {
				headerAt = i
			}
			continue
		}
		if cur == section && lineKey(raw) == k {
			lines[i] = k + " = " + value
			return strings.Join(lines, "\n")
		}
	}
	if headerAt >= 0 {
		lines = append(lines[:headerAt+1], append([]string{k + " = " + value}, lines[headerAt+1:]...)...)
		return strings.Join(lines, "\n")
	}
	out := content
	if out != "" && !strings.HasSuffix(out, "\n") {
		out += "\n"
	}
	if out != "" {
		out += "\n"
	}
	return out + header + "\n" + k + " = " + value + "\n"
}

// RemoveLine deletes key's line from content; absent key is a no-op. An
// emptied section keeps its header — harmless, and preserves layout.
func RemoveLine(content, key string) string {
	section, k := splitKey(key)
	lines := strings.Split(content, "\n")
	cur := ""
	for i, raw := range lines {
		t := strings.TrimSpace(raw)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			cur = t[1 : len(t)-1]
			continue
		}
		if cur == section && lineKey(raw) == k {
			return strings.Join(append(lines[:i], lines[i+1:]...), "\n")
		}
	}
	return content
}
