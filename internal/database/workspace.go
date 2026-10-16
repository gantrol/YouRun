package database

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
)

type Snapshot struct {
	RunsRevision     int64
	MetadataKeys     map[string]string
	Definitions      json.RawMessage
	Runs             json.RawMessage
	MetadataRevision int64
	SettingsRevision int64
}

func definitionsFrom(ctx context.Context, tx *sql.Tx) (json.RawMessage, error) {
	var revision int64
	var locale, projectData, commandData, collectionData string
	if err := tx.QueryRowContext(ctx, `SELECT version,coalesce((SELECT json_extract(value,'$') FROM settings WHERE key='locale'),'zh') FROM revisions WHERE scope='definitions'`).Scan(&revision, &locale); err != nil {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, projects.listSQL()).Scan(&projectData); err != nil {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, commands.listSQL()).Scan(&commandData); err != nil {
		return nil, err
	}
	if err := tx.QueryRowContext(ctx, collections.listSQL()).Scan(&collectionData); err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Revision    int64           `json:"revision"`
		Locale      string          `json:"locale"`
		Projects    json.RawMessage `json:"projects"`
		Commands    json.RawMessage `json:"commands"`
		Collections json.RawMessage `json:"collections"`
	}{revision, locale, json.RawMessage(projectData), json.RawMessage(commandData), json.RawMessage(collectionData)})
}

// ReadDefinitions avoids loading execution history for project metadata callers.
func (s *Store) ReadDefinitions(ctx context.Context) (json.RawMessage, error) {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	data, err := definitionsFrom(ctx, tx)
	if err != nil {
		return nil, err
	}
	return data, tx.Commit()
}

