package desktop

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type StartupSettings struct {
	Enabled   bool   `json:"enabled"`
	Supported bool   `json:"supported"`
	Reason    string `json:"reason,omitempty"`
}

var startupMu sync.Mutex

// The OS registration is the source of truth. Opening a host never installs or
// repairs a login item, so an external disable remains effective.
func (h *Host) serveStartup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fail := func(status int, code string) {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": code})
	}
	if r.Method != http.MethodGet && r.Method != http.MethodPut {
		fail(http.StatusMethodNotAllowed, "invalidRequest")
		return
	}
	startupMu.Lock()
	defer startupMu.Unlock()
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
		executable := ""
		if *input.Enabled {
			var err error
			executable, err = startupExecutable()
			if err != nil {
				fail(http.StatusConflict, err.Error())
				return
			}
		}
		if err := writeStartup(r.Context(), *input.Enabled, executable); err != nil {
			code := "startupSaveFailed"
			if err.Error() == "startupUnsupported" || err.Error() == "startupDisabledBySystem" {
				code = err.Error()
			}
			fail(http.StatusInternalServerError, code)
			return
		}
	}
	settings, err := readStartup(r.Context())
	if err != nil {
		fail(http.StatusInternalServerError, "startupLoadFailed")
		return
	}
	json.NewEncoder(w).Encode(settings)
}

func startupExecutable() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", errors.New("startupSaveFailed")
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil || !filepath.IsAbs(executable) {
		return "", errors.New("startupSaveFailed")
	}
	// go run/go test executables and debugger binaries disappear after use.
	for _, part := range strings.FieldsFunc(executable, func(r rune) bool { return r == '/' || r == '\\' }) {
		if strings.HasPrefix(part, "go-build") || strings.HasPrefix(part, "__debug_bin") {
			return "", errors.New("startupDevelopment")
		}
	}
	if strings.HasSuffix(executable, ".test") || strings.HasSuffix(executable, ".test.exe") {
		return "", errors.New("startupDevelopment")
	}
	return executable, nil
}
