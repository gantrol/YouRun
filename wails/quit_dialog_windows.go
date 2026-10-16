//go:build windows

package main

import (
	"commandmanager.local/core/desktop"
	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

const (
	messageOK     = 1
	messageCancel = 2
	messageYes    = 6
	messageNo     = 7
)

func showQuitPrompt(_ *application.App, window application.Window, copy desktop.ClosePrompt) quitChoice {
	message := copy.Message + "\r\n\r\n" + copy.Detail + "\r\n\r\n" + copy.StandardKeys
	style := uint32(windows.MB_ICONWARNING | windows.MB_SETFOREGROUND | windows.MB_DEFBUTTON1)
	if copy.CanStop {
		style |= windows.MB_YESNOCANCEL
	} else {
		style |= windows.MB_OKCANCEL
	}
	result, err := windows.MessageBox(windowHandle(window), windows.StringToUTF16Ptr(message), windows.StringToUTF16Ptr(copy.Title), style)
	if err != nil {
		return quitCancel
	}
	switch result {
	case messageOK, messageYes:
		return quitKeep
	case messageNo:
		return quitStop
	default:
		return quitCancel
	}
}

func showStopError(_ *application.App, window application.Window, copy desktop.ClosePrompt) {
	_, _ = windows.MessageBox(windowHandle(window), windows.StringToUTF16Ptr(copy.StopErrorBody), windows.StringToUTF16Ptr(copy.StopError), windows.MB_OK|windows.MB_ICONERROR|windows.MB_SETFOREGROUND)
}

func windowHandle(window application.Window) windows.HWND {
	if window == nil || window.NativeWindow() == nil {
		return 0
	}
	return windows.HWND(uintptr(window.NativeWindow()))
}
