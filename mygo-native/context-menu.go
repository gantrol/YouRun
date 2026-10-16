package main

import (
	"commandmanager.local/core/internal/engine"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"
)

func (a *app) copyText(text string) {
	if err := mygo.Clipboard.Write(transfer.TextData(text)); err != nil {
		a.issue = "copyFailed"
	}
}

func (a *app) runMenu(menu *ui.Menu, cmd engine.Command, named bool) {
	suffix := ""
	if named {
		suffix = " · " + cmd.Title
	}
	run := a.running(cmd.ID)
	label := "start"
	if run != nil {
		label = "viewRun"
	}
	if menu.Item(a.m(label) + suffix).Chosen() {
		a.launch(cmd, false)
	}
	if run != nil {
		if menu.Item(a.m("restart") + suffix).Disabled(run.Status == "stopping").Chosen() {
			a.replay(run.ID)
		}
		if menu.Item(a.m("stop") + suffix).Disabled(run.Status == "stopping").Chosen() {
			a.stop(run.ID)
		}
	}
}

func (a *app) commandMenu(cmd engine.Command) func(*ui.Menu) {
	return func(menu *ui.Menu) {
		a.runMenu(menu, cmd, false)
		if menu.Item(a.m("viewCommand")).Chosen() {
			a.openCommand(cmd.ID)
		}
		menu.Separator()
		if menu.Item(a.m("copyCommand")).Chosen() {
			a.copyText(cmd.Text)
		}
		if menu.Item(a.m("edit")).Chosen() {
			a.editCommand(cmd.ID, pointer(cmd.Project))
		}
		if menu.Item(a.m("hide")).Chosen() {
			d := cloneDefinitions(a.state.Definitions)
			for i := range d.Commands {
				if d.Commands[i].ID == cmd.ID {
					d.Commands[i].Hidden = true
				}
			}
			a.save(d, nil)
		}
		menu.Separator()
		if menu.Item(a.m("remove")).Chosen() {
			a.open(page{Kind: "deleteCommand", ID: cmd.ID})
		}
	}
}

func (a *app) projectMenu(project engine.Project) func(*ui.Menu) {
	return func(menu *ui.Menu) {
		shortcuts := false
		for _, button := range project.Buttons {
			cmd := a.command(button.CommandID)
			if button.Hidden || cmd == nil || cmd.Hidden || pointer(cmd.Project) != project.ID {
				continue
			}
			a.runMenu(menu, *cmd, true)
			shortcuts = true
		}
		if shortcuts {
			menu.Separator()
		}
		directory := a.projectDetails[project.ID].Directory
		menu.Submenu(a.m("openProjectWith"), func(sub *ui.Menu) {
			a.projectOpenersMenu(sub, project.ID)
		}).Disabled(directory == "")
		if menu.Item(a.m("openProjectDirectory")).Disabled(directory == "").Chosen() {
			a.work(func() (func(), error) {
				return nil, a.api.request("POST", "open-project-directory", map[string]string{"id": project.ID}, nil)
			})
		}
		if menu.Item(a.m("copyProjectPath")).Disabled(directory == "").Chosen() {
			a.copyText(directory)
		}
		if menu.Item(a.m("refreshProjectLogo")).Chosen() {
			a.loadProjectDetails(project.ID)
		}
		detail := a.projectDetails[project.ID]
		if menu.Item(a.m("chooseProjectAvatar")).Chosen() {
			a.chooseProjectAvatar(project.ID)
		}
		if len(detail.Logos) > 0 || detail.SelectedLogo != "" || detail.CustomAvatar {
			menu.Separator()
			for _, issue := range detail.Issues {
				if issue.Code == "logoSelectionMissing" || issue.Code == "avatarReadFailed" {
					menu.Item(a.m(issue.Code)).Disabled(true)
					break
				}
			}
			if menu.Item(a.m("automaticProjectLogo")).Checked(detail.SelectedLogo == "" && !detail.CustomAvatar).Chosen() {
				a.selectProjectLogo(project.ID, "")
			}
			for _, logo := range detail.Logos {
				if menu.Item(logo.Name).Checked(detail.SelectedLogo == logo.ID).Chosen() {
					a.selectProjectLogo(project.ID, logo.ID)
				}
			}
		}
		menu.Separator()
		if menu.Item(a.m("projectSettings")).Chosen() {
			a.open(page{Kind: "project", ID: project.ID})
		}
		if menu.Item(a.m("renameProject")).Chosen() {
			a.editProject(project.ID)
		}
		if menu.Item(a.m("hide")).Chosen() {
			d := cloneDefinitions(a.state.Definitions)
			for i := range d.Projects {
				if d.Projects[i].ID == project.ID {
					d.Projects[i].Hidden = true
				}
			}
			a.save(d, nil)
		}
		menu.Separator()
		if menu.Item(a.m("deleteProject")).Chosen() {
			a.open(page{Kind: "deleteProject", ID: project.ID})
		}
	}
}
