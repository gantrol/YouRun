package main

import (
	"commandmanager.local/core/internal/discovery"
	"commandmanager.local/core/internal/engine"
	"errors"
	"github.com/egoist/mygo/ui"
)

func (a *app) chooseProjectDirectory(replace bool) {
	a.work(func() (func(), error) {
		var folder struct {
			Directory string `json:"directory"`
		}
		if err := a.api.request("POST", "choose-directory", nil, &folder); err != nil {
			return nil, err
		}
		if folder.Directory == "" {
			return nil, nil
		}
		return func() {
			a.scan = discovery.Result{Directory: folder.Directory}
			a.importScanned = false
			a.checked = map[string]bool{}
			a.importProject = ""
			a.importMetadataProject, a.importMetadataDirectory = "", ""
			if !replace {
				a.open(page{Kind: "import"})
			}
			a.scanProjectDirectory(folder.Directory)
		}, nil
	})
}

func (a *app) scanProjectDirectory(directory string) {
	a.work(func() (func(), error) {
		var result discovery.Result
		if err := a.api.request("POST", "discover", map[string]string{"directory": directory}, &result); err != nil {
			return nil, err
		}
		return func() {
			a.scan = result
			a.importScanned = true
			a.checked = map[string]bool{}
			for _, item := range result.Candidates {
				if !item.Review && !a.imported(item) && !commandUnavailable(engine.Command{Platform: item.Platform}) {
					a.checked[item.ID] = true
				}
			}
		}, nil
	})
}

func (a *app) importView(c *ui.Context) {
	ui.Text(c, a.scan.Directory).FontSize(12).TextColor(a.theme.TextMuted)
	if a.importMetadataProject != "" {
		if a.primaryButton(c, a.m("retry")).Clicked() {
			a.saveImportDirectory(a.importMetadataProject, a.importMetadataDirectory)
		}
		return
	}
	if a.button(c, a.m("changeImportDirectory")).Clicked() {
		a.chooseProjectDirectory(true)
	}
	if a.busy {
		ui.Text(c, a.m("scanning"))
		return
	}
	if !a.importScanned {
		if a.scan.Directory != "" && a.button(c, a.m("retry")).Clicked() {
			a.scanProjectDirectory(a.scan.Directory)
		}
		return
	}
	options := []option{{"", a.m("newProject")}}
	for _, p := range a.state.Definitions.Projects {
		options = append(options, option{p.ID, p.Name})
	}
	a.selectField(c, "project", &a.importProject, options)
	if a.importProject == "" {
		a.field(c, "projectName", &a.scan.Name)
	}
	count := 0
	for _, item := range a.scan.Candidates {
		if a.imported(item) {
			continue
		}
		selected := a.checked[item.ID]
		if selected {
			count++
		}
		ui.Column(c.Key(item.ID)).FillWidth().Gap(4).Children(func() {
			if a.checkbox(c, &selected, item.Title).Changed() {
				a.checked[item.ID] = selected
			}
			ui.Text(c, item.Text).FontSize(12).TextColor(a.theme.TextMuted)
			ui.Text(c, item.Source).FontSize(12).TextColor(a.theme.TextMuted)
			if item.Review {
				ui.Text(c, a.m("reviewRequired")).FontSize(12).TextColor(a.theme.Warning)
			}
		})
	}
	for _, issue := range a.scan.Issues {
		ui.Text(c, a.m(issue.Code)+" · "+issue.Path).FontSize(12).TextColor(a.theme.Warning)
	}
	if len(a.scan.Candidates) == 0 {
		ui.Text(c, a.m("noProjectCommands"))
	}
	if a.primaryButton(c, a.m("importSelected")).Disabled(count == 0).Clicked() {
		d := cloneDefinitions(a.state.Definitions)
		projectID := a.importProject
		if projectID == "" {
			projectID = engine.ID()
			d.Projects = append(d.Projects, engine.Project{ID: projectID, Name: a.scan.Name, Buttons: []engine.Button{{ID: engine.ID(), Icon: "play"}}})
		}
		for _, item := range a.scan.Candidates {
			if !a.checked[item.ID] || a.imported(item) {
				continue
			}
			source := item.Source
			pid := projectID
			d.Commands = append(d.Commands, engine.Command{ID: item.ID, Title: item.Title, Text: item.Text, Project: &pid, Source: &source, Directory: item.Directory, Shell: item.Shell, Platform: item.Platform, Icon: item.Icon, Fields: []string{}})
		}
		directory := a.scan.Directory
		a.save(d, func() {
			a.importProject = projectID
			a.importMetadataProject, a.importMetadataDirectory = projectID, directory
			a.saveImportDirectory(projectID, directory)
		})
	}
}

func (a *app) saveImportDirectory(id, directory string) {
	a.work(func() (func(), error) {
		if err := a.api.requestProject("POST", "project-metadata", map[string]string{"id": id, "directory": directory}, nil); err != nil {
			return nil, errors.New("importProjectMetadataFailed")
		}
		return func() {
			a.importMetadataProject, a.importMetadataDirectory = "", ""
			a.loadProjectDetails(id)
			a.back(true)
		}, nil
	})
}
func (a *app) imported(item discovery.Candidate) bool {
	for _, cmd := range a.state.Definitions.Commands {
		if cmd.ID == item.ID || (pointer(cmd.Source) == item.Source && cmd.Text == item.Text) {
			return true
		}
	}
	return false
}