func (s *Store) Snapshot(ctx context.Context) (Snapshot, error) {
	// One transaction prevents a reader from combining different commits.
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Snapshot{}, err
	}
	defer tx.Rollback()
	var result Snapshot
	result.Definitions, err = definitionsFrom(ctx, tx)
	if err != nil {
		return result, err
	}
	var raw string
	if err = tx.QueryRowContext(ctx, runs.listSQL()).Scan(&raw); err != nil {
		return result, err
	}
	result.Runs = json.RawMessage(raw)
	if err = tx.QueryRowContext(ctx, `SELECT (SELECT version FROM revisions WHERE scope='metadata'),(SELECT version FROM revisions WHERE scope='settings'),(SELECT version FROM revisions WHERE scope='runs')`).Scan(&result.MetadataRevision, &result.SettingsRevision, &result.RunsRevision); err != nil {
		return result, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT id,"+metadata.rowJSON()+" FROM project_metadata")
	if err != nil {
		return result, err
	}
	result.MetadataKeys = map[string]string{}
	for rows.Next() {
		var id, value string
		if err = rows.Scan(&id, &value); err != nil {
			rows.Close()
			return result, err
		}
		digest := sha256.Sum256([]byte(value))
		result.MetadataKeys[id] = hex.EncodeToString(digest[:])
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	return result, tx.Commit()
}

func (s *Store) SaveDefinitions(ctx context.Context, expected int64, raw []byte) (json.RawMessage, error) {
	var next struct {
		Locale                          string
		Projects, Commands, Collections json.RawMessage
	}
	if err := json.Unmarshal(raw, &next); err != nil {
		return nil, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var revision int64
	if err = tx.QueryRowContext(ctx, "SELECT version FROM revisions WHERE scope='definitions'").Scan(&revision); err != nil {
		return nil, err
	}
	if revision != expected {
		return nil, ErrConflict
	}
	// Older clients detached a project's commands in the same request that
	// deleted the project. Keep that deletion cascading even when the incoming
	// project field is null; an explicit move to another project is still kept.
	var commandData string
	if err = tx.QueryRowContext(ctx, `SELECT coalesce(json_group_array(json(CASE WHEN json_type(incoming.value,'$.aliases') IS NULL OR json_type(incoming.value,'$.aliases')='null'
 THEN json_set(incoming.value,'$.aliases',json(coalesce((SELECT aliases FROM commands WHERE id=json_extract(incoming.value,'$.id')),'[]')))
 ELSE incoming.value END)),'[]')
FROM json_each(?) AS incoming
WHERE NOT EXISTS (
 SELECT 1 FROM commands AS saved JOIN projects AS parent ON parent.id=saved.project
 WHERE saved.id=json_extract(incoming.value,'$.id')
 AND json_extract(incoming.value,'$.project') IS NULL
 AND NOT EXISTS (SELECT 1 FROM json_each(?) AS retained WHERE json_extract(retained.value,'$.id')=parent.id)
)`, string(next.Commands), string(next.Projects)).Scan(&commandData); err != nil {
		return nil, err
	}
	next.Commands = json.RawMessage(commandData)
	for i, t := range []table{projects, commands} {
		data := []json.RawMessage{next.Projects, next.Commands}[i]
		if _, err = tx.ExecContext(ctx, t.upsertSQL(), string(data)); err != nil {
			return nil, err
		}
	}
	// Clients predating collections omit this field; their saves preserve it.
	if len(next.Collections) > 0 && string(next.Collections) != "null" {
		if _, err = tx.ExecContext(ctx, collections.upsertSQL(), string(next.Collections)); err != nil {
			return nil, err
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM collections WHERE id NOT IN (SELECT json_extract(value,'$.id') FROM json_each(?))", string(next.Collections)); err != nil {
			return nil, err
		}
	}
	// Move retained commands before deleting projects, so cascade cleanup never
	// removes a command (or its new button binding) that is merely reassigned.
	for i, t := range []table{commands, projects} {
		data := []json.RawMessage{next.Commands, next.Projects}[i]
		if _, err = tx.ExecContext(ctx, "DELETE FROM "+t.name+" WHERE id NOT IN (SELECT json_extract(value,'$.id') FROM json_each(?) WHERE type='object')", string(data)); err != nil {
			return nil, err
		}
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM project_metadata WHERE id NOT IN (SELECT id FROM projects)"); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES ('locale',json_quote(?)) ON CONFLICT(key) DO UPDATE SET value=excluded.value WHERE settings.value IS NOT excluded.value`, next.Locale); err != nil {
		return nil, err
	}
	result, err := definitionsFrom(ctx, tx)
	if err != nil {
		return nil, err
	}
	return result, tx.Commit()
}

func (s *Store) ReadMetadata(ctx context.Context) (map[string]json.RawMessage, error) {
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT coalesce(json_group_object(id,json(record)),'{}') FROM (SELECT id,"+metadata.rowJSON()+" AS record FROM project_metadata)").Scan(&raw)
	if err != nil {
		return nil, err
	}
	var result map[string]json.RawMessage
	err = json.Unmarshal([]byte(raw), &result)
	return result, err
}

// UpdateMetadata keeps the read/merge/write operation inside one SQLite write
// transaction. The callback also handles an avatar file before committing its
// reference, so readers never observe a missing newly selected image.
func (s *Store) UpdateMetadata(ctx context.Context, id string, change func(json.RawMessage) (json.RawMessage, error)) (json.RawMessage, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	result, err := updateMetadataTx(ctx, tx, id, change)
	if err != nil {
		return nil, err
	}
	return result, tx.Commit()
}

func updateMetadataTx(ctx context.Context, tx *sql.Tx, id string, change func(json.RawMessage) (json.RawMessage, error)) (json.RawMessage, error) {
	var err error
	var raw string
	err = tx.QueryRowContext(ctx, "SELECT "+metadata.rowJSON()+" FROM project_metadata WHERE id=?", id).Scan(&raw)
	if err == sql.ErrNoRows {
		var count int
		if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM project_metadata").Scan(&count); err != nil {
			return nil, err
		}
		if count >= 1000 {
			return nil, errors.New("project metadata limit")
		}
		raw = `{"roots":[],"revision":0}`
	} else if err != nil {
		return nil, err
	}
	next, err := change(json.RawMessage(raw))
	if err != nil {
		return nil, err
	}
	var entry map[string]json.RawMessage
	if err = json.Unmarshal(next, &entry); err != nil {
		return nil, err
	}
	entry["id"], _ = json.Marshal(id)
	rows, err := json.Marshal([]any{entry})
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, metadata.upsertSQL(), string(rows)); err != nil {
		return nil, err
	}
	var size int
	if err = tx.QueryRowContext(ctx, `SELECT coalesce(sum(length(roots)+length("logoId")+length(avatar)+length(id)+80),0) FROM project_metadata`).Scan(&size); err != nil {
		return nil, err
	}
	if size >= 1<<20 {
		return nil, errors.New("project metadata limit")
	}
	if err = tx.QueryRowContext(ctx, "SELECT "+metadata.rowJSON()+" FROM project_metadata WHERE id=?", id).Scan(&raw); err != nil {
		return nil, err
	}
	return json.RawMessage(raw), nil
}

// UpdateProject commits a project and its display metadata together. The callback
// compares the editor's baseline against the same transaction used for the write.
func (s *Store) UpdateProject(ctx context.Context, id string, change func(json.RawMessage, json.RawMessage) (json.RawMessage, json.RawMessage, error)) (json.RawMessage, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	definitions, err := definitionsFrom(ctx, tx)
	if err != nil {
		return nil, err
	}
	var project json.RawMessage
	_, err = updateMetadataTx(ctx, tx, id, func(entry json.RawMessage) (json.RawMessage, error) {
		var next json.RawMessage
		project, next, err = change(definitions, entry)
		return next, err
	})
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, projects.upsertSQL(), "["+string(project)+"]"); err != nil {
		return nil, err
	}
	result, err := definitionsFrom(ctx, tx)
	if err != nil {
		return nil, err
	}
	return result, tx.Commit()
}
