package desktop

import (
	"os"
	"runtime"
	"strings"

	"commandmanager.local/core/internal/cli"
)

// CommandLine dispatches before GUI, database, HTTP host or update startup.
// Misspelled commands must fail through the CLI instead of opening a window.
func CommandLine() (handled bool, exitCode int) {
	args := os.Args[1:]
	if runtime.GOOS == "darwin" {
		filtered := make([]string, 0, len(args))
		for _, arg := range args {
			if !strings.HasPrefix(arg, "-psn_") {
				filtered = append(filtered, arg)
			}
		}
		args = filtered
	}
	if len(args) == 0 {
		return false, 0
	}
	return true, cli.Main(args)
}
