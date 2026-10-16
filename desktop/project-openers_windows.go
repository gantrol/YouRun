package desktop

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

func discoverProjectOpeners(ctx context.Context) []ProjectOpener {
	installed := installedProjectEditors(ctx)
	result := make([]ProjectOpener, 0, len(projectEditors)+4)
	for _, editor := range projectEditors {
		if ctx.Err() != nil {
			break
		}
		path := commandPath(editor.executable)
		if path == "" {
			// Electron and Toolbox command shims live beside the real executable.
			if shim := commandPath(editor.command + ".cmd"); shim != "" {
				path = firstExecutable(filepath.Join(filepath.Dir(shim), editor.executable), filepath.Join(filepath.Dir(shim), "..", editor.executable), filepath.Join(filepath.Dir(shim), "..", "..", "..", editor.executable))
			}
		}
		if path == "" {
			path = registeredProjectExecutable(editor.executable)
		}
		if path == "" {
			path = installed[editor.id]
		}
		if path == "" {
			for _, root := range []string{filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs"), os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)")} {
				if root == "" {
					continue
				}
				path = firstExecutable(filepath.Join(root, editor.folder, editor.executable), filepath.Join(root, editor.folder, "bin", editor.executable))
				if path != "" {
					break
				}
			}
		}
		if path != "" {
			result = append(result, ProjectOpener{ID: editor.id, Name: editor.name, path: path})
		}
	}
	if path := visualStudioExecutable(ctx); path != "" {
		result = append(result, ProjectOpener{ID: "visual-studio", Name: "Visual Studio", path: path, kind: "visual-studio"})
	}
	powershell := firstExecutable(filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe"))
	if powershell != "" {
		if path := commandPath("codex.exe", "codex.cmd", "codex.ps1"); path != "" {
			result = append(result, ProjectOpener{ID: "codex-cli", Name: "Codex CLI", path: path, kind: "codex-cli"})
		}
		result = append(result, ProjectOpener{ID: "powershell", Name: "Windows PowerShell", path: powershell, kind: "terminal"})
	}
	if path := gitBashExecutable(); path != "" {
		result = append(result, ProjectOpener{ID: "git-bash", Name: "Git Bash", path: path, kind: "git-bash"})
	}
	return result
}

func registryText(key registry.Key, name string) string {
	value, kind, err := key.GetStringValue(name)
	if err != nil {
		return ""
	}
	if kind == registry.EXPAND_SZ {
		value, _ = registry.ExpandString(value)
	}
	return value
}

