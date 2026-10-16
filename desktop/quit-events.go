package desktop

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// The authenticated subscription is also the renderer's readiness signal.
// Native menus and window-close hooks use it without evaluating page scripts.
func (h *Host) serveQuitEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "subscriptionUnavailable", http.StatusServiceUnavailable)
		return
	}
	h.quit.mu.Lock()
	if h.quit.finish == nil || h.quit.events != nil {
		h.quit.mu.Unlock()
		http.Error(w, "windowUnavailable", http.StatusServiceUnavailable)
		return
	}
	events := make(chan uint64, 1)
	h.quit.events = events
	h.quit.mu.Unlock()
	defer func() {
		h.quit.mu.Lock()
		h.quit.events = nil
		h.quit.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("X-Accel-Buffering", "no")
	controller := http.NewResponseController(w)
	send := func(value any) bool {
		data, err := json.Marshal(value)
		if err != nil {
			return false
		}
		controller.SetWriteDeadline(time.Now().Add(10 * time.Second))
		if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}
	if !send(map[string]bool{"ready": true}) {
		return
	}
	heartbeat := time.NewTicker(20 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case generation := <-events:
			if !send(map[string]uint64{"generation": generation}) {
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
