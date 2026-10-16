//go:build !windows

package main

import (
	"commandmanager.local/core/desktop"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func showQuitPrompt(app *application.App, window application.Window, copy desktop.ClosePrompt) quitChoice {
	choice := quitCancel
	dialog := app.Dialog.Warning().SetTitle(copy.Title).SetMessage(copy.Message + "\n\n" + copy.Detail).AttachToWindow(window)
	dialog.AddButton(copy.Keep).SetAsDefault().OnClick(func() { choice = quitKeep })
	if copy.CanStop {
		dialog.AddButton(copy.Stop).OnClick(func() { choice = quitStop })
	}
	dialog.AddButton(copy.Cancel).SetAsCancel()
	dialog.Show()
	return choice
}

func showStopError(app *application.App, window application.Window, copy desktop.ClosePrompt) {
	dialog := app.Dialog.Error().SetTitle(copy.StopError).SetMessage(copy.StopErrorBody).AttachToWindow(window)
	dialog.AddButton(copy.Acknowledge).SetAsDefault()
	dialog.Show()
}
