package desktop

import (
	"context"
	"crypto/ed25519"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"commandmanager.local/core/internal/engine"
	"commandmanager.local/core/internal/updater"
	"github.com/gofrs/flock"
)

type updateConfig struct {
	Repository string `json:"repository"`
	PublicKey  string `json:"publicKey"`
}
type updateState struct {
	Disabled      bool         `json:"disabled"`
	StoreManaged  bool         `json:"storeManaged"`
	Phase         string       `json:"phase"`
	Version       string       `json:"version"`
	Build         string       `json:"build"`
	TargetVersion string       `json:"targetVersion,omitempty"`
	Downloaded    int64        `json:"downloaded"`
	Total         int64        `json:"total"`
	Error         string       `json:"error,omitempty"`
	Config        updateConfig `json:"config"`
	LocalSource   string       `json:"localSource,omitempty"`
	LocalRestart  bool         `json:"localRestart,omitempty"`
	BuildLog      string       `json:"buildLog,omitempty"`
}
type updatePointer struct {
	Current  *updater.Staged `json:"current"`
	Previous *updater.Staged `json:"previous,omitempty"`
}
type updateManager struct {
	mu                  sync.Mutex
	host                *Host
	framework, root     string
	state               updateState
	ready               *updater.Staged
	readyLease          *flock.Flock
	cancel              context.CancelFunc
	quit                func()
	pick                func() (string, error)
	resume              json.RawMessage
	parent, parentToken string
	candidate           bool
	transaction         *handoff
	buildLog            *buildOutput
	buildDone           <-chan struct{}
}

var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)

// MVP installs are replaced manually after their runs finish. Keep the older
// update implementation dormant without reading or changing its saved state.
const applicationUpdatesEnabled = false

