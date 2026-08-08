// Package config implements persistent settings over ai-fleet.ini files:
// a closed [agent]/[git] schema, two scopes (local and global), surgical
// line edits, and defaults application into run.Options. The [project]
// section belongs to init — config never reads or writes it.
package config

import (
	"fmt"
	"regexp"
	"sort"

	"github.com/altafard/ai-fleet/internal/run"
)

// noSpaceRe rejects whitespace — enough for opaque identifiers whose real
// validity only the provider can judge (app IDs, tokens).
var noSpaceRe = regexp.MustCompile(`^\S+$`)

var digitsRe = regexp.MustCompile(`^[0-9]+$`)

// schema is the closed key set. A key outside it is a usage error: a typo
// that silently did nothing would defeat the point of `config set`.
var schema = map[string]func(string) error{
	"agent.model": func(v string) error {
		if !run.ValidModel(v) {
			return fmt.Errorf("invalid agent.model %q: allowed characters are A-Z a-z 0-9 . _ [ ] -", v)
		}
		return nil
	},
	"agent.effort": func(v string) error {
		if !run.ValidEffort(v) {
			return fmt.Errorf("invalid agent.effort %q: must be one of low, medium, high, xhigh, max", v)
		}
		return nil
	},
	"git.provider": func(v string) error {
		if v != "github" {
			return fmt.Errorf("unsupported git.provider %q: v1 supports \"github\"", v)
		}
		return nil
	},
	"git.repository":  nonEmpty("git.repository"),
	"git.author.name": nonEmpty("git.author.name"),
	"git.author.email": func(v string) error {
		if v == "" {
			return fmt.Errorf("git.author.email must not be empty")
		}
		return nil
	},
	"git.type": func(v string) error {
		if v != "user" && v != "bot" {
			return fmt.Errorf("invalid git.type %q: must be \"user\" or \"bot\"", v)
		}
		return nil
	},
	"git.token":           opaque("git.token"),
	"git.app.id":          opaque("git.app.id"),
	"git.app.private-key": nonEmpty("git.app.private-key"),
	"git.app.installation-id": func(v string) error {
		if !digitsRe.MatchString(v) {
			return fmt.Errorf("invalid git.app.installation-id %q: must be a number", v)
		}
		return nil
	},
}

func nonEmpty(key string) func(string) error {
	return func(v string) error {
		if v == "" {
			return fmt.Errorf("%s must not be empty", key)
		}
		return nil
	}
}

func opaque(key string) func(string) error {
	return func(v string) error {
		if !noSpaceRe.MatchString(v) {
			return fmt.Errorf("invalid %s: must be non-empty without whitespace", key)
		}
		return nil
	}
}

// Keys returns every schema key, sorted.
func Keys() []string {
	ks := make([]string, 0, len(schema))
	for k := range schema {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// ValidateKey rejects keys outside the closed schema.
func ValidateKey(key string) error {
	if _, ok := schema[key]; !ok {
		return fmt.Errorf("unknown config key %q", key)
	}
	return nil
}

// ValidateValue rejects a value the corresponding flag would reject.
func ValidateValue(key, value string) error {
	v, ok := schema[key]
	if !ok {
		return fmt.Errorf("unknown config key %q", key)
	}
	return v(value)
}
