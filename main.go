// Ai-fleet runs a headless Claude Code session in a Docker container on a
// disposable clone of a git project and brings the resulting commits back
// as a git bundle, optionally opening a pull request.
//
// Usage:
//
//	ai-fleet deploy unit --dockerfile <path> (--prompt <text> | --prompt-file <path>) \
//	    --git-author-name <name> --git-author-email <email> [flags]
//
// The process exit code reports the outcome: 0 success, 1 run or publish
// failure, 2 preflight/usage error, 3 success with no commits, 130
// interrupted.
package main

import (
	"os"

	"github.com/altafard/ai-fleet/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
