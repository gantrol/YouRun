package desktop

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

type quitControls struct {
	mu                                sync.Mutex
	notify, finish, fallback          func()
	events                            chan uint64
	prepared, canStop, busy, awaiting bool
	generation                        uint64
}

func (h *Host) SetQuitControls(notify, finish, fallback func()) {
	h.quit.mu.Lock()
	h.quit.notify, h.quit.finish, h.quit.fallback = notify, finish, fallback
	h.quit.mu.Unlock()
}

// False retains the platform fallback until the application UI is ready.
func (h *Host) RequestThemedQuit() bool {
	h.quit.mu.Lock()
	if h.quit.awaiting {
		h.quit.mu.Unlock()
		return true
	}
	notify, events := h.quit.notify, h.quit.events
	if events == nil || notify == nil {
		h.quit.mu.Unlock()
		return false
	}
	h.quit.awaiting = true
	h.quit.generation++
	generation := h.quit.generation
	// Replace an expired notification if the renderer has not consumed it yet.
	select {
	case <-events:
	default:
	}
	events <- generation
	h.quit.mu.Unlock()
	notify()
	// A stalled or reloading WebView must not make the application impossible to quit.
	time.AfterFunc(3*time.Second, func() {
		h.quit.mu.Lock()
		if !h.quit.awaiting || h.quit.generation != generation {
			h.quit.mu.Unlock()
			return
		}
		h.quit.awaiting = false
		fallback := h.quit.fallback
		h.quit.mu.Unlock()
		if fallback != nil {
			fallback()
		}
	})
	return true
}

func (h *Host) serveQuit(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		h.serveQuitEvents(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	fail := func(status int, code string) {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": code})
	}
	if r.Method != http.MethodPost {
		fail(405, "invalidRequest")
		return
	}
	var request struct {
		Action     string `json:"action"`
		Generation uint64 `json:"generation,omitempty"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&request) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		fail(400, "invalidRequest")
		return
	}
	if request.Action != "received" && request.Action != "cancel" && h.updates.blocked() {
		fail(409, "updateBusy")
		return
	}
	h.quit.mu.Lock()
	if h.quit.finish == nil {
		h.quit.mu.Unlock()
		fail(503, "windowUnavailable")
		return
	}
	if request.Action == "received" {
		if !h.quit.awaiting || request.Generation != h.quit.generation {
			h.quit.mu.Unlock()
			fail(409, "invalidRequest")
			return
		}
		h.quit.awaiting = false
		h.quit.mu.Unlock()
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		return
	}
	if h.quit.busy {
		h.quit.mu.Unlock()
		fail(409, "quitBusy")
		return
	}
	switch request.Action {
	case "cancel":
		h.quit.prepared = false
		h.quit.mu.Unlock()
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		return
	case "prepare":
		h.quit.busy = true
		h.quit.mu.Unlock()
		activity, err := h.Activity()
		prompt := ClosePromptFor(activity, err != nil)
		h.quit.mu.Lock()
		if r.Context().Err() != nil {
			h.quit.busy = false
			h.quit.mu.Unlock()
			return
		}
		h.quit.busy, h.quit.prepared, h.quit.canStop = false, true, prompt.CanStop
		h.quit.mu.Unlock()
		json.NewEncoder(w).Encode(struct {
			Idle   bool        `json:"idle"`
			Prompt ClosePrompt `json:"prompt"`
		}{Idle: err == nil && activity.Active == 0 && activity.Unknown == 0, Prompt: prompt})
		return
	case "keep", "stop":
		if !h.quit.prepared || request.Action == "stop" && !h.quit.canStop {
			h.quit.mu.Unlock()
			fail(409, "invalidRequest")
			return
		}
		h.quit.busy = true
		finish := h.quit.finish
		h.quit.mu.Unlock()
		if request.Action == "stop" {
			if err := h.StopAll(); err != nil {
				h.quit.mu.Lock()
				h.quit.busy = false
				h.quit.mu.Unlock()
				fail(503, "stopFailed")
				return
			}
		}
		if r.Context().Err() != nil {
			h.quit.mu.Lock()
			h.quit.busy = false
			h.quit.mu.Unlock()
			return
		}
		json.NewEncoder(w).Encode(map[string]bool{"ok": true})
		if flush, ok := w.(http.Flusher); ok {
			flush.Flush()
		}
		go finish()
	default:
		h.quit.mu.Unlock()
		fail(400, "invalidRequest")
	}
}
