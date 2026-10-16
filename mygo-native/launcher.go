package main

import "github.com/egoist/mygo/ui"

func (a *app) homeToolbar(c *ui.Context) {
	blocked := a.busy || a.candidate || a.host.UpdatePending() || a.leaving
	ui.Toolbar(c.Key("projects-toolbar"), func() {
		ui.Text(c, a.m("projects")).Bold().Grow(1).MinWidth(0)
		if a.toolIcon(c.Key("import"), "folder", a.m("importProject"), blocked || a.dirty()) {
			a.chooseProjectDirectory(false)
		}
		if a.toolIcon(c.Key("add"), "plus", a.m("newCommand"), blocked || a.dirty()) {
			a.editCommand("", "")
		}
		if a.toolIcon(c.Key("history"), "history", a.m("history"), blocked || a.dirty()) {
			a.open(page{Kind: "history"})
		}
		if a.toolIcon(c.Key("settings"), "settings", a.m("settings"), blocked || a.dirty()) {
			a.open(page{Kind: "settings"})
		}
	}).Label(a.m("projects")).FillWidth().Height(56).Shrink(0).PaddingX(12).BorderWidth(0, 0, 1, 0).BorderColor(a.theme.Border)
}

func (a *app) projectCommands(c *ui.Context, id string) {
	project := a.project(id)
	if project == nil {
		ui.Text(c, a.m("commandMissing"))
		return
	}
	ui.Toolbar(c.Key("project-toolbar"), func() {
		ui.Text(c, project.Name).Bold().Grow(1).MinWidth(0).MaxLines(1)
		if a.toolIcon(c.Key("add"), "plus", a.m("newCommand"), a.busy) {
			a.editCommand("", id)
		}
		if a.toolIcon(c.Key("settings"), "settings", a.m("projectSettings"), a.busy) {
			a.open(page{Kind: "project", ID: id})
		}
	}).Label(a.m("project")).FillWidth().Padding(0)
	a.projectOpenerButton(c.Key("project-openers"), id, a.projectDetails[id].Directory == "")
	shown := false
	for _, command := range a.state.Definitions.Commands {
		if pointer(command.Project) == id && !command.Hidden {
			a.commandRow(c, command)
			shown = true
		}
	}
	if !shown {
		ui.Text(c, a.m("noCommands")).TextColor(a.theme.TextMuted)
	}
}
