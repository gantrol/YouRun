package main

import (
	"runtime"
	"strings"

	"commandmanager.local/core/resources"
	"github.com/egoist/mygo/ui"
)

func appTheme(skinID string) *ui.Theme {
	t := ui.LightTheme()
	t.Background = ui.Hex("#f8f7f3")
	t.Surface = ui.Hex("#fffefb")
	t.SurfaceHover = ui.Hex("#f1efea")
	t.SurfacePressed = ui.Hex("#dedad2")
	t.Text = ui.Hex("#262320")
	t.TextMuted = ui.Hex("#6c665e")
	t.Border = ui.Hex("#dedad2")
	t.Accent = ui.Hex("#80566a")
	t.AccentHover = ui.Hex("#684456")
	t.AccentPressed = ui.Hex("#6e4052")
	t.Focus = ui.Hex("#6e4052")
	t.Selection = ui.Hex("#efe5e9")
	t.Success = ui.Hex("#2f6b4f")
	t.Danger = ui.Hex("#b42318")
	t.Warning = ui.Hex("#a15c07")
	t.Radius = 4
	t.FontSize = 14
	if runtime.GOOS == "windows" {
		t.Font = "Segoe UI, Microsoft YaHei UI, sans-serif"
	}
	if skin, ok := resources.LookupSkin(skinID); ok {
		t.Radius = skin.Radius
		for key, target := range map[string]*ui.Color{
			"background": &t.Background, "surface": &t.Surface,
			"hover": &t.SurfaceHover, "pressed": &t.SurfacePressed,
			"foreground": &t.Text, "muted-foreground": &t.TextMuted,
			"border": &t.Border, "button-primary": &t.Accent,
			"button-primary-hover": &t.AccentHover, "button-primary-foreground": &t.AccentText,
			"selected": &t.Selection, "ring": &t.Focus,
		} {
			if value := skin.Colors[key]; value != "" {
				*target = ui.Hex(value)
			}
		}
		t.AccentPressed = t.AccentHover
	}
	return t
}

var icons = func() map[string]*ui.SVG {
	paths := map[string]string{
		"play":     `<path d="M8 5 19 12 8 19Z"/>`,
		"bolt":     `<path d="m13 3-8 11h6l-1 7 9-12h-6Z"/>`,
		"terminal": `<path d="m4 6 6 6-6 6m9 0h7"/>`,
		"package":  `<path d="m3 7 9-4 9 4v10l-9 4-9-4Zm0 0 9 5 9-5M12 12v9M8 5l9 5"/>`,
		"upload":   `<path d="M12 16V3m-5 5 5-5 5 5M4 15v6h16v-6"/>`,
		"rocket":   `<path d="M8 15C8 8 15 3 21 3c0 6-5 13-12 13l-1-1ZM8 10 3 12v5l5-2m6 1-2 5H7l2-5M5 19l-2 2"/><circle cx="16" cy="8" r="2"/>`,
		"file":     `<path d="M6 3h8l4 4v14H6Zm8 0v5h4m-9 5h6m-6 4h6"/>`,
		"output":   `<path d="M5 3h14v18H5Zm3 5h8m-8 4h8m-8 4h5"/>`,
		"restart":  `<path d="M20 7v5h-5m5-1a8 8 0 1 0-2 7"/>`,
		"stop":     `<rect x="6" y="6" width="12" height="12" rx="1"/>`,
		"settings": `<circle cx="12" cy="12" r="6"/><circle cx="12" cy="12" r="2"/><path d="M12 2v4m0 12v4M2 12h4m12 0h4M5 5l3 3m8 8 3 3M5 19l3-3m8-8 3-3"/>`,
		"right":    `<path d="m9 5 7 7-7 7"/>`,
		"down":     `<path d="m5 9 7 7 7-7"/>`,
		"back":     `<path d="m14 5-7 7 7 7"/>`,
		"history":  `<circle cx="12" cy="12" r="9"/><path d="M12 6v6l4 3"/>`,
		"minus":    `<path d="M5 12h14"/>`,
		"maximize": `<rect x="5" y="5" width="14" height="14"/>`,
		"close":    `<path d="m6 6 12 12M6 18 18 6"/>`,
		"plus":     `<path d="M12 5v14M5 12h14"/>`,
		"folder":   `<path d="M3 8V5h6l2 2h9v3M3 8v12h16l3-10H8l-2 3H3"/>`,
		"search":   `<circle cx="10.5" cy="10.5" r="6.5"/><path d="m16 16 4 4"/>`,
		"check":    `<path d="m5 12 4 4L19 6"/>`,
	}
	result := map[string]*ui.SVG{}
	for id, p := range paths {
		result[id] = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round">` + p + `</svg>`))
	}
	return result
}()

