package desktop

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"commandmanager.local/core/internal/discovery"
	"commandmanager.local/core/internal/engine"
	"commandmanager.local/core/internal/projectmeta"
)

type ProjectLogoOption struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Source string `json:"source"`
	Data   string `json:"data"`
}

func projectLogoOption(directory string, icon discovery.IconCandidate) ProjectLogoOption {
	source := filepath.Join(directory, filepath.FromSlash(icon.Source))
	identity := source
	if runtime.GOOS == "windows" {
		identity = strings.ToLower(identity)
	}
	digest := sha256.Sum256([]byte(identity))
	return ProjectLogoOption{ID: hex.EncodeToString(digest[:]), Name: icon.Name, Source: source, Data: icon.Data}
}

type ProjectWorkspaceDetail struct {
	ID        string                `json:"id"`
	Name      string                `json:"name"`
	Directory string                `json:"directory"`
	IsDefault bool                  `json:"isDefault,omitempty"`
	Source    engine.SourceSnapshot `json:"source"`
}

type ProjectDetails struct {
	Directory       string                    `json:"directory"`
	Logo            *discovery.Logo           `json:"logo"`
	Logos           []ProjectLogoOption       `json:"logos"`
	Names           []discovery.NameCandidate `json:"names"`
	SelectedLogo    string                    `json:"selectedLogo"`
	CustomAvatar    bool                      `json:"customAvatar"`
	Revision        int64                     `json:"revision"`
	ScannedAt       string                    `json:"scannedAt"`
	Issues          []discovery.Issue         `json:"issues"`
	Workspaces      []ProjectWorkspaceDetail  `json:"workspaces,omitempty"`
	ActiveWorkspace string                    `json:"activeWorkspace,omitempty"`
	cacheKey        string
	scanned         time.Time
	dependencies    map[string]fileStamp
}

type fileStamp struct {
	Size int64
	Time int64
	Mode os.FileMode
}

func stamp(path string) fileStamp {
	info, err := os.Lstat(path)
	if err != nil {
		return fileStamp{Size: -1}
	}
	return fileStamp{Size: info.Size(), Time: info.ModTime().UnixNano(), Mode: info.Mode()}
}

func (details ProjectDetails) fresh(key string) bool {
	if details.cacheKey != key || time.Since(details.scanned) >= 30*time.Second {
		return false
	}
	for path, old := range details.dependencies {
		if stamp(path) != old {
			return false
		}
	}
	return true
}

func projectRoots(id string, definitions engine.Definitions, entry projectmeta.Entry) []string {
	if len(entry.Roots) > 0 {
		return entry.Roots
	}
	if directory := projectDirectory(id, definitions); directory != "" {
		return []string{directory}
	}
	return nil
}

// A directory must already belong to this project's saved commands. Never walk
// upward to a guessed common ancestor of unrelated command locations.
func projectDirectory(id string, definitions engine.Definitions) string {
	paths := []string{}
	for _, command := range definitions.Commands {
		if command.Project != nil && *command.Project == id && filepath.IsAbs(command.Directory) && !strings.Contains(strings.ToLower(command.Directory), "trash") {
			paths = append(paths, filepath.Clean(command.Directory))
		}
	}
	for _, candidate := range paths {
		contains := true
		for _, path := range paths {
			rel, err := filepath.Rel(candidate, path)
			if err != nil || !filepath.IsLocal(rel) {
				contains = false
				break
			}
		}
		if contains {
			return candidate
		}
	}
	return ""
}

func (h *Host) projectDefinitions(ctx context.Context) (engine.Definitions, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	definitions, err := h.workspace.Definitions(ctx)
	if err != nil {
		return engine.Definitions{}, errors.New("connectionLost")
	}
	return definitions, nil
}

