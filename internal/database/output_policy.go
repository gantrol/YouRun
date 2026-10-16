package database

import "context"

func (s *Store) migrateOutputPolicy(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version >= 4 {
		return tx.Commit()
	}
	for _, name := range []string{"commands", "runs"} {
		if _, err = tx.ExecContext(ctx, "ALTER TABLE "+name+" ADD COLUMN omitOutput "+boolean("omitOutput").declaration); err != nil {
			return err
		}
	}
	if _, err = tx.ExecContext(ctx, `UPDATE revisions SET version=version+1 WHERE scope IN ('definitions','runs');
PRAGMA user_version=4;`); err != nil {
		return err
	}
	return tx.Commit()
}
