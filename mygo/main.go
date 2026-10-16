package main

import (
	"log"
	"os"
	"runtime"
	"sync/atomic"

	assets "commandmanager.local/core"
	"commandmanager.local/core/desktop"
	"commandmanager.local/core/resources"
	"github.com/egoist/mygo"
)

func main() {
	if handled, code := desktop.CommandLine(); handled {
		os.Exit(code)
	}
	redirected, closeLease, err := desktop.Bootstrap("mygo")
	if err != nil {
		log.Fatal(err)
	}
	defer closeLease()
	if redirected {
		return
	}
	host, err := desktop.New("mygo", assets.Assets)
	if err != nil {
		log.Fatal(err)
	}
	defer host.Close()
	productName := resources.Messages("en")["productName"]
	mygo.App.SetName(productName)
	closeWebView, err := prepareWebView(productName)
	if err != nil {
		log.Fatal(err)
	}
	defer closeWebView()
	if runtime.GOOS == "darwin" {
		mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
			{Role: mygo.RoleAppMenu, Submenu: []*mygo.MenuItem{
				{Role: mygo.RoleAbout}, mygo.Separator(), {Role: mygo.RoleServices},
				mygo.Separator(), {Role: mygo.RoleQuit},
			}},
			{Role: mygo.RoleFileMenu}, {Role: mygo.RoleEditMenu},
			{Role: mygo.RoleViewMenu}, {Role: mygo.RoleWindowMenu},
		}))
	}
	mygo.App.WhenReady(func() {
		window := mygo.NewWindow(mygo.WindowOptions{
			Title: productName, URL: host.URL, Width: 520, Height: 720, MinWidth: 360, MinHeight: 420,
			Frameless: runtime.GOOS != "darwin", TitleBarStyle: mygo.TitleBarHiddenInset,
			TitleBarHeight: 44, TrafficLightPosition: &mygo.Point{X: 12, Y: 15}, BackgroundColor: "#f8f7f3",
		})
		if err := window.SetIcon(resources.RabbitPNG); err != nil {
			log.Printf("Set window icon: %v", err)
		}
		// Older launchers can suppress the first ShowWindow call with SW_HIDE.
		if runtime.GOOS == "windows" && !window.IsVisible() {
			window.Show()
		}
		host.SetWindowControls(func() desktop.WindowState {
			return desktop.WindowState{Framework: "mygo", Platform: runtime.GOOS, Maximized: window.IsMaximized(), Fullscreen: window.IsFullScreen()}
		}, func(action string) {
			switch action {
			case "minimize":
				window.Minimize()
			case "toggleMaximize":
				window.ToggleMaximize()
			case "close":
				window.Close()
			}
		})
		var approved atomic.Bool
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
					mygo.App.Quit()
					return
				}
				copy := desktop.ClosePromptFor(activity, activityErr != nil)
				if runtime.GOOS == "darwin" {
					window.Show()
				}
				buttons := []string{copy.Keep}
				stopIndex := -1
				if copy.CanStop {
					stopIndex = len(buttons)
					buttons = append(buttons, copy.Stop)
				}
				cancelIndex := len(buttons)
				buttons = append(buttons, copy.Cancel)
				result, err := mygo.Dialog.Message(mygo.MessageOptions{Parent: window, Type: mygo.MessageWarning, Title: copy.Title, Message: copy.Message, Detail: copy.Detail, Buttons: buttons, DefaultButton: 0, CancelButton: cancelIndex})
				if err != nil || result.Button == cancelIndex {
					return
				}
				if result.Button == stopIndex {
					if err := host.StopAll(); err != nil {
						_, _ = mygo.Dialog.Message(mygo.MessageOptions{Parent: window, Type: mygo.MessageError, Title: copy.StopError, Message: copy.StopErrorBody, Buttons: []string{copy.Acknowledge}})
						return
					}
				}
				approved.Store(true)
				mygo.App.Quit()
			}()
		}
		host.SetQuitControls(window.Show, func() { approved.Store(true); mygo.App.Quit() }, fallbackQuit)
		beginQuit := func() {
			if host.UpdatePending() || prompting.Load() || host.RequestThemedQuit() {
				return
			}
			fallbackQuit()
		}
		mygo.App.OnBeforeQuit(func(event *mygo.QuitEvent) {
			if approved.Load() {
				return
			}
			event.PreventDefault()
			beginQuit()
		})
		window.OnClose(func(event *mygo.CloseEvent) {
			if approved.Load() {
				return
			}
			event.PreventDefault()
			beginQuit()
		})
		if runtime.GOOS == "darwin" {
			mygo.App.OnActivate(func(visible bool) {
				if !visible {
					window.Show()
				}
			})
		}
		host.SetDirectoryPicker(func() (string, error) {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Directory: true, Parent: window})
			if err != nil || len(paths) == 0 {
				return "", err
			}
			return paths[0], nil
		})
		host.SetUpdateControls(func() (string, error) {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window})
			if err != nil || len(paths) == 0 {
				return "", err
			}
			return paths[0], nil
		}, func() { approved.Store(true); mygo.App.Quit() })
		host.SetProjectAvatarPicker(func() (string, error) {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: window, Filters: []mygo.FileFilter{{Name: "PNG, JPEG, GIF, ICO, ICNS, SVG", Extensions: []string{"png", "jpg", "jpeg", "gif", "ico", "icns", "svg"}}}})
			if err != nil || len(paths) == 0 {
				return "", err
			}
			return paths[0], nil
		})
	})
	if err = mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