func registeredProjectExecutable(name string) string {
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
			key, err := registry.OpenKey(root, `Software\Microsoft\Windows\CurrentVersion\App Paths\`+name, registry.QUERY_VALUE|view)
			if err != nil {
				continue
			}
			path := unquoteExecutable(registryText(key, ""))
			key.Close()
			if executableFile(path) && strings.EqualFold(filepath.Base(path), name) {
				return path
			}
		}
	}
	return ""
}

func installedProjectEditors(ctx context.Context) map[string]string {
	result := map[string]string{}
	for _, root := range []registry.Key{registry.CURRENT_USER, registry.LOCAL_MACHINE} {
		for _, view := range []uint32{registry.WOW64_64KEY, registry.WOW64_32KEY} {
			parent, err := registry.OpenKey(root, `Software\Microsoft\Windows\CurrentVersion\Uninstall`, registry.ENUMERATE_SUB_KEYS|view)
			if err != nil {
				continue
			}
			names, _ := parent.ReadSubKeyNames(-1)
			for _, name := range names {
				if ctx.Err() != nil {
					break
				}
				key, err := registry.OpenKey(parent, name, registry.QUERY_VALUE|view)
				if err != nil {
					continue
				}
				display, publisher := strings.ToLower(registryText(key, "DisplayName")), strings.ToLower(registryText(key, "Publisher"))
				location, icon := unquoteExecutable(registryText(key, "InstallLocation")), registryText(key, "DisplayIcon")
				key.Close()
				for _, editor := range projectEditors {
					if result[editor.id] != "" || !strings.Contains(display, strings.ToLower(editor.name)) {
						continue
					}
					if editor.id == "vscode" && strings.Contains(display, "insiders") {
						continue
					}
					if !strings.Contains(publisher, strings.ToLower(editor.publisher)) && !(editor.id == "cursor" && strings.Contains(publisher, "cursor")) {
						continue
					}
					iconPath := projectIconExecutable(icon)
					if !strings.EqualFold(filepath.Base(iconPath), editor.executable) && !strings.EqualFold(filepath.Base(iconPath), strings.Replace(editor.executable, "64.exe", ".exe", 1)) {
						iconPath = ""
					}
					result[editor.id] = firstExecutable(iconPath, filepath.Join(location, editor.executable), filepath.Join(location, "bin", editor.executable), filepath.Join(location, "bin", strings.Replace(editor.executable, "64.exe", ".exe", 1)))
				}
			}
			parent.Close()
		}
	}
	return result
}

func projectIconExecutable(icon string) string {
	icon = strings.TrimSpace(icon)
	// Strip only a numeric resource index, preserving commas inside paths.
	if comma := strings.LastIndexByte(icon, ','); comma >= 0 {
		if _, err := strconv.Atoi(strings.TrimSpace(icon[comma+1:])); err == nil {
			icon = icon[:comma]
		}
	}
	return unquoteExecutable(icon)
}

func visualStudioExecutable(ctx context.Context) string {
	locator := firstExecutable(filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft Visual Studio", "Installer", "vswhere.exe"))
	if locator == "" {
		return registeredProjectExecutable("devenv.exe")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, locator, "-latest", "-property", "productPath", "-utf8")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	data, err := cmd.Output()
	if err == nil {
		path := strings.TrimPrefix(strings.TrimSpace(string(data)), "\ufeff")
		if strings.EqualFold(filepath.Base(path), "devenv.exe") && executableFile(path) {
			return path
		}
	}
	return registeredProjectExecutable("devenv.exe")
}

func gitBashExecutable() string {
	if git := commandPath("git.exe"); git != "" {
		if path := firstExecutable(filepath.Join(filepath.Dir(git), "..", "git-bash.exe")); path != "" {
			return path
		}
	}
	return firstExecutable(filepath.Join(os.Getenv("ProgramFiles"), "Git", "git-bash.exe"), filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "Git", "git-bash.exe"))
}

func customProjectExecutable(path string) (string, error) {
	path = unquoteExecutable(path)
	if !strings.EqualFold(filepath.Ext(path), ".exe") || !executableFile(path) {
		return "", errors.New("invalid executable")
	}
	return filepath.Clean(path), nil
}

func launchProjectOpener(opener ProjectOpener, directory string) error {
	args := []string{directory}
	path := opener.path
	console := false
	if opener.kind == "" {
		switch strings.ToLower(filepath.Base(path)) {
		case "devenv.exe":
			opener.kind = "visual-studio"
		case "codex.exe":
			opener.kind = "codex-cli"
		}
	}
	switch opener.kind {
	case "visual-studio":
		args = []string{"/Edit", directory}
	case "terminal":
		args, console = []string{"-NoLogo", "-NoProfile", "-NoExit"}, true
	case "git-bash":
		args = []string{"--cd=" + directory}
	case "codex-cli":
		// The project is passed as the process working directory, never shell text.
		// PowerShell can run npm's .cmd/.ps1 shims as well as native Codex binaries.
		script := "& '" + strings.ReplaceAll(path, "'", "''") + "'"
		units := utf16.Encode([]rune(script))
		data := make([]byte, len(units)*2)
		for i, unit := range units {
			binary.LittleEndian.PutUint16(data[2*i:], unit)
		}
		path = filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
		args, console = []string{"-NoLogo", "-NoProfile", "-NoExit", "-EncodedCommand", base64.StdEncoding.EncodeToString(data)}, true
	}
	if console {
		// Shell activation provides an interactive console. os/exec would pass
		// NUL standard handles from the GUI host to a new PowerShell console.
		file, err := windows.UTF16PtrFromString(path)
		if err != nil {
			return err
		}
		cwd, err := windows.UTF16PtrFromString(directory)
		if err != nil {
			return err
		}
		quoted := make([]string, len(args))
		for i, arg := range args {
			quoted[i] = syscall.EscapeArg(arg)
		}
		parameters, err := windows.UTF16PtrFromString(strings.Join(quoted, " "))
		if err != nil {
			return err
		}
		return windows.ShellExecute(0, nil, file, parameters, cwd, windows.SW_SHOWNORMAL)
	}
	cmd := exec.Command(path, args...)
	cmd.Dir = directory
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	return startProjectOpener(cmd)
}
