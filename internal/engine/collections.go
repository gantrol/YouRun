package engine

import (
	"errors"
	"slices"
	"strings"
)

func validateCollections(collections []Collection, commands map[string]Command) error {
	if len(collections) > 500 {
		return errors.New("invalidDefinition")
	}
	byID := make(map[string]Collection, len(collections))
	for _, collection := range collections {
		if !validID.MatchString(collection.ID) || byID[collection.ID].ID != "" || strings.TrimSpace(collection.Name) == "" || len(collection.Name) > 200 || strings.ContainsRune(collection.Name, 0) || len(collection.CommandIDs) > 2000 {
			return errors.New("invalidDefinition")
		}
		byID[collection.ID] = collection
		seen := map[string]bool{}
		for _, id := range collection.CommandIDs {
			if commands[id].ID == "" || seen[id] {
				return errors.New("invalidDefinition")
			}
			seen[id] = true
		}
	}
	for _, collection := range collections {
		seen := map[string]bool{collection.ID: true}
		for parent := collection.Parent; parent != nil; {
			next, ok := byID[*parent]
			if !ok || seen[*parent] {
				return errors.New("invalidDefinition")
			}
			seen[*parent] = true
			parent = next.Parent
		}
	}
	return nil
}

// Deleting a definition removes its references, never its historical runs.
func PruneCollectionCommands(d *Definitions) {
	d.Collections = slices.Clone(d.Collections)
	ids := make(map[string]bool, len(d.Commands))
	for _, command := range d.Commands {
		ids[command.ID] = true
	}
	for i, collection := range d.Collections {
		members := make([]string, 0, len(collection.CommandIDs))
		for _, id := range collection.CommandIDs {
			if ids[id] {
				members = append(members, id)
			}
		}
		d.Collections[i].CommandIDs = members
	}
}
