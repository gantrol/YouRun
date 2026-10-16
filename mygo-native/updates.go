package main

import (
	"context"
	"time"

	"github.com/egoist/mygo/ui"
)

// The dormant updater has no event endpoint. Read it on entry/actions and only
// monitor an explicitly active update; disabled and idle updates have no timer.
func (a *app) observeUpdates() {
	if a.updatesCancel != nil {
		a.updatesCancel()
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.updatesCancel = cancel
	go func() {
		for ctx.Err() == nil {
			var next updateState
			err := a.api.requestContext(ctx, "GET", "updates", nil, &next)
			if err != nil {
				a.dispatchSubscription(ctx, func() { a.issue = err.Error() })
				return
			}
			if !a.dispatchSubscription(ctx, func() { a.acceptUpdate(next) }) {
				return
			}
			waitingForRestart := next.Phase == "ready" && next.LocalRestart && a.rebuilding.Load()
			if next.Disabled || (!waitingForRestart && next.Phase != "building" && next.Phase != "checking" && next.Phase != "downloading" && next.Phase != "switching") {
				return
			}
			timer := time.NewTimer(time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
}

func (a *app) acceptUpdate(next updateState) {
	a.updates = next
	if !a.updateEditing || !a.updateLoaded {
		a.updateDraft = next.Config
	}
	a.updateLoaded = true
	if next.Phase == "failed" {
		a.rebuilding.Store(false)
		if next.Error != "" {
			a.issue = next.Error
		}
	}
	if next.Phase == "ready" && next.LocalRestart && a.rebuilding.Load() {
		if a.dirty() {
			a.issue = "updateUnsaved"
		} else if !a.busy && !a.quitOpen && !a.prompting.Load() {
			a.rebuilding.Store(false)
			a.updateAction("apply", a.session)
		}
	}
}

func (a *app) updateView(c *ui.Context) {
	if !a.updateLoaded {
		ui.Text(c, a.m("loading"))
		return
	}
	if a.updates.Disabled {
		ui.Text(c, a.m("updateDisabled"))
		return
	}
	if a.updates.StoreManaged {
		ui.Text(c, a.updates.Version).FontSize(12).TextColor(a.theme.TextMuted)
		ui.Text(c, a.m("updateStoreManaged"))
		return
	}
	if !a.updateEditing {
		a.updateDraft = a.updates.Config
		a.updateEditing = true
	}
	busy := a.updates.Phase == "building" || a.updates.Phase == "checking" || a.updates.Phase == "downloading" || a.updates.Phase == "switching" || a.updates.Phase == "complete"
	a.field(c, "updateRepository", &a.updateDraft.Repository)
	a.field(c, "updatePublicKey", &a.updateDraft.PublicKey)
	if a.button(c, a.m("save")).Disabled(busy).Clicked() {
		config := a.updateDraft
		a.work(func() (func(), error) {
			err := a.api.request("POST", "updates/config", config, nil)
			return func() { a.updates.Config = config; a.updateEditing = false; a.observeUpdates() }, err
		})
	}
	if a.updates.Version != "" {
		ui.Text(c, a.updates.Version).FontSize(12).TextColor(a.theme.TextMuted)
	}
	phase := map[string]string{"idle": "updateIdle", "checking": "updateChecking", "downloading": "updateDownloading", "building": "updateBuilding", "ready": "updateReady", "current": "updateCurrent", "switching": "updateSwitching", "complete": "updateSwitching", "failed": "updateFailed"}[a.updates.Phase]
	if phase != "" {
		ui.Text(c, a.m(phase))
	}
	if a.updates.Error != "" {
		ui.Text(c, a.m(a.updates.Error)).TextColor(a.theme.Danger)
	}
	ui.Row(c).Wrap().Gap(8).Children(func() {
		if a.button(c, a.m("updateCheck")).Disabled(busy || a.dirty()).Clicked() {
			a.updateAction("check", nil)
		}
		if a.button(c, a.m("updateLocal")).Disabled(busy || a.dirty()).Clicked() {
			a.updateAction("local", nil)
		}
		if a.updates.Phase == "ready" && a.primaryButton(c, a.m("updateApply")).Disabled(a.dirty()).Clicked() {
			a.updateAction("apply", a.session)
		}
		if a.updates.Phase == "checking" || a.updates.Phase == "downloading" || a.updates.Phase == "building" {
			if a.button(c, a.m("cancel")).Clicked() {
				a.updateAction("cancel", nil)
			}
		}
	})
	source := a.updates.LocalSource
	a.textInput(c, &source).ReadOnly(true).FillWidth().Label(a.m("updateLocalSource"))
	ui.Row(c).Wrap().Gap(8).Children(func() {
		if a.primaryButton(c, a.m("updateRebuild")).Disabled(busy || a.dirty() || source == "").Clicked() {
			a.updateAction("rebuild", nil)
		}
		if a.button(c, a.m("chooseDirectory")).Disabled(busy || a.dirty()).Clicked() {
			a.updateAction("source", nil)
		}
	})
	if a.updates.BuildLog != "" {
		output := a.updates.BuildLog
		a.textArea(c, &output).ReadOnly(true).FillWidth().Height(180).Label(a.m("updateBuildLog"))
	}
}
func (a *app) updateAction(action string, body any) {
	if (action == "rebuild" || action == "apply") && a.dirty() {
		a.issue = "updateUnsaved"
		return
	}
	a.work(func() (func(), error) {
		err := a.api.request("POST", "updates/"+action, body, nil)
		return func() {
			if action == "rebuild" {
				a.rebuilding.Store(true)
				a.updates.Phase = "building"
				a.updates.BuildLog = ""
			}
			a.observeUpdates()
		}, err
	})
}
