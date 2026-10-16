package desktop

import (
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"commandmanager.local/core/internal/database"
	"commandmanager.local/core/resources"
)

type Appearance struct {
	Skin string `json:"skin"`
}

// All hosts read and commit the same SQLite preference.
func (h *Host) serveAppearance(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fail := func(status int, code string) {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": code})
	}
	store, err := database.Open(h.dataDir)
	if err != nil {
		fail(http.StatusInternalServerError, "storageFailed")
		return
	}
	switch r.Method {
	case http.MethodGet:
		appearance := Appearance{Skin: "comic"}
		if err := store.ReadSetting(r.Context(), "appearance", &appearance); err != nil && !errors.Is(err, sql.ErrNoRows) {
			fail(http.StatusInternalServerError, "skinLoadFailed")
			return
		}
		if _, ok := resources.LookupSkin(appearance.Skin); !ok {
			appearance.Skin = "comic"
		}
		json.NewEncoder(w).Encode(appearance)
	case http.MethodPut:
		if h.updates.blocked() {
			fail(http.StatusConflict, "updateBusy")
			return
		}
		var appearance Appearance
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&appearance) != nil || decoder.Decode(&struct{}{}) != io.EOF {
			fail(http.StatusBadRequest, "invalidRequest")
			return
		}
		if _, ok := resources.LookupSkin(appearance.Skin); !ok {
			fail(http.StatusBadRequest, "invalidRequest")
			return
		}
		if err := store.WriteSetting(r.Context(), "appearance", appearance); err != nil {
			fail(http.StatusInternalServerError, "skinSaveFailed")
			return
		}
		json.NewEncoder(w).Encode(appearance)
	default:
		fail(http.StatusMethodNotAllowed, "invalidRequest")
	}
}
