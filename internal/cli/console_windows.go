package cli

import (
	"os"

	"golang.org/x/sys/windows"
)

// GUI-subsystem binaries do not automatically attach to a terminal. Preserve
// inherited pipes/files, and repair only missing streams after attaching to the
// parent's console. Never allocate a new console for scripts or background runs.
func prepareConsole() {
	streams := []struct {
		id   uint32
		file **os.File
	}{
		{windows.STD_INPUT_HANDLE, &os.Stdin},
		{windows.STD_OUTPUT_HANDLE, &os.Stdout},
		{windows.STD_ERROR_HANDLE, &os.Stderr},
	}
	saved := make([]windows.Handle, len(streams))
	missing := false
	for i, stream := range streams {
		handle := windows.Handle((*stream.file).Fd())
		if _, err := windows.GetFileType(handle); handle != 0 && handle != windows.InvalidHandle && err == nil {
			saved[i] = handle
		} else {
			missing = true
		}
	}
	if !missing {
		return
	}
	attach := windows.NewLazySystemDLL("kernel32.dll").NewProc("AttachConsole")
	attach.Call(uintptr(^uint32(0))) // ATTACH_PARENT_PROCESS; failure is normal without a console.
	for i, stream := range streams {
		if saved[i] != 0 {
			// Restore redirections that AttachConsole may have replaced.
			_ = windows.SetStdHandle(stream.id, saved[i])
			continue
		}
		handle, err := windows.GetStdHandle(stream.id)
		if err != nil || handle == 0 || handle == windows.InvalidHandle {
			continue
		}
		if _, err := windows.GetFileType(handle); err == nil {
			*stream.file = os.NewFile(uintptr(handle), (*stream.file).Name())
		}
	}
}
