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
	args := filterPlatformArgs(runtime.GOOS, os.Args[1:])
	if len(args) == 0 {
		return false, 0
	}
	return true, cli.Main(args)
}

func filterPlatformArgs(goos string, args []string) []string {
	if goos != "darwin" {
		return args
	}
	filtered := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-psn_") {
			continue
		}
		if strings.HasPrefix(arg, "-NS") || strings.HasPrefix(arg, "-Apple") {
			if !strings.Contains(arg, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
			}
			continue
		}
		filtered = append(filtered, arg)
	}
	return filtered
}
