package main

import (
	"runtime"
	"slices"
	"strings"

	"commandmanager.local/core/desktop"
	"commandmanager.local/core/internal/engine"
	"commandmanager.local/core/resources"
	"github.com/egoist/mygo/ui"
)

func (a *app) settings(c *ui.Context) {
	var skins []option
	for _, id := range []string{"classic", "comic", "graphic", "pixel"} {
		definition, _ := resources.LookupSkin(id)
		skins = append(skins, option{id, a.m(definition.Label)})
	}
	skin := a.skin
	if a.selectField(c, "skin", &skin, skins) {
		a.work(func() (func(), error) {
			var saved desktop.Appearance
			if err := a.api.request("PUT", "appearance", desktop.Appearance{Skin: skin}, &saved); err != nil {
				return nil, err
			}
			return func() { a.skin = saved.Skin; a.theme = appTheme(saved.Skin) }, nil
		})
	}
	if a.button(c.Key("components"), a.m("componentLibrary")).Clicked() {
		a.open(page{Kind: "components"})
	}
	ui.Row(c).Gap(8).Wrap().Children(func() {
		if a.button(c, a.m("addProject")).Clicked() {
			a.editProject("")
		}
		if a.button(c, a.m("newCommand")).Clicked() {
			a.editCommand("", "")
		}
		if a.button(c, a.m("importProject")).Clicked() {
			a.chooseProjectDirectory(false)
		}
	})
	locale := a.state.Definitions.Locale
	if a.selectField(c, "language", &locale, []option{{"en", "English"}, {"zh", "简体中文"}}) {
		d := cloneDefinitions(a.state.Definitions)
		d.Locale = locale
		a.save(d, nil)
	}
	a.startupSetting(c)
	ui.Text(c, a.m("projects")).Bold()
	for _, p := range a.state.Definitions.Projects {
		visible := !p.Hidden
		row := a.managementRow(c.Key(p.ID), p.Name, &visible)
		if row.edit {
			a.open(page{Kind: "project", ID: p.ID})
		}
		if row.visibility {
			d := cloneDefinitions(a.state.Definitions)
			for i := range d.Projects {
				if d.Projects[i].ID == p.ID {
					d.Projects[i].Hidden = !visible
				}
			}
			a.save(d, nil)
		}
		if row.remove {
			a.open(page{Kind: "deleteProject", ID: p.ID})
		}
	}
	ui.Text(c, a.m("independentCommands")).Bold()
	for _, cmd := range a.state.Definitions.Commands {
		if cmd.Project == nil {
			a.commandSettingRow(c, cmd)
		}
	}
	if a.updateLoaded && !a.updates.Disabled && a.button(c, a.m("updateTitle")).Clicked() {
		a.open(page{Kind: "updates"})
	}
}
func (a *app) commandSettingRow(c *ui.Context, cmd engine.Command) {
	visible := !cmd.Hidden
	row := a.managementRow(c.Key(cmd.ID), cmd.Title, &visible)
	if row.edit {
		a.editCommand(cmd.ID, "")
	}
	if row.visibility {
		d := cloneDefinitions(a.state.Definitions)
		for i := range d.Commands {
			if d.Commands[i].ID == cmd.ID {
				d.Commands[i].Hidden = !visible
			}
		}
		a.save(d, nil)
	}
	if row.remove {
		a.open(page{Kind: "deleteCommand", ID: cmd.ID})
	}
}
func (a *app) editCommand(id, projectID string) {
	if a.dirty() {
		return
	}
	a.open(page{Kind: "editCommand", ID: id})
	draft := engine.Command{ID: engine.ID(), Shell: "default", Platform: "macOS / Windows", Icon: "play", Fields: []string{}}
	if projectID != "" {
		draft.Project = &projectID
	}
	if current := a.command(id); current != nil {
		draft = *current
		draft.Fields = slices.Clone(current.Fields)
	}
	a.commandDraft = &draft
	a.draftOriginal = encoded(&draft)
}
func (a *app) editProject(id string) {
	if a.dirty() {
		return
	}
	a.open(page{Kind: "editProject", ID: id})
	draft := engine.Project{ID: engine.ID(), Buttons: []engine.Button{{ID: engine.ID(), Icon: "play"}}}
	if current := a.project(id); current != nil {
		draft = *current
		draft.Buttons = slices.Clone(current.Buttons)
	}
	a.projectDraft = &draft
	a.draftOriginal = encoded(&draft)
}
func (a *app) commandEditor(c *ui.Context) {
	cmd := a.commandDraft
	if cmd == nil {
		return
	}
	a.field(c, "name", &cmd.Title)
	ui.Text(c, a.m("commandText")).FontSize(12).TextColor(a.theme.TextMuted)
	a.textArea(c.Key("script"), &cmd.Text).FillWidth().Height(120).Label(a.m("commandText"))
	a.directoryField(c, &cmd.Directory)
	projectID := pointer(cmd.Project)
	options := []option{{"", a.m("independentCommands")}}
	for _, p := range a.state.Definitions.Projects {
		options = append(options, option{p.ID, p.Name})
	}
	if a.selectField(c, "project", &projectID, options) {
		cmd.Project = nil
		if projectID != "" {
			cmd.Project = &projectID
		}
	}
	shellOptions := []option{{"default", a.m("defaultShell")}, {"PowerShell", "PowerShell"}, {"zsh", "zsh"}, {"bash", "bash"}}
	if (runtime.GOOS == "windows" || cmd.Platform == "Windows" || cmd.Shell == "cmd") && cmd.Platform != "macOS" {
		shellOptions = append(shellOptions, option{"cmd", "cmd"})
	}
	a.selectField(c, "shell", &cmd.Shell, shellOptions)
	a.selectField(c, "platform", &cmd.Platform, []option{{"macOS / Windows", a.m("allPlatforms")}, {"macOS", "macOS"}, {"Windows", "Windows"}})
	a.selectField(c, "buttonIcon", &cmd.Icon, a.iconOptions())
	ui.Text(c, a.m("inputFields")).FontSize(12).TextColor(a.theme.TextMuted)
	ui.Row(c).Gap(12).Children(func() {
		for _, field := range []string{"input", "output", "port"} {
			checked := slices.Contains(cmd.Fields, field)
			if a.checkbox(c.Key(field), &checked, a.m(field)).Changed() {
				if checked {
					cmd.Fields = append(cmd.Fields, field)
				} else {
					cmd.Fields = slices.DeleteFunc(cmd.Fields, func(v string) bool { return v == field })
				}
			}
		}
	})
	visible := !cmd.Hidden
	if a.checkbox(c, &visible, a.m("showOnHome")).Changed() {
		cmd.Hidden = !visible
	}
	a.checkbox(c, &cmd.Persistent, a.m("persistentCommand"))
	if a.primaryButton(c, a.m("save")).Disabled(strings.TrimSpace(cmd.Title) == "" || strings.TrimSpace(cmd.Text) == "").Clicked() {
		d := cloneDefinitions(a.state.Definitions)
		saved := *cmd
		saved.Fields = slices.Clone(cmd.Fields)
		found := false
		for i := range d.Commands {
			if d.Commands[i].ID == saved.ID {
				d.Commands[i] = saved
				found = true
			}
		}
		if !found {
			d.Commands = append(d.Commands, saved)
		}
		for i := range d.Projects {
			if d.Projects[i].ID != pointer(saved.Project) {
				for j := range d.Projects[i].Buttons {
					if d.Projects[i].Buttons[j].CommandID == saved.ID {
						d.Projects[i].Buttons[j].CommandID = ""
					}
				}
			}
		}
		a.save(d, func() { delete(a.Directories, saved.ID); a.back(true) })
	}
}
func (a *app) directoryField(c *ui.Context, directory *string) {
	a.field(c, "directory", directory)
	if a.button(c, a.m("chooseDirectory")).Clicked() {
		a.work(func() (func(), error) {
			var result struct {
				Directory string `json:"directory"`
			}
			err := a.api.request("POST", "choose-directory", nil, &result)
			return func() {
				if result.Directory != "" {
					*directory = result.Directory
				}
			}, err
		})
	}
}
func (a *app) projectEditor(c *ui.Context) {
	p := a.projectDraft
	if p == nil {
		return
	}
	a.field(c, "projectName", &p.Name)
	if a.primaryButton(c, a.m("save")).Disabled(strings.TrimSpace(p.Name) == "").Clicked() {
		d := cloneDefinitions(a.state.Definitions)
		found := false
		for i := range d.Projects {
			if d.Projects[i].ID == p.ID {
				d.Projects[i] = *p
				found = true
			}
		}
		if !found {
			d.Projects = append(d.Projects, *p)
		}
		a.save(d, func() { a.back(true) })
	}
}
func (a *app) projectSettings(c *ui.Context, id string) {
	current := a.project(id)
	if current == nil {
		ui.Text(c, a.m("commandMissing"))
		return
	}
	if a.projectDraft == nil {
		p := *current
		p.Buttons = slices.Clone(current.Buttons)
		a.projectDraft = &p
		a.draftOriginal = encoded(&p)
	}
	p := a.projectDraft
	a.projectOpenerButton(c.Key("project-openers"), id, a.dirty() || a.projectDetails[id].Directory == "")
	a.field(c, "projectName", &p.Name)
	visible := !p.Hidden
	if a.checkbox(c, &visible, a.m("showOnHome")).Changed() {
		p.Hidden = !visible
	}
	ui.Text(c, a.m("launchButtons")).Bold()
	for index := range p.Buttons {
		b := &p.Buttons[index]
		ui.Column(c.Key(b.ID)).FillWidth().Gap(8).PaddingY(8).Children(func() {
			options := []option{{"", a.m("notAssigned")}}
			for _, cmd := range a.state.Definitions.Commands {
				if pointer(cmd.Project) == id {
					used := false
					for j, other := range p.Buttons {
						if j != index && other.CommandID == cmd.ID {
							used = true
						}
					}
					if !used {
						options = append(options, option{cmd.ID, cmd.Title})
					}
				}
			}
			a.selectField(c, "buttonCommand", &b.CommandID, options)
			a.selectField(c, "buttonIcon", &b.Icon, a.iconOptions())
			ui.Row(c).Wrap().Gap(8).Children(func() {
				show := !b.Hidden
				if a.checkbox(c, &show, a.m("showOnHome")).Changed() {
					b.Hidden = !show
				}
				if a.button(c, a.m("moveEarlier")).Disabled(index == 0).Clicked() {
					p.Buttons[index-1], p.Buttons[index] = p.Buttons[index], p.Buttons[index-1]
				}
				if a.button(c, a.m("moveLater")).Disabled(index == len(p.Buttons)-1).Clicked() {
					p.Buttons[index+1], p.Buttons[index] = p.Buttons[index], p.Buttons[index+1]
				}
				if a.button(c, a.m("removeButton")).Clicked() {
					p.Buttons = slices.Delete(p.Buttons, index, index+1)
				}
			})
		})
		// A removal changes the slice length inside this frame.
		if index >= len(p.Buttons)-1 {
			break
		}
	}
	if a.button(c, a.m("addButton")).Disabled(len(p.Buttons) >= 3).Clicked() {
		p.Buttons = append(p.Buttons, engine.Button{ID: engine.ID(), Icon: "play"})
	}
	if a.primaryButton(c, a.m("save")).Disabled(strings.TrimSpace(p.Name) == "").Clicked() {
		d := cloneDefinitions(a.state.Definitions)
		for i := range d.Projects {
			if d.Projects[i].ID == id {
				d.Projects[i] = *p
			}
		}
		a.save(d, func() { a.projectDraft = nil; a.draftOriginal = "" })
	}
	ui.Text(c, a.m("commandsList")).Bold()
	ui.Column(c).FillWidth().Gap(8).Disabled(a.dirty()).Children(func() {
		for _, cmd := range a.state.Definitions.Commands {
			if pointer(cmd.Project) == id {
				a.commandSettingRow(c, cmd)
			}
		}
	})
	ui.Row(c).Gap(8).Wrap().Children(func() {
		if a.button(c, a.m("newCommand")).Disabled(a.dirty()).Clicked() {
			a.projectDraft = nil
			a.editCommand("", id)
		}
		if a.button(c, a.m("deleteProject")).Clicked() {
			a.open(page{Kind: "deleteProject", ID: id})
		}
	})
}
func (a *app) deletePage(c *ui.Context, p page) {
	key := "deleteCommandEffect"
	name := ""
	if p.Kind == "deleteProject" {
		key = "deleteProjectEffect"
		if project := a.project(p.ID); project != nil {
			name = project.Name
		}
	} else if cmd := a.command(p.ID); cmd != nil {
		name = cmd.Title
	}
	ui.Text(c, name).Bold()
	ui.Text(c, a.m(key))
	ui.Row(c).Gap(8).Children(func() {
		if a.button(c, a.m("cancel")).Clicked() {
			a.back(true)
		}
		if a.button(c, a.m("remove")).TextColor(a.theme.Danger).Clicked() {
			d := cloneDefinitions(a.state.Definitions)
			if p.Kind == "deleteProject" {
				d.Projects = slices.DeleteFunc(d.Projects, func(v engine.Project) bool { return v.ID == p.ID })
				d.Commands = slices.DeleteFunc(d.Commands, func(v engine.Command) bool { return pointer(v.Project) == p.ID })
			} else {
				d.Commands = slices.DeleteFunc(d.Commands, func(v engine.Command) bool { return v.ID == p.ID })
				for i := range d.Projects {
					for j := range d.Projects[i].Buttons {
						if d.Projects[i].Buttons[j].CommandID == p.ID {
							d.Projects[i].Buttons[j].CommandID = ""
						}
					}
				}
			}
			a.save(d, func() {
				a.back(true)
				for p.Kind == "deleteProject" && a.current().ID == p.ID && (a.current().Kind == "project" || a.current().Kind == "projectCommands") {
					a.back(true)
				}
			})
		}
	})
}
