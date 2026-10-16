package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"time"

	"commandmanager.local/core/internal/database"
	"commandmanager.local/core/internal/engine"
	"github.com/fsnotify/fsnotify"
)

var eventRunID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)

// File events are invalidation hints, never uncommitted data. Subscribe first,
// then read committed SQLite/execution state. Reconnect always starts a snapshot.
func (h *Host) serveEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method", 405)
		return
	}
	if h.streams.Add(1) > 8 {
		h.streams.Add(-1)
		http.Error(w, "busy", 429)
		return
	}
	defer h.streams.Add(-1)
	logs := r.URL.Path == "/api/log-events"
	id := r.URL.Query().Get("id")
	if logs && !eventRunID.MatchString(id) {
		http.Error(w, "invalidRequest", 400)
		return
	}
	watcher, err := fsnotify.NewBufferedWatcher(256)
	if err != nil {
		http.Error(w, "subscriptionUnavailable", 503)
		return
	}
	defer watcher.Close()
	executions := filepath.Join(h.dataDir, "executions")
	paths := []string{h.dataDir, executions}
	if logs {
		paths = []string{filepath.Join(h.dataDir, "logs")}
	}
	for _, path := range paths {
		if err = watcher.Add(path); err != nil {
			http.Error(w, "subscriptionUnavailable", 503)
			return
		}
	}
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "subscriptionUnavailable", 503)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	send := func(value any) bool {
		data, err := json.Marshal(value)
		if err != nil {
			return false
		}
		controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		_, err = fmt.Fprintf(w, "data: %s\n\n", data)
		if err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	wake := make(chan struct{}, 1)
	notify := func() {
		select {
		case wake <- struct{}{}:
		default:
		}
	}
	watched := map[string]bool{}
	owners := map[string]context.CancelFunc{}
	deadlines := map[string]*time.Timer{}
	defer func() {
		for _, timer := range deadlines {
			timer.Stop()
		}
	}()
	defer func() {
		for _, stop := range owners {
			stop()
		}
	}()
	var previous *engine.Snapshot
	firstLog := true
	var cursor *int64
	if raw := r.URL.Query().Get("cursor"); raw != "" {
		value, e := strconv.ParseInt(raw, 10, 64)
		if e != nil || value < 0 {
			return
		}
		cursor = &value
	}
	publish := func() bool {
		if logs {
			for {
				query := url.Values{"id": {id}}
				if cursor != nil {
					query.Set("cursor", strconv.FormatInt(*cursor, 10))
				}
				status, data, err := h.client.Request("GET", "/api/logs?"+query.Encode(), nil)
				if err != nil {
					return false
				}
				if status != 200 {
					var failure struct {
						Error string `json:"error"`
					}
					json.Unmarshal(data, &failure)
					if failure.Error == "logCursorBeyondEnd" {
						cursor = nil
						continue
					}
					send(map[string]string{"error": "logMissing"})
					return false
				}
				var chunk engine.LogChunk
				if json.Unmarshal(data, &chunk) != nil {
					return false
				}
				reset := cursor == nil
				if firstLog || reset || len(chunk.Data) > 0 {
					if !send(map[string]any{"chunk": chunk, "reset": reset}) {
						return false
					}
				}
				firstLog = false
				next := chunk.NextCursor
				cursor = &next
				if chunk.EOF {
					return true
				}
				if ctx.Err() != nil {
					return false
				}
			}
		}
		status, data, err := h.workspace.Request(ctx, "GET", "/api/state", nil)
		if err != nil || status != 200 {
			send(map[string]string{"error": "connectionLost"})
			return false
		}
		var next engine.Snapshot
		if json.Unmarshal(data, &next) != nil {
			return false
		}
		active := map[string]bool{}
		for _, run := range next.Runs {
			if run.Status != "running" && run.Status != "stopping" {
				continue
			}
			if !eventRunID.MatchString(run.ID) {
				continue
			}
			active[run.ID] = true
			if !watched[run.ID] {
				if err := watcher.Add(filepath.Join(executions, run.ID)); err != nil && !os.IsNotExist(err) {
					return false
				}
				watched[run.ID] = true
				if started, err := time.Parse(time.RFC3339Nano, run.Time); err == nil {
					if remaining := 16*time.Second - time.Since(started); remaining > 0 {
						deadlines[run.ID] = time.AfterFunc(remaining, notify)
					}
				}
				// Covers a commit between the initial read and installation of this watch.
				notify()
			}
			if owners[run.ID] == nil {
				ownerCtx, stop := context.WithCancel(ctx)
				if h.client.SubscribeWorkerExit(ownerCtx, run.ID, notify) {
					owners[run.ID] = stop
				} else {
					stop()
				}
			}
		}
		for runID := range watched {
			if !active[runID] {
				watcher.Remove(filepath.Join(executions, runID))
				delete(watched, runID)
				if timer := deadlines[runID]; timer != nil {
					timer.Stop()
					delete(deadlines, runID)
				}
				if stop := owners[runID]; stop != nil {
					stop()
					delete(owners, runID)
				}
			}
		}
		if previous == nil {
			if !send(map[string]any{"snapshot": next}) {
				return false
			}
		} else {
			patch := map[string]any{}
			if !reflect.DeepEqual(previous.Definitions, next.Definitions) {
				patch["definitions"] = next.Definitions
			}
			if !reflect.DeepEqual(previous.Runs, next.Runs) {
				old := map[string]engine.Run{}
				for _, run := range previous.Runs {
					old[run.ID] = run
				}
				upserts := []engine.Run{}
				order := []string{}
				removed := []string{}
				for _, run := range next.Runs {
					if prior, exists := old[run.ID]; !exists || !reflect.DeepEqual(prior, run) {
						upserts = append(upserts, run)
					}
					order = append(order, run.ID)
					delete(old, run.ID)
				}
				for runID := range old {
					removed = append(removed, runID)
				}
				patch["runs"] = map[string]any{"upserts": upserts, "removed": removed, "order": order, "revision": next.RunsRevision}
			}
			if previous.MetadataRevision != next.MetadataRevision {
				patch["metadataRevision"] = next.MetadataRevision
				patch["metadataKeys"] = next.MetadataKeys
			}
			if previous.SettingsRevision != next.SettingsRevision {
				patch["settingsRevision"] = next.SettingsRevision
			}
			if previous.StorageError != next.StorageError {
				patch["storageError"] = next.StorageError
			}
			if len(patch) > 0 && !send(map[string]any{"patch": patch}) {
				return false
			}
		}
		previous = &next
		return true
	}
	if !publish() {
		return
	}
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	var batch <-chan time.Time
	var timer *time.Timer
	defer func() {
		if timer != nil {
			timer.Stop()
		}
	}()
	schedule := func() {
		if batch == nil {
			timer = time.NewTimer(25 * time.Millisecond)
			batch = timer.C
		}
	}
	for {
		select {
		case <-ctx.Done():
			return
		case <-wake:
			schedule()
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			if event.Op&(fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename) == 0 {
				continue
			}
			name := filepath.Base(event.Name)
			parent := filepath.Dir(event.Name)
			if logs {
				if name == id+".log" || event.Name == paths[0] {
					schedule()
				}
				continue
			}
			if parent == h.dataDir && (name == database.Filename || name == database.Filename+"-wal" || name == "runs.json" || name == "executions") {
				schedule()
			}
			if parent == executions && eventRunID.MatchString(name) {
				schedule()
			}
			if filepath.Dir(parent) == executions && (name == "state.json" || name == "stop.json") {
				schedule()
			}
		case _, ok := <-watcher.Errors:
			if !ok {
				return
			}
			send(map[string]string{"error": "connectionLost"})
			return
		case <-batch:
			batch = nil
			if !publish() {
				return
			}
		case <-heartbeat.C:
			controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if _, err := fmt.Fprint(w, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}
