package main

import (
	"commandmanager.local/core/internal/engine"
	"slices"
)

// The native view binds to the same authoritative snapshots as the React view.
// Drafts are presentation state and never replace subscribed, committed data.
type workspaceViewModel struct {
	state         engine.Snapshot
	pending       *engine.Snapshot
	commandDraft  *engine.Command
	projectDraft  *engine.Project
	draftOriginal string
	draftConflict bool
}

func (v *workspaceViewModel) draftDirty() bool {
	if v.commandDraft != nil {
		return encoded(v.commandDraft) != v.draftOriginal
	}
	if v.projectDraft != nil {
		return encoded(v.projectDraft) != v.draftOriginal
	}
	return false
}
func (v *workspaceViewModel) accept(next engine.Snapshot) (bool, bool) {
	// Saves and reconnect snapshots can arrive in either order. Compare each
	// independently versioned domain before updating the presented model.
	if next.Definitions.Revision < v.state.Definitions.Revision {
		next.Definitions = v.state.Definitions
	}
	if next.RunsRevision < v.state.RunsRevision {
		next.Runs, next.RunsRevision = v.state.Runs, v.state.RunsRevision
	}
	if next.MetadataRevision < v.state.MetadataRevision {
		next.MetadataRevision, next.MetadataKeys = v.state.MetadataRevision, v.state.MetadataKeys
	}
	if next.SettingsRevision < v.state.SettingsRevision {
		next.SettingsRevision = v.state.SettingsRevision
	}
	changed := next.Definitions.Revision != v.state.Definitions.Revision
	metadataChanged := next.MetadataRevision != v.state.MetadataRevision
	dirty := v.draftDirty()
	if changed && !dirty {
		nextView := workspaceViewModel{state: next}
		if v.commandDraft != nil && v.command(v.commandDraft.ID) != nil && nextView.command(v.commandDraft.ID) == nil {
			v.commandDraft = nil
		}
		if v.projectDraft != nil && v.project(v.projectDraft.ID) != nil && nextView.project(v.projectDraft.ID) == nil {
			v.projectDraft = nil
		}
	}
	if changed && dirty {
		if v.commandDraft != nil && v.command(v.commandDraft.ID) != nil {
			v.draftConflict = true
			for _, command := range next.Definitions.Commands {
				if command.ID == v.commandDraft.ID && encoded(&command) == v.draftOriginal {
					v.draftConflict = false
				}
			}
		}
		if v.projectDraft != nil && v.project(v.projectDraft.ID) != nil {
			v.draftConflict = true
			for _, project := range next.Definitions.Projects {
				if project.ID == v.projectDraft.ID && encoded(&project) == v.draftOriginal {
					v.draftConflict = false
				}
			}
		}
	}
	v.state = next
	if changed && !dirty {
		v.refreshDrafts()
	}
	return changed, metadataChanged
}
func (v *workspaceViewModel) refreshDrafts() {
	if v.projectDraft != nil {
		if current := v.project(v.projectDraft.ID); current != nil {
			copy := *current
			copy.Buttons = slices.Clone(current.Buttons)
			v.projectDraft = &copy
			v.draftOriginal = encoded(&copy)
		} else if v.draftConflict {
			v.projectDraft = nil
		}
	}
	if v.commandDraft != nil {
		if current := v.command(v.commandDraft.ID); current != nil {
			copy := *current
			copy.Fields = slices.Clone(current.Fields)
			v.commandDraft = &copy
			v.draftOriginal = encoded(&copy)
		} else if v.draftConflict {
			v.commandDraft = nil
		}
	}
	v.draftConflict = false
}
func (v *workspaceViewModel) command(id string) *engine.Command {
	for _, command := range v.state.Definitions.Commands {
		if command.ID == id {
			return &command
		}
	}
	return nil
}
func (v *workspaceViewModel) project(id string) *engine.Project {
	for _, project := range v.state.Definitions.Projects {
		if project.ID == id {
			return &project
		}
	}
	return nil
}
