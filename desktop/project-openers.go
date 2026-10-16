package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ProjectOpener exposes identities, never executable paths or shell arguments.
type ProjectOpener struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Icon string `json:"icon,omitempty"`
	path string
	kind string
}

type projectEditor struct {
	id, name, executable, command, folder, publisher string
}

var projectEditors = []projectEditor{
	{"vscode", "Visual Studio Code", "Code.exe", "code", "Microsoft VS Code", "Microsoft"},
	{"vscode-insiders", "Visual Studio Code Insiders", "Code - Insiders.exe", "code-insiders", "Microsoft VS Code Insiders", "Microsoft"},
	{"cursor", "Cursor", "Cursor.exe", "cursor", "cursor", "Anysphere"},
	{"zed", "Zed", "zed.exe", "zed", "Zed", "Zed"},
	{"antigravity", "Antigravity", "Antigravity.exe", "antigravity", "Antigravity", "Google"},
	{"windsurf", "Windsurf", "Windsurf.exe", "windsurf", "Windsurf", "Codeium"},
	{"pycharm", "PyCharm", "pycharm64.exe", "pycharm", "PyCharm", "JetBrains"},
	{"webstorm", "WebStorm", "webstorm64.exe", "webstorm", "WebStorm", "JetBrains"},
	{"idea", "IntelliJ IDEA", "idea64.exe", "idea", "IntelliJ IDEA", "JetBrains"},
	{"rider", "Rider", "rider64.exe", "rider", "Rider", "JetBrains"},
	{"sublime", "Sublime Text", "sublime_text.exe", "subl", "Sublime Text", "Sublime"},
}

func (h *Host) availableProjectOpeners(ctx context.Context, refresh bool) ([]ProjectOpener, error) {
	h.projectOpenersMu.Lock()
	defer h.projectOpenersMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !refresh && !h.projectOpenersAt.IsZero() && time.Since(h.projectOpenersAt) < time.Minute {
		return h.projectOpeners, nil
	}
	scanCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	openers := discoverProjectOpeners(scanCtx)
	if err := scanCtx.Err(); err != nil {
		return nil, err
	}
	loadProjectOpenerIcons(ctx, openers)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	h.projectOpeners, h.projectOpenersAt = openers, time.Now()
	return openers, nil
}

func (h *Host) serveProjectOpeners(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalidRequest"})
		return
	}
	openers, err := h.availableProjectOpeners(r.Context(), r.URL.Query().Get("refresh") == "1")
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "editorDetectionFailed"})
		return
	}
	json.NewEncoder(w).Encode(openers)
}

func (h *Host) openProjectWith(ctx context.Context, directory, id, executable string) error {
	if executable != "" {
		path, err := customProjectExecutable(executable)
		if err != nil {
			return errors.New("invalidEditorExecutable")
		}
		if err := launchProjectOpener(ProjectOpener{path: path}, directory); err != nil {
			return errors.New("editorOpenFailed")
		}
		return nil
	}
	openers, err := h.availableProjectOpeners(ctx, false)
	if err != nil {
		return errors.New("editorDetectionFailed")
	}
	for _, opener := range openers {
		if opener.ID != id {
			continue
		}
		if _, err := os.Stat(opener.path); err != nil {
			return errors.New("editorUnavailable")
		}
		if err := launchProjectOpener(opener, directory); err != nil {
			return errors.New("editorOpenFailed")
		}
		return nil
	}
	return errors.New("editorUnavailable")
}

func executableFile(path string) bool {
	if path == "" || !filepath.IsAbs(path) {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && (runtime.GOOS == "windows" || info.Mode().Perm()&0111 != 0)
}

func firstExecutable(paths ...string) string {
	for _, path := range paths {
		if executableFile(path) {
			return filepath.Clean(path)
		}
	}
	return ""
}

func commandPath(names ...string) string {
	dirs := unixToolDirectories()
	for _, name := range names {
		if path, err := exec.LookPath(name); err == nil && executableFile(path) {
			return path
		}
		for _, dir := range dirs {
			if path := filepath.Join(dir, name); executableFile(path) {
				return filepath.Clean(path)
			}
		}
	}
	return ""
}

func unixToolDirectories() []string {
	if runtime.GOOS == "windows" {
		return nil
	}
	dirs := []string{"/opt/homebrew/bin", "/opt/homebrew/sbin", "/usr/local/bin", "/usr/local/go/bin"}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		for _, rel := range []string{
			".local/bin",
			".volta/bin",
			".bun/bin",
			".cargo/bin",
			".asdf/shims",
			".local/share/mise/shims",
			"Library/Application Support/fnm/aliases/default/bin",
			".fnm/aliases/default/bin",
			"go/bin",
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

func unquoteExecutable(path string) string {
	return strings.Trim(strings.TrimSpace(path), "\"")
}

func startProjectOpener(cmd *exec.Cmd) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait()
	return nil
}
