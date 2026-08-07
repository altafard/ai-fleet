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
