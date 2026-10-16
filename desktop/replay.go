package desktop

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"commandmanager.local/core/internal/engine"
)

const owningRunHeader = "X-Command-Manager-Owning-Run"

func (h *Host) owningRun() string {
	h.ownerMu.Lock()
	defer h.ownerMu.Unlock()
	return h.ownerRunID
}

func (h *Host) rememberOwningRun(id string) {
	h.ownerMu.Lock()
	h.ownerRunID = id
	h.ownerMu.Unlock()
}

// Capture ownership while launcher ancestors still exist. Successor windows
// receive this identity through the authenticated handoff, never the renderer.
func (h *Host) captureOwningRun() {
	if !applicationUpdatesEnabled || h.owningRun() != "" {
		return
	}
	status, data, err := h.request(http.MethodGet, "/api/state", nil)
	var state engine.Snapshot
	if err != nil || status != http.StatusOK || json.Unmarshal(data, &state) != nil {
		return
	}
	for _, run := range state.Runs {
		if run.Status != "running" && run.Status != "stopping" {
			continue
		}
		started, err := time.Parse(time.RFC3339, run.Time)
		if err != nil {
			continue
		}
		if owned, err := ownsCurrentProcess(run.PID, started); err == nil && owned {
			h.rememberOwningRun(run.ID)
			return
		}
	}
}

// Build/handoff is an optional self-restart shortcut. With updates disabled,
// every replay uses the independent helper, including this window's owning run.
func (h *Host) prepareReplay(body []byte) (bool, error) {
	if !applicationUpdatesEnabled {
		// The helper starts outside the old run's process group before stopping
		// it, and completes the accepted replay even when this GUI exits.
		return false, nil
	}
	var request struct {
		RunID string `json:"runId"`
	}
	if json.Unmarshal(body, &request) != nil || request.RunID == "" {
		return false, nil // The execution controller validates the request.
	}
	status, data, err := h.request(http.MethodGet, "/api/state", nil)
	var state engine.Snapshot
	if err != nil || status != http.StatusOK || json.Unmarshal(data, &state) != nil {
		return false, errors.New("connectionLost")
	}
	for _, run := range state.Runs {
		if run.ID != request.RunID || (run.Status != "running" && run.Status != "stopping") {
			continue
		}
		if owner := h.owningRun(); owner != "" {
			// Execution helpers start independent groups; this window belongs to one.
			return run.ID == owner, nil
		}
		started, err := time.Parse(time.RFC3339, run.Time)
		if err != nil {
			return false, errors.New("replayOwnershipUnavailable")
		}
		owned, err := ownsCurrentProcess(run.PID, started)
		if err != nil {
			// An update can leave dead ancestors. This probe only selects the
			// self-build shortcut; it no longer authorizes process control.
			// Replay validates the execution's live owner lock and starts its
			// replacement helper independently, even if this GUI then exits.
			return false, nil
		}
		if owned {
			h.rememberOwningRun(run.ID)
		}
		return owned, nil
	}
	return false, nil
}
