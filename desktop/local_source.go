package desktop

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"commandmanager.local/core/internal/database"
	"commandmanager.local/core/internal/updater"
)

type localSource struct {
	Directory string `json:"directory"`
}

// Build metadata stays beside local executables and outside release archives.
// A selected source is remembered separately from the signed release settings.
func (m *updateManager) loadLocalSource() {
	store, err := database.Open(m.host.dataDir)
	if err != nil {
		return
	}
	var source localSource
	err = store.ReadSetting(context.Background(), "local-source/"+m.framework, &source)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return
	}
	if source.Directory != "" {
		if filepath.IsAbs(source.Directory) && !strings.Contains(strings.ToLower(source.Directory), "trash") {
			m.state.LocalSource = source.Directory
		}
		return
	}
	if updater.Development() {
		if source := developmentSource(m.framework); source != "" {
			m.state.LocalSource = source
			return
		}
	}
	{
		exe, err := os.Executable()
		if err != nil {
			return
		}
		directory := filepath.Dir(exe)
		if runtime.GOOS == "darwin" {
			directory = filepath.Clean(filepath.Join(directory, "../../.."))
		}
		_ = updater.ReadJSON(filepath.Join(directory, "local-source-"+m.framework+".json"), &source)
	}
	if filepath.IsAbs(source.Directory) && !strings.Contains(strings.ToLower(source.Directory), "trash") {
		m.state.LocalSource = source.Directory
	}
}

func validateLocalSource(directory, framework string) (string, error) {
	fail := errors.New("updateLocalSourceInvalid")
	if !filepath.IsAbs(directory) || strings.Contains(strings.ToLower(directory), "trash") {
		return "", fail
	}
	directory, err := filepath.EvalSymlinks(directory)
	if err != nil || strings.Contains(strings.ToLower(directory), "trash") {
		return "", fail
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return "", fail
	}
	defer root.Close()
	for _, name := range []string{"build.mjs", "go.mod", "frontend/package.json", framework + "/go.mod"} {
		info, err := root.Lstat(name)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 1<<20 {
			return "", fail
		}
	}
	f, err := root.Open("go.mod")
	if err != nil {
		return "", fail
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 1<<20))
	if err != nil {
		return "", fail
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.TrimSpace(line) == "module commandmanager.local/core" {
			return directory, nil
		}
	}
	return "", fail
}

func (m *updateManager) chooseLocalSource(r *http.Request) error {
	m.mu.Lock()
	if m.cancel != nil || m.candidate || m.state.Phase == "switching" || m.state.Phase == "complete" {
		m.mu.Unlock()
		return errors.New("updateBusy")
	}
	m.host.pickerMu.RLock()
	picker := m.host.chooseDirectory
	m.host.pickerMu.RUnlock()
	if picker == nil || !m.host.choosing.CompareAndSwap(false, true) {
		m.mu.Unlock()
		return errors.New("updateBusy")
	}
	ctx, cancel := context.WithCancel(r.Context())
	m.cancel = cancel
	m.mu.Unlock()
	defer cancel()
	defer m.host.choosing.Store(false)
	defer func() { m.mu.Lock(); m.cancel = nil; m.mu.Unlock() }()
	directory, err := picker()
	if err != nil {
		return errors.New("updateLocalSourceInvalid")
	}
	if directory == "" {
		return nil
	}
	directory, err = validateLocalSource(directory, m.framework)
	if err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	store, err := database.Open(m.host.dataDir)
	if err != nil || store.WriteSetting(ctx, "local-source/"+m.framework, localSource{directory}) != nil {
		return errors.New("updateStorageFailed")
	}
	m.mu.Lock()
	m.state.LocalSource = directory
	m.state.LocalRestart = false
	m.mu.Unlock()
	return nil
}
