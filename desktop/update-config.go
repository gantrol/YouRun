package desktop

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"

	"commandmanager.local/core/internal/database"
	"commandmanager.local/core/internal/updater"
)

// Called before publication or with the manager lock held. Download jobs keep
// the configuration captured at their start; subsequent jobs read SQLite.
func (m *updateManager) loadConfig(ctx context.Context) error {
	store, err := database.Open(m.host.dataDir)
	if err != nil {
		return err
	}
	config := updateConfig{Repository: updater.DefaultRepository, PublicKey: updater.DefaultPublicKey}
	if err = store.ReadSetting(ctx, "updates/"+m.framework, &config); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	m.state.Config = config
	var source localSource
	if err = store.ReadSetting(ctx, "local-source/"+m.framework, &source); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if source.Directory != "" && filepath.IsAbs(source.Directory) && !strings.Contains(strings.ToLower(source.Directory), "trash") {
		m.state.LocalSource = source.Directory
	}
	return nil
}
func (m *updateManager) saveConfig(ctx context.Context, config updateConfig) error {
	store, err := database.Open(m.host.dataDir)
	if err != nil {
		return err
	}
	return store.WriteSetting(ctx, "updates/"+m.framework, config)
}