func newUpdateManager(host *Host, framework string) (*updateManager, error) {
	if !applicationUpdatesEnabled {
		return &updateManager{host: host, framework: framework, state: updateState{
			Disabled: true, StoreManaged: updater.StoreManaged(), Phase: "idle", Version: updater.Version, Build: updater.Build,
		}}, nil
	}
	if updater.StoreManaged() {
		return &updateManager{host: host, framework: framework, state: updateState{
			StoreManaged: true, Phase: "idle", Version: updater.Version, Build: updater.Build,
		}}, nil
	}
	root := filepath.Join(host.dataDir, "updates", framework)
	if err := os.MkdirAll(filepath.Join(root, "versions"), 0700); err != nil {
		return nil, err
	}
	m := &updateManager{host: host, framework: framework, root: root, state: updateState{Phase: "idle", Version: updater.Version, Build: updater.Build, Config: updateConfig{Repository: updater.DefaultRepository, PublicKey: updater.DefaultPublicKey}}}
	if err := m.loadConfig(context.Background()); err != nil {
		return nil, err
	}
	m.loadLocalSource()
	m.parent, m.parentToken = os.Getenv("COMMAND_MANAGER_UPDATE_PARENT"), os.Getenv("COMMAND_MANAGER_UPDATE_TOKEN")
	m.candidate = m.parent != ""
	if m.candidate {
		if !validParent(m.parent) || len(m.parentToken) != 48 {
			return nil, errors.New("updateHandoffFailed")
		}
		body, err := m.parentRequest(http.MethodGet)
		if err != nil {
			return nil, err
		}
		m.resume = body
	}
	return m, nil
}
func (h *Host) SetUpdateControls(pick func() (string, error), quit func()) {
	h.updates.mu.Lock()
	h.updates.pick, h.updates.quit = pick, quit
	h.updates.mu.Unlock()
}
func (m *updateManager) snapshot() updateState {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.state
	if m.buildLog != nil {
		state.BuildLog = m.buildLog.text()
	}
	return state
}
func (m *updateManager) blocked() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.candidate || m.state.Phase == "switching" || m.state.Phase == "complete"
}
func (m *updateManager) finish(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cancel = nil
	if err != nil {
		m.state.Phase, m.state.Error = "failed", updateError(err)
	}
}
func updateError(err error) string {
	if err == nil {
		return ""
	}
	if err.Error() == "updateStoreManaged" || err.Error() == "updateDisabled" {
		return err.Error()
	}
	switch err.Error() {
	case "updateInvalidPackage", "updateTooLarge", "updateSignatureFailed", "updateHashFailed", "updateWrongPlatform", "updateIncompatible", "updateNoNewVersion", "updateSourceMissing", "updateBusy", "updateHandoffFailed", "updateStartFailed", "updateStorageFailed", "updateLocalSourceInvalid", "updateBuildFailed":
		return err.Error()
	}
	if errors.Is(err, context.Canceled) {
		return "updateCancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "updateTimedOut"
	}
	return "updateFailed"
}
func (m *updateManager) begin(local bool) error {
	if updater.StoreManaged() {
		return errors.New("updateStoreManaged")
	}
	m.mu.Lock()
	if err := m.loadConfig(context.Background()); err != nil {
		m.mu.Unlock()
		return err
	}
	if m.cancel != nil || m.state.Phase == "switching" || m.state.Phase == "complete" || m.candidate {
		m.mu.Unlock()
		return errors.New("updateBusy")
	}
	config, picker := m.state.Config, m.pick
	if !local && (config.Repository == "" || config.PublicKey == "") {
		m.mu.Unlock()
		return errors.New("updateSourceMissing")
	}
	if local && picker == nil {
		m.mu.Unlock()
		return errors.New("updateFailed")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	if m.readyLease != nil {
		m.readyLease.Close()
		m.readyLease = nil
	}
	m.cancel, m.ready = cancel, nil
	m.state.LocalRestart, m.buildLog = false, nil
	m.state.Phase, m.state.Error = "checking", ""
	m.state.Downloaded, m.state.Total, m.state.TargetVersion = 0, 0, ""
	m.mu.Unlock()
	go func() {
		defer cancel()
		var archive string
		var err error
		if local {
			archive, err = picker()
			if err == nil && archive == "" {
				err = context.Canceled
			}
		} else {
			archive, err = m.download(ctx, config)
			if archive != "" {
				defer os.Remove(archive)
			}
		}
		if err == nil {
			info, statErr := os.Stat(archive)
			if statErr != nil || !info.Mode().IsRegular() || info.Size() > updater.MaxArchive {
				err = errors.New("updateInvalidPackage")
			}
		}
		if err == nil {
			err = m.stage(ctx, archive, config.PublicKey)
		}
		if err != nil && err.Error() == "updateNoNewVersion" {
			m.mu.Lock()
			m.state.Phase = "current"
			m.mu.Unlock()
			err = nil
		}
		m.finish(err)
	}()
	return nil
}
func (m *updateManager) stage(ctx context.Context, archive, key string) error {
	return m.prepareUpdate(ctx, func(root string) (*updater.Staged, error) {
		return updater.Stage(ctx, archive, root, key, m.framework, engine.Protocol)
	})
}
func (m *updateManager) prepareUpdate(ctx context.Context, prepare func(string) (*updater.Staged, error)) error {
	lock := flock.New(filepath.Join(m.root, "install.lock"))
	ok, err := lock.TryLock()
	if err != nil || !ok {
		lock.Close()
		return errors.New("updateBusy")
	}
	defer lock.Close()
	staged, err := prepare(filepath.Join(m.root, "versions"))
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		updater.RemoveManaged(filepath.Join(m.root, "versions"), staged.Directory)
		return err
	}
	current, _ := strconv.ParseInt(updater.Build, 10, 64)
	if staged.Release.Build <= current {
		updater.RemoveManaged(filepath.Join(m.root, "versions"), staged.Directory)
		return errors.New("updateNoNewVersion")
	}
	lease := flock.New(filepath.Join(m.root, filepath.Base(staged.Directory)+".lease"))
	ok, err = lease.TryRLock()
	if err != nil || !ok {
		lease.Close()
		return errors.New("updateBusy")
	}
	m.mu.Lock()
	m.ready, m.readyLease = staged, lease
	if m.state.Phase != "building" {
		m.state.Phase = "ready"
	}
	m.state.TargetVersion = staged.Release.Version
	m.mu.Unlock()
	return nil
}
func secureHTTP() *http.Client {
	return &http.Client{Timeout: 15 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
		if r.URL.Scheme != "https" || r.URL.User != nil || len(via) >= 5 {
			return errors.New("updateFailed")
		}
		return nil
	}}
}
func getHTTPS(ctx context.Context, address string) (*http.Response, error) {
	u, err := url.Parse(address)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return nil, errors.New("updateFailed")
	}
	r, err := http.NewRequestWithContext(ctx, "GET", address, nil)
	if err != nil {
		return nil, err
	}
	r.Header.Set("User-Agent", "CommandManager-Updater/"+updater.Version)
	response, err := secureHTTP().Do(r)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != 200 {
		response.Body.Close()
		return nil, errors.New("updateFailed")
	}
	return response, nil
}
func (m *updateManager) download(ctx context.Context, config updateConfig) (string, error) {
	feed := "https://github.com/" + config.Repository + "/releases/latest/download/update-" + runtime.GOOS + "-" + runtime.GOARCH + "-" + m.framework + ".json"
	response, err := getHTTPS(ctx, feed)
	if err != nil {
		return "", err
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, updater.MaxMetadata+1))
	response.Body.Close()
	if err != nil {
		return "", err
	}
	var offer updater.Offer
	if err = updater.OpenEnvelope(body, config.PublicKey, &offer); err != nil {
		return "", err
	}
	hash, hashErr := hex.DecodeString(offer.SHA256)
	if offer.Schema != 1 || offer.Size <= 0 || offer.Size > updater.MaxArchive || hashErr != nil || len(hash) != 32 || offer.Platform != runtime.GOOS || offer.Arch != runtime.GOARCH || offer.Framework != m.framework {
		return "", errors.New("updateInvalidPackage")
	}
	current, _ := strconv.ParseInt(updater.Build, 10, 64)
	if offer.Build <= current {
		return "", errors.New("updateNoNewVersion")
	}
	m.mu.Lock()
	m.state.Phase, m.state.TargetVersion, m.state.Total = "downloading", offer.Version, offer.Size
	m.mu.Unlock()
	response, err = getHTTPS(ctx, offer.URL)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	f, err := os.CreateTemp(m.root, "download-*.zip")
	if err != nil {
		return "", err
	}
	filename := f.Name()
	_, err = io.Copy(f, &downloadReader{reader: io.LimitReader(response.Body, offer.Size+1), manager: m})
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil {
		digest, size, e := updater.HashFile(filename)
		if e != nil || digest != offer.SHA256 || size != offer.Size {
			err = errors.New("updateHashFailed")
		}
	}
	if err == nil {
		release, e := updater.Inspect(filename, config.PublicKey)
		if e != nil {
			err = e
		} else if release.Build != offer.Build || release.Version != offer.Version {
			err = errors.New("updateInvalidPackage")
		}
	}
	return filename, err
}

