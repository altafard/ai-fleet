package run

import (
	"strings"
	"testing"

	"github.com/altafard/ai-fleet/internal/initx"
)

func TestUninitializedProjectMessage(t *testing.T) {
	// The error a user sees when omitting --dockerfile without init.
	_, _, err := initx.ResolveProject(t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "ai-fleet init") {
		t.Errorf("error = %v, want mention of `ai-fleet init`", err)
	}
}
