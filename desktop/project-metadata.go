package desktop

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"commandmanager.local/core/internal/engine"
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

func (h *Host) serveProjectWorkspaces(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fail := func(status int, code string) {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": code})
	}
	if r.Method != http.MethodPost {
		fail(405, "invalidRequest")
		return
	}
	if h.updates.blocked() {
		fail(409, "updateBusy")
		return
	}
	var request struct {
		ID              string `json:"id"`
		Action          string `json:"action"`
		LocationID      string `json:"locationId,omitempty"`
		Name            string `json:"name,omitempty"`
		Directory       string `json:"directory,omitempty"`
		ChooseDirectory bool   `json:"chooseDirectory,omitempty"`
		Revision        int64  `json:"revision,omitempty"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	d.DisallowUnknownFields()
	if d.Decode(&request) != nil || d.Decode(&struct{}{}) != io.EOF || request.ID == "" || request.Action == "" {
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
	if request.Action == "add" && request.ChooseDirectory {
		h.pickerMu.RLock()
		picker := h.chooseDirectory
		h.pickerMu.RUnlock()
		if picker == nil {
			fail(503, "directoryPickerFailed")
			return
		}
		if !h.choosing.CompareAndSwap(false, true) {
			fail(409, "avatarPickerBusy")
			return
		}
		path, pickErr := picker()
		h.choosing.Store(false)
		if r.Context().Err() != nil {
			return
		}
		if pickErr != nil {
			fail(500, "directoryPickerFailed")
			return
		}
		if path == "" {
			json.NewEncoder(w).Encode(map[string]bool{"cancelled": true})
			return
		}
		request.Directory = path
	}
	if request.Action == "add" {
		clean := filepath.Clean(strings.TrimSpace(request.Directory))
		info, statErr := os.Stat(clean)
		if statErr != nil || !info.IsDir() || strings.Contains(strings.ToLower(clean), "trash") {
			fail(400, "invalidDirectory")
			return
		}
		request.Directory = clean
	}
	var fallbackDirs []string
	for _, cmd := range definitions.Commands {
		if cmd.Project != nil && *cmd.Project == request.ID && cmd.Directory != "" {
			fallbackDirs = append(fallbackDirs, cmd.Directory)
		}
	}
	entry, err := projectmeta.UpdateWorkspaces(r.Context(), h.dataDir, request.ID, projectmeta.WorkspaceAction{
		Action:     request.Action,
		LocationID: request.LocationID,
		Name:       request.Name,
		Directory:  request.Directory,
		Revision:   request.Revision,
	}, fallbackDirs)
	if err != nil {
		switch {
		case errors.Is(err, projectmeta.ErrConflict):
			fail(409, "conflict")
		case err.Error() == "alreadyImported" || err.Error() == "invalidDirectory" || err.Error() == "invalidRequest":
			fail(400, err.Error())
		default:
			fail(500, "projectMetadataSaveFailed")
		}
		return
	}
	h.projectDetailsMu.Lock()
	delete(h.projectDetails, request.ID)
	h.projectDetailsMu.Unlock()
	locations, activeID := projectmeta.ResolveLocations(entry, fallbackDirs)
	workspaces := make([]ProjectWorkspaceDetail, 0, len(locations))
	activeDir := ""
	for _, loc := range locations {
		src := engine.InspectSource(r.Context(), loc.Directory, loc.ID, loc.Name, true)
		workspaces = append(workspaces, ProjectWorkspaceDetail{
			ID:        loc.ID,
			Name:      loc.Name,
			Directory: loc.Directory,
			IsDefault: loc.IsDefault,
			Source:    src,
		})
		if loc.ID == activeID {
			activeDir = loc.Directory
		}
	}
	json.NewEncoder(w).Encode(map[string]any{
		"revision":        entry.Revision,
		"directory":       activeDir,
		"activeWorkspace": activeID,
		"workspaces":      workspaces,
	})
}

func (h *Host) serveSourceInspect(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalidRequest"})
		return
	}
	var request struct {
		Directory     string `json:"directory"`
		WorkspaceID   string `json:"workspaceId,omitempty"`
		WorkspaceName string `json:"workspaceName,omitempty"`
		Refresh       bool   `json:"refresh,omitempty"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	d.DisallowUnknownFields()
	if d.Decode(&request) != nil || d.Decode(&struct{}{}) != io.EOF {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"error": "invalidRequest"})
		return
	}
	snap := engine.InspectSource(r.Context(), request.Directory, request.WorkspaceID, request.WorkspaceName, request.Refresh)
	json.NewEncoder(w).Encode(map[string]any{"source": snap})
}
