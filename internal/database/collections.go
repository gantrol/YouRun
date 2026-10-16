package database

import "context"

func (s *Store) migrateCollections(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version >= 3 {
		return tx.Commit()
	}
	if _, err = tx.ExecContext(ctx, collections.schema()); err != nil {
		return err
	}
	// A newly imported database already uses the current command column list.
	var hasAliases int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM pragma_table_info('commands') WHERE name='aliases'").Scan(&hasAliases); err != nil {
		return err
	}
	if hasAliases == 0 {
		if _, err = tx.ExecContext(ctx, "ALTER TABLE commands ADD COLUMN aliases "+array("aliases").declaration); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `
CREATE TRIGGER collections_delete_children AFTER DELETE ON collections BEGIN
 UPDATE collections SET parent=OLD.parent WHERE parent=OLD.id;
END;
CREATE TRIGGER commands_delete_memberships AFTER DELETE ON commands BEGIN
 UPDATE collections SET commandIds=(SELECT coalesce(json_group_array(value),'[]') FROM json_each(commandIds) WHERE value<>OLD.id)
 WHERE EXISTS (SELECT 1 FROM json_each(commandIds) WHERE value=OLD.id);
END;
UPDATE revisions SET version=version+1 WHERE scope='definitions';
PRAGMA user_version=3;`); err != nil {
		return err
	}
	return tx.Commit()
}
