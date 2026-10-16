package main

import (
	"commandmanager.local/core/internal/engine"
	"github.com/egoist/mygo/ui"
)

func (a *app) setCommandIssue(id, code string) {
	if a.commandIssues == nil {
		a.commandIssues = map[string]string{}
	}
	a.commandIssues[id] = code
}

func (a *app) commandFeedback(c *ui.Context, command engine.Command) {
	code := a.commandIssues[command.ID]
	if code == "" {
		return
	}
	if a.catalogs["en"][code] == "" {
		code = "startFailed"
	}
	ui.Row(c).FillWidth().Padding(4, 12).Gap(8).Wrap().AlignItems(ui.Center).Children(func() {
		ui.Text(c, command.Title+" · "+a.m(code)).FontSize(12).TextColor(a.theme.Danger).Shrink(1).MinWidth(0)
		if a.icon(c.Key("dismiss"), "close", a.m("close"), false) {
			delete(a.commandIssues, command.ID)
		}
	})
}
