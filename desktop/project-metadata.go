package desktop

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"commandmanager.local/core/internal/projectmeta"
)

// Only remember successful, explicit discoveries; saving metadata cannot turn an
// arbitrary ancestor guessed from command directories into an authorized root.
type projectRootPreview struct {
	scanned time.Time
	logoIDs map[string]bool
}

func (h *Host) rememberProjectRoot(directory string, logos []ProjectLogoOption) {
	h.projectRootsMu.Lock()
	defer h.projectRootsMu.Unlock()
	directory = filepath.Clean(directory)
	if h.projectRoots == nil {
		h.projectRoots = map[string]projectRootPreview{}
	}
	if _, exists := h.projectRoots[directory]; !exists && len(h.projectRoots) >= 64 {
		oldest := directory
		oldestAt := time.Now()
		for path, preview := range h.projectRoots {
			if preview.scanned.Before(oldestAt) {
				oldest, oldestAt = path, preview.scanned
			}
		}
		delete(h.projectRoots, oldest)
	}
	ids := make(map[string]bool, len(logos))
	for _, logo := range logos {
		ids[logo.ID] = true
	}
	h.projectRoots[directory] = projectRootPreview{scanned: time.Now(), logoIDs: ids}
}

func (h *Host) serveProjectMetadata(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fail := func(status int, code string) {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": code})
	}
	if r.Method != http.MethodPut && r.Method != http.MethodPost {
		fail(405, "invalidRequest")
		return
	}
	if h.updates.blocked() {
		fail(409, "updateBusy")
		return
	}
	var request struct {
		ID        string  `json:"id"`
		Directory string  `json:"directory,omitempty"`
		LogoID    *string `json:"logoId,omitempty"`
		Revision  int64   `json:"revision,omitempty"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	d.DisallowUnknownFields()
	if d.Decode(&request) != nil || d.Decode(&struct{}{}) != io.EOF || request.ID == "" ||
		(r.Method == http.MethodPut && (request.LogoID == nil || request.Directory != "")) ||
		(r.Method == http.MethodPost && request.Directory == "") {
		fail(400, "invalidRequest")
		return
	}
	definitions, err := h.projectDefinitions(r.Context())
	if err != nil {
		fail(503, "connectionLost")
		return
	}
	known := false
	for _, project := range definitions.Projects {
		known = known || project.ID == request.ID
	}
	if !known {
		fail(404, "projectMissing")
		return
	}
	if r.Method == http.MethodPost {
		request.Directory = filepath.Clean(request.Directory)
		h.projectRootsMu.Lock()
		_, scanned := h.projectRoots[request.Directory]
		h.projectRootsMu.Unlock()
		// A retry may follow a host restart; roots already persisted for this
		// project remain valid without a new scan.
		if !scanned {
			entries, loadErr := projectmeta.Load(h.dataDir)
			if loadErr != nil {
				fail(500, "projectMetadataLoadFailed")
				return
			}
			for _, root := range entries[request.ID].Roots {
				rel, relErr := filepath.Rel(root, request.Directory)
				scanned = scanned || (relErr == nil && rel == ".")
			}
		}
		if !scanned {
			fail(400, "invalidDirectory")
			return
		}
	}
	if request.LogoID != nil && *request.LogoID != "" {
		available := false
		h.projectDetailsMu.Lock()
		for _, logo := range h.projectDetails[request.ID].Logos {
			available = available || logo.ID == *request.LogoID
		}
		h.projectDetailsMu.Unlock()
		if r.Method == http.MethodPost {
			h.projectRootsMu.Lock()
			available = available || h.projectRoots[request.Directory].logoIDs[*request.LogoID]
			h.projectRootsMu.Unlock()
		}
		if !available {
			fail(400, "invalidRequest")
			return
		}
	}
	entry, err := projectmeta.Update(r.Context(), h.dataDir, request.ID, request.Directory, request.LogoID, request.Revision)
	if err != nil {
		if errors.Is(err, projectmeta.ErrConflict) {
			fail(409, "conflict")
		} else {
			fail(500, "projectMetadataSaveFailed")
		}
		return
	}
	h.projectDetailsMu.Lock()
	delete(h.projectDetails, request.ID)
	h.projectDetailsMu.Unlock()
	json.NewEncoder(w).Encode(map[string]int64{"revision": entry.Revision})
}
