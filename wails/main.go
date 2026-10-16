package main

import (
	"log"
	"os"
	"runtime"
	"sync/atomic"

	assets "commandmanager.local/core"
	"commandmanager.local/core/desktop"
	"commandmanager.local/core/resources"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type quitChoice uint8

const (
	quitCancel quitChoice = iota
	quitKeep
	quitStop
)

func main() {
	if handled, code := desktop.CommandLine(); handled {
		os.Exit(code)
	}
	redirected, closeLease, err := desktop.Bootstrap("wails")
	if err != nil {
		log.Fatal(err)
	}
	defer closeLease()
	if redirected {
		return
	}
	host, err := desktop.New("wails", assets.Assets)
	if err != nil {
		log.Fatal(err)
	}
	defer host.Close()
	var approved atomic.Bool
	var beginQuit func()
	productName := resources.Messages("en")["productName"]
	app := application.New(application.Options{
		Name: productName + " · Wails",
		Icon: resources.RabbitPNG,
		Mac:  application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: false},
		ShouldQuit: func() bool {
			if approved.Load() {
				return true
			}
			beginQuit()
			return false
		},
	})
	if runtime.GOOS == "darwin" {
		menu := application.DefaultApplicationMenu()
		menu.RemoveMenuItem(menu.FindByRole(application.Hide))
		app.Menu.Set(menu)
	}
	window := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: productName, URL: host.URL, Width: 520, Height: 720, MinWidth: 360, MinHeight: 420,
		Frameless:        runtime.GOOS != "darwin",
		BackgroundColour: application.RGBA{Red: 248, Green: 247, Blue: 243, Alpha: 255},
		Windows:          application.WindowsWindow{Theme: application.Light},
		Mac:              application.MacWindow{TitleBar: application.MacTitleBarHiddenInset, Appearance: application.NSAppearanceNameAqua},
	})
	if runtime.GOOS == "windows" {
		window.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
			// Recover from a hidden startup hint supplied by an older launcher.
			if !window.IsVisible() && !window.IsMinimised() {
				window.Show()
			}
		})
	}
	host.SetWindowControls(func() desktop.WindowState {
		return desktop.WindowState{Framework: "wails", Platform: runtime.GOOS, Maximized: window.IsMaximised(), Fullscreen: window.IsFullscreen()}
	}, func(action string) {
		switch action {
		case "minimize":
			window.Minimise()
		case "toggleMaximize":
			window.ToggleMaximise()
		case "close":
			window.Close()
		}
	})
	var prompting atomic.Bool
	fallbackQuit := func() {
		if host.UpdatePending() {
			return
		}
		if !prompting.CompareAndSwap(false, true) {
			return
		}
		go func() {
			defer prompting.Store(false)
			activity, activityErr := host.Activity()
			if activityErr == nil && activity.Active == 0 && activity.Unknown == 0 {
				approved.Store(true)
				app.Quit()
				return
			}
			copy := desktop.ClosePromptFor(activity, activityErr != nil)
			if runtime.GOOS == "darwin" {
				window.Show()
			}
			choice := showQuitPrompt(app, window, copy)
			if choice == quitStop {
				if err := host.StopAll(); err != nil {
					showStopError(app, window, copy)
					return
				}
			}
			if choice == quitKeep || choice == quitStop {
				approved.Store(true)
				app.Quit()
			}
		}()
	}
	host.SetQuitControls(func() { window.Show() }, func() { approved.Store(true); app.Quit() }, fallbackQuit)
	beginQuit = func() {
		if host.UpdatePending() || prompting.Load() || host.RequestThemedQuit() {
			return
		}
		fallbackQuit()
	}
	window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if approved.Load() {
			return
		}
		event.Cancel()
		beginQuit()
	})
	if runtime.GOOS == "darwin" {
		app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) { window.Show() })
	}
	host.SetDirectoryPicker(func() (string, error) {
		return app.Dialog.OpenFile().CanChooseFiles(false).CanChooseDirectories(true).AttachToWindow(window).PromptForSingleSelection()
	})
	host.SetUpdateControls(func() (string, error) {
		return app.Dialog.OpenFile().CanChooseFiles(true).CanChooseDirectories(false).AttachToWindow(window).PromptForSingleSelection()
	}, func() { approved.Store(true); app.Quit() })
	host.SetProjectAvatarPicker(func() (string, error) {
		return app.Dialog.OpenFile().CanChooseFiles(true).CanChooseDirectories(false).AddFilter("PNG, JPEG, GIF, ICO, ICNS, SVG", "*.png;*.jpg;*.jpeg;*.gif;*.ico;*.icns;*.svg").AttachToWindow(window).PromptForSingleSelection()
	})
	if err = app.Run(); err != nil {
		log.Fatal(err)
	}
}
