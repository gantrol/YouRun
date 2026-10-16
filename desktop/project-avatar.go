package desktop

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"commandmanager.local/core/internal/discovery"
	"commandmanager.local/core/internal/projectmeta"
)

func (h *Host) SetProjectAvatarPicker(picker func() (string, error)) {
	h.pickerMu.Lock()
	h.chooseProjectAvatar = picker
	h.pickerMu.Unlock()
}

// The renderer supplies only a project and revision. A host-owned dialog is
// the only way to authorize the local file read; its path is never accepted
// from HTTP or persisted as a dependency on the user's original image.
func (h *Host) serveProjectAvatar(w http.ResponseWriter, r *http.Request) {
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
		ID       string `json:"id"`
		Preview  bool   `json:"preview,omitempty"`
		Revision int64  `json:"revision"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
	d.DisallowUnknownFields()
	if d.Decode(&request) != nil || d.Decode(&struct{}{}) != io.EOF || request.ID == "" || len(request.ID) > 80 || request.Revision < 0 {
		fail(400, "invalidRequest")
		return
	}
	knownProject := func() bool {
		definitions, err := h.projectDefinitions(r.Context())
		if err != nil {
			fail(503, "connectionLost")
			return false
		}
		for _, project := range definitions.Projects {
			if project.ID == request.ID {
				return true
			}
		}
		fail(404, "projectMissing")
		return false
	}
	if !knownProject() {
		return
	}
	entries, err := projectmeta.Load(h.dataDir)
	if err != nil {
		fail(500, "projectMetadataLoadFailed")
		return
	}
	if entries[request.ID].Revision != request.Revision {
		fail(409, "conflict")
		return
	}
	h.pickerMu.RLock()
	picker := h.chooseProjectAvatar
	h.pickerMu.RUnlock()
	if picker == nil {
		fail(503, "avatarPickerFailed")
		return
	}
	if !h.choosing.CompareAndSwap(false, true) {
		fail(409, "avatarPickerBusy")
		return
	}
	defer h.choosing.Store(false)
	path, err := picker()
	if r.Context().Err() != nil {
		return
	}
	if err != nil {
		fail(500, "avatarPickerFailed")
		return
	}
	if path == "" {
		json.NewEncoder(w).Encode(map[string]bool{"cancelled": true})
		return
	}
	if !filepath.IsAbs(path) || strings.Contains(strings.ToLower(path), "trash") {
		fail(400, "avatarInvalidImage")
		return
	}
	root, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		fail(400, "avatarReadFailed")
		return
	}
	defer root.Close()
	info, err := root.Lstat(filepath.Base(path))
	if err != nil || !info.Mode().IsRegular() {
		fail(400, "avatarInvalidImage")
		return
	}
	if info.Size() > 2<<20 {
		fail(400, "avatarTooLarge")
		return
	}
	f, err := root.Open(filepath.Base(path))
	if err != nil {
		fail(400, "avatarReadFailed")
		return
	}
	data, err := io.ReadAll(io.LimitReader(f, (2<<20)+1))
	f.Close()
	if err != nil {
		fail(400, "avatarReadFailed")
		return
	}
	if len(data) > 2<<20 {
		fail(400, "avatarTooLarge")
		return
	}
	encoded := discovery.EncodeLogo(data, filepath.Ext(path))
	prefix, payload, ok := strings.Cut(encoded, ",")
	if !ok {
		fail(400, "avatarInvalidImage")
		return
	}
	extension := ".png"
	if prefix == "data:image/svg+xml;base64" {
		extension = ".svg"
	}
	normalized, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		fail(400, "avatarInvalidImage")
		return
	}
	if r.Context().Err() != nil {
		return
	}
	if h.updates.blocked() {
		fail(409, "updateBusy")
		return
	}
	if !knownProject() {
		return
	}
	if request.Preview {
		name, err := projectmeta.PrepareAvatar(h.dataDir, extension, normalized)
		if err != nil {
			fail(500, "projectMetadataSaveFailed")
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"avatar": name, "logo": discovery.Logo{Source: projectmeta.AvatarPath(h.dataDir, name), Data: encoded}})
		return
	}
	entry, err := projectmeta.SaveAvatar(r.Context(), h.dataDir, request.ID, extension, normalized, request.Revision)
	if err != nil {
		if errors.Is(err, projectmeta.ErrConflict) {
			fail(409, "conflict")
		} else {
			fail(500, "projectMetadataSaveFailed")
		}
		return
	}
	json.NewEncoder(w).Encode(struct {
		Revision int64          `json:"revision"`
		Logo     discovery.Logo `json:"logo"`
	}{Revision: entry.Revision, Logo: discovery.Logo{Source: projectmeta.AvatarPath(h.dataDir, entry.Avatar), Data: encoded}})
}
