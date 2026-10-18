package database

import (
	"context"
	"encoding/json"
	"strings"
)

// Version 1 remains unchanged; version 2 adds an application-side observation
// cursor. The runner neither opens this database nor participates in migrations.
func (s *Store) migrateRuntime(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version == 1 {
		if _, err = tx.ExecContext(ctx, `CREATE TABLE runner_observation(id INTEGER PRIMARY KEY CHECK(id=1), observed INTEGER NOT NULL, runs TEXT NOT NULL CHECK(json_valid(runs)));
INSERT INTO runner_observation VALUES (1,0,'[]');
PRAGMA user_version=2;`); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SyncRuntime imports only execution changes. A transaction rejects out-of-order
// polls from other windows and preserves edits/deletions of unchanged history.
func (s *Store) SyncRuntime(ctx context.Context, observed int64, data []byte) error {
	var previousTime int64
	var previous string
	if err := s.db.QueryRowContext(ctx, "SELECT observed,runs FROM runner_observation WHERE id=1").Scan(&previousTime, &previous); err == nil {
		if observed <= previousTime || (previousTime > 0 && previous == string(data)) {
			return nil
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, "SELECT observed,runs FROM runner_observation WHERE id=1").Scan(&previousTime, &previous); err != nil {
		return err
	}
	if observed <= previousTime || (previousTime > 0 && previous == string(data)) {
		return tx.Commit()
	}
	if previous != string(data) {
		var before, after []json.RawMessage
		if err = json.Unmarshal([]byte(previous), &before); err != nil {
			return err
		}
		if err = json.Unmarshal(data, &after); err != nil {
			return err
		}
		old := map[string]string{}
		for _, raw := range before {
			var row struct{ ID string }
			if err = json.Unmarshal(raw, &row); err != nil {
				return err
			}
			old[row.ID] = string(raw)
		}
		changed := []json.RawMessage{}
		for _, raw := range after {
			var row struct{ ID string }
			if err = json.Unmarshal(raw, &row); err != nil {
				return err
			}
			if old[row.ID] != string(raw) {
				changed = append(changed, raw)
			}
			delete(old, row.ID)
		}
		updates, err := json.Marshal(changed)
		if err != nil {
			return err
		}
		insert := strings.Split(runs.upsertSQL(), " ON CONFLICT(id)")[0]
		if _, err = tx.ExecContext(ctx, insert+` ON CONFLICT(id) DO UPDATE SET status=excluded.status,pid=excluded.pid,"exitCode"=excluded."exitCode","launchSource"=coalesce(excluded."launchSource",runs."launchSource") WHERE runs.status IS NOT excluded.status OR runs.pid IS NOT excluded.pid OR runs."exitCode" IS NOT excluded."exitCode" OR runs."launchSource" IS NOT excluded."launchSource"`, string(updates)); err != nil {
			return err
		}
		removed := []string{}
		for id := range old {
			removed = append(removed, id)
		}
		deletes, err := json.Marshal(removed)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM runs WHERE id IN (SELECT value FROM json_each(?)) AND status NOT IN ('running','stopping','unknown')`, string(deletes)); err != nil {
			return err
		}
	}
	// Also recover imported active records when the new execution journal is empty.
	if _, err = tx.ExecContext(ctx, `UPDATE runs SET status='unknown' WHERE status IN ('running','stopping') AND id NOT IN (SELECT json_extract(value,'$.id') FROM json_each(?))`, string(data)); err != nil {
		return err
	}
	// The cursor only needs to advance when execution facts change. Repeated
	// identical polls must not rewrite the bounded execution journal to SQLite.
	if _, err = tx.ExecContext(ctx, "UPDATE runner_observation SET observed=?,runs=? WHERE id=1", observed, string(data)); err != nil {
		return err
	}
	return tx.Commit()
}
