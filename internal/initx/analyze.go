package initx

import (
	"context"
	"fmt"

	"github.com/altafard/ai-fleet/internal/execx"
)

// runClaude is a seam for Analyze's own tests; production runs the real
// CLI from the repo root so claude sees the project.
var runClaude = func(ctx context.Context, dir string) (execx.Result, error) {
	return execx.RunCtx(ctx, dir, nil, "claude", "-p", inventoryPrompt, "--output-format", "json")
}

// Analyze asks claude for the project inventory. Fail fast by design: any
// error aborts init before anything is written to disk. The raw output is
// included on parse failures — it is the only way to debug a bad reply.
// Deliberately no timeout: analysis takes as long as it takes, and Ctrl-C
// remains available.
func Analyze(root string) (Inventory, error) {
	r, err := runClaude(context.Background(), root)
	if err != nil {
		return Inventory{}, fmt.Errorf("claude could not run: %w", err)
	}
	if r.ExitCode != 0 {
		return Inventory{}, fmt.Errorf("claude exited with code %d: %s", r.ExitCode, r.Stderr)
	}
	inv, err := ParseInventory(r.Stdout)
	if err != nil {
		return Inventory{}, fmt.Errorf("%w\nraw claude output:\n%s", err, r.Stdout)
	}
	return inv, nil
}
