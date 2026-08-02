package main

import (
	"os"

	"github.com/altafard/ai-fleet/internal/cli"
)

func main() {
	os.Exit(cli.Execute())
}
