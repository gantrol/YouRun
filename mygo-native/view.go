package main

import (
	"runtime"
	"sort"
	"strings"

	"commandmanager.local/core/internal/engine"
	"github.com/egoist/mygo/ui"
)

func (a *app) view(c *ui.Context) {
	c.SetTheme(a.theme)
	if a.loaded && !a.ready {
		a.ready = true
		go func() {
			err := a.api.request("POST", "updates/ready", nil, nil)
			a.win.Update(func() {
				if err != nil {
					a.issue = "updateHandoffFailed"
				} else {
					a.candidate = false
				}
			})
		}()
	}
	mod := ui.Ctrl
	if runtime.GOOS == "darwin" {
		mod = ui.Cmd
	}
	width, _ := c.Size()
	current := a.current().Kind
	quitting := a.quitOpen || a.prompting.Load()
	if !quitting && c.Shortcut(mod, ui.KeyK) && (current == "home" || width >= 688 && (current == "command" || current == "run")) {
		a.searchFocus = true
	}
	if !quitting && c.Shortcut(0, ui.KeyEscape) {
		if a.leaving {
			a.pendingPage = nil
			a.leaving = false
			a.closeAfterDiscard = false
		} else if a.current().Kind != "home" {
			a.back(false)
		} else {
			a.Query = ""
		}
	}
	ui.Column(c).Fill().Background(a.theme.Background).Disabled(quitting).Children(func() {
		a.toolbar(c)
		if a.issue != "" {
			ui.Row(c).FillWidth().Padding(8, 16).Gap(8).Wrap().Children(func() {
				key := a.issue
				if a.catalogs["en"][key] == "" {
					key = "requestFailed"
				}
				ui.Text(c, a.m(key)).TextColor(a.theme.Danger)
				if a.issue == "conflict" && a.button(c, a.m("reload")).Disabled(a.busy || (a.dirty() && !a.draftConflict)).Clicked() {
					if a.draftConflict {
						a.refreshDrafts()
					}
					a.bootstrap()
				}
				if a.icon(c.Key("dismiss"), "close", a.m("close"), false) {
					a.issue = ""
				}
			})
		}
		if a.leaving {
			ui.Column(c).FillWidth().Padding(12).Gap(8).Children(func() {
				ui.Text(c, a.m("unsaved"))
				ui.Row(c).Gap(8).Children(func() {
					if a.button(c, a.m("discard")).Clicked() {
						quit := a.closeAfterDiscard
						a.closeAfterDiscard = false
						if next := a.pendingPage; next != nil {
							a.pendingPage = nil
							a.discardDraft()
							a.open(*next)
						} else {
							a.back(true)
						}
						if quit {
							a.requestQuit()
						}
					}
					if a.primaryButton(c, a.m("continueEditing")).Clicked() {
						a.pendingPage = nil
						a.leaving = false
						a.closeAfterDiscard = false
					}
				})
			})
		}
		if !a.loaded {
			ui.Text(c, a.m("loading")).Padding(20)
			return
		}
		p := a.current()
		width, _ := c.Size()
		split := width >= 688 && (p.Kind == "command" || p.Kind == "run")
		blocked := a.busy || a.candidate || a.host.UpdatePending() || a.leaving
		ui.Row(c.Key("workspace")).FillWidth().Grow(1).MinHeight(0).Children(func() {
			if split {
				ui.Column(c.Key("home-pane")).WidthPercent(50).FillHeight().MinHeight(0).BorderWidth(0, 1, 0, 0).BorderColor(a.theme.Border).Children(func() {
					a.homeToolbar(c)
					ui.Scroll(c.Key("home-list")).FillWidth().Grow(1).MinHeight(0).Children(func() {
						ui.Column(c).FillWidth().Disabled(blocked).Children(func() { a.home(c) })
					})
					a.search(c)
				})
			}
			ui.Column(c.Key("page")).Grow(1).MinWidth(0).FillHeight().MinHeight(0).Children(func() {
				if p.Kind == "home" {
					a.homeToolbar(c)
				}
				ui.Scroll(c.Key("content-" + p.Kind + "-" + p.ID)).FillWidth().Grow(1).MinHeight(0).Children(func() {
					content := ui.Column(c).FillWidth().Disabled(blocked)
					if p.Kind != "home" {
						content.Padding(16).Gap(12)
					}
					content.Children(func() {
						switch p.Kind {
						case "home":
							a.home(c)
						case "settings":
							a.settings(c)
						case "components":
							a.componentCatalog(c)
						case "project":
							a.projectSettings(c, p.ID)
						case "projectCommands":
							a.projectCommands(c, p.ID)
						case "openWith":
							a.customProjectOpener(c, p.ID)
						case "editCommand":
							a.commandEditor(c)
						case "editProject":
							a.projectEditor(c)
						case "command":
							a.commandDetails(c, p.ID)
						case "run":
							a.runDetails(c, p.ID)
						case "history":
							a.history(c)
						case "deleteCommand", "deleteProject":
							a.deletePage(c, p)
						case "import":
							a.importView(c)
						case "updates":
							a.updateView(c)
						}
					})
				})
			})
		})
		if p.Kind == "home" {
			a.search(c)
		}
	})
	a.quitDialog(c)
}
func (a *app) search(c *ui.Context) {
	width, color := float32(1), a.theme.Border
	if a.skin != "classic" {
		width, color = 2, a.theme.Text
	}
	ui.Row(c.Key("search-bar")).FillWidth().Shrink(0).Padding(10, 12).BorderWidth(width, 0, 0, 0).BorderColor(color).Children(func() {
		field := ui.Row(c).FillWidth().MinWidth(0).Height(40).PaddingX(10).Gap(8).AlignItems(ui.Center).Background(a.theme.Surface).Border(width, color).Radius(a.theme.Radius)
		field.Children(func() {
			ui.Icon(c, icons["search"]).Width(16).Height(16).Shrink(0).TextColor(a.theme.TextMuted)
			e := ui.TextInputBase(c.Key("search"), &a.Query).Grow(1).MinWidth(0).Label(a.m("search")).Placeholder(a.m("search"))
			if a.skin == "pixel" {
				e.Font("monospace")
			}
			if a.searchFocus {
				e.Focus()
				a.searchFocus = false
			}
			if e.Focused() {
				field.Border(width, a.theme.Focus)
			}
			if a.Query != "" {
				if a.toolIcon(c.Key("clear"), "close", a.m("clear"), false) {
					a.Query = ""
					e.Focus()
				}
			} else {
				shortcut := "Ctrl K"
				if runtime.GOOS == "darwin" {
					shortcut = "⌘ K"
				}
				ui.Text(c, shortcut).FontSize(11).TextColor(a.theme.TextMuted).Shrink(0)
			}
		})
	})
}
func (a *app) toolbar(c *ui.Context) {
	width, color := float32(1), a.theme.Border
	if a.skin != "classic" {
		width, color = 2, a.theme.Text
	}
	ui.Row(c).FillWidth().Height(44).AlignItems(ui.Center).Gap(8).PaddingX(12).Background(a.theme.Surface).BorderWidth(0, 0, width, 0).BorderColor(color).Children(func() {
		if runtime.GOOS == "darwin" {
			ui.Row(c).Width(68)
		}
		if a.current().Kind != "home" && a.toolIcon(c.Key("back"), "back", a.m("back"), a.busy) {
			a.back(false)
		}
		ui.Image(c, rabbitLogos[a.skin]).Width(24).Height(24).Fit(ui.Contain).Shrink(0).DragWindow()
		title := ui.Text(c, a.m("productName")).Bold().Grow(1).DragWindow()
		if a.skin == "comic" {
			title.FontSize(18)
		}
		if a.skin == "pixel" {
			title.Font("monospace")
		}
		if a.busy {
			ui.Text(c, a.m("loading")).FontSize(12).TextColor(a.theme.TextMuted)
		}
		if runtime.GOOS != "darwin" {
			if a.toolIcon(c.Key("min"), "minus", a.m("windowMinimize"), false) {
				a.win.Minimize()
			}
			if a.toolIcon(c.Key("max"), "maximize", a.m("windowMaximize"), false) {
				a.win.ToggleMaximize()
			}
			if a.toolIcon(c.Key("close"), "close", a.m("windowClose"), a.busy) {
				a.requestQuit()
			}
		}
	})
}

