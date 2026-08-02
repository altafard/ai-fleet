package logstream

import "testing"

func TestClaudeSummary(t *testing.T) {
	cases := []struct {
		name      string
		ev        map[string]any
		wantMsg   string
		wantLevel string
	}{
		{"init", map[string]any{"type": "system", "subtype": "init"}, "session started", "info"},
		{"tool use", map[string]any{"type": "assistant", "message": map[string]any{
			"content": []any{map[string]any{"type": "tool_use", "name": "Bash",
				"input": map[string]any{"command": "npm test"}}}}},
			"Bash(npm test)", "info"},
		{"text", map[string]any{"type": "assistant", "message": map[string]any{
			"content": []any{map[string]any{"type": "text", "text": "Looking at the code"}}}},
			"Looking at the code", "info"},
		{"result ok", map[string]any{"type": "result", "subtype": "success", "num_turns": float64(14)},
			"result: success (14 turns)", "info"},
		{"result err", map[string]any{"type": "result", "subtype": "error_during_execution"},
			"result: error_during_execution", "error"},
	}
	for _, c := range cases {
		msg, level := ClaudeSummary(c.ev)
		if msg != c.wantMsg || level != c.wantLevel {
			t.Errorf("%s: got (%q,%q) want (%q,%q)", c.name, msg, level, c.wantMsg, c.wantLevel)
		}
	}
}
