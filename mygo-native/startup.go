package main

import (
	"commandmanager.local/core/desktop"
	"github.com/egoist/mygo/ui"
)

func (a *app) loadStartup() {
	if a.loadingStartup || a.savingStartup {
		return
	}
	a.loadingStartup = true
	go func() {
		var next desktop.StartupSettings
		err := a.api.request("GET", "startup", nil, &next)
		a.dispatchSubscription(a.ctx, func() {
			a.loadingStartup = false
			a.startupLoaded = err == nil
			if err != nil {
				a.startupError = "startupLoadFailed"
				return
			}
			a.startup, a.startupError = next, ""
		})
	}()
}

func (a *app) startupSetting(c *ui.Context) {
	enabled := a.startup.Enabled
	if a.checkbox(c.Key("startup"), &enabled, a.m("startupAtLogin")).
		Disabled(a.busy || a.candidate || a.host.UpdatePending() || a.loadingStartup || a.savingStartup || !a.startupLoaded || !a.startup.Supported).Changed() {
		a.savingStartup, a.startupError = true, ""
		a.work(func() (func(), error) {
			var next desktop.StartupSettings
			err := a.api.request("PUT", "startup", map[string]bool{"enabled": enabled}, &next)
			loaded := err == nil
			if err != nil {
				loaded = a.api.request("GET", "startup", nil, &next) == nil
			}
			return func() {
				a.savingStartup, a.startupLoaded = false, loaded
				if loaded {
					a.startup = next
				}
				if err != nil {
					a.startupError = err.Error()
				}
			}, nil
		})
	}
	if a.loadingStartup {
		ui.Text(c, a.m("loading")).FontSize(12).TextColor(a.theme.TextMuted)
	} else if a.savingStartup {
		ui.Text(c, a.m("saving")).FontSize(12).TextColor(a.theme.TextMuted)
	}
	if a.startupError != "" {
		ui.Text(c, a.m(a.startupError)).FontSize(12).TextColor(a.theme.Danger)
		if a.button(c.Key("startup-retry"), a.m("retry")).Disabled(a.loadingStartup || a.savingStartup).Clicked() {
			a.loadStartup()
		}
	} else if a.startup.Reason != "" {
		ui.Text(c, a.m(a.startup.Reason)).FontSize(12).TextColor(a.theme.TextMuted)
	}
}
