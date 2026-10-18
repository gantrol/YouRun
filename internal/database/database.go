// Package database owns the authoritative SQLite store shared by all local hosts.
package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const Filename = "command-manager.sqlite3"

var ErrConflict = errors.New("conflict")

type Store struct{ db *sql.DB }

var stores = struct {
	sync.Mutex
	byPath map[string]*Store
}{byPath: map[string]*Store{}}

// Open reuses a bounded pool for the lifetime of the process. Callers perform
// database work on server/background goroutines, never the native UI thread.
func Open(directory string) (*Store, error) {
	path, err := filepath.Abs(filepath.Join(directory, Filename))
	if err != nil {
		return nil, err
	}
	if strings.Contains(strings.ToLower(path), "trash") {
		return nil, errors.New("invalidDirectory")
	}
	stores.Lock()
	defer stores.Unlock()
	if store := stores.byPath[path]; store != nil {
		return store, nil
	}
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	uriPath := filepath.ToSlash(path)
	if !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	u := url.URL{Scheme: "file", Path: uriPath}
	q := u.Query()
	q.Add("_pragma", "busy_timeout(5000)")
	q.Add("_pragma", "foreign_keys(1)")
	q.Add("_pragma", "journal_mode(WAL)")
	q.Add("_pragma", "synchronous(FULL)")
	q.Set("_txlock", "immediate")
	u.RawQuery = q.Encode()
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(4)
	store := &Store{db: db}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err = store.migrate(ctx, filepath.Dir(path)); err != nil {
		db.Close()
		return nil, err
	}
	if err = store.migrateRuntime(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err = store.migrateCollections(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err = store.migrateOutputPolicy(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err = store.migrateSourceAndWorkspaces(ctx); err != nil {
		db.Close()
		return nil, err
	}
	if err = os.Chmod(path, 0600); err != nil {
		db.Close()
		return nil, err
	}
	stores.byPath[path] = store
	return store, nil
}

func legacy(directory, name string, fallback string) ([]byte, error) {
	f, err := os.Open(filepath.Join(directory, name))
	if errors.Is(err, os.ErrNotExist) {
		return []byte(fallback), nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (8<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 8<<20 || !json.Valid(data) {
		return nil, fmt.Errorf("invalid legacy %s", name)
	}
	return data, nil
}

func (s *Store) migrate(ctx context.Context, directory string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version >= 1 && version <= 4 {
		return tx.Commit()
	}
	if version != 0 {
		return fmt.Errorf("unsupported database version %d", version)
	}
	if _, err = tx.ExecContext(ctx, `CREATE TABLE revisions(scope TEXT PRIMARY KEY, version INTEGER NOT NULL DEFAULT 0);
INSERT INTO revisions(scope) VALUES ('definitions'),('runs'),('metadata'),('settings');
CREATE TABLE settings(key TEXT PRIMARY KEY, value TEXT NOT NULL CHECK(json_valid(value)));
CREATE TRIGGER settings_insert AFTER INSERT ON settings BEGIN UPDATE revisions SET version=version+1 WHERE scope='settings' OR (scope='definitions' AND NEW.key='locale'); END;
CREATE TRIGGER settings_update AFTER UPDATE ON settings BEGIN UPDATE revisions SET version=version+1 WHERE scope='settings' OR (scope='definitions' AND NEW.key='locale'); END;
CREATE TRIGGER settings_delete AFTER DELETE ON settings BEGIN UPDATE revisions SET version=version+1 WHERE scope='settings' OR (scope='definitions' AND OLD.key='locale'); END;`); err != nil {
		return err
	}
	for _, t := range []table{projects, commandsV3, runsV3, metadataV1} {
		if _, err = tx.ExecContext(ctx, t.schema()); err != nil {
			return err
		}
	}
	// Direct SQL changes also invalidate the per-project optimistic lock.
	if _, err = tx.ExecContext(ctx, `CREATE TRIGGER metadata_revision AFTER UPDATE ON project_metadata WHEN NEW.revision <= OLD.revision BEGIN UPDATE project_metadata SET revision=OLD.revision+1 WHERE id=NEW.id; END;`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `
CREATE TRIGGER projects_delete_children AFTER DELETE ON projects BEGIN
 DELETE FROM commands WHERE project=OLD.id;
 DELETE FROM project_metadata WHERE id=OLD.id;
END;
CREATE TRIGGER commands_delete_bindings AFTER DELETE ON commands BEGIN
 UPDATE projects SET buttons=(SELECT json_group_array(json(CASE WHEN json_extract(value,'$.commandId')=OLD.id THEN json_set(value,'$.commandId','') ELSE value END)) FROM json_each(buttons))
 WHERE EXISTS (SELECT 1 FROM json_each(buttons) WHERE json_extract(value,'$.commandId')=OLD.id);
END;
CREATE TRIGGER commands_move_bindings AFTER UPDATE OF project ON commands WHEN NEW.project IS NOT OLD.project BEGIN
 UPDATE projects SET buttons=(SELECT json_group_array(json(CASE WHEN json_extract(value,'$.commandId')=OLD.id THEN json_set(value,'$.commandId','') ELSE value END)) FROM json_each(buttons))
 WHERE id IS NOT NEW.project AND EXISTS (SELECT 1 FROM json_each(buttons) WHERE json_extract(value,'$.commandId')=OLD.id);
END;`); err != nil {
		return err
	}
	definitions, err := legacy(directory, "definitions.json", `{"revision":0,"locale":"zh","projects":[],"commands":[]}`)
	if err != nil {
		return err
	}
	var def struct {
		Revision           int64  `json:"revision"`
		Locale             string `json:"locale"`
		Projects, Commands json.RawMessage
	}
	if err = json.Unmarshal(definitions, &def); err != nil {
		return err
	}
	if def.Revision < 0 || (def.Locale != "zh" && def.Locale != "en") || !isArray(def.Projects) || !isArray(def.Commands) {
		return errors.New("invalid legacy definitions")
	}
	for i, t := range []table{projects, commandsV3} {
		data := []json.RawMessage{def.Projects, def.Commands}[i]
		if _, err = tx.ExecContext(ctx, t.upsertSQL(), string(data)); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO settings VALUES ('locale',json_quote(?))`, def.Locale); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `UPDATE revisions SET version=max(version,?) WHERE scope='definitions'`, def.Revision); err != nil {
		return err
	}
	runData, err := legacy(directory, "runs.json", "[]")
	if err != nil {
		return err
	}
	if !isArray(runData) {
		return errors.New("invalid legacy runs")
	}
	if _, err = tx.ExecContext(ctx, runsV3.upsertSQL(), string(runData)); err != nil {
		return err
	}
	metaData, err := legacy(directory, "project-metadata.json", `{"version":1,"projects":{}}`)
	if err != nil {
		return err
	}
	var meta struct {
		Version  int
		Projects map[string]json.RawMessage
	}
	if err = json.Unmarshal(metaData, &meta); err != nil {
		return err
	}
	if meta.Version != 1 || meta.Projects == nil {
		return errors.New("invalid legacy project metadata")
	}
	entries := []json.RawMessage{}
	for id, entry := range meta.Projects {
		var fields map[string]json.RawMessage
		if err = json.Unmarshal(entry, &fields); err != nil {
			return err
		}
		if fields == nil {
			return errors.New("invalid legacy project metadata")
		}
		fields["id"], _ = json.Marshal(id)
		encoded, err := json.Marshal(fields)
		if err != nil {
			return err
		}
		entries = append(entries, encoded)
	}
	encoded, _ := json.Marshal(entries)
	if _, err = tx.ExecContext(ctx, metadataV1.upsertSQL(), string(encoded)); err != nil {
		return err
	}
	appearance, err := legacy(directory, "appearance.json", `{"skin":"comic"}`)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `INSERT INTO settings VALUES ('appearance',?)`, string(appearance)); err != nil {
		return err
	}
	for _, framework := range []string{"mygo", "mygo-native", "wails"} {
		config, err := legacy(filepath.Join(directory, "updates", framework), "config.json", `{}`)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO settings VALUES (?,?)`, "updates/"+framework, string(config)); err != nil {
			return err
		}
		source, err := legacy(filepath.Join(directory, "updates", framework), "local-source.json", `{}`)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO settings VALUES (?,?)`, "local-source/"+framework, string(source)); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `PRAGMA user_version=1`); err != nil {
		return err
	}
	// The transaction is the migration marker. Legacy files are never rewritten or
	// consulted again once this commits, including when a table becomes empty.
	return tx.Commit()
}

func isArray(data []byte) bool {
	var values []json.RawMessage
	return len(data) > 0 && strings.HasPrefix(strings.TrimSpace(string(data)), "[") && json.Unmarshal(data, &values) == nil
}

func (s *Store) ReadSetting(ctx context.Context, key string, value any) error {
	var raw string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key=?", key).Scan(&raw)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(raw), value)
}
func (s *Store) WriteSetting(ctx context.Context, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `INSERT INTO settings(key,value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value WHERE settings.value IS NOT excluded.value`, key, string(raw))
	return err
}
