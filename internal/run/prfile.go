package run

import (
	"fmt"
	"os"
	"strings"
)

// ParsePRFile reads /out/pull-request.md: line 1 is the title, the rest is
// the body. Both are required — there is deliberately no fallback (spec).
func ParsePRFile(path string) (string, string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", fmt.Errorf("pull-request.md was not produced by the session: %w", err)
	}
	content := strings.ReplaceAll(string(b), "\r\n", "\n")
	lines := strings.SplitN(content, "\n", 2)
	title := strings.TrimSpace(lines[0])
	if title == "" {
		return "", "", fmt.Errorf("pull-request.md has an empty title line")
	}
	body := ""
	if len(lines) == 2 {
		body = strings.TrimSpace(lines[1])
	}
	if body == "" {
		return "", "", fmt.Errorf("pull-request.md has no body — the PR body is required")
	}
	return title, body, nil
}
