package config

import (
	"errors"
	"strings"
)

// CheckConflict rejects credential combinations that are contradictory
// within one scope. Only explicit contradictions are caught here — an
// unset git.type constrains nothing (the deploy-time merge check owns the
// cross-scope cases), so set-order never matters.
func CheckConflict(scope map[string]string, key, value string) error {
	typ := scope["git.type"]
	if key == "git.type" {
		typ = value
	}
	hasApp := func() bool {
		for k := range scope {
			if strings.HasPrefix(k, "git.app.") {
				return true
			}
		}
		return strings.HasPrefix(key, "git.app.")
	}
	hasToken := scope["git.token"] != "" || key == "git.token"
	switch {
	case typ == "bot" && hasToken:
		return errors.New("git.type is \"bot\": git.token must not be set (bot auth uses git.app.id and git.app.private-key)")
	case typ == "user" && hasApp():
		return errors.New("git.app.* settings require git.type = \"bot\"")
	}
	return nil
}