type downloadReader struct {
	reader  io.Reader
	manager *updateManager
}

func (d *downloadReader) Read(p []byte) (int, error) {
	n, err := d.reader.Read(p)
	d.manager.mu.Lock()
	d.manager.state.Downloaded += int64(n)
	d.manager.mu.Unlock()
	return n, err
}

func (h *Host) serveUpdates(w http.ResponseWriter, r *http.Request) {
	m := h.updates
	w.Header().Set("Content-Type", "application/json")
	fail := func(err error) {
		w.WriteHeader(409)
		json.NewEncoder(w).Encode(map[string]string{"error": updateError(err)})
	}
	if !applicationUpdatesEnabled {
		switch {
		case r.Method == "GET" && r.URL.Path == "/api/updates":
			json.NewEncoder(w).Encode(m.snapshot())
		case r.Method == "GET" && r.URL.Path == "/api/updates/session":
			json.NewEncoder(w).Encode(map[string]any{"resume": nil, "candidate": false})
		case r.Method == "POST" && r.URL.Path == "/api/updates/ready":
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		default:
			fail(errors.New("updateDisabled"))
		}
		return
	}
	if r.Method == "GET" && r.URL.Path == "/api/updates" {
		m.mu.Lock()
		err := m.loadConfig(r.Context())
		m.mu.Unlock()
		if err != nil {
			fail(err)
			return
		}
		json.NewEncoder(w).Encode(m.snapshot())
		return
	}
	if r.Method == "GET" && r.URL.Path == "/api/updates/session" {
		m.mu.Lock()
		resume, candidate := m.resume, m.candidate
		m.mu.Unlock()
		json.NewEncoder(w).Encode(struct {
			Resume    json.RawMessage `json:"resume"`
			Candidate bool            `json:"candidate"`
		}{resume, candidate})
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	if updater.StoreManaged() {
		// Both clients signal readiness on every launch. No update work is needed.
		if r.URL.Path == "/api/updates/ready" {
			json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		} else {
			fail(errors.New("updateStoreManaged"))
		}
		return
	}
	var err error
	switch r.URL.Path {
	case "/api/updates/config":
		var config updateConfig
		if !updateDecode(w, r, &config, 4096) {
			return
		}
		config.Repository, config.PublicKey = strings.TrimSpace(config.Repository), strings.TrimSpace(config.PublicKey)
		key, keyErr := base64.StdEncoding.DecodeString(config.PublicKey)
		if (config.Repository != "" && !repositoryPattern.MatchString(config.Repository)) || (config.PublicKey != "" && (keyErr != nil || len(key) != ed25519.PublicKeySize)) {
			fail(errors.New("updateSourceMissing"))
			return
		}
		m.mu.Lock()
		if m.cancel != nil || m.state.Phase == "switching" || m.state.Phase == "complete" || m.candidate {
			err = errors.New("updateBusy")
		} else {
			err = m.saveConfig(r.Context(), config)
			if err == nil {
				m.state.Config = config
				m.ready = nil
				if m.readyLease != nil {
					m.readyLease.Close()
					m.readyLease = nil
				}
				m.state.Phase = "idle"
				m.state.LocalRestart = false
			}
		}
		m.mu.Unlock()
	case "/api/updates/check":
		err = m.begin(false)
	case "/api/updates/local":
		err = m.begin(true)
	case "/api/updates/source":
		err = m.chooseLocalSource(r)
	case "/api/updates/rebuild":
		err = m.rebuild()
	case "/api/updates/cancel":
		m.mu.Lock()
		if m.cancel != nil {
			m.cancel()
		}
		m.state.LocalRestart = false
		m.mu.Unlock()
	case "/api/updates/apply":
		var resume json.RawMessage
		if !updateDecode(w, r, &resume, 256<<10) {
			return
		}
		err = m.apply(resume)
	case "/api/updates/ready":
		m.mu.Lock()
		candidate, quit := m.candidate, m.quit
		m.mu.Unlock()
		if candidate {
			_, err = m.parentRequest("POST")
			if err == nil {
				_, err = m.parentRequest("PUT")
			}
			if err == nil {
				m.mu.Lock()
				m.candidate = false
				m.resume = nil
				m.mu.Unlock()
			} else if quit != nil {
				go quit()
			}
		} else {
			// Configured sources check and stage in the background once per launch.
			h.updateOnce.Do(func() {
				state := m.snapshot()
				if state.Config.Repository != "" && state.LocalSource == "" {
					_ = m.begin(false)
				}
				go m.cleanup()
			})
		}
	default:
		w.WriteHeader(404)
		return
	}
	if err != nil {
		fail(err)
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
func updateDecode(w http.ResponseWriter, r *http.Request, value any, limit int64) bool {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, limit))
	d.DisallowUnknownFields()
	if d.Decode(value) != nil || d.Decode(&struct{}{}) != io.EOF {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalidRequest"})
		return false
	}
	return true
}

func (m *updateManager) cleanup() {
	lock := flock.New(filepath.Join(m.root, "install.lock"))
	ok, err := lock.TryLock()
	if err != nil || !ok {
		return
	}
	defer lock.Close()
	var pointer updatePointer
	if updater.ReadJSON(filepath.Join(m.root, "current.json"), &pointer) != nil {
		return
	}
	if _, err := m.host.Activity(); err != nil {
		return
	}
	folder, err := os.Open(filepath.Join(m.root, "versions"))
	if err != nil {
		return
	}
	entries, err := folder.ReadDir(128)
	folder.Close()
	if err != nil && err != io.EOF {
		return
	}
	// Avoid a large cleanup job on the startup path; retain both current and rollback builds.
	removed := 0
	for _, entry := range entries {
		if removed >= 4 {
			return
		}
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "release-") {
			continue
		}
		directory := filepath.Join(m.root, "versions", entry.Name())
		if (pointer.Current != nil && pointer.Current.Directory == directory) || (pointer.Previous != nil && pointer.Previous.Directory == directory) {
			continue
		}
		m.mu.Lock()
		protected := m.ready != nil && m.ready.Directory == directory
		m.mu.Unlock()
		if protected {
			continue
		}
		var staged updater.Staged
		if updater.ReadJSON(filepath.Join(directory, "release.json"), &staged) != nil {
			continue
		}
		lease := flock.New(filepath.Join(m.root, entry.Name()+".lease"))
		ok, err := lease.TryLock()
		if err != nil || !ok {
			lease.Close()
			continue
		}
		if updater.RemoveManaged(filepath.Join(m.root, "versions"), directory) == nil {
			removed++
		}
		lease.Close()
	}
}

