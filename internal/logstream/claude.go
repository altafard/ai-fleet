package logstream

import (
	"fmt"
	"strings"
)

// ClaudeSummary derives the envelope message and level for one stream-json event.
func ClaudeSummary(ev map[string]any) (string, string) {
	switch ev["type"] {
	case "system":
		return "session started", "info"
	case "assistant":
		msg, _ := ev["message"].(map[string]any)
		content, _ := msg["content"].([]any)
		for _, c := range content {
			b, _ := c.(map[string]any)
			if b["type"] == "tool_use" {
				return toolLabel(b), "info"
			}
		}
		for _, c := range content {
			b, _ := c.(map[string]any)
			if b["type"] == "text" {
				return truncate(b["text"], 80), "info"
			}
		}
		return "assistant message", "info"
	case "user":
		return "tool result", "info"
	case "result":
		if ev["subtype"] == "success" {
			return fmt.Sprintf("result: success (%v turns)", ev["num_turns"]), "info"
		}
		return fmt.Sprintf("result: %v", ev["subtype"]), "error"
	}
	return fmt.Sprintf("%v", ev["type"]), "info"
}

func toolLabel(b map[string]any) string {
	name, _ := b["name"].(string)
	if in, ok := b["input"].(map[string]any); ok {
		for _, k := range []string{"command", "file_path", "pattern", "url", "prompt"} {
			if v, _ := in[k].(string); v != "" {
				return name + "(" + truncate(v, 60) + ")"
			}
		}
	}
	return name
}

func truncate(v any, n int) string {
	s, _ := v.(string)
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}
