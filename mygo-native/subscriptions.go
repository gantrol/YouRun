package main

import (
	"context"

	"commandmanager.local/core/desktop"
	"commandmanager.local/core/internal/engine"
)

// Wait for the UI consumer before reading another event. Cancellation releases
// the reader even if the window is closing; queued callbacks recheck their lease.
func (a *app) dispatchSubscription(ctx context.Context, apply func()) bool {
	if ctx.Err() != nil {
		return false
	}
	done := make(chan struct{})
	a.win.Update(func() {
		defer close(done)
		if ctx.Err() == nil {
			apply()
		}
	})
	select {
	case <-ctx.Done():
		return false
	case <-done:
		return true
	}
}

func (a *app) subscribeWorkspace() {
	if a.workspaceCancel != nil {
		a.workspaceCancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.workspaceCancel = cancel
	go a.repository.watch(ctx, func(state engine.Snapshot) {
		a.dispatchSubscription(ctx, func() { a.receiveWorkspace(state) })
	}, func(err error) {
		a.dispatchSubscription(ctx, func() { a.issue = "connectionLost" })
	})
}

func (a *app) receiveWorkspace(next engine.Snapshot) {
	if a.busy {
		// Do not discard deltas during a save or folder dialog. Run status keeps
		// updating, while definitions wait until the mutation callback finishes.
		copy := next
		a.pending = &copy
		next.Definitions = a.state.Definitions
	}
	changed, metadataChanged := a.workspaceViewModel.accept(next)
	if a.issue == "connectionLost" || (a.issue == "storageFailed" && !next.StorageError) {
		a.issue = ""
	}
	if next.StorageError {
		a.issue = "storageFailed"
	}
	if a.draftConflict {
		a.issue = "conflict"
	}
	if changed || metadataChanged {
		a.loadProjectDetails("")
	}
	if a.state.SettingsRevision != a.appearanceRevision {
		a.loadAppearance(a.state.SettingsRevision)
	}
}

func (a *app) flushWorkspace() {
	if a.busy || a.pending == nil {
		return
	}
	next := *a.pending
	a.pending = nil
	a.receiveWorkspace(next)
}

func (a *app) loadAppearance(revision int64) {
	if a.appearanceRequested == revision {
		return
	}
	if a.appearanceCancel != nil {
		a.appearanceCancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.appearanceCancel, a.appearanceRequested = cancel, revision
	go func() {
		var appearance desktop.Appearance
		err := a.api.requestContext(ctx, "GET", "appearance", nil, &appearance)
		a.dispatchSubscription(ctx, func() {
			a.appearanceRequested = -1
			if revision != a.state.SettingsRevision {
				return
			}
			if err != nil {
				a.issue = "skinLoadFailed"
				return
			}
			a.appearanceRevision = revision
			if appearance.Skin != a.skin {
				a.skin, a.theme = appearance.Skin, appTheme(appearance.Skin)
			}
			if a.issue == "skinLoadFailed" {
				a.issue = ""
			}
		})
	}()
}

func (a *app) subscribeLog(id string) {
	if a.logRun == id {
		return
	}
	if a.logCancel != nil {
		a.logCancel()
	}
	a.logRun = id
	a.runOutputViewModel = runOutputViewModel{}
	if id == "" {
		return
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.logCancel = cancel
	go a.repository.watchLog(ctx, id, func(event logEvent) error {
		var err error
		if !a.dispatchSubscription(ctx, func() { err = a.runOutputViewModel.accept(event) }) {
			return ctx.Err()
		}
		return err
	}, func(err error) {
		a.dispatchSubscription(ctx, func() {
			a.logIssue = "connectionLost"
			if err.Error() == "logMissing" {
				a.logIssue = "logMissing"
			}
		})
	})
}
