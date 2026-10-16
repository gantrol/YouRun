package desktop

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"commandmanager.local/core/internal/engine"
	"commandmanager.local/core/internal/updater"
	"github.com/gofrs/flock"
)

type handoff struct {
	token       string
	resume      json.RawMessage
	ready, done chan struct{}
	ack         chan struct{}
	once        sync.Once
	ackOnce     sync.Once
	err         error
	previous    updatePointer
	process     *os.Process
}

func managedVersion(root, directory string) bool {
	a, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	b, err := filepath.Abs(directory)
	if err != nil {
		return false
	}
	return filepath.Dir(b) == filepath.Join(a, "versions") && strings.HasPrefix(filepath.Base(b), "release-")
}
func applicationEnvironment(dataDir string, extra ...string) []string {
	env := []string{}
	pathValue := ""
	for _, item := range os.Environ() {
		key, val, _ := strings.Cut(item, "=")
		upper := strings.ToUpper(key)
		if upper == "COMMAND_MANAGER_HOME" || strings.HasPrefix(upper, "COMMAND_MANAGER_UPDATE_") {
			continue
		}
		if runtime.GOOS != "windows" && upper == "PATH" {
			pathValue = val
			continue
		}
		env = append(env, item)
	}
	if runtime.GOOS != "windows" {
		parts := filepath.SplitList(pathValue)
		seen := map[string]bool{}
		for _, p := range parts {
			if p != "" {
				seen[p] = true
			}
		}
		for _, dir := range unixToolDirectories() {
			if !seen[dir] {
				parts = append(parts, dir)
				seen[dir] = true
			}
		}
		env = append(env, "PATH="+strings.Join(parts, string(os.PathListSeparator)))
	}
	return append(append(env, "COMMAND_MANAGER_HOME="+dataDir), extra...)
}

// Bootstrap preserves the user's existing shortcut while immutable versions live beside it.
// The release lease stays held until the GUI exits; cleanup requires an exclusive lease.
func Bootstrap(framework string) (redirected bool, closeLease func(), err error) {
	closeLease = func() {}
	if !applicationUpdatesEnabled || updater.StoreManaged() || updater.Development() {
		return false, closeLease, nil
	}
	dir, err := engine.DataDir()
	if err != nil {
		return false, closeLease, err
	}
	root := filepath.Join(dir, "updates", framework)
	exe, err := os.Executable()
	if err != nil {
		return false, closeLease, err
	}
	if os.Getenv("COMMAND_MANAGER_UPDATE_PARENT") == "" && os.Getenv("COMMAND_MANAGER_UPDATE_REDIRECT") == "" {
		var pointer updatePointer
		if updater.ReadJSON(filepath.Join(root, "current.json"), &pointer) == nil && pointer.Current != nil {
			target := pointer.Current
			current, _ := strconv.ParseInt(updater.Build, 10, 64)
			if managedVersion(root, target.Directory) && target.Release.Build > current && target.Release.Platform == runtime.GOOS && target.Release.Arch == runtime.GOARCH && target.Release.Framework == framework && target.Release.ProtocolMin <= engine.Protocol && target.Release.ProtocolMax >= engine.Protocol {
				lease := flock.New(filepath.Join(root, filepath.Base(target.Directory)+".lease"))
				ok, lockErr := lease.TryRLock()
				if lockErr == nil && ok && updater.Verify(target) == nil {
					cmd := exec.Command(filepath.Join(target.Directory, filepath.FromSlash(target.Release.Executable())))
					cmd.Dir = target.Directory
					cmd.Env = applicationEnvironment(dir, "COMMAND_MANAGER_UPDATE_REDIRECT=1")
					updater.Detached(cmd)
					if cmd.Start() == nil {
						cmd.Process.Release()
						lease.Close()
						return true, closeLease, nil
					}
				}
				lease.Close()
			}
		}
	}
	rel, relErr := filepath.Rel(filepath.Join(root, "versions"), exe)
	parts := strings.Split(rel, string(filepath.Separator))
	if relErr == nil && filepath.IsLocal(rel) && len(parts) > 1 && strings.HasPrefix(parts[0], "release-") {
		lease := flock.New(filepath.Join(root, parts[0]+".lease"))
		ok, lockErr := lease.TryRLock()
		if lockErr != nil || !ok {
			return false, closeLease, errors.New("updateBusy")
		}
		closeLease = func() { lease.Close() }
	}
	return false, closeLease, nil
}

func (h *Host) UpdatePending() bool { return h.updates.blocked() }

