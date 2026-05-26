package main

import (
	"os"

	"github.com/kaufmann-dev/git-wrangler/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
