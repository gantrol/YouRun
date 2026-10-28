package desktop

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"
	"time"

	"commandmanager.local/core/internal/database"
	"commandmanager.local/core/internal/engine"
	"commandmanager.local/core/internal/updater"
	"github.com/gofrs/flock"
)

// Keep a bounded tail for diagnostics; builds run off the UI and HTTP threads.
type buildOutput struct {
	mu   sync.Mutex
	tail []byte
}

func (b *buildOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	const limit = 64 << 10
	n := len(p)
	if len(p) >= limit {
		b.tail = append(b.tail[:0], p[len(p)-limit:]...)
	} else {
		if excess := len(b.tail) + len(p) - limit; excess > 0 {
			b.tail = b.tail[excess:]
		}
		b.tail = append(b.tail, p...)
	}
	return n, nil
}

func (b *buildOutput) text() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.tail)
}

func (m *updateManager) rebuild() error {
	if updater.StoreManaged() {
		return errors.New("updateStoreManaged")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil || m.candidate || m.quit == nil || m.state.Phase == "switching" || m.state.Phase == "complete" {
		return errors.New("updateBusy")
	}
	if m.state.LocalSource == "" {
		return errors.New("updateLocalSourceInvalid")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	if m.readyLease != nil {
		m.readyLease.Close()
		m.readyLease = nil
	}
	m.cancel, m.ready = cancel, nil
	m.buildLog = &buildOutput{}
	m.state.Phase, m.state.Error, m.state.TargetVersion = "building", "", ""
	m.state.LocalRestart = false
	source, output := m.state.LocalSource, m.buildLog
	done := make(chan struct{})
	m.buildDone = done
	go func() {
		defer close(done)
		defer cancel()
		err := m.buildLocal(ctx, source, output)
		m.mu.Lock()
		defer m.mu.Unlock()
		m.cancel = nil
		if err == nil {
			err = ctx.Err()
		}
		if err != nil {
			fmt.Fprintln(output, err)
			m.state.Phase, m.state.Error = "failed", updateError(err)
			m.ready = nil
			if m.readyLease != nil {
				m.readyLease.Close()
				m.readyLease = nil
			}
			return
		}
		// The UI flushes current edits and captures a fresh session before apply.
		m.state.Phase, m.state.LocalRestart = "ready", true
	}()
	return nil
}

func (m *updateManager) buildLocal(ctx context.Context, source string, output *buildOutput) error {
	source, err := validateLocalSource(source, m.framework)
	if err != nil {
		return err
	}
	// The frontend output is shared across framework builds in this checkout.
	lock := flock.New(filepath.Join(source, ".local-build.lock"))
	ok, err := lock.TryLock()
	if err != nil || !ok {
		lock.Close()
		return errors.New("updateBusy")
	}
	defer lock.Close()
	store, err := database.Open(m.host.dataDir)
	if err != nil || store.WriteSetting(ctx, "local-source/"+m.framework, localSource{source}) != nil {
		return errors.New("updateStorageFailed")
	}
	build := time.Now().UnixMilli()
	current, _ := strconv.ParseInt(updater.Build, 10, 64)
	var pointer updatePointer
	if err := updater.ReadJSON(filepath.Join(m.root, "current.json"), &pointer); err == nil && pointer.Current != nil && pointer.Current.Release.Build > current {
		current = pointer.Current.Release.Build
	}
	if build <= current {
		build = current + 1
	}
	if build <= 0 {
		return errors.New("updateNoNewVersion")
	}
	buildID := strconv.FormatInt(build, 10)
	// Only a fixed build script in the selected checkout is executable here.
	// No renderer-provided command, archive, key or build arguments are accepted.
	node := commandPath("node")
	if node == "" {
		node = "node"
	}
	cmd := exec.Command(node, filepath.Join(source, "build.mjs"), "--framework="+m.framework,
		"--target="+runtime.GOOS, "--arch="+runtime.GOARCH, "--build="+buildID, "--package=false", "--manifest=true")
	cmd.Dir = source
	cmd.Env = applicationEnvironment(m.host.dataDir)
	if err = engine.Execute(ctx, cmd, output); err != nil {
		fmt.Fprintln(output, err)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("updateBuildFailed")
	}
	directory := filepath.Join(source, "dist", "builds", buildID, runtime.GOOS+"-"+runtime.GOARCH)
	// Trust is the explicitly selected local source, not a downloaded release.
	// Normal local-package and remote-feed signature checks remain unchanged.
	return m.prepareUpdate(ctx, func(root string) (*updater.Staged, error) {
		return updater.StageLocal(ctx, directory, root, m.framework, engine.Protocol, build)
	})
}