func (m *updateManager) apply(resume json.RawMessage) error {
	if updater.StoreManaged() {
		return errors.New("updateStoreManaged")
	}
	m.mu.Lock()
	if m.cancel != nil || m.ready == nil || m.state.Phase != "ready" || m.candidate || m.quit == nil {
		m.mu.Unlock()
		return errors.New("updateBusy")
	}
	lock := flock.New(filepath.Join(m.root, "install.lock"))
	ok, err := lock.TryLock()
	if err != nil || !ok {
		m.mu.Unlock()
		lock.Close()
		return errors.New("updateBusy")
	}
	staged := m.ready
	transaction := &handoff{token: engine.ID(), resume: append(json.RawMessage{}, resume...), ready: make(chan struct{}), done: make(chan struct{}), ack: make(chan struct{})}
	m.transaction, m.state.Phase, m.state.Error = transaction, "switching", ""
	m.state.LocalRestart = false
	quit := m.quit
	m.mu.Unlock()
	go func() {
		defer lock.Close()
		err := m.switchWindow(staged, transaction)
		transaction.err = err
		close(transaction.done)
		if err == nil {
			select {
			case <-transaction.ack:
			case <-time.After(8 * time.Second):
				err = errors.New("updateHandoffFailed")
				m.mu.Lock()
				m.transaction = nil
				m.mu.Unlock()
				if restoreErr := updater.WriteJSON(filepath.Join(m.root, "current.json"), transaction.previous); restoreErr != nil {
					err = errors.New("updateStorageFailed")
				}
				if transaction.process != nil {
					_ = transaction.process.Kill()
				}
			}
		}
		m.mu.Lock()
		if err != nil {
			m.state.Phase, m.state.Error = "failed", updateError(err)
		} else {
			m.state.Phase = "complete"
		}
		m.mu.Unlock()
		if err == nil {
			// Give the authenticated readiness response time to reach the new window.
			time.AfterFunc(500*time.Millisecond, quit)
		}
	}()
	return nil
}
func (m *updateManager) switchWindow(staged *updater.Staged, transaction *handoff) error {
	if !managedVersion(m.root, staged.Directory) || updater.Verify(staged) != nil {
		return errors.New("updateHashFailed")
	}
	var old updatePointer
	if err := updater.ReadJSON(filepath.Join(m.root, "current.json"), &old); err != nil && !os.IsNotExist(err) {
		return errors.New("updateStorageFailed")
	}
	if old.Current != nil && old.Current.Release.Build >= staged.Release.Build {
		return errors.New("updateNoNewVersion")
	}
	transaction.previous = old
	// Both windows can inspect the same versioned execution records. Neither
	// owns an executing helper, and GUI updates never replace helper binaries.
	if _, err := m.host.client.State(); err != nil {
		return errors.New("updateIncompatible")
	}
	cmd := exec.Command(filepath.Join(staged.Directory, filepath.FromSlash(staged.Release.Executable())))
	cmd.Dir = staged.Directory
	parent := strings.Split(m.host.URL, "/#")[0] + "/api/update-handoff"
	cmd.Env = applicationEnvironment(m.host.dataDir, "COMMAND_MANAGER_UPDATE_PARENT="+parent, "COMMAND_MANAGER_UPDATE_TOKEN="+transaction.token)
	updater.Detached(cmd)
	log, err := os.OpenFile(filepath.Join(m.root, "candidate-start.log"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("updateStorageFailed")
	}
	defer log.Close()
	cmd.Stdout, cmd.Stderr = log, log
	if err = cmd.Start(); err != nil {
		return errors.New("updateStartFailed")
	}
	transaction.process = cmd.Process
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	accepted := false
	defer func() {
		if !accepted {
			_ = cmd.Process.Kill()
		}
	}() // Only this candidate GUI, never the runner or managed programs.
	timer := time.NewTimer(45 * time.Second)
	defer timer.Stop()
	select {
	case <-exited:
		return errors.New("updateStartFailed")
	case <-timer.C:
		return errors.New("updateHandoffFailed")
	case <-transaction.ready:
	}
	// Readiness follows session restoration and the first React mount or native UI frame.
	grace := time.NewTimer(2 * time.Second)
	defer grace.Stop()
	select {
	case <-exited:
		return errors.New("updateStartFailed")
	case <-grace.C:
	}
	if _, err := m.host.client.State(); err != nil {
		return errors.New("updateHandoffFailed")
	}
	if err = updater.WriteJSON(filepath.Join(m.root, "current.json"), updatePointer{Current: staged, Previous: old.Current}); err != nil {
		return errors.New("updateStorageFailed")
	}
	accepted = true
	return nil
}
