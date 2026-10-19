package desktop

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"unsafe"
)

func canElevateProcessControl() bool { return true }
func launchElevatedControl(endpoint, token string) (func(), error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, errors.New("processControlFailed")
	}
	file, _ := windows.UTF16PtrFromString(executable)
	verb, _ := windows.UTF16PtrFromString("runas")
	params, _ := windows.UTF16PtrFromString(windows.ComposeCommandLine([]string{"_process-control", endpoint, token}))
	type shellExecuteInfo struct {
		Size       uint32
		Mask       uint32
		Window     uintptr
		Verb       *uint16
		File       *uint16
		Parameters *uint16
		Directory  *uint16
		Show       int32
		Instance   uintptr
		IDList     uintptr
		Class      *uint16
		ClassKey   uintptr
		HotKey     uint32
		Icon       uintptr
		Process    windows.Handle
	}
	info := shellExecuteInfo{Mask: 0x40 | 0x100 | 0x400, Verb: verb, File: file, Parameters: params, Show: windows.SW_HIDE}
	info.Size = uint32(unsafe.Sizeof(info))
	ok, _, callErr := windows.NewLazySystemDLL("shell32.dll").NewProc("ShellExecuteExW").Call(uintptr(unsafe.Pointer(&info)))
	if ok == 0 {
		if errors.Is(callErr, windows.ERROR_CANCELLED) {
			return nil, errors.New("processElevationCancelled")
		}
		return nil, errors.New("processAccessDenied")
	}
	return func() {
		if info.Process != 0 {
			windows.CloseHandle(info.Process)
		}
	}, nil
}
