package initx

import (
	"encoding/json"
	"strings"
	"testing"
)

// envelope wraps a reply string the way `claude --output-format json` does.
func envelope(t *testing.T, result string, isError bool) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"type": "result", "subtype": "success", "is_error": isError, "result": result,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestParseInventoryValid(t *testing.T) {
	body := `{"base_image":"golang:1.26-bookworm","packages":["make"],"env":{"GOTOOLCHAIN":"local"}}`
	cases := []struct{ name, result string }{
		{"bare", body},
		{"whitespace", "\n  " + body + "\n"},
		{"fenced", "```json\n" + body + "\n```"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			inv, err := ParseInventory(envelope(t, c.result, false))
			if err != nil {
				t.Fatal(err)
			}
			if inv.BaseImage != "golang:1.26-bookworm" || len(inv.Packages) != 1 || inv.Env["GOTOOLCHAIN"] != "local" {
				t.Errorf("got %+v", inv)
			}
		})
	}
}

func TestParseInventoryOptionalFields(t *testing.T) {
	inv, err := ParseInventory(envelope(t, `{"base_image":"debian:bookworm-slim"}`, false))
	if err != nil {
		t.Fatal(err)
	}
	if inv.BaseImage != "debian:bookworm-slim" || len(inv.Packages) != 0 || len(inv.Env) != 0 {
		t.Errorf("got %+v", inv)
	}
}

func TestParseInventoryErrors(t *testing.T) {
	cases := []struct {
		name, raw, result, wantErr string
		isError                    bool
	}{
		{name: "not json at all", raw: "claude exploded", wantErr: "envelope"},
		{name: "error result", result: `{}`, isError: true, wantErr: "error result"},
		{name: "result not json", result: "sorry, I cannot", wantErr: "not valid JSON"},
		{name: "unknown field", result: `{"base_image":"x","distro":"alpine"}`, wantErr: "not valid JSON"},
		{name: "missing base_image", result: `{"packages":[]}`, wantErr: "base_image"},
		{name: "trailing garbage", result: `{"base_image":"x"} extra`, wantErr: "trailing"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stdout := c.raw
			if stdout == "" {
				stdout = envelope(t, c.result, c.isError)
			}
			_, err := ParseInventory(stdout)
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("error = %v, want containing %q", err, c.wantErr)
			}
		})
	}
}
