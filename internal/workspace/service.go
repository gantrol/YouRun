// Package workspace owns application data independently of the execution records.
package workspace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"commandmanager.local/core/internal/database"
	"commandmanager.local/core/internal/engine"
	"commandmanager.local/core/internal/projectmeta"
)

type Request func(method, path string, body []byte) (int, []byte, error)

type Service struct {
	store     *database.Store
	execution Request
}

func New(directory string, execution Request) (*Service, error) {
	store, err := database.Open(directory)
	if err != nil {
		return nil, err
	}
	return &Service{store: store, execution: execution}, nil
}

func result(status int, value any) (int, []byte, error) {
	data, err := json.Marshal(value)
	return status, data, err
}

func failure(status int, code string) (int, []byte, error) {
	return result(status, map[string]string{"error": code})
}

func decode(data []byte, value any) bool {
	if len(data) > 2<<20 {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	return d.Decode(value) == nil && d.Decode(&struct{}{}) == io.EOF
}

func (s *Service) Definitions(ctx context.Context) (engine.Definitions, error) {
	data, err := s.store.ReadDefinitions(ctx)
	if err != nil {
		return engine.Definitions{}, err
	}
	var definitions engine.Definitions
	if err = json.Unmarshal(data, &definitions); err != nil {
		return engine.Definitions{}, err
	}
	return definitions, engine.ValidateDefinitions(definitions)
}

func (s *Service) snapshot(ctx context.Context) (engine.Snapshot, error) {
	stored, err := s.store.Snapshot(ctx)
	if err != nil {
		return engine.Snapshot{}, err
	}
	state := engine.Snapshot{RunsRevision: stored.RunsRevision, MetadataKeys: stored.MetadataKeys, Protocol: engine.Protocol, MetadataRevision: stored.MetadataRevision, SettingsRevision: stored.SettingsRevision}
	if err = json.Unmarshal(stored.Definitions, &state.Definitions); err != nil {
		return state, err
	}
	if err = engine.ValidateDefinitions(state.Definitions); err != nil {
		return state, err
	}
	err = json.Unmarshal(stored.Runs, &state.Runs)
	return state, err
}

func (s *Service) Request(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	switch method + " " + path {
	case "GET /api/state":
		status, data, err := s.execution(method, path, nil)
		if err != nil || status != http.StatusOK {
			return status, data, err
		}
		var runtime engine.RuntimeState
		if err = json.Unmarshal(data, &runtime); err != nil {
			return 0, nil, err
		}
		runs, err := json.Marshal(runtime.Runs)
		if err != nil {
			return 0, nil, err
		}
		if err = s.store.SyncRuntime(ctx, runtime.ObservedAt, runs); err != nil {
			return failure(500, "storageFailed")
		}
		state, err := s.snapshot(ctx)
		if err != nil {
			return failure(500, "storageFailed")
		}
		state.Platform, state.StorageError = runtime.Platform, runtime.StorageError
		// Diagnostics belong to the execution record, outside the editable SQL
		// history schema. Preserve database edits while surfacing current errors.
		diagnostics := map[string]string{}
		for _, run := range runtime.Runs {
			diagnostics[run.ID] = run.Error
		}
		for i := range state.Runs {
			state.Runs[i].Error = diagnostics[state.Runs[i].ID]
		}
		return result(200, state)
	case "PUT /api/definitions":
		var next engine.Definitions
		if !decode(body, &next) || engine.ValidateDefinitions(next) != nil {
			return failure(400, "invalidDefinition")
		}
		saved, err := s.store.SaveDefinitions(ctx, next.Revision, body)
		if errors.Is(err, database.ErrConflict) {
			return failure(409, "conflict")
		}
		if err != nil {
			return failure(500, "storageFailed")
		}
		return 200, saved, nil
	case "POST /api/run":
		var launch engine.Launch
		if !decode(body, &launch) {
			return failure(400, "invalidRequest")
		}
		state, err := s.snapshot(ctx)
		if err != nil {
			return failure(500, "storageFailed")
		}
		for _, command := range state.Definitions.Commands {
			if command.ID != launch.CommandID {
				continue
			}
			title := command.Title
			for _, project := range state.Definitions.Projects {
				if command.Project != nil && project.ID == *command.Project {
					title = project.Name + " · " + title
				}
			}
			values := map[string]string{}
			for _, field := range command.Fields {
				values[field] = launch.Values[field]
			}
			directory := launch.Directory
			if directory == "" {
				directory = command.Directory
			}
			var wsID, wsName string
			if command.Project != nil {
				if metaRows, metaErr := s.store.ReadMetadata(ctx); metaErr == nil {
					var entry projectmeta.Entry
					if raw, ok := metaRows[*command.Project]; ok {
						_ = json.Unmarshal(raw, &entry)
					}
					var fallbackDirs []string
					for _, cmd := range state.Definitions.Commands {
						if cmd.Project != nil && *cmd.Project == *command.Project && cmd.Directory != "" {
							fallbackDirs = append(fallbackDirs, cmd.Directory)
						}
					}
					locations, _ := projectmeta.ResolveLocations(entry, fallbackDirs)
					cleanDir := filepath.Clean(directory)
					for _, loc := range locations {
						rel, relErr := filepath.Rel(filepath.Clean(loc.Directory), cleanDir)
						if relErr == nil && (rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))) {
							wsID, wsName = loc.ID, loc.Name
							break
						}
					}
				}
			}
			execution := engine.Execution{ID: launch.ID, CommandID: command.ID, Title: title, Text: command.Text, Shell: command.Shell, Platform: command.Platform, Directory: directory, Fields: command.Fields, Values: values, OmitOutput: command.OmitOutput, WorkspaceID: wsID, WorkspaceName: wsName}
			if err = execution.Validate(); err != nil {
				return failure(400, err.Error())
			}
			data, err := json.Marshal(execution)
			if err != nil {
				return 0, nil, err
			}
			return s.execution(method, path, data)
		}
		return failure(404, "commandMissing")
	case "POST /api/import":
		return s.importProject(ctx, body)
	}
	if method == http.MethodPost && (path == "/api/replay" || path == "/api/stop") || method == http.MethodGet && strings.HasPrefix(path, "/api/logs?") {
		return s.execution(method, path, body)
	}
	return failure(404, "invalidRequest")
}

func (s *Service) Locale(ctx context.Context) string {
	locale := "zh"
	_ = s.store.ReadSetting(ctx, "locale", &locale)
	return locale
}
