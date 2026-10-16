package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *app) confirmQuit(stop bool) {
	if a.quitBusy || a.host.UpdatePending() {
		return
	}
	a.quitBusy, a.quitError = true, false
	go func() {
		if stop {
			if err := a.host.StopAll(); err != nil {
				a.win.Update(func() { a.quitBusy, a.quitError = false, true })
				return
			}
		}
		a.approved.Store(true)
		mygo.App.Quit()
	}()
}

func (a *app) quitDialog(c *ui.Context) {
	if !a.quitOpen {
		return
	}
	width, height := c.Size()
	open := a.quitOpen
	copy := a.quitPrompt
	ui.DialogBase(c.Key("quit"), &open, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.2))
		a.popupSurface(panel).Width(420).MaxWidth(width - 32).Padding(20).Role(ui.RoleAlertDialog).Label(copy.Title).Description(copy.Message)
		ui.Scroll(c).FillWidth().MaxHeight(height - 80).Children(func() {
			ui.Column(c).FillWidth().Gap(16).Children(func() {
				title := ui.Text(c, copy.Title).Bold().FontSize(16)
				if a.skin == "pixel" {
					title.Font("monospace")
				}
				ui.Text(c, copy.Message).TextColor(a.theme.TextMuted)
				if a.quitError {
					ui.Text(c, copy.StopErrorBody).TextColor(a.theme.Danger).Role(ui.RoleStatus)
				}
				ui.Row(c).FillWidth().Wrap().Justify(ui.End).Gap(8).Disabled(a.quitBusy).Children(func() {
					if a.button(c, copy.Cancel).Clicked() {
						open = false
					}
					if copy.CanStop {
						stop := a.button(c, copy.Stop).TextColor(a.theme.Danger)
						if stop.Clicked() {
							a.confirmQuit(true)
						}
					}
					label := copy.Keep
					if a.quitBusy {
						label = a.m("quitBusy")
					}
					keep := a.primaryButton(c, label)
					if a.quitFocus {
						keep.Focus()
						a.quitFocus = false
					}
					if keep.Clicked() {
						a.confirmQuit(false)
					}
				})
			})
		})
	})
	if !a.quitBusy {
		a.quitOpen = open
	}
}
