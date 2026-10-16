// Package projectmeta manages discovery choices in the shared SQLite store.
package projectmeta

import (
	"commandmanager.local/core/internal/database"
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"
)

type Entry struct {
	Roots    []string `json:"roots"`
	LogoID   string   `json:"logoId,omitempty"`
	Avatar   string   `json:"avatar,omitempty"`
	Revision int64    `json:"revision"`
}

var ErrConflict = database.ErrConflict

func Load(directory string) (map[string]Entry, error) {
	return LoadContext(context.Background(), directory)
}

func LoadContext(ctx context.Context, directory string) (map[string]Entry, error) {
	store, err := database.Open(directory)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := store.ReadMetadata(ctx)
	if err != nil {
		return nil, err
	}
	entries := map[string]Entry{}
	for id, raw := range rows {
		var entry Entry
		if err = json.Unmarshal(raw, &entry); err != nil {
			return nil, err
		}
		if !validEntry(id, entry) {
			return nil, errors.New("invalid project metadata")
		}
		entries[id] = entry
	}
	return entries, nil
}
func validRoot(root string) bool {
	return filepath.IsAbs(root) && len(root) <= 4096 && !strings.ContainsRune(root, 0) && !strings.Contains(strings.ToLower(root), "trash")
}
func validEntry(id string, entry Entry) bool {
	if id == "" || len(id) > 80 || entry.Revision < 0 || len(entry.Roots) > 8 || len(entry.LogoID) > 160 || (entry.Avatar != "" && !validAvatar(entry.Avatar)) {
		return false
	}
	for _, root := range entry.Roots {
		if !validRoot(root) {
			return false
		}
	}
	return true
}
func Update(ctx context.Context, directory, id, root string, logoID *string, revision int64) (Entry, error) {
	return update(ctx, directory, id, root, logoID, "", nil, revision)
}
func update(ctx context.Context, directory, id, root string, logoID *string, avatar string, avatarData []byte, revision int64) (Entry, error) {
	if id == "" || len(id) > 80 || (root != "" && !validRoot(root)) || (logoID != nil && len(*logoID) > 160) {
		return Entry{}, errors.New("invalidRequest")
	}
	store, err := database.Open(directory)
	if err != nil {
		return Entry{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	raw, err := store.UpdateMetadata(ctx, id, func(raw json.RawMessage) (json.RawMessage, error) {
		var entry Entry
		if err := json.Unmarshal(raw, &entry); err != nil {
			return nil, err
		}
		if !validEntry(id, entry) {
			return nil, errors.New("invalid project metadata")
		}
		if (logoID != nil || avatar != "") && entry.Revision != revision {
			return nil, ErrConflict
		}
		changed := false
		if root != "" {
			root = filepath.Clean(root)
			found := false
			for _, existing := range entry.Roots {
				rel, err := filepath.Rel(existing, root)
				found = found || (err == nil && rel == ".")
			}
			if !found {
				if len(entry.Roots) >= 8 {
					return nil, errors.New("invalidDirectory")
				}
				entry.Roots = append(entry.Roots, root)
				changed = true
			}
		}
		if logoID != nil && (entry.LogoID != *logoID || entry.Avatar != "") {
			entry.LogoID = *logoID
			entry.Avatar = ""
			changed = true
		}
		if avatar != "" {
			if err := writeAvatar(directory, avatar, avatarData); err != nil {
				return nil, err
			}
			entry.Avatar, entry.LogoID = avatar, ""
			changed = true
		}
		if changed {
			entry.Revision++
		}
		if entry.Roots == nil {
			entry.Roots = []string{}
		}
		return json.Marshal(entry)
	})
	if err != nil {
		return Entry{}, err
	}
	var entry Entry
	err = json.Unmarshal(raw, &entry)
	// Keep content-addressed files valid for concurrent readers; only their
	// selected reference changes inside the database transaction.
	return entry, err
}
