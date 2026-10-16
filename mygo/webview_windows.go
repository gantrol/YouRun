package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/egoist/mygo"
)

// MyGo 0.4.8 derives its WebView2 directory from the backend name, ignoring
// App.SetPath. A shared directory also shares the browser process: stopping a
// managed YouRun can otherwise destroy an unrelated YouRun window's renderer.
func prepareWebView(productName string) (func(), error) {
	base := os.Getenv("LOCALAPPDATA")
	if base == "" {
		base = os.TempDir()
	}
	base, err := filepath.Abs(base)
	if err != nil {
		return nil, err
	}
	directory, err := os.MkdirTemp(base, "YouRun-WebView-")
	if err != nil {
		return nil, err
	}
	// Run captures this name for the Windows backend before WhenReady runs.
	// Restore the public name before any window or application paths are used;
	// the window title, dialogs and persisted application data keep their names.
	mygo.App.SetName(filepath.Base(directory))
	mygo.App.WhenReady(func() { mygo.App.SetName(productName) })
	return func() {
		// Only this instance ever uses this freshly created cache. Controllers
		// have closed when Run returns, but WebView2 may briefly retain files.
		for deadline := time.Now().Add(3 * time.Second); ; {
			err := os.RemoveAll(directory)
			if err == nil {
				return
			}
			if time.Now().After(deadline) {
				log.Printf("Remove WebView2 cache: %v", err)
				return
			}
			time.Sleep(100 * time.Millisecond)
		}
	}, nil
}
