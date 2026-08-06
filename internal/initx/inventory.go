package initx

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Inventory is the JSON contract of the analysis prompt: what the container
// image needs so Claude Code can work on this project.
type Inventory struct {
	BaseImage string            `json:"base_image"`
	Packages  []string          `json:"packages"`
	Env       map[string]string `json:"env"`
}

// resultEnvelope is the subset of `claude --output-format json` output that
// ParseInventory needs.
type resultEnvelope struct {
	Result  string `json:"result"`
	IsError bool   `json:"is_error"`
}

// ParseInventory unwraps the claude result envelope and strictly decodes
// the inventory. Strictness is deliberate (fail fast, no fallback): unknown
// fields, trailing content, and a missing base_image are all errors. The
// only tolerated cosmetics are surrounding whitespace and one ```json fence.
func ParseInventory(claudeStdout string) (Inventory, error) {
	var env resultEnvelope
	if err := json.Unmarshal([]byte(claudeStdout), &env); err != nil {
		return Inventory{}, fmt.Errorf("claude output is not the expected JSON envelope: %w", err)
	}
	if env.IsError {
		return Inventory{}, errors.New("claude reported an error result")
	}
	body := strings.TrimSpace(env.Result)
	body = strings.TrimPrefix(body, "```json")
	body = strings.TrimPrefix(body, "```")
	body = strings.TrimSuffix(body, "```")
	body = strings.TrimSpace(body)

	dec := json.NewDecoder(strings.NewReader(body))
	dec.DisallowUnknownFields()
	var inv Inventory
	if err := dec.Decode(&inv); err != nil {
		return Inventory{}, fmt.Errorf("inventory is not valid JSON: %w", err)
	}
	if dec.More() {
		return Inventory{}, errors.New("unexpected trailing content after the inventory JSON")
	}
	if inv.BaseImage == "" {
		return Inventory{}, errors.New("inventory is missing base_image")
	}
	return inv, nil
}