func validParent(value string) bool {
	u, err := url.Parse(value)
	return err == nil && u.Scheme == "http" && u.Hostname() == "127.0.0.1" && u.Port() != "" && u.User == nil && u.Path == "/api/update-handoff" && u.RawQuery == "" && u.Fragment == ""
}
func (m *updateManager) parentRequest(method string) ([]byte, error) {
	r, _ := http.NewRequest(method, m.parent, nil)
	r.Header.Set("Authorization", "Bearer "+m.parentToken)
	client := &http.Client{Timeout: 12 * time.Second, Transport: &http.Transport{Proxy: nil}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(r)
	if err != nil {
		return nil, errors.New("updateHandoffFailed")
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, errors.New("updateHandoffFailed")
	}
	if method == http.MethodGet && runtime.GOOS == "windows" {
		// This header comes only from the authenticated parent, outside the
		// renderer-controlled session payload. Keep it across repeated handoffs.
		if owner := response.Header.Get(owningRunHeader); owner != "" {
			m.host.rememberOwningRun(owner)
		}
	}
	return io.ReadAll(io.LimitReader(response.Body, (256<<10)+1))
}
func (h *Host) serveHandoff(w http.ResponseWriter, r *http.Request) {
	m := h.updates
	m.mu.Lock()
	transaction := m.transaction
	m.mu.Unlock()
	if transaction == nil || subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+transaction.token)) != 1 {
		http.Error(w, "forbidden", 403)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if r.Method == "GET" {
		// Windows successors inherit the job; POSIX successors start a new session.
		if runtime.GOOS == "windows" {
			w.Header().Set(owningRunHeader, h.owningRun())
		}
		w.Write(transaction.resume)
		return
	}
	if r.Method == "PUT" {
		select {
		case <-transaction.done:
			if transaction.err != nil {
				w.WriteHeader(409)
				return
			}
			transaction.ackOnce.Do(func() { close(transaction.ack) })
			w.Write([]byte(`{"ok":true}`))
		default:
			w.WriteHeader(409)
		}
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	transaction.once.Do(func() { close(transaction.ready) })
	select {
	case <-transaction.done:
		if transaction.err != nil {
			http.Error(w, "handoff failed", 409)
		} else {
			w.Write([]byte(`{"ok":true}`))
		}
	case <-r.Context().Done():
	}
}