var rabbitLogos = func() map[string]*ui.SVG {
	result := make(map[string]*ui.SVG)
	for _, skin := range []string{"classic", "comic", "graphic", "pixel"} {
		result[skin] = ui.MustParseSVG([]byte(resources.RabbitLogoSVG(skin)))
	}
	return result
}()

func (a *app) toolIcon(c *ui.Context, id, label string, disabled bool) bool {
	e := ui.ButtonBase(c).Width(32).Height(32).Shrink(0).Padding(0).Center().Radius(a.theme.Radius).TextColor(a.theme.Text).Label(label).Tooltip(label).Disabled(disabled)
	if e.Hovered() {
		e.Background(a.theme.SurfaceHover)
	}
	e.Children(func() { ui.Icon(c, icons[id]).Width(16).Height(16).Shrink(0) })
	return e.Clicked()
}

func (a *app) icon(c *ui.Context, id, label string, disabled bool) bool {
	e := a.actionButton(c, id, label, disabled)
	return e.Clicked()
}

func (a *app) controlBorder() (float32, ui.Color) {
	if a.skin == "comic" || a.skin == "pixel" {
		return 2, a.theme.Text
	}
	return 1, a.theme.Border
}

func (a *app) buttonSurface(e ui.Element, primary bool) ui.Element {
	width, border := a.controlBorder()
	e.Border(width, border).Radius(a.theme.Radius).Background(a.theme.Surface).TextColor(a.theme.Text)
	if primary {
		e.Background(a.theme.Accent).TextColor(a.theme.AccentText)
	}
	if e.Hovered() {
		if primary {
			e.Background(a.theme.AccentHover)
		} else {
			e.Background(a.theme.SurfaceHover)
		}
	}
	if e.Pressed() {
		if primary {
			e.Background(a.theme.AccentPressed)
		} else {
			e.Background(a.theme.SurfacePressed)
		}
	}
	if a.skin == "comic" || a.skin == "pixel" {
		e.Bold()
		if !e.Pressed() && !e.IsDisabled() {
			e.Shadow(2, 2, 0, 0, border)
		}
	}
	return e
}

func (a *app) button(c *ui.Context, label string) ui.Element {
	e := a.buttonSurface(ui.ButtonBase(c).MinHeight(36).Padding(6, 12).Label(label), false)
	e.Children(func() { ui.Text(c, label).MaxLines(1) })
	return e
}

func (a *app) primaryButton(c *ui.Context, label string) ui.Element {
	e := a.buttonSurface(ui.ButtonBase(c).MinHeight(36).Padding(6, 12).Label(label), true)
	e.Children(func() { ui.Text(c, label).MaxLines(1) })
	return e
}

func (a *app) actionButton(c *ui.Context, id, label string, disabled bool) ui.Element {
	// Keep the 16-DIP glyph independent of the border and padding budget.
	e := ui.ButtonBase(c).Width(32).Height(32).Shrink(0).Padding(0).Center().Label(label).Tooltip(label).Disabled(disabled)
	a.buttonSurface(e, false)
	if a.skin == "comic" || a.skin == "pixel" {
		e.Background(a.theme.Selection)
		if a.skin == "comic" {
			switch id {
			case "output":
				e.Background(ui.Hex("#c4ebda")).TextColor(ui.Hex("#173c2a"))
			case "stop":
				e.Background(ui.Hex("#ffd0bf")).TextColor(ui.Hex("#722214"))
			}
		}
	}
	if e.Hovered() {
		e.Background(a.theme.SurfaceHover)
	}
	if e.Pressed() {
		e.Background(a.theme.SurfacePressed)
	}
	e.Children(func() { ui.Icon(c, icons[id]).Width(16).Height(16).Shrink(0) })
	return e
}

func (a *app) inputSurface(e ui.Element) ui.Element {
	width, border := a.controlBorder()
	if e.Focused() {
		border = a.theme.Focus
	}
	return e.MinHeight(36).Padding(8, 10).Border(width, border).Radius(a.theme.Radius).Background(a.theme.Surface).TextColor(a.theme.Text)
}

func (a *app) textInput(c *ui.Context, value *string) ui.Element {
	return a.inputSurface(ui.TextInputBase(c, value))
}

