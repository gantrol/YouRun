// Optional standalone CLI for development and headless deployments. Desktop
// packages invoke the same command layer from their application executable.
package main

import (
	"os"

	"commandmanager.local/core/desktop"
	"commandmanager.local/core/internal/cli"
)

func main() {
	desktop.RegisterCLIResourceHandler()
	code := cli.Main(os.Args[1:])
	desktop.CloseResourceMetrics()
	os.Exit(code)
}
