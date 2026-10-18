//go:build !windows

package cli

import (
	"os"
	"path/filepath"
	"strings"
)

func checkUserPath(binDir string) (configured bool, pathLocation string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, "~/.local/bin/yourun"
	}
	linkPath := filepath.Join(home, ".local", "bin", "yourun")
	if target, err := os.Readlink(linkPath); err == nil && strings.TrimSpace(target) != "" {
		if binDir == "" || filepath.Clean(filepath.Dir(target)) == filepath.Clean(binDir) {
			return true, linkPath
		}
	}
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		if binDir != "" && filepath.Clean(p) == filepath.Clean(binDir) {
			return true, p
		}
	}
	return false, linkPath
}

func updateUserPath(enabled bool, binDir string, state *integrationReceipt) (configured bool, pathLocation string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return false, "~/.local/bin/yourun", nil
	}
	localBin := filepath.Join(home, ".local", "bin")
	linkPath := filepath.Join(localBin, "yourun")
	pathLocation = linkPath
	if enabled {
		if strings.TrimSpace(binDir) == "" {
			return false, linkPath, nil
		}
		target := filepath.Join(binDir, "yourun")
		if _, statErr := os.Stat(target); statErr != nil {
			return false, linkPath, nil
		}
		if err := os.MkdirAll(localBin, 0o755); err != nil {
			return false, linkPath, err
		}
		_ = os.Remove(linkPath)
		if err := os.Symlink(target, linkPath); err != nil {
			return false, linkPath, err
		}
		if state != nil {
			state.AddedPathDir = linkPath
		}
		return true, linkPath, nil
	}
	if state != nil && state.AddedPathDir != "" {
		_ = os.Remove(state.AddedPathDir)
		state.AddedPathDir = ""
	}
	return false, linkPath, nil
}
