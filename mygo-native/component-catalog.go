package main

import (
	"log"
	"runtime"

	"commandmanager.local/core/resources"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// All examples use the production controls and local presentation state only.
// No example starts a command, edits definitions or writes application settings.
type componentCatalogState struct {
	skin, name, command, shell, menu, action      string
	checked, unchecked, shortVisible, longVisible bool
	dialogOpen                                    bool
}

func newComponentCatalogState(skin string) *componentCatalogState {
	return &componentCatalogState{skin: skin, name: "YouRun", command: "yourun help", shell: "PowerShell", checked: true, shortVisible: true, longVisible: true}
}

func (a *app) componentCatalog(c *ui.Context) {
	if a.catalog == nil {
		a.catalog = newComponentCatalogState(a.skin)
	}
	s := a.catalog
	ui.Row(c.Key("catalog-header")).FillWidth().Gap(12).AlignItems(ui.Center).Children(func() {
		ui.Text(c, a.m("componentLibrary")).Bold().Grow(1)
		if a.button(c, a.m("resetPreview")).Clicked() {
			a.catalog = newComponentCatalogState(s.skin)
		}
	})
	var skins []option
	for _, id := range []string{"classic", "comic", "graphic", "pixel"} {
		definition, _ := resources.LookupSkin(id)
		skins = append(skins, option{id, a.m(definition.Label)})
	}
	a.selectField(c.Key("preview-skin"), "skin", &s.skin, skins)
	p := &app{skin: s.skin, theme: appTheme(s.skin), catalogs: a.catalogs}
	p.state.Definitions.Locale = a.state.Definitions.Locale
	c.SetTheme(p.theme)
	defer c.SetTheme(a.theme)
	ui.Column(c.Key("preview")).FillWidth().Padding(12).Gap(20).Background(p.theme.Background).TextColor(p.theme.Text).Font(p.theme.Font).FontSize(p.theme.FontSize).Children(func() {
		p.catalogSection(c.Key("buttons"), p.m("componentButtons"), func() {
			ui.Row(c).Wrap().Gap(12).AlignItems(ui.Center).Children(func() {
				if p.primaryButton(c.Key("primary"), p.m("start")).Clicked() {
					s.action = "start"
				}
				if p.button(c.Key("outline"), p.m("save")).Clicked() {
					s.action = "save"
				}
				if p.button(c.Key("danger"), p.m("remove")).TextColor(p.theme.Danger).Clicked() {
					s.action = "remove"
				}
				ui.Row(c.Key("disabled")).Disabled(true).Children(func() { p.button(c, p.m("disabled")) })
			})
		})
		p.catalogSection(c.Key("icons"), p.m("buttonIcon"), func() {
			ui.Row(c).Wrap().Gap(12).AlignItems(ui.Center).Children(func() {
				for _, action := range []option{{"play", "iconPlay"}, {"bolt", "iconBolt"}, {"package", "iconPackage"}, {"output", "viewRun"}, {"restart", "restart"}, {"stop", "stop"}} {
					if p.icon(c.Key(action.id), action.id, p.m(action.label), false) {
						s.action = "start"
					}
				}
				p.icon(c.Key("disabled"), "play", p.m("disabled"), true)
			})
			ui.Toolbar(c.Key("tools"), func() {
				for _, key := range []string{"back", "history", "settings", "close"} {
					p.toolIcon(c.Key(key), key, p.m(key), false)
				}
			}).Label(p.m("controls")).FillWidth().Padding(0)
		})
		p.catalogSection(c.Key("inputs"), p.m("inputFields"), func() {
			p.field(c, "name", &s.name)
			p.selectField(c, "shell", &s.shell, []option{{"PowerShell", "PowerShell"}, {"bash", "bash"}, {"zsh", "zsh"}})
			p.textArea(c.Key("command"), &s.command).FillWidth().Height(80).Label(p.m("commandText"))
			readonly := "yourun help"
			p.textInput(c.Key("readonly"), &readonly).FillWidth().ReadOnly(true).Label(p.m("componentReadOnly"))
		})
		p.catalogSection(c.Key("checks"), p.m("componentSelection"), func() {
			ui.Row(c).Wrap().Gap(16).AlignItems(ui.Center).Children(func() {
				p.checkbox(c.Key("checked"), &s.checked, p.m("showOnHome"))
				p.checkbox(c.Key("unchecked"), &s.unchecked, p.m("persistentCommand"))
				checked := true
				p.checkbox(c.Key("disabled"), &checked, p.m("disabled")).Disabled(true)
			})
		})
		p.catalogSection(c.Key("menu"), p.m("openProjectWith"), func() {
			p.choiceButton(c, &s.menu, p.m("openProjectWith"), []choice{
				{id: "editor", label: "Visual Studio Code"}, {id: "terminal", label: "PowerShell"},
				{id: "disabled", label: p.m("disabled"), disabled: true},
				{id: "other", label: p.m("otherProjectEditor"), divider: true}, {id: "refresh", label: p.m("refresh")},
			}, false).MaxWidth(280)
		})
		p.catalogSection(c.Key("rows"), p.m("projects"), func() {
			p.managementRow(c.Key("short"), "YouRun", &s.shortVisible)
			p.managementRow(c.Key("long"), p.m("componentLongName"), &s.longVisible)
		})
		if p.button(c.Key("dialog"), p.m("componentDialog")).Clicked() {
			s.dialogOpen = true
		}
		width, _ := c.Size()
		ui.DialogBase(c.Key("sample-dialog"), &s.dialogOpen, func(backdrop, panel ui.Element) {
			backdrop.Background(ui.RGBA(0, 0, 0, 0.2))
			p.popupSurface(panel).Width(360).MaxWidth(width - 32).Padding(20).Label(p.m("componentDialog"))
			ui.Column(c).FillWidth().Gap(16).Children(func() {
				ui.Text(c, p.m("componentDialog")).Bold()
				ui.Row(c).FillWidth().Wrap().Gap(12).Justify(ui.End).Children(func() {
					if p.button(c.Key("cancel"), p.m("cancel")).Clicked() {
						s.dialogOpen = false
					}
					if p.primaryButton(c.Key("save"), p.m("save")).AutoFocus().Clicked() {
						s.dialogOpen = false
					}
				})
			})
		})
		if s.action != "" {
			ui.Text(c, p.m(s.action)).Role(ui.RoleStatus).TextColor(p.theme.TextMuted)
		}
	})
}

func (a *app) catalogSection(c *ui.Context, title string, content func()) {
	ui.Column(c).FillWidth().Gap(10).Children(func() {
		ui.Text(c, title).Bold()
		content()
	})
}

// --components opens only the catalog; it does not bootstrap the application host.
func runComponentCatalog() {
	a := &app{skin: "comic", theme: appTheme("comic"), catalogs: map[string]map[string]string{"en": resources.Messages("en"), "zh": resources.Messages("zh")}}
	a.state.Definitions.Locale = "zh"
	mygo.App.SetName(a.m("productName"))
	mygo.App.WhenReady(func() {
		window := mygo.NewWindow(mygo.WindowOptions{Title: a.m("productName") + " · " + a.m("componentLibrary"), Width: 720, Height: 900, MinWidth: 360, MinHeight: 420, Content: ui.View(func(c *ui.Context) {
			c.SetTheme(a.theme)
			ui.Scroll(c).Fill().Children(func() {
				ui.Column(c).FillWidth().Padding(16).Gap(16).Children(func() { a.componentCatalog(c) })
			})
		})})
		if err := window.SetIcon(resources.RabbitPNG); err != nil {
			log.Printf("Set catalog icon: %v", err)
		}
		if runtime.GOOS == "windows" && !window.IsVisible() {
			window.Show()
		}
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
