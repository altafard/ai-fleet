package initx

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
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
	if err := validateInventory(inv); err != nil {
		return Inventory{}, err
	}
	return inv, nil
}

// The analysis runs claude inside a repository nobody here has audited, so
// its reply is attacker-controlled input: prompt injection in a README can
// dictate these fields. They are interpolated into the Dockerfile by
// text/template, which escapes nothing — a newline in any of them becomes
// an extra Dockerfile directive that `docker build` runs on the host,
// outside any sandbox. Each field is therefore constrained to a shape that
// cannot span a line or introduce shell metacharacters.
var (
	imageReference = regexp.MustCompile(`^[a-z0-9]+(?:[._/-][a-z0-9]+)*(?::[\w][\w.-]{0,127})?(?:@sha256:[a-f0-9]{64})?$`)
	packageName    = regexp.MustCompile(`^[a-z0-9][a-z0-9+._-]*$`)
	envKey         = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func validateInventory(inv Inventory) error {
	if !imageReference.MatchString(inv.BaseImage) {
		return fmt.Errorf("inventory base_image %q is not a valid image reference", inv.BaseImage)
	}
	for _, p := range inv.Packages {
		if !packageName.MatchString(p) {
			return fmt.Errorf("inventory package %q is not a valid package name", p)
		}
	}
	for k, v := range inv.Env {
		if !envKey.MatchString(k) {
			return fmt.Errorf("inventory env key %q is not a valid environment variable name", k)
		}
		// Docker interprets backslash escapes inside an ENV value, so quoting
		// alone would not contain one; reject them along with line breaks.
		if strings.ContainsAny(v, "\n\r\\") {
			return fmt.Errorf("inventory env value for %q contains a line break or backslash", k)
		}
	}
	return nil
}
