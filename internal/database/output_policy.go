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

func (s *Store) migrateSourceAndWorkspaces(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var hasLaunchSource int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM pragma_table_info('runs') WHERE name='launchSource'").Scan(&hasLaunchSource); err != nil {
		return err
	}
	if hasLaunchSource == 0 {
		if _, err = tx.ExecContext(ctx, "ALTER TABLE runs ADD COLUMN launchSource "+nullableObject("launchSource").declaration); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE revisions SET version=version+1 WHERE scope='runs'"); err != nil {
			return err
		}
	}
	var hasLocations int
	if err = tx.QueryRowContext(ctx, "SELECT count(*) FROM pragma_table_info('project_metadata') WHERE name='locations'").Scan(&hasLocations); err != nil {
		return err
	}
	if hasLocations == 0 {
		if _, err = tx.ExecContext(ctx, "ALTER TABLE project_metadata ADD COLUMN locations "+array("locations").declaration); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "ALTER TABLE project_metadata ADD COLUMN activeLocationId "+text("activeLocationId").declaration); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, "UPDATE revisions SET version=version+1 WHERE scope='metadata'"); err != nil {
			return err
		}
	}
	return tx.Commit()
}
