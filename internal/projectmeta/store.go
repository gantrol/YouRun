package projectmeta

import (
	"commandmanager.local/core/internal/database"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"time"
)

type Location struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Directory string `json:"directory"`
	IsDefault bool   `json:"isDefault,omitempty"`
}

type Entry struct {
	Roots            []string   `json:"roots"`
	Locations        []Location `json:"locations,omitempty"`
	ActiveLocationID string     `json:"activeLocationId,omitempty"`
	LogoID           string     `json:"logoId,omitempty"`
	Avatar           string     `json:"avatar,omitempty"`
	Revision         int64      `json:"revision"`
}

type WorkspaceAction struct {
	Action     string `json:"action"`
	LocationID string `json:"locationId,omitempty"`
	Name       string `json:"name,omitempty"`
	Directory  string `json:"directory,omitempty"`
	Revision   int64  `json:"revision"`
}

var ErrConflict = database.ErrConflict

func locationIDForPath(path string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(path))))
	return "loc-" + hex.EncodeToString(sum[:6])
}

func sameCleanedPath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	rel, err := filepath.Rel(filepath.Clean(a), filepath.Clean(b))
	return err == nil && rel == "."
}

func ResolveLocations(entry Entry, fallbackDirs []string) ([]Location, string) {
	locations := make([]Location, 0, len(entry.Locations)+len(entry.Roots)+1)
	seenPaths := []string{}
	seenIDs := map[string]bool{}
	hasPath := func(dir string) bool {
		for _, p := range seenPaths {
			if sameCleanedPath(p, dir) {
				return true
			}
		}
		return false
	}
	for _, loc := range entry.Locations {
		if !validRoot(loc.Directory) {
			continue
		}
		clean := filepath.Clean(loc.Directory)
		if hasPath(clean) {
			continue
		}
		id := strings.TrimSpace(loc.ID)
		if id == "" || seenIDs[id] {
			id = locationIDForPath(clean)
		}
		name := strings.TrimSpace(loc.Name)
		if name == "" {
			if len(locations) == 0 {
				name = "主工作区"
			} else {
				name = filepath.Base(clean)
			}
		}
		seenIDs[id] = true
		seenPaths = append(seenPaths, clean)
		locations = append(locations, Location{
			ID:        id,
			Name:      name,
			Directory: clean,
			IsDefault: loc.IsDefault,
		})
	}
	for _, root := range entry.Roots {
		if !validRoot(root) {
			continue
		}
		clean := filepath.Clean(root)
		if hasPath(clean) {
			continue
		}
		id := locationIDForPath(clean)
		name := filepath.Base(clean)
		if len(locations) == 0 {
			name = "主工作区"
		}
		seenIDs[id] = true
		seenPaths = append(seenPaths, clean)
		locations = append(locations, Location{
			ID:        id,
			Name:      name,
			Directory: clean,
			IsDefault: len(locations) == 0,
		})
	}
	if len(locations) == 0 {
		for _, dir := range fallbackDirs {
			if !validRoot(dir) {
				continue
			}
			clean := filepath.Clean(dir)
			id := locationIDForPath(clean)
			locations = append(locations, Location{
				ID:        id,
				Name:      "主工作区",
				Directory: clean,
				IsDefault: true,
			})
			break
		}
	}
	if len(locations) == 0 {
		return nil, ""
	}
	defaultIdx := -1
	for i := range locations {
		if locations[i].IsDefault {
			if defaultIdx < 0 {
				defaultIdx = i
			} else {
				locations[i].IsDefault = false
			}
		}
	}
	if defaultIdx < 0 {
		locations[0].IsDefault = true
		defaultIdx = 0
	}
	activeID := entry.ActiveLocationID
	foundActive := false
	for _, loc := range locations {
		if loc.ID == activeID {
			foundActive = true
			break
		}
	}
	if !foundActive {
		activeID = locations[defaultIdx].ID
	}
	return locations, activeID
}

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
	if id == "" || len(id) > 80 || entry.Revision < 0 || len(entry.Roots) > 8 || len(entry.Locations) > 8 || len(entry.ActiveLocationID) > 80 || len(entry.LogoID) > 160 || (entry.Avatar != "" && !validAvatar(entry.Avatar)) {
		return false
	}
	for _, root := range entry.Roots {
		if !validRoot(root) {
			return false
		}
	}
	for _, loc := range entry.Locations {
		if loc.ID == "" || len(loc.ID) > 80 || len(loc.Name) > 120 || !validRoot(loc.Directory) {
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
		if entry.Locations == nil {
			entry.Locations = []Location{}
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

func UpdateWorkspaces(ctx context.Context, directory, id string, action WorkspaceAction, fallbackDirs []string) (Entry, error) {
	if id == "" || len(id) > 80 {
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
		locations, activeID := ResolveLocations(entry, fallbackDirs)
		switch action.Action {
		case "select":
			found := false
			for _, loc := range locations {
				if loc.ID == action.LocationID {
					found = true
					break
				}
			}
			if !found {
				return nil, errors.New("invalidRequest")
			}
			activeID = action.LocationID
		case "add":
			clean := filepath.Clean(strings.TrimSpace(action.Directory))
			if !validRoot(clean) {
				return nil, errors.New("invalidDirectory")
			}
			for _, loc := range locations {
				if sameCleanedPath(loc.Directory, clean) {
					return nil, errors.New("alreadyImported")
				}
			}
			if len(locations) >= 8 {
				return nil, errors.New("invalidDirectory")
			}
			name := strings.TrimSpace(action.Name)
			if name == "" {
				if len(locations) == 0 {
					name = "主工作区"
				} else {
					name = filepath.Base(clean)
				}
			}
			if len(name) > 120 {
				return nil, errors.New("invalidRequest")
			}
			newLoc := Location{
				ID:        locationIDForPath(clean),
				Name:      name,
				Directory: clean,
				IsDefault: len(locations) == 0,
			}
			locations = append(locations, newLoc)
			activeID = newLoc.ID
		case "rename":
			name := strings.TrimSpace(action.Name)
			if name == "" || len(name) > 120 {
				return nil, errors.New("invalidRequest")
			}
			found := false
			for i := range locations {
				if locations[i].ID == action.LocationID {
					locations[i].Name = name
					found = true
					break
				}
			}
			if !found {
				return nil, errors.New("invalidRequest")
			}
		case "setDefault":
			found := false
			for i := range locations {
				if locations[i].ID == action.LocationID {
					locations[i].IsDefault = true
					found = true
				} else {
					locations[i].IsDefault = false
				}
			}
			if !found {
				return nil, errors.New("invalidRequest")
			}
		case "remove":
			if len(locations) <= 1 {
				return nil, errors.New("invalidRequest")
			}
			next := make([]Location, 0, len(locations)-1)
			removedDefault := false
			found := false
			for _, loc := range locations {
				if loc.ID == action.LocationID {
					found = true
					if loc.IsDefault {
						removedDefault = true
					}
					continue
				}
				next = append(next, loc)
			}
			if !found {
				return nil, errors.New("invalidRequest")
			}
			if removedDefault && len(next) > 0 {
				next[0].IsDefault = true
			}
			locations = next
			if activeID == action.LocationID {
				activeID = locations[0].ID
				for _, loc := range locations {
					if loc.IsDefault {
						activeID = loc.ID
						break
					}
				}
			}
		default:
			return nil, errors.New("invalidRequest")
		}
		roots := make([]string, 0, len(locations))
		for _, loc := range locations {
			roots = append(roots, loc.Directory)
		}
		entry.Roots = roots
		entry.Locations = locations
		entry.ActiveLocationID = activeID
		entry.Revision++
		return json.Marshal(entry)
	})
	if err != nil {
		return Entry{}, err
	}
	var entry Entry
	err = json.Unmarshal(raw, &entry)
	return entry, err
}