func (a *app) textArea(c *ui.Context, value *string) ui.Element {
	return a.inputSurface(ui.TextAreaBase(c, value)).MinHeight(80)
}

func (a *app) popupSurface(panel ui.Element) ui.Element {
	width, border := a.controlBorder()
	panel.Background(a.theme.Surface).Border(width, border).Radius(a.theme.Radius)
	if a.skin == "comic" || a.skin == "pixel" {
		panel.Shadow(2, 2, 0, 0, border)
	}
	return panel
}

func (a *app) checkbox(c *ui.Context, checked *bool, label string) ui.Element {
	row := ui.CheckboxBase(c, checked).MinHeight(32).AlignItems(ui.Center).Gap(8).Label(label)
	row.Children(func() {
		width, border := a.controlBorder()
		box := ui.Row(c).Width(18).Height(18).Shrink(0).Padding(0).Center().Border(width, border).Radius(a.theme.Radius).Background(a.theme.Surface)
		if *checked {
			box.Background(a.theme.Selection)
		}
		box.Children(func() {
			if *checked {
				ui.Icon(c, icons["check"]).Width(12).Height(12).Shrink(0).TextColor(a.theme.Text)
			}
		})
		ui.Text(c, label).MaxLines(1).MinWidth(0).Shrink(1)
	})
	return row
}
func (a *app) field(c *ui.Context, key string, value *string) bool {
	changed := false
	ui.Column(c.Key(key)).FillWidth().Gap(6).Children(func() {
		ui.Text(c, a.m(key)).FontSize(12).TextColor(a.theme.TextMuted)
		changed = a.textInput(c, value).Label(a.m(key)).FillWidth().Changed()
	})
	return changed
}

type option struct{ id, label string }

type choice struct {
	id, label         string
	disabled, divider bool
}

func (a *app) choiceButton(c *ui.Context, value *string, label string, options []choice, disabled bool) ui.Element {
	sel := ui.SelectBase(c, value)
	trigger := a.inputSurface(sel.Trigger.Disabled(disabled)).FillWidth().Label(label)
	trigger.Children(func() {
		ui.Row(c).FillWidth().MinWidth(0).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).Grow(1).MinWidth(0).MaxLines(1)
			ui.Icon(c, icons["down"]).Width(14).Height(14).Shrink(0).TextColor(a.theme.TextMuted)
		})
	})
	sel.Popup(func(panel ui.Element) {
		a.popupSurface(panel).Padding(4).MarginY(4)
		ui.Scroll(c).FillWidth().MaxHeight(300).Children(func() {
			for _, o := range options {
				if o.divider {
					ui.Row(c).FillWidth().Height(1).MarginY(4).Background(a.theme.Border)
				}
				// Status rows are not selectable, including from the keyboard.
				if o.disabled {
					ui.Text(c, o.label).Padding(8, 10).TextColor(a.theme.TextMuted)
					continue
				}
				item := sel.Item(o.id).FillWidth().MinHeight(32).Padding(6, 10).Gap(8).AlignItems(ui.Center).Radius(a.theme.Radius).Label(o.label)
				if item.Highlighted() {
					item.Background(a.theme.Selection)
				}
				item.Children(func() {
					ui.Text(c, o.label).Grow(1).MinWidth(0).MaxLines(1)
					ui.Row(c).Width(14).Height(14).Shrink(0).Center().Children(func() {
						if *value == o.id {
							ui.Icon(c, icons["check"]).Width(14).Height(14).Shrink(0)
						}
					})
				})
			}
		})
	})
	return trigger
}

func (a *app) selectField(c *ui.Context, key string, value *string, options []option) bool {
	changed := false
	ui.Column(c.Key(key)).FillWidth().Gap(6).Children(func() {
		ui.Text(c, a.m(key)).FontSize(12).TextColor(a.theme.TextMuted)
		label := *value
		items := make([]choice, 0, len(options))
		for _, o := range options {
			items = append(items, choice{id: o.id, label: o.label})
			if o.id == *value {
				label = o.label
			}
		}
		changed = a.choiceButton(c, value, label, items, false).Label(a.m(key)).Changed()
	})
	return changed
}
func (a *app) iconOptions() []option {
	var result []option
	for _, id := range []string{"play", "bolt", "terminal", "package", "upload", "rocket"} {
		result = append(result, option{id, a.m("icon" + strings.ToUpper(id[:1]) + id[1:])})
	}
	return result
}
