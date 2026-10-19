// Package desktop is the shared narrow native-host boundary for both frameworks.
package desktop

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"commandmanager.local/core/internal/engine"
	"commandmanager.local/core/internal/workspace"
)

type Host struct {
	processActions      processActions
	streams             atomic.Int32
	URL                 string
	server              *http.Server
	dataDir             string
	client              *engine.Client
	workspace           *workspace.Service
	ownerMu             sync.Mutex
	ownerRunID          string
	choosing            atomic.Bool
	discovering         atomic.Bool
	loadingProjects     atomic.Int32
	projectDetailsMu    sync.Mutex
	projectDetails      map[string]ProjectDetails
	projectRootsMu      sync.Mutex
	projectRoots        map[string]projectRootPreview
	projectOpenersMu    sync.Mutex
	projectOpeners      []ProjectOpener
	projectOpenersAt    time.Time
	pickerMu            sync.RWMutex
	chooseDirectory     func() (string, error)
	chooseFile          func() (string, error)
	chooseProjectAvatar func() (string, error)
	windowMu            sync.RWMutex
	windowState         func() WindowState
	windowAction        func(string)
	quit                quitControls
	updates             *updateManager
	updateOnce          sync.Once
}

func New(framework string, frontend fs.FS) (*Host, error) {
	dir, err := engine.DataDir()
	if err != nil {
		return nil, err
	}
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	client, err := engine.Open(dir, exe)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	origin := "http://" + listener.Addr().String()
	token := engine.ID()
	host := &Host{URL: origin + "/#" + token, dataDir: dir, client: client}
	host.workspace, err = workspace.New(dir, client.Request)
	if err != nil {
		listener.Close()
		return nil, err
	}
	host.updates, err = newUpdateManager(host, framework)
	if err != nil {
		listener.Close()
		return nil, err
	}
	host.captureOwningRun()
	var static http.Handler = http.NotFoundHandler()
	if frontend != nil {
		files, err := fs.Sub(frontend, "frontend/dist")
		if err != nil {
			listener.Close()
			return nil, err
		}
		static = http.FileServer(http.FS(files))
	}
	allowed := map[string]bool{"GET /api/state": true, "PUT /api/definitions": true, "POST /api/run": true, "POST /api/replay": true, "POST /api/stop": true, "GET /api/logs": true, "POST /api/import": true}
	host.server = &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'; object-src 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		if r.Host != listener.Addr().String() || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != origin) {
			http.Error(w, "forbidden", 403)
			return
		}
		if len(r.URL.Path) < 5 || r.URL.Path[:5] != "/api/" {
			if r.Method != "GET" {
				http.Error(w, "method", 405)
				return
			}
			static.ServeHTTP(w, r)
			return
		}
		if r.URL.Path == "/api/update-handoff" {
			host.serveHandoff(w, r)
			return
		}
		if subtle.ConstantTimeCompare([]byte(r.Header.Get("Authorization")), []byte("Bearer "+token)) != 1 {
			http.Error(w, "unauthorized", 401)
			return
		}
		if r.URL.Path == "/api/events" || r.URL.Path == "/api/log-events" {
			host.serveEvents(w, r)
			return
		}
		if r.URL.Path == "/api/updates" || strings.HasPrefix(r.URL.Path, "/api/updates/") {
			host.serveUpdates(w, r)
			return
		}
		if r.URL.Path == "/api/window" {
			host.serveWindow(w, r)
			return
		}
		if r.URL.Path == "/api/quit" {
			host.serveQuit(w, r)
			return
		}
		if r.URL.Path == "/api/appearance" {
			host.serveAppearance(w, r)
			return
		}
		if r.URL.Path == "/api/startup" {
			host.serveStartup(w, r)
			return
		}
		if r.URL.Path == "/api/agent-integration" {
			host.serveAgentIntegration(w, r)
			return
		}
		if r.URL.Path == "/api/project-details" {
			host.serveProjectDetails(w, r)
			return
		}
		if r.URL.Path == "/api/project-metadata" {
			host.serveProjectMetadata(w, r)
			return
		}
		if r.URL.Path == "/api/project-workspaces" {
			host.serveProjectWorkspaces(w, r)
			return
		}
		if r.URL.Path == "/api/source-inspect" {
			host.serveSourceInspect(w, r)
			return
		}
		if r.URL.Path == "/api/resources/processes" {
			host.serveResourcesProcesses(w, r)
			return
		}
		if r.URL.Path == "/api/resources/bounties" {
			host.serveResourcesBounties(w, r)
			return
		}
		if r.URL.Path == "/api/resources/control" {
			host.serveProcessControl(w, r)
			return
		}
		if r.URL.Path == "/api/resources/ports" {
			host.serveResourcesPorts(w, r)
			return
		}
		if r.URL.Path == "/api/resources/files" {
			host.serveResourcesFiles(w, r)
			return
		}
		if r.URL.Path == "/api/choose-file" {
			host.serveChooseFile(w, r)
			return
		}
		if r.URL.Path == "/api/project-settings" {
			host.serveProjectSettings(w, r)
			return
		}
		if r.URL.Path == "/api/project-avatar" {
			host.serveProjectAvatar(w, r)
			return
		}
		if r.URL.Path == "/api/open-project-directory" {
			host.serveOpenProject(w, r)
			return
		}
		if r.URL.Path == "/api/project-openers" {
			host.serveProjectOpeners(w, r)
			return
		}
		if r.URL.Path == "/api/discover" {
			host.discover(w, r)
			return
		}
		if r.Method == "POST" && r.URL.Path == "/api/choose-directory" {
			host.pickerMu.RLock()
			picker := host.chooseDirectory
			host.pickerMu.RUnlock()
			if picker == nil || !host.choosing.CompareAndSwap(false, true) {
				http.Error(w, "busy", 409)
				return
			}
			defer host.choosing.Store(false)
			directory, err := picker()
			w.Header().Set("Content-Type", "application/json")
			if err != nil {
				w.WriteHeader(500)
				json.NewEncoder(w).Encode(map[string]string{"error": "directoryPickerFailed"})
				return
			}
			json.NewEncoder(w).Encode(map[string]string{"directory": directory})
			return
		}
		if !allowed[r.Method+" "+r.URL.Path] {
			http.NotFound(w, r)
			return
		}
		if r.Method != "GET" && host.updates.blocked() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(409)
			w.Write([]byte(`{"error":"updateBusy"}`))
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
		if err != nil {
			http.Error(w, "too large", 413)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/api/replay" {
			selfRestart, err := host.prepareReplay(body)
			if err != nil {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusConflict)
				json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
				return
			}
			if selfRestart {
				// Let the client save drafts/session before entering build+handoff.
				// Nothing is stopped or rebuilt by this classification response.
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]bool{"selfRestart": true})
				return
			}
		}
		status, data, err := host.workspace.Request(r.Context(), r.Method, r.URL.RequestURI(), body)
		if err == nil && status == http.StatusNotFound && r.URL.Path == "/api/replay" {
			data = []byte(`{"error":"replayUnavailable"}`)
		}
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(503)
			w.Write([]byte(`{"error":"connectionLost"}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write(data)
	})}
	go host.server.Serve(listener)
	return host, nil
}

// Closing a GUI host never terminates independent executions.
func (h *Host) Close() {
	h.updates.mu.Lock()
	if h.updates.cancel != nil {
		h.updates.cancel()
	}
	buildDone := h.updates.buildDone
	if h.updates.readyLease != nil {
		h.updates.readyLease.Close()
	}
	h.updates.mu.Unlock()
	if buildDone != nil {
		// Give cancellation time to reap the local build tree before this host exits.
		select {
		case <-buildDone:
		case <-time.After(3 * time.Second):
		}
	}
	h.server.Close()
	CloseResourceMetrics()
}

func (h *Host) SetDirectoryPicker(picker func() (string, error)) {
	h.pickerMu.Lock()
	h.chooseDirectory = picker
	h.pickerMu.Unlock()
}

func (h *Host) request(method, path string, body []byte) (int, []byte, error) {
	return h.workspace.Request(context.Background(), method, path, body)
}

type Activity struct {
	Active  int
	Unknown int
	Locale  string
}

func (h *Host) Activity() (Activity, error) {
	result, err := h.client.Activity()
	if err != nil {
		return Activity{Locale: "zh"}, err
	}
	return Activity{Active: result.Active, Unknown: result.Unknown, Locale: h.workspace.Locale(context.Background())}, nil
}

func (h *Host) StopAll() error {
	return h.client.StopAll()
}
