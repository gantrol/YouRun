package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"

	"commandmanager.local/core/internal/engine"
)

// Transport and event reconstruction are independent of MyGo and editable drafts.
type workspaceRepository struct{ api *client }

type workspaceEvent struct {
	Snapshot *engine.Snapshot `json:"snapshot"`
	Patch    *workspacePatch  `json:"patch"`
	Error    string           `json:"error"`
}

type workspacePatch struct {
	Definitions      *engine.Definitions `json:"definitions"`
	Runs             *runPatch           `json:"runs"`
	MetadataRevision *int64              `json:"metadataRevision"`
	MetadataKeys     map[string]string   `json:"metadataKeys"`
	SettingsRevision *int64              `json:"settingsRevision"`
	StorageError     *bool               `json:"storageError"`
}

type runPatch struct {
	Upserts  []engine.Run `json:"upserts"`
	Removed  []string     `json:"removed"`
	Order    []string     `json:"order"`
	Revision int64        `json:"revision"`
}

func (r *workspaceRepository) read(ctx context.Context) (engine.Snapshot, error) {
	var state engine.Snapshot
	err := r.api.requestContext(ctx, "GET", "state", nil, &state)
	return state, err
}

func (r *workspaceRepository) watch(ctx context.Context, receive func(engine.Snapshot), failed func(error)) {
	var current *engine.Snapshot
	r.api.subscribe(ctx, func() string { return "events" }, func(data []byte) error {
		var event workspaceEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return err
		}
		if event.Error != "" {
			return errors.New(event.Error)
		}
		if event.Snapshot != nil {
			current = event.Snapshot
		} else if event.Patch != nil && current != nil {
			next := *current
			if err := event.Patch.apply(&next); err != nil {
				return err
			}
			current = &next
		} else {
			return errors.New("connectionLost")
		}
		receive(*current)
		return nil
	}, func(err error) {
		// Every new connection must supply a fresh full snapshot before patches.
		current = nil
		failed(err)
	})
}

func (p *workspacePatch) apply(next *engine.Snapshot) error {
	if p.Definitions != nil && p.Definitions.Revision >= next.Definitions.Revision {
		next.Definitions = *p.Definitions
	}
	if p.Runs != nil && p.Runs.Revision >= next.RunsRevision {
		rows := make(map[string]engine.Run, len(next.Runs)+len(p.Runs.Upserts))
		for _, run := range next.Runs {
			rows[run.ID] = run
		}
		for _, id := range p.Runs.Removed {
			delete(rows, id)
		}
		for _, run := range p.Runs.Upserts {
			rows[run.ID] = run
		}
		runs := make([]engine.Run, 0, len(p.Runs.Order))
		for _, id := range p.Runs.Order {
			run, ok := rows[id]
			if !ok {
				return errors.New("connectionLost")
			}
			runs = append(runs, run)
			delete(rows, id)
		}
		if len(rows) != 0 {
			return errors.New("connectionLost")
		}
		next.Runs, next.RunsRevision = runs, p.Runs.Revision
	}
	if p.MetadataRevision != nil && *p.MetadataRevision >= next.MetadataRevision {
		next.MetadataRevision, next.MetadataKeys = *p.MetadataRevision, p.MetadataKeys
	}
	if p.SettingsRevision != nil && *p.SettingsRevision >= next.SettingsRevision {
		next.SettingsRevision = *p.SettingsRevision
	}
	if p.StorageError != nil {
		next.StorageError = *p.StorageError
	}
	return nil
}

type logEvent struct {
	Chunk *engine.LogChunk `json:"chunk"`
	Reset bool             `json:"reset"`
	Error string           `json:"error"`
}

func (r *workspaceRepository) watchLog(ctx context.Context, id string, receive func(logEvent) error, failed func(error)) {
	var cursor *int64
	r.api.subscribe(ctx, func() string {
		path := "log-events?id=" + url.QueryEscape(id)
		if cursor != nil {
			path += "&cursor=" + strconv.FormatInt(*cursor, 10)
		}
		return path
	}, func(data []byte) error {
		var event logEvent
		if err := json.Unmarshal(data, &event); err != nil {
			return err
		}
		if event.Error != "" {
			return errors.New(event.Error)
		}
		if event.Chunk == nil {
			return errors.New("connectionLost")
		}
		if err := receive(event); err != nil {
			cursor = nil // Restart from the bounded tail if the cursor no longer fits.
			return err
		}
		next := event.Chunk.NextCursor
		cursor = &next
		return nil
	}, failed)
}
