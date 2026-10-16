package desktop

import (
	"encoding/json"
	"io"
	"net/http"
)

type WindowState struct {
	Framework  string `json:"framework"`
	Platform   string `json:"platform"`
	Maximized  bool   `json:"maximized"`
	Fullscreen bool   `json:"fullscreen"`
}

// The page can operate only its own native window, through these named actions.
func (h *Host) SetWindowControls(state func() WindowState, action func(string)) {
	h.windowMu.Lock()
	h.windowState, h.windowAction = state, action
	h.windowMu.Unlock()
}

func (h *Host) serveWindow(w http.ResponseWriter, r *http.Request) {
	h.windowMu.RLock()
	state, action := h.windowState, h.windowAction
	h.windowMu.RUnlock()
	w.Header().Set("Content-Type", "application/json")
	if state == nil || action == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "windowUnavailable"})
		return
	}
	if r.Method == http.MethodGet {
		json.NewEncoder(w).Encode(state())
		return
	}
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	var request struct {
		Action string `json:"action"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalidRequest"})
		return
	}
	switch request.Action {
	case "minimize", "toggleMaximize", "close":
		action(request.Action)
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
	default:
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalidRequest"})
	}
}
