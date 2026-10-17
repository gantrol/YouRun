package engine

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func shellCommand(c Command, request Launch) (*exec.Cmd, string, error) {
	environment, pathDirs := commandEnvironment()
	shell := resolveShell(c, pathDirs)
	text := adaptCommandText(c.Text, request.Directory, shell)
	for _, field := range c.Fields {
		value := request.Values[field]
		if strings.TrimSpace(value) == "" || len(value) > 8192 || strings.ContainsRune(value, 0) {
			return nil, "", errors.New("invalidInput")
		}
		// Expand an environment variable, never splice user input into shell syntax.
		name := "COMMAND_MANAGER_" + strings.ToUpper(field)
		environment = append(environment, name+"="+value)
		quoted := "\"${" + name + "}\""
		if shell == "PowerShell" {
			quoted = "$env:" + name
		}
		if shell == "cmd" {
			return nil, "", errors.New("cmdParametersUnsupported")
		}
		placeholder := "{{" + field + "}}"
		if !strings.Contains(text, placeholder) {
			return nil, "", errors.New("missingPlaceholder")
		}
		text = strings.ReplaceAll(text, placeholder, quoted)
	}
	var cmd *exec.Cmd
	switch shell {
	case "PowerShell":
		exe := lookCommand("pwsh", pathDirs)
		if exe == "" {
			exe = lookCommand("powershell.exe", pathDirs)
		}
		if exe == "" {
			return nil, "", errors.New("shellMissing")
		}
		prefix := "$ErrorActionPreference='Stop'; [Console]::OutputEncoding=[System.Text.UTF8Encoding]::new(); $OutputEncoding=[Console]::OutputEncoding; "
		cmd = exec.Command(exe, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", prefix+text+"\nif ($null -ne $LASTEXITCODE) { exit $LASTEXITCODE }")
	case "zsh", "bash":
		exe := lookCommand(shell, pathDirs)
		if exe == "" {
			return nil, "", errors.New("shellMissing")
		}
		cmd = exec.Command(exe, "-l", "-c", text)
	case "cmd":
		if runtime.GOOS != "windows" {
			return nil, "", errors.New("shellMissing")
		}
		cmd = exec.Command("cmd.exe", "/d", "/s", "/c", text)
	default:
		return nil, "", errors.New("shellMissing")
	}
	cmd.Dir = request.Directory
	cmd.Env = environment
	prepareChild(cmd)
	return cmd, text, nil
}

func resolveShell(c Command, pathDirs []string) string {
	shell := c.Shell
	crossPlatform := c.Platform == "" || c.Platform == "macOS / Windows"
	if shell == "default" {
		if runtime.GOOS == "windows" {
			return "PowerShell"
		}
		return defaultUnixShell(pathDirs)
	}
	if crossPlatform {
		if runtime.GOOS == "windows" && (shell == "zsh" || shell == "bash") && lookCommand(shell, pathDirs) == "" {
			return "PowerShell"
		}
		if runtime.GOOS != "windows" && shell == "PowerShell" && lookCommand("pwsh", pathDirs) == "" {
			return defaultUnixShell(pathDirs)
		}
	}
	if runtime.GOOS != "windows" && shell == "zsh" && lookCommand("zsh", pathDirs) == "" && lookCommand("bash", pathDirs) != "" {
		return "bash"
	}
	return shell
}

func defaultUnixShell(pathDirs []string) string {
	if current := filepath.Base(strings.TrimSpace(os.Getenv("SHELL"))); current == "zsh" || current == "bash" {
		if lookCommand(current, pathDirs) != "" {
			return current
		}
	}
	if lookCommand("zsh", pathDirs) != "" {
		return "zsh"
	}
	if lookCommand("bash", pathDirs) != "" {
		return "bash"
	}
	return "zsh"
}

func lookCommand(name string, dirs []string) string {
	if path, err := exec.LookPath(name); err == nil && isExecutableFile(path) {
		return path
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		candidate := filepath.Join(dir, name)
		if isExecutableFile(candidate) {
			return filepath.Clean(candidate)
		}
	}
	return ""
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode().Perm()&0111 != 0)
}

