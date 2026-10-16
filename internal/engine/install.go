package engine

import (
	"debug/buildinfo"
	"fmt"
	"path/filepath"
	"runtime"
)

func CLIName() string {
	if runtime.GOOS == "windows" {
		return "yourun.exe"
	}
	return "yourun"
}

// Run the installed program in place. Starting a command must not install or
// copy executable code into the user's data directory.
func resolveExecutor(source string) (string, error) {
	source, err := filepath.Abs(source)
	if err != nil {
		return "", err
	}
	info, err := buildinfo.ReadFile(source)
	if err != nil {
		return "", fmt.Errorf("read YouRun executable: %w", err)
	}
	switch info.Path {
	case "commandmanager.local/core/cmd/manager", "commandmanager.local/mygo", "commandmanager.local/core/native", "commandmanager.local/wails":
		return source, nil
	default:
		return "", fmt.Errorf("invalid YouRun executable: %s", source)
	}
}
