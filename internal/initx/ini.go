package initx

import (
	"errors"
	"fmt"
	"strings"
)

// Config is the parsed .ai-fleet/ai-fleet.ini. The file is deliberately
// machine-local (the hash depends on the absolute path), so it is never
// committed; each clone runs `ai-fleet init` itself.
type Config struct {
	Name string
	Hash string
}

// RenderINI writes the fixed one-section format. Hand-rolled on purpose:
// the format is tiny and fixed, not worth a dependency.
func RenderINI(c Config) string {
	return fmt.Sprintf("[project]\nname = %s\nhash = %s\n", c.Name, c.Hash)
}

// ParseINI reads the fixed format back. Unknown keys are ignored so future
// versions can add keys without breaking older binaries — this also covers
// the retired [config] section's "global" key still present in old files; a
// missing project name or hash is an error because nothing downstream works
// without them.
func ParseINI(data string) (Config, error) {
	var c Config
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
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return Config{}, fmt.Errorf("ai-fleet.ini line %d: not a key = value pair: %q", i+1, line)
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch section + "." + k {
		case "project.name":
			c.Name = v
		case "project.hash":
			c.Hash = v
		}
	}
	if c.Name == "" || c.Hash == "" {
		return Config{}, errors.New("ai-fleet.ini is missing project.name or project.hash")
	}
	return c, nil
}
