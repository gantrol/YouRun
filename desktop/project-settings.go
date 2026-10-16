package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"time"

	"commandmanager.local/core/internal/database"
	"commandmanager.local/core/internal/engine"
	"commandmanager.local/core/internal/projectmeta"
)

func (h *Host) serveProjectSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fail := func(status int, code string) {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": code})
	}
	if r.Method != http.MethodPut {
		fail(405, "invalidRequest")
		return
	}
	if h.updates.blocked() {
		fail(409, "updateBusy")
		return
	}
	var request struct {
		Base    engine.Project `json:"base"`
		Project engine.Project `json:"project"`
		Logo    *struct {
			Revision int64  `json:"revision"`
			LogoID   string `json:"logoId"`
			Avatar   string `json:"avatar"`
		} `json:"logo,omitempty"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 32768))
	d.DisallowUnknownFields()
	if d.Decode(&request) != nil || d.Decode(&struct{}{}) != io.EOF || request.Project.ID == "" || request.Project.ID != request.Base.ID {
		fail(400, "invalidRequest")
		return
	}
	if logo := request.Logo; logo != nil {
		if logo.Revision < 0 || (logo.Avatar != "" && logo.LogoID != "") {
			fail(400, "invalidRequest")
			return
		}
		if logo.Avatar != "" {
			if _, err := projectmeta.ReadAvatar(h.dataDir, logo.Avatar); err != nil {
				fail(400, "avatarInvalidImage")
				return
			}
		}
		if logo.LogoID != "" {
			found := false
			h.projectDetailsMu.Lock()
			for _, option := range h.projectDetails[request.Project.ID].Logos {
				found = found || option.ID == logo.LogoID
			}
			h.projectDetailsMu.Unlock()
			if !found {
				fail(400, "logoSelectionMissing")
				return
			}
		}
	}
	store, err := database.Open(h.dataDir)
	if err != nil {
		fail(500, "storageFailed")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	saved, err := store.UpdateProject(ctx, request.Project.ID, func(raw, metadata json.RawMessage) (json.RawMessage, json.RawMessage, error) {
		var definitions engine.Definitions
		var entry projectmeta.Entry
		if err := json.Unmarshal(raw, &definitions); err != nil {
			return nil, nil, err
		}
		if err := json.Unmarshal(metadata, &entry); err != nil {
			return nil, nil, err
		}
		found := false
		for i, current := range definitions.Projects {
			if current.ID != request.Project.ID {
				continue
			}
			found = true
			// Repeating a successful request after a lost response is safe, but a
			// different concurrent change to this project must never be overwritten.
			if !reflect.DeepEqual(current, request.Base) && !reflect.DeepEqual(current, request.Project) {
				return nil, nil, database.ErrConflict
			}
			definitions.Projects[i] = request.Project
		}
		if !found {
			return nil, nil, errors.New("projectMissing")
		}
		if engine.ValidateDefinitions(definitions) != nil {
			return nil, nil, errors.New("invalidDefinition")
		}
		if logo := request.Logo; logo != nil {
			same := entry.LogoID == logo.LogoID && entry.Avatar == logo.Avatar
			if entry.Revision != logo.Revision && !same {
				return nil, nil, database.ErrConflict
			}
			if !same {
				entry.LogoID, entry.Avatar = logo.LogoID, logo.Avatar
				entry.Revision++
			}
		}
		if entry.Roots == nil {
			entry.Roots = []string{}
		}
		project, err := json.Marshal(request.Project)
		if err != nil {
			return nil, nil, err
		}
		next, err := json.Marshal(entry)
		return project, next, err
	})
	if err != nil {
		switch err.Error() {
		case "conflict":
			fail(409, "conflict")
		case "projectMissing":
			fail(404, "projectMissing")
		case "invalidDefinition":
			fail(400, "invalidDefinition")
		default:
			fail(500, "storageFailed")
		}
		return
	}
	h.projectDetailsMu.Lock()
	delete(h.projectDetails, request.Project.ID)
	h.projectDetailsMu.Unlock()
	w.Write(saved)
}
