package main

import (
	"slices"
	"time"

	"commandmanager.local/core/internal/engine"
	"github.com/egoist/mygo/ui"
)

func (a *app) commandDetails(c *ui.Context, id string) {
	cmd := a.command(id)
	if cmd == nil {
		ui.Text(c, a.m("commandMissing"))
		return
	}
	ui.Text(c, cmd.Title).Bold()
	text := cmd.Text
	a.textArea(c, &text).ReadOnly(true).FillWidth().Height(90).Label(a.m("commandText"))
	directory := a.directory(*cmd)
	if a.field(c, "directory", &directory) {
		a.Directories[id] = directory
	}
	if a.button(c, a.m("chooseDirectory")).Clicked() {
		a.work(func() (func(), error) {
			var result struct {
				Directory string `json:"directory"`
			}
			err := a.api.request("POST", "choose-directory", nil, &result)
			return func() {
				if result.Directory != "" {
					a.Directories[id] = result.Directory
				}
			}, err
		})
	}
	if a.Inputs[id] == nil {
		a.Inputs[id] = map[string]string{}
	}
	for _, field := range cmd.Fields {
		value := a.Inputs[id][field]
		if a.field(c, field, &value) {
			a.Inputs[id][field] = value
		}
	}
	if a.running(id) != nil {
		a.actions(c, *cmd, cmd.Icon)
	}
	// Parameterized commands use the entered values when explicitly started here.
	if a.running(id) == nil && a.primaryButton(c, a.m("start")).Clicked() {
		a.launch(*cmd, true)
	}
	a.commandFeedback(c.Key("launch-issue"), *cmd)
	for _, run := range a.state.Runs {
		if run.CommandID == id {
			a.historyRow(c, run)
		}
	}
	if a.button(c, a.m("editCommand")).Clicked() {
		a.editCommand(id, "")
	}
}
func (a *app) runDetails(c *ui.Context, id string) {
	var run *engine.Run
	for _, item := range a.state.Runs {
		if item.ID == id {
			run = &item
			break
		}
	}
	if run == nil {
		ui.Text(c, a.m("runNotActive"))
		return
	}
	ui.Text(c, run.Title).Bold()
	ui.Row(c).Wrap().Gap(8).Children(func() {
		ui.Text(c, a.m(run.Status)).TextColor(a.statusColor(*run))
		if run.Status != "unknown" {
			label := "runAgain"
			if active(*run) {
				label = "restart"
			}
			if a.button(c, a.m(label)).Disabled(run.Status == "stopping").Clicked() {
				a.replay(run.ID)
			}
		}
		if active(*run) && a.button(c, a.m("stop")).Disabled(run.Status == "stopping").Clicked() {
			a.stop(run.ID)
		}
	})
	ui.Text(c, run.Directory).FontSize(12).TextColor(a.theme.TextMuted)
	ui.Text(c, run.Shell).FontSize(12).TextColor(a.theme.TextMuted)
	if run.ExitCode != nil {
		ui.Textf(c, "%s: %d", a.m("exitCode"), *run.ExitCode).FontSize(12)
	}
	if run.Error != "" {
		ui.Text(c, a.m(run.Error)).TextColor(a.theme.Danger)
	}
	text := run.Text
	a.textArea(c.Key("command"), &text).FillWidth().Height(80).ReadOnly(true).Label(a.m("commandText"))
	a.textArea(c.Key("output-"+id), &a.output).FillWidth().Height(320).ReadOnly(true).Label(a.m("outputTab"))
	if a.logIssue != "" {
		ui.Text(c, a.m(a.logIssue)).TextColor(a.theme.Danger)
	} else if a.outputTruncated {
		ui.Text(c, a.m("logsTruncated")).TextColor(a.theme.TextMuted)
	}
}
func (a *app) statusColor(run engine.Run) ui.Color {
	switch run.Status {
	case "running", "success":
		return a.theme.Success
	case "failed":
		return a.theme.Danger
	case "stopping", "unknown":
		return a.theme.Warning
	default:
		return a.theme.TextMuted
	}
}
func (a *app) history(c *ui.Context) {
	runs := slices.Clone(a.state.Runs)
	group := func(r engine.Run) int {
		if active(r) {
			return 0
		}
		if r.Status == "unknown" {
			return 1
		}
		return 2
	}
	slices.SortStableFunc(runs, func(x, y engine.Run) int {
		if group(x) != group(y) {
			return group(x) - group(y)
		}
		if x.Time > y.Time {
			return -1
		}
		if x.Time < y.Time {
			return 1
		}
		return 0
	})
	for _, run := range runs {
		a.historyRow(c, run)
	}
	if len(runs) == 0 {
		ui.Text(c, a.m("noActiveRuns")).TextColor(a.theme.TextMuted)
	}
}
func (a *app) historyRow(c *ui.Context, run engine.Run) {
	b := ui.ButtonBase(c.Key(run.ID)).FillWidth().PaddingY(10).Label(run.Title + " · " + a.m(run.Status))
	b.Children(func() {
		ui.Row(c).FillWidth().Gap(12).Children(func() {
			ui.Column(c).Grow(1).MinWidth(0).Children(func() {
				ui.Text(c, run.Title).MaxLines(1)
				if t, err := time.Parse(time.RFC3339, run.Time); err == nil {
					ui.Text(c, t.Local().Format(time.DateTime)).FontSize(12).TextColor(a.theme.TextMuted)
				}
			})
			ui.Text(c, a.m(run.Status)).Width(90).TextColor(a.statusColor(run)).FontSize(12)
		})
	})
	if b.Clicked() {
		a.open(page{Kind: "run", ID: run.ID})
	}
}