func commandEnvironment() ([]string, []string) {
	environment := []string{}
	pathValue := ""
	for _, entry := range os.Environ() {
		key, val, _ := strings.Cut(entry, "=")
		upper := strings.ToUpper(key)
		if strings.HasPrefix(upper, "COMMAND_MANAGER_UPDATE_") || upper == "MYGO_GENERATE" {
			continue
		}
		if upper == "PATH" {
			pathValue = val
			if runtime.GOOS != "windows" {
				continue
			}
		}
		environment = append(environment, entry)
	}
	parts := filepath.SplitList(pathValue)
	if runtime.GOOS != "windows" {
		seen := map[string]bool{}
		merged := make([]string, 0, len(parts)+16)
		for _, p := range parts {
			if p != "" && !seen[p] {
				seen[p] = true
				merged = append(merged, p)
			}
		}
		for _, dir := range unixDeveloperDirectories() {
			if dir != "" && !seen[dir] {
				seen[dir] = true
				merged = append(merged, dir)
			}
		}
		parts = merged
		environment = append(environment, "PATH="+strings.Join(parts, string(os.PathListSeparator)))
	}
	return environment, parts
}

func unixDeveloperDirectories() []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	dirs := []string{"/opt/homebrew/bin", "/opt/homebrew/sbin", "/usr/local/bin", "/usr/local/sbin", "/usr/local/go/bin", "/snap/bin", "/bin", "/usr/bin"}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		for _, rel := range []string{
			".local/bin",
			".volta/bin",
			".bun/bin",
			".cargo/bin",
			".asdf/shims",
			".local/share/mise/shims",
			".pyenv/shims",
			"Library/Application Support/fnm/aliases/default/bin",
			".fnm/aliases/default/bin",
			"go/bin",
			".sdkman/candidates/gradle/current/bin",
			".sdkman/candidates/maven/current/bin",
			".sdkman/candidates/java/current/bin",
		} {
			dirs = append(dirs, filepath.Join(home, filepath.FromSlash(rel)))
		}
		if entries, err := os.ReadDir(filepath.Join(home, ".nvm", "versions", "node")); err == nil {
			for i := len(entries) - 1; i >= 0; i-- {
				if entries[i].IsDir() {
					dirs = append(dirs, filepath.Join(home, ".nvm", "versions", "node", entries[i].Name(), "bin"))
				}
			}
		}
	}
	return dirs
}

func adaptCommandText(text, directory, shell string) string {
	trimmed := strings.TrimLeft(text, " \t")
	leading := text[:len(text)-len(trimmed)]
	token, rest := trimmed, ""
	if idx := strings.IndexAny(trimmed, " \t\r\n"); idx >= 0 {
		token, rest = trimmed[:idx], trimmed[idx:]
	}
	if directory != "" {
		normalized := strings.ReplaceAll(token, "\\", "/")
		if runtime.GOOS == "windows" {
			switch normalized {
			case "./gradlew":
				if info, err := os.Stat(filepath.Join(directory, "gradlew.bat")); err == nil && info.Mode().IsRegular() {
					token = "./gradlew.bat"
				}
			case "./mvnw":
				if info, err := os.Stat(filepath.Join(directory, "mvnw.cmd")); err == nil && info.Mode().IsRegular() {
					token = "./mvnw.cmd"
				}
			}
		} else {
			switch strings.ToLower(normalized) {
			case "./gradlew.bat":
				if info, err := os.Stat(filepath.Join(directory, "gradlew")); err == nil && info.Mode().IsRegular() {
					token = "./gradlew"
				}
			case "./mvnw.cmd":
				if info, err := os.Stat(filepath.Join(directory, "mvnw")); err == nil && info.Mode().IsRegular() {
					token = "./mvnw"
				}
			}
		}
	}
	if shell == "cmd" && strings.HasPrefix(token, "./") {
		token = strings.ReplaceAll(token, "/", `\`)
	}
	return leading + token + rest
}
