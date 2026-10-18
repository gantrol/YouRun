package desktop

import (
	"encoding/json"
	"io"
	"net/http"

	"commandmanager.local/core/internal/cli"
)

type AgentIntegrationSettings = cli.AgentIntegrationSettings

func (h *Host) serveAgentIntegration(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fail := func(status int, code string) {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": code})
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		fail(http.StatusMethodNotAllowed, "invalidRequest")
		return
	}
	if r.Method == http.MethodPut {
		if h.updates.blocked() {
			fail(http.StatusConflict, "updateBusy")
			return
		}
		var input struct {
			Enabled *bool `json:"enabled"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&input) != nil || input.Enabled == nil || decoder.Decode(&struct{}{}) != io.EOF {
			fail(http.StatusBadRequest, "invalidRequest")
			return
		}
		settings, err := cli.WriteAgentIntegration(r.Context(), h.dataDir, *input.Enabled)
		if err != nil {
			code := "agentIntegrationSaveFailed"
			if err.Error() == "agentIntegrationDevelopment" {
				fail(http.StatusConflict, err.Error())
				return
			}
			fail(http.StatusInternalServerError, code)
			return
		}
		json.NewEncoder(w).Encode(settings)
		return
	}
	settings, err := cli.ReadAgentIntegration(r.Context(), h.dataDir)
	if err != nil {
		fail(http.StatusInternalServerError, "agentIntegrationLoadFailed")
		return
	}
	json.NewEncoder(w).Encode(settings)
}
