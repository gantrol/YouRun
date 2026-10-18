package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"commandmanager.local/core/internal/cli"
	"commandmanager.local/core/internal/workspace"
)

// CommandLine dispatches before GUI, database, HTTP host or update startup.
// Misspelled commands must fail through the CLI instead of opening a window.
func CommandLine() (handled bool, exitCode int) {
	args := filterPlatformArgs(runtime.GOOS, os.Args[1:])
	if len(args) == 0 {
		return false, 0
	}
	RegisterCLIResourceHandler()
	return true, cli.Main(args)
}

// RegisterCLIResourceHandler wires OS process, port, and file handle queries into the CLI and MCP server.
func RegisterCLIResourceHandler() {
	cli.ResourceHandler = func(ctx context.Context, mode string, dataDir string, ws *workspace.Service, targetPath string, recursive bool) (any, error) {
		qCtx, cancel := context.WithTimeout(WithSkipProcessIcons(ctx, !cli.ShouldIncludeImages(ctx)), 8*time.Second)
		defer cancel()
		switch mode {
		case "processes":
			return QueryProcesses(qCtx, dataDir, ws)
		case "ports":
			return QueryPorts(qCtx, dataDir, ws)
		case "files":
			trimmed := strings.TrimSpace(targetPath)
			if trimmed == "" || strings.Contains(strings.ToLower(trimmed), "trash") {
				return nil, errors.New("invalidDirectory")
			}
			absPath, err := filepath.Abs(trimmed)
			if err != nil {
				return nil, errors.New("invalidDirectory")
			}
			return QueryFiles(qCtx, dataDir, ws, filepath.Clean(absPath), recursive)
		default:
			return nil, errors.New("unknownResourceMode")
		}
	}
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
