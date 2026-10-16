package main

import (
	"strings"

	"commandmanager.local/core/desktop"
	"github.com/egoist/mygo/ui"
)

func (a *app) loadProjectOpeners(refresh bool) {
	if a.loadingOpeners {
		return
	}
	a.loadingOpeners, a.openersError = true, ""
	path := "project-openers"
	if refresh {
		path += "?refresh=1"
	}
	go func() {
		var openers []desktop.ProjectOpener
		err := a.api.request("GET", path, nil, &openers)
		a.win.Update(func() {
			if a.ctx.Err() != nil {
				return
			}
			a.loadingOpeners = false
			if err != nil {
				a.openersError = err.Error()
			} else {
				a.projectOpeners = openers
			}
		})
	}()
}

func (a *app) projectOpenersMenu(menu *ui.Menu, id string) {
	for _, opener := range a.projectOpeners {
		if menu.Item(opener.Name).Chosen() {
			a.work(func() (func(), error) {
				return nil, a.api.request("POST", "open-project-directory", map[string]string{"id": id, "opener": opener.ID}, nil)
			})
		}
	}
	if a.loadingOpeners {
		menu.Item(a.m("scanning")).Disabled(true)
	} else if a.openersError != "" {
		menu.Item(a.m(a.openersError)).Disabled(true)
	} else if len(a.projectOpeners) == 0 {
		menu.Item(a.m("noProjectEditors")).Disabled(true)
	}
	menu.Separator()
	if menu.Item(a.m("otherProjectEditor")).Chosen() {
		a.editorExecutable = ""
		a.open(page{Kind: "openWith", ID: id})
	}
	if menu.Item(a.m("refresh")).Disabled(a.loadingOpeners).Chosen() {
		a.loadProjectOpeners(true)
	}
}

func (a *app) projectOpenerButton(c *ui.Context, id string, disabled bool) {
	items := make([]choice, 0, len(a.projectOpeners)+3)
	for _, opener := range a.projectOpeners {
		items = append(items, choice{id: "open:" + opener.ID, label: opener.Name})
	}
	if a.loadingOpeners {
		items = append(items, choice{label: a.m("scanning"), disabled: true})
	} else if a.openersError != "" {
		items = append(items, choice{label: a.m(a.openersError), disabled: true})
	} else if len(a.projectOpeners) == 0 {
		items = append(items, choice{label: a.m("noProjectEditors"), disabled: true})
	}
	items = append(items, choice{id: "other", label: a.m("otherProjectEditor"), divider: true},
		choice{id: "refresh", label: a.m("refresh"), disabled: a.loadingOpeners})
	selected := ""
	button := a.choiceButton(c, &selected, a.m("openProjectWith"), items, disabled).MaxWidth(280)
	if !button.Changed() {
		return
	}
	switch selected {
	case "other":
		a.editorExecutable = ""
		a.open(page{Kind: "openWith", ID: id})
	case "refresh":
		a.loadProjectOpeners(true)
	default:
		if opener, ok := strings.CutPrefix(selected, "open:"); ok {
			a.work(func() (func(), error) {
				return nil, a.api.request("POST", "open-project-directory", map[string]string{"id": id, "opener": opener}, nil)
			})
		}
	}
}

func (a *app) customProjectOpener(c *ui.Context, id string) {
	ui.Text(c, a.m("otherProjectEditor")).Bold()
	if project := a.project(id); project != nil {
		ui.Text(c, project.Name)
	}
	a.field(c, "editorExecutable", &a.editorExecutable)
	ui.Row(c).Gap(8).Children(func() {
		if a.button(c, a.m("cancel")).Clicked() {
			a.back(false)
		}
		if a.primaryButton(c, a.m("openProjectWith")).Disabled(strings.TrimSpace(a.editorExecutable) == "").Clicked() {
			executable := strings.TrimSpace(a.editorExecutable)
			a.work(func() (func(), error) {
				err := a.api.request("POST", "open-project-directory", map[string]string{"id": id, "executable": executable}, nil)
				return func() { a.back(false) }, err
			})
		}
	})
}
