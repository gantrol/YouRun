//go:build !windows

package desktop

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func discoverProjectOpeners(ctx context.Context) []ProjectOpener {
	result := make([]ProjectOpener, 0, len(projectEditors)+6)
	home, _ := os.UserHomeDir()
	roots := []string{"/Applications"}
	if home != "" {
		roots = append(roots, filepath.Join(home, "Applications"))
	}
	for _, editor := range projectEditors {
		if ctx.Err() != nil {
			break
		}
		opener := ProjectOpener{ID: editor.id, Name: editor.name, path: commandPath(editor.command)}
		if runtime.GOOS == "darwin" {
			if path := findDarwinBundle(roots,
				editor.name,
				strings.ReplaceAll(editor.name, " Insiders", " - Insiders"),
				editor.name+" CE",
				editor.name+" Ultimate",
				editor.name+" Professional Edition",
			); path != "" {
				opener.path, opener.kind = path, "app"
			}
		}
		if opener.path != "" {
			result = append(result, opener)
		}
	}
	if runtime.GOOS == "darwin" {
		if path := findDarwinBundle(roots, "Xcode"); path != "" {
			result = append(result, ProjectOpener{ID: "xcode", Name: "Xcode", path: path, kind: "app"})
		}
	}
	if path := commandPath("codex"); path != "" {
		result = append(result, ProjectOpener{ID: "codex-cli", Name: "Codex CLI", path: path, kind: "codex-cli"})
	}
	if runtime.GOOS == "darwin" {
		terminals := []struct {
			id, name string
			paths    []string
		}{
			{"terminal", "Terminal", []string{"/System/Applications/Utilities/Terminal.app", "/Applications/Utilities/Terminal.app"}},
			{"iterm", "iTerm", nil},
			{"warp", "Warp", nil},
			{"ghostty", "Ghostty", nil},
		}
		for _, t := range terminals {
			path := ""
			for _, candidate := range t.paths {
				if info, err := os.Stat(candidate); err == nil && info.IsDir() {
					path = candidate
					break
				}
			}
			if path == "" {
				path = findDarwinBundle(roots, t.name)
			}
			if path != "" {
				result = append(result, ProjectOpener{ID: t.id, Name: t.name, path: path, kind: "app"})
			}
		}
	} else {
		for _, t := range []struct{ id, name, command string }{
			{"ghostty", "Ghostty", "ghostty"},
			{"warp", "Warp", "warp-terminal"},
			{"kitty", "Kitty", "kitty"},
			{"alacritty", "Alacritty", "alacritty"},
			{"wezterm", "WezTerm", "wezterm"},
			{"terminal", "Terminal", "x-terminal-emulator"},
		} {
			if path := commandPath(t.command); path != "" {
				result = append(result, ProjectOpener{ID: t.id, Name: t.name, path: path, kind: "terminal"})
			}
		}
	}
	return result
}

func findDarwinBundle(roots []string, names ...string) string {
	for _, root := range roots {
		for _, name := range names {
			path := filepath.Join(root, name+".app")
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				return path
			}
		}
	}
	return ""
}

func customProjectExecutable(path string) (string, error) {
	path = unquoteExecutable(path)
	if !filepath.IsAbs(path) {
		return "", errors.New("invalid executable")
	}
	path = filepath.Clean(path)
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" && info.IsDir() && strings.HasSuffix(strings.ToLower(path), ".app") {
		return path, nil
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return "", errors.New("invalid executable")
	}
	return path, nil
}

func launchProjectOpener(opener ProjectOpener, directory string) error {
	if opener.kind == "" && strings.EqualFold(filepath.Base(opener.path), "codex") {
		opener.kind = "codex-cli"
	}
	cmd := exec.Command(opener.path, directory)
	if runtime.GOOS == "darwin" {
		if opener.kind == "codex-cli" {
			const script = `on run argv
tell application "Terminal"
	activate
	do script ("cd " & quoted form of item 1 of argv & " && exec " & quoted form of item 2 of argv)
end tell
end run`
			cmd = exec.Command("/usr/bin/osascript", "-e", script, directory, opener.path)
		} else if opener.kind == "app" || strings.HasSuffix(strings.ToLower(opener.path), ".app") {
			cmd = exec.Command("/usr/bin/open", "-a", opener.path, directory)
		}
	} else if opener.kind == "terminal" {
		cmd = exec.Command(opener.path)
	}
	cmd.Dir = directory
	return startProjectOpener(cmd)
}