func (h *Host) serveProjectDetails(w http.ResponseWriter, r *http.Request) {
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
		IDs       []string `json:"ids"`
		RefreshID string   `json:"refreshId"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 2048))
	d.DisallowUnknownFields()
	if d.Decode(&request) != nil || d.Decode(&struct{}{}) != io.EOF || len(request.IDs) > 8 {
		fail(400, "invalidRequest")
		return
	}
	// Two workers bound simultaneous reads to two inspection budgets. A slow
	// volume must not prevent every other project's icon from becoming ready.
	if h.loadingProjects.Add(1) > 2 {
		h.loadingProjects.Add(-1)
		fail(409, "scanBusy")
		return
	}
	defer h.loadingProjects.Add(-1)
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	definitions, err := h.projectDefinitions(ctx)
	if err != nil {
		fail(503, "connectionLost")
		return
	}
	metadata, err := projectmeta.LoadContext(ctx, h.dataDir)
	if err != nil {
		fail(500, "projectMetadataLoadFailed")
		return
	}
	rules, err := discovery.LoadRegistry(filepath.Join(h.dataDir, "discovery-rules"))
	if err != nil {
		fail(500, "scanFailed")
		return
	}
	known := map[string]bool{}
	for _, project := range definitions.Projects {
		known[project.ID] = true
	}
	for _, id := range request.IDs {
		if !known[id] {
			fail(404, "projectMissing")
			return
		}
	}
	result := map[string]ProjectDetails{}
	for _, id := range request.IDs {
		if ctx.Err() != nil {
			fail(408, "scanFailed")
			return
		}
		entry := metadata[id]
		roots := projectRoots(id, definitions, entry)
		name := ""
		for _, project := range definitions.Projects {
			if project.ID == id {
				name = project.Name
			}
		}
		// Keep every application available for explicit selection. The display
		// name influences only automatic selection, never a saved user choice.
		options := discovery.InspectOptions{Registry: rules}
		for _, command := range definitions.Commands {
			if command.Project != nil && *command.Project == id {
				options.Directories = append(options.Directories, command.Directory)
				if command.Source != nil {
					options.Sources = append(options.Sources, *command.Source)
				}
			}
		}
		keyBytes, _ := json.Marshal([]any{roots, name, options.Sources, options.Directories, entry, rules.Digest()})
		key := string(keyBytes)
		h.projectDetailsMu.Lock()
		cached, ok := h.projectDetails[id]
		h.projectDetailsMu.Unlock()
		if !ok || !cached.fresh(key) || request.RefreshID == id {
			cached = ProjectDetails{SelectedLogo: entry.LogoID, CustomAvatar: entry.Avatar != "", Revision: entry.Revision, Logos: []ProjectLogoOption{}, Names: []discovery.NameCandidate{}, Issues: []discovery.Issue{}, dependencies: map[string]fileStamp{}}
			seenIcons, namedIcons := map[string]bool{}, map[string]bool{}
			if len(roots) > 0 {
				cached.Directory = roots[0]
			}
			for _, directory := range roots {
				inspection, inspectErr := discovery.Inspect(ctx, directory, options)
				if inspectErr != nil {
					cached.Issues = append(cached.Issues, discovery.Issue{Path: directory, Code: "scanReadFailed"})
					continue
				}
				cached.Issues = append(cached.Issues, inspection.Issues...)
				for _, candidate := range inspection.Names {
					if len(cached.Names) < 128 {
						cached.Names = append(cached.Names, candidate)
					}
				}
				for _, icon := range inspection.Icons {
					logo := projectLogoOption(directory, icon)
					iconID := logo.ID
					if strings.TrimSpace(name) != "" && strings.EqualFold(strings.TrimSpace(icon.Name), strings.TrimSpace(name)) {
						namedIcons[iconID] = true
					}
					if !seenIcons[iconID] {
						seenIcons[iconID] = true
						cached.Logos = append(cached.Logos, logo)
					}
				}
				for _, dependency := range inspection.Dependencies {
					if filepath.IsLocal(dependency) && !strings.Contains(strings.ToLower(dependency), "trash") && len(cached.dependencies) < 512 {
						path := filepath.Join(directory, filepath.FromSlash(dependency))
						cached.dependencies[path] = stamp(path)
					}
				}
			}
			automaticID := ""
			if len(cached.Logos) == 1 {
				automaticID = cached.Logos[0].ID
			} else if len(namedIcons) == 1 {
				for iconID := range namedIcons {
					automaticID = iconID
				}
			}
			preferredID := entry.LogoID
			if preferredID == "" {
				preferredID = automaticID
			}
			// Reserve space for the selected image, then bound each project's
			// response so a batch fits the native client's 8 MiB response limit.
			sort.SliceStable(cached.Logos, func(i, j int) bool {
				return cached.Logos[i].ID == preferredID && cached.Logos[j].ID != preferredID
			})
			logos, remaining, limited := cached.Logos[:0], 512<<10, false
			for _, candidate := range cached.Logos {
				if len(logos) >= 32 || len(candidate.Data) > remaining {
					limited = true
					continue
				}
				logos = append(logos, candidate)
				remaining -= len(candidate.Data)
			}
			cached.Logos = logos
			if limited {
				cached.Issues = append(cached.Issues, discovery.Issue{Path: cached.Directory, Code: "scanLimit"})
			}
			for _, candidate := range cached.Logos {
				if candidate.ID == preferredID {
					cached.Logo = &discovery.Logo{Source: candidate.Source, Data: candidate.Data}
					break
				}
			}
			if entry.LogoID != "" && cached.Logo == nil {
				cached.Issues = append(cached.Issues, discovery.Issue{Path: cached.Directory, Code: "logoSelectionMissing"})
			}
			if entry.Avatar != "" {
				path := projectmeta.AvatarPath(h.dataDir, entry.Avatar)
				cached.dependencies[path] = stamp(path)
				data, readErr := projectmeta.ReadAvatar(h.dataDir, entry.Avatar)
				encoded := discovery.EncodeLogo(data, filepath.Ext(entry.Avatar))
				if readErr == nil && encoded != "" {
					cached.Logo = &discovery.Logo{Source: path, Data: encoded}
				} else {
					cached.Issues = append(cached.Issues, discovery.Issue{Code: "avatarReadFailed"})
				}
			}
			if ctx.Err() != nil {
				fail(408, "scanFailed")
				return
			}
			h.projectDetailsMu.Lock()
			if h.projectDetails == nil {
				h.projectDetails = map[string]ProjectDetails{}
			}
			cached.cacheKey, cached.scanned = key, time.Now()
			cached.ScannedAt = cached.scanned.UTC().Format(time.RFC3339Nano)
			h.projectDetails[id] = cached
			h.projectDetailsMu.Unlock()
		}
		locations, activeID := projectmeta.ResolveLocations(entry, options.Directories)
		workspaces := make([]ProjectWorkspaceDetail, 0, len(locations))
		for _, loc := range locations {
			src := engine.InspectSource(ctx, loc.Directory, loc.ID, loc.Name, request.RefreshID == id)
			workspaces = append(workspaces, ProjectWorkspaceDetail{
				ID:        loc.ID,
				Name:      loc.Name,
				Directory: loc.Directory,
				IsDefault: loc.IsDefault,
				Source:    src,
			})
			if loc.ID == activeID {
				cached.Directory = loc.Directory
			}
		}
		cached.Workspaces = workspaces
		cached.ActiveWorkspace = activeID
		result[id] = cached
	}
	h.projectDetailsMu.Lock()
	for id := range h.projectDetails {
		if !known[id] {
			delete(h.projectDetails, id)
		}
	}
	h.projectDetailsMu.Unlock()
	json.NewEncoder(w).Encode(result)
}

func (h *Host) serveOpenProject(w http.ResponseWriter, r *http.Request) {
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
		ID         string `json:"id"`
		Directory  string `json:"directory,omitempty"`
		Opener     string `json:"opener,omitempty"`
		Executable string `json:"executable,omitempty"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	d.DisallowUnknownFields()
	if d.Decode(&request) != nil || d.Decode(&struct{}{}) != io.EOF || (request.Opener != "" && request.Executable != "") {
		fail(400, "invalidRequest")
		return
	}
	directory := ""
	if request.ID == "" && request.Directory != "" && request.Opener == "" && request.Executable == "" {
		if filepath.IsAbs(request.Directory) && !strings.Contains(strings.ToLower(request.Directory), "trash") {
			directory = filepath.Clean(request.Directory)
		}
	} else {
		definitions, err := h.projectDefinitions(r.Context())
		if err != nil {
			fail(503, "connectionLost")
			return
		}
		metadata, err := projectmeta.Load(h.dataDir)
		if err != nil {
			fail(500, "projectMetadataLoadFailed")
			return
		}
		var cmdDirs []string
		for _, cmd := range definitions.Commands {
			if cmd.Project != nil && *cmd.Project == request.ID && cmd.Directory != "" {
				cmdDirs = append(cmdDirs, cmd.Directory)
			}
		}
		locations, activeID := projectmeta.ResolveLocations(metadata[request.ID], cmdDirs)
		roots := projectRoots(request.ID, definitions, metadata[request.ID])
		for _, project := range definitions.Projects {
			if project.ID == request.ID {
				if len(roots) > 0 {
					directory = roots[0]
				}
				for _, loc := range locations {
					if (request.Directory != "" && filepath.Clean(loc.Directory) == filepath.Clean(request.Directory)) || (request.Directory == "" && loc.ID == activeID) {
						directory = loc.Directory
						break
					}
				}
			}
		}
	}
	info, err := os.Stat(directory)
	if directory == "" || err != nil || !info.IsDir() {
		fail(400, "invalidDirectory")
		return
	}
	if request.Opener != "" || request.Executable != "" {
		if err := h.openProjectWith(r.Context(), directory, request.Opener, request.Executable); err != nil {
			fail(400, err.Error())
			return
		}
	} else if err := openProjectDirectory(directory); err != nil {
		fail(500, "projectOpenFailed")
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}

func normalizeExternalURL(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || len(trimmed) > 2048 {
		return "", errors.New("invalidRequest")
	}
	for _, ch := range trimmed {
		if ch < 0x20 || ch == 0x7f {
			return "", errors.New("invalidRequest")
		}
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return "", errors.New("invalidRequest")
	}
	hostname := parsed.Hostname()
	if hostname == "" {
		return "", errors.New("invalidRequest")
	}
	port := parsed.Port()
	if hostname == "0.0.0.0" {
		if port != "" {
			parsed.Host = net.JoinHostPort("127.0.0.1", port)
		} else {
			parsed.Host = "127.0.0.1"
		}
	} else if hostname == "::" {
		if port != "" {
			parsed.Host = net.JoinHostPort("localhost", port)
		} else {
			parsed.Host = "localhost"
		}
	}
	return parsed.String(), nil
}

func (h *Host) serveOpenURL(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fail := func(status int, code string) {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": code})
	}
	if r.Method != http.MethodPost {
		fail(http.StatusMethodNotAllowed, "invalidRequest")
		return
	}
	var request struct {
		URL string `json:"url"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096))
	d.DisallowUnknownFields()
	if d.Decode(&request) != nil || d.Decode(&struct{}{}) != io.EOF {
		fail(http.StatusBadRequest, "invalidRequest")
		return
	}
	target, err := normalizeExternalURL(request.URL)
	if err != nil {
		fail(http.StatusBadRequest, "invalidRequest")
		return
	}
	if err := openExternalURL(target); err != nil {
		fail(http.StatusInternalServerError, "urlOpenFailed")
		return
	}
	json.NewEncoder(w).Encode(map[string]bool{"ok": true})
}
