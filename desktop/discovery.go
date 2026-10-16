package desktop

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"time"

	"commandmanager.local/core/internal/discovery"
	"commandmanager.local/core/internal/projectmeta"
)

// Discovery belongs to the GUI host so new adapters work with a busy old runner.
// The result is a preview; saving still uses the existing revision-checked API.
func (h *Host) discover(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fail := func(status int, code string) {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": code})
	}
	if r.Method != http.MethodPost {
		fail(405, "invalidRequest")
		return
	}
	if !h.discovering.CompareAndSwap(false, true) {
		fail(409, "scanBusy")
		return
	}
	defer h.discovering.Store(false)
	var request struct {
		Directory string `json:"directory"`
		ProjectID string `json:"projectId"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	d.DisallowUnknownFields()
	if d.Decode(&request) != nil || d.Decode(&struct{}{}) != io.EOF || (request.Directory == "") == (request.ProjectID == "") {
		fail(400, "invalidRequest")
		return
	}
	rules, err := discovery.LoadRegistry(filepath.Join(h.dataDir, "discovery-rules"))
	if err != nil {
		fail(500, "scanFailed")
		return
	}
	directories := []string{request.Directory}
	projectName := ""
	if request.ProjectID != "" {
		definitions, err := h.projectDefinitions(r.Context())
		if err != nil {
			fail(503, "connectionLost")
			return
		}
		for _, project := range definitions.Projects {
			if project.ID == request.ProjectID {
				projectName = project.Name
				break
			}
		}
		if projectName == "" {
			fail(404, "projectMissing")
			return
		}
		entries, err := projectmeta.Load(h.dataDir)
		if err != nil {
			fail(500, "projectMetadataLoadFailed")
			return
		}
		directories = projectRoots(request.ProjectID, definitions, entries[request.ProjectID])
		if len(directories) == 0 {
			fail(400, "invalidDirectory")
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	result := discovery.Result{Candidates: []discovery.Candidate{}, Issues: []discovery.Issue{}}
	seen := map[string]bool{}
	for _, directory := range directories {
		next, err := discovery.DiscoverWithRegistry(ctx, directory, rules)
		if err != nil {
			if len(directories) == 1 {
				code := "scanFailed"
				if err.Error() == "invalidDirectory" {
					code = "invalidDirectory"
				}
				fail(400, code)
				return
			}
			result.Issues = append(result.Issues, discovery.Issue{Path: directory, Code: "scanReadFailed"})
			continue
		}
		h.rememberProjectRoot(next.Directory, nil)
		if result.Directory == "" {
			result.Directory, result.Name = next.Directory, next.Name
		}
		for _, candidate := range next.Candidates {
			if seen[candidate.ID] {
				continue
			}
			if len(result.Candidates) >= 256 {
				result.Issues = append(result.Issues, discovery.Issue{Path: directory, Code: "scanLimit"})
				break
			}
			seen[candidate.ID] = true
			result.Candidates = append(result.Candidates, candidate)
		}
		result.Issues = append(result.Issues, next.Issues...)
	}
	if result.Directory == "" {
		fail(400, "scanFailed")
		return
	}
	logos := []ProjectLogoOption{}
	if request.ProjectID == "" {
		// Metadata is optional; a partial scan must not discard command choices.
		metadataCtx, stop := context.WithTimeout(ctx, 4*time.Second)
		inspection, err := discovery.Inspect(metadataCtx, result.Directory, discovery.InspectOptions{Registry: rules})
		stop()
		remaining, limited := 512<<10, false
		for _, icon := range inspection.Icons {
			if len(logos) >= 32 || len(icon.Data) > remaining {
				limited = true
				continue
			}
			logos = append(logos, projectLogoOption(result.Directory, icon))
			remaining -= len(icon.Data)
		}
		if limited {
			result.Issues = append(result.Issues, discovery.Issue{Path: result.Directory, Code: "scanLimit"})
		}
		h.rememberProjectRoot(result.Directory, logos)
		result.Issues = append(result.Issues, inspection.Issues...)
		result.Names = inspection.Names
		result.Names = append(result.Names, discovery.NameCandidate{Name: result.Name, Source: result.Directory})
		if len(inspection.Names) > 0 {
			result.Name = inspection.Names[0].Name
		}
		if err != nil {
			result.Issues = append(result.Issues, discovery.Issue{Path: result.Directory, Code: "scanPartial"})
		}
	} else {
		result.Name = projectName
	}
	if len(result.Issues) > 128 {
		result.Issues = append(result.Issues[:127], discovery.Issue{Path: result.Directory, Code: "scanLimit"})
	}
	projectID := request.ProjectID
	if projectID == "" {
		definitions, err := h.projectDefinitions(r.Context())
		if err != nil {
			fail(503, "connectionLost")
			return
		}
		entries, err := projectmeta.Load(h.dataDir)
		if err != nil {
			fail(500, "projectMetadataLoadFailed")
			return
		}
		// Match saved roots even when a project has no commands.
		for _, project := range definitions.Projects {
			for _, root := range projectRoots(project.ID, definitions, entries[project.ID]) {
				if relative, err := filepath.Rel(root, result.Directory); err == nil && relative == "." {
					projectID = project.ID
					break
				}
			}
			if projectID != "" {
				break
			}
		}
	}
	json.NewEncoder(w).Encode(struct {
		discovery.Result
		ProjectID string              `json:"projectId,omitempty"`
		Logos     []ProjectLogoOption `json:"logos"`
	}{Result: result, ProjectID: projectID, Logos: logos})
}
