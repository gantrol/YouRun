package main

import (
	"log"
	"os"
	"runtime"

	"commandmanager.local/core/desktop"
	"commandmanager.local/core/resources"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--components" {
		runComponentCatalog()
		return
	}
	if handled, code := desktop.CommandLine(); handled {
		os.Exit(code)
	}
	redirected, lease, err := desktop.Bootstrap("mygo-native")
	if err != nil {
		log.Fatal(err)
	}
	defer lease()
	if redirected {
		return
	}
	host, err := desktop.New("mygo-native", nil)
	if err != nil {
		log.Fatal(err)
	}
	defer host.Close()
	a := newApp(host)
	defer a.cancel()
	mygo.App.SetName(a.m("productName"))
	if runtime.GOOS == "darwin" {
		mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
			{Role: mygo.RoleAppMenu, Submenu: []*mygo.MenuItem{
				{Role: mygo.RoleAbout}, mygo.Separator(), {Role: mygo.RoleServices},
				mygo.Separator(), {Role: mygo.RoleQuit},
			}},
			{Role: mygo.RoleEditMenu}, {Role: mygo.RoleWindowMenu},
		}))
	}
	mygo.App.WhenReady(func() {
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title: a.m("productName") + " · MyGo Native", Width: 528, Height: 720, MinWidth: 360, MinHeight: 420,
			Frameless: runtime.GOOS != "darwin", TitleBarStyle: mygo.TitleBarHiddenInset, TitleBarHeight: 44,
			TrafficLightPosition: &mygo.Point{X: 12, Y: 15}, BackgroundColor: "#f8f7f3", Content: ui.View(a.view),
		})
		if err := a.win.SetIcon(resources.RabbitPNG); err != nil {
			log.Printf("Set window icon: %v", err)
		}
		// Older launchers can suppress the first ShowWindow call with SW_HIDE.
		if runtime.GOOS == "windows" && !a.win.IsVisible() {
			a.win.Show()
		}
		refreshOnFocus := func() {
			a.refreshProjectDetailsOnFocus()
			if a.current().Kind == "settings" {
				a.loadStartup()
			}
		}
		a.win.OnFocus(refreshOnFocus)
		a.win.OnShow(refreshOnFocus)
		host.SetDirectoryPicker(func() (string, error) {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Directory: true, Parent: a.win})
			if err != nil || len(paths) == 0 {
				return "", err
			}
			return paths[0], nil
		})
		host.SetUpdateControls(func() (string, error) {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win})
			if err != nil || len(paths) == 0 {
				return "", err
			}
			return paths[0], nil
		}, func() { a.approved.Store(true); mygo.App.Quit() })
		host.SetProjectAvatarPicker(func() (string, error) {
			paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{Parent: a.win, Filters: []mygo.FileFilter{{Name: "PNG, JPEG, GIF, ICO, ICNS, SVG", Extensions: []string{"png", "jpg", "jpeg", "gif", "ico", "icns", "svg"}}}})
			if err != nil || len(paths) == 0 {
				return "", err
			}
			return paths[0], nil
		})
		mygo.App.OnBeforeQuit(func(e *mygo.QuitEvent) {
			if a.approved.Load() {
				return
			}
			e.PreventDefault()
			a.requestQuit()
		})
		a.win.OnClose(func(e *mygo.CloseEvent) {
			if a.approved.Load() {
				return
			}
			e.PreventDefault()
			a.requestQuit()
		})
		if runtime.GOOS == "darwin" {
			mygo.App.OnActivate(func(visible bool) {
				if !visible {
					a.win.Show()
				}
			})
		}
		a.bootstrap()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *app) requestQuit() {
	if a.busy || a.quitOpen || a.host.UpdatePending() {
		return
	}
	if a.dirty() {
		a.pendingPage = nil
		a.leaving = true
		a.closeAfterDiscard = true
		a.win.Show()
		a.win.Invalidate()
		return
	}
	if !a.prompting.CompareAndSwap(false, true) {
		return
	}
	a.win.Invalidate()
	go func() {
		defer a.prompting.Store(false)
		activity, err := a.host.Activity()
		if err == nil && activity.Active == 0 && activity.Unknown == 0 {
			a.approved.Store(true)
			mygo.App.Quit()
			return
		}
		copy := desktop.ClosePromptFor(activity, err != nil)
		a.win.Show()
		a.win.Update(func() {
			if a.ctx.Err() != nil {
				return
			}
			a.quitPrompt = copy
			a.quitOpen, a.quitFocus, a.quitError = true, true, false
		})
	}()
}
