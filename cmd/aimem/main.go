// Command aimem is a memory tool for AI agents.
package main

import (
	"os"

	"github.com/lendradev/aimem/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