func (a *app) home(c *ui.Context) {
	hasQuery := len(engine.SearchTerms(a.Query)) > 0
	shown := false
	type scoredProject struct {
		project  engine.Project
		children []engine.Command
		score    int
		index    int
	}
	var projects []scoredProject
	for idx, p := range a.state.Definitions.Projects {
		if p.Hidden {
			continue
		}
		var children []engine.Command
		bestCmdScore := 0
		for _, cmd := range a.state.Definitions.Commands {
			if pointer(cmd.Project) != p.ID || cmd.Hidden {
				continue
			}
			cmdScore := engine.ScoreCommand(cmd, a.Query, p.Name)
			if cmdScore > bestCmdScore {
				bestCmdScore = cmdScore
			}
			if !hasQuery || cmdScore > 0 {
				children = append(children, cmd)
			}
		}
		projScore := engine.ScoreProject(p, a.Query, a.projectDetails[p.ID].Directory, bestCmdScore)
		if hasQuery && projScore <= 0 && len(children) == 0 {
			continue
		}
		if hasQuery && len(children) == 0 {
			for _, cmd := range a.state.Definitions.Commands {
				if pointer(cmd.Project) == p.ID && !cmd.Hidden {
					children = append(children, cmd)
				}
			}
		}
		projects = append(projects, scoredProject{project: p, children: children, score: projScore, index: idx})
	}
	if hasQuery {
		sort.SliceStable(projects, func(i, j int) bool {
			if projects[i].score != projects[j].score {
				return projects[i].score > projects[j].score
			}
			return projects[i].index < projects[j].index
		})
	}
	for _, entry := range projects {
		p := entry.project
		children := entry.children
		shown = true
		ui.Column(c.Key(p.ID)).FillWidth().Gap(3).Children(func() {
			ui.Row(c).FillWidth().MinHeight(56).Wrap().Justify(ui.End).AlignItems(ui.Center).Gap(8).Padding(8, 12).ContextMenu(a.projectMenu(p)).Children(func() {
				b := ui.ButtonBase(c.Key("title")).Grow(1).Basis(96).MinWidth(0).Padding(4, 0).Label(p.Name).Tooltip(p.Name)
				b.Children(func() {
					ui.Row(c).FillWidth().MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
						runes := []rune(strings.TrimSpace(p.Name))
						badge := ui.Row(c).Width(32).Height(32).Shrink(0).AlignItems(ui.Center).Justify(ui.Center).Background(a.theme.Surface).Radius(a.theme.Radius)
						if a.skin == "comic" || a.skin == "pixel" {
							badge.Border(2, a.theme.Text).Shadow(2, 2, 0, 0, a.theme.Text)
						}
						badge.Children(func() {
							if logo := a.projectLogos[p.ID]; logo != nil {
								ui.Image(c, logo).Width(24).Height(24).Fit(ui.Contain)
							} else if options := a.projectLogoOptions[p.ID]; len(options) > 0 {
								ui.Column(c).Width(26).Height(26).Gap(1).Justify(ui.Center).Children(func() {
									for start := 0; start < len(options); start += 2 {
										ui.Row(c).Gap(1).Justify(ui.Center).Children(func() {
											for _, image := range options[start:min(start+2, len(options))] {
												ui.Image(c, image).Width(12).Height(12).Fit(ui.Contain)
											}
										})
									}
								})
							} else if len(runes) > 0 {
								ui.Text(c, strings.ToUpper(string(runes[0]))).Bold()
							}
						})
						name := ui.Text(c, p.Name).Bold().MaxLines(1).Shrink(1).MinWidth(0)
						if a.skin == "pixel" {
							name.Font("monospace")
						}
					})
				})
				if b.Clicked() {
					a.open(page{Kind: "projectCommands", ID: p.ID})
				}
				ui.Row(c.Key("project-actions")).Wrap().Justify(ui.End).Gap(8).AlignItems(ui.Center).Children(func() {
					for _, button := range p.Buttons {
						cmd := a.command(button.CommandID)
						if button.Hidden || (cmd != nil && cmd.Hidden) {
							continue
						}
						if cmd == nil {
							if a.icon(c.Key(button.ID), button.Icon, p.Name+" · "+a.m("configure"), false) {
								a.open(page{Kind: "project", ID: p.ID})
							}
						} else {
							a.actions(c.Key(button.ID), *cmd, button.Icon)
						}
					}
				})
			})
			for _, command := range children {
				a.commandFeedback(c.Key("issue-"+command.ID), command)
			}
		})
	}
	type scoredCmd struct {
		cmd   engine.Command
		score int
		index int
	}
	var commands []scoredCmd
	for idx, cmd := range a.state.Definitions.Commands {
		if cmd.Hidden || (cmd.Project != nil && !hasQuery) {
			continue
		}
		projName := ""
		if cmd.Project != nil {
			project := a.project(*cmd.Project)
			if project == nil || project.Hidden {
				continue
			}
			projName = project.Name
		}
		score := engine.ScoreCommand(cmd, a.Query, projName)
		if hasQuery && score <= 0 {
			continue
		}
		commands = append(commands, scoredCmd{cmd: cmd, score: score, index: idx})
	}
	if hasQuery {
		sort.SliceStable(commands, func(i, j int) bool {
			if commands[i].score != commands[j].score {
				return commands[i].score > commands[j].score
			}
			return commands[i].index < commands[j].index
		})
	}
	independent := false
	for _, entry := range commands {
		if !independent {
			label := "independentCommands"
			if hasQuery {
				label = "commandsList"
			}
			ui.Text(c, a.m(label)).Padding(8, 12).FontSize(12).TextColor(a.theme.TextMuted)
			independent = true
		}
		a.commandRow(c, entry.cmd)
		shown = true
	}
	if !shown {
		ui.Text(c, a.m("emptyResults")).Padding(16).TextColor(a.theme.TextMuted)
	}
}
func pointer(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
func (a *app) commandRow(c *ui.Context, cmd engine.Command) {
	ui.Row(c.Key(cmd.ID)).FillWidth().MinHeight(48).PaddingY(6).Wrap().Justify(ui.End).Gap(8).AlignItems(ui.Center).ContextMenu(a.commandMenu(cmd)).Children(func() {
		b := ui.ButtonBase(c.Key("name")).Grow(1).Basis(96).MinWidth(0).Padding(4).Label(cmd.Title).Tooltip(cmd.Title)
		b.Children(func() {
			ui.Row(c).FillWidth().MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
				ui.Icon(c, icons["file"]).Width(16).Height(16).Shrink(0).TextColor(a.theme.TextMuted)
				name := ui.Text(c, cmd.Title).MaxLines(1).Shrink(1).MinWidth(0)
				if a.skin == "pixel" {
					name.Font("monospace")
				}
			})
		})
		if b.Clicked() {
			a.openCommand(cmd.ID)
		}
		a.actions(c.Key("actions"), cmd, cmd.Icon)
	})
	a.commandFeedback(c.Key("issue-"+cmd.ID), cmd)
}
func commandUnavailable(cmd engine.Command) bool {
	return (cmd.Platform == "Windows" && runtime.GOOS != "windows") || (cmd.Platform == "macOS" && runtime.GOOS == "windows")
}
func (a *app) actions(c *ui.Context, cmd engine.Command, icon string) {
	run := a.running(cmd.ID)
	ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() {
		if run == nil {
			if a.icon(c.Key("launch"), icon, cmd.Title+" · "+a.m("start"), commandUnavailable(cmd)) {
				a.launch(cmd, false)
			}
			return
		}
		if a.icon(c.Key("output"), "output", cmd.Title+" · "+a.m("viewRun"), false) {
			a.open(page{Kind: "run", ID: run.ID})
		}
		if a.icon(c.Key("restart"), "restart", cmd.Title+" · "+a.m("restart"), run.Status == "stopping") {
			a.replay(run.ID)
		}
		if a.icon(c.Key("stop"), "stop", cmd.Title+" · "+a.m("stop"), run.Status == "stopping") {
			a.stop(run.ID)
		}
	})
}
