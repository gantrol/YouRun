//go:build windows

package cli

import (
	"errors"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"commandmanager.local/core/internal/updater"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	user32SendMessageTimeoutW = windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")
)

const (
	hwndBroadcast   = 0xffff
	wmSettingChange = 0x001A
	smtoAbortIfHung = 0x0002
)

func checkUserPath(binDir string) (configured bool, pathLocation string) {
	if updater.Distribution == "msix" {
		return true, "Windows AppExecutionAlias (yourun.exe)"
	}
	pathLocation = `HKCU\Environment\Path`
	if strings.TrimSpace(binDir) == "" {
		return false, pathLocation
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE)
	if err != nil {
		return false, pathLocation
	}
	defer key.Close()
	val, _, err := key.GetStringValue("Path")
	if err != nil {
		return false, pathLocation
	}
	return pathListContains(val, binDir), pathLocation
}

func updateUserPath(enabled bool, binDir string, state *integrationReceipt) (configured bool, pathLocation string, err error) {
	if updater.Distribution == "msix" {
		return true, "Windows AppExecutionAlias (yourun.exe)", nil
	}
	pathLocation = `HKCU\Environment\Path`
	binDir = strings.TrimSpace(binDir)
	if binDir == "" {
		return false, pathLocation, nil
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Environment`, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return false, pathLocation, err
	}
	defer key.Close()

	val, valType, err := key.GetStringValue("Path")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return false, pathLocation, err
	}

	if enabled {
		if pathListContains(val, binDir) {
			return true, pathLocation, nil
		}
		parts := splitPathList(val)
		parts = append(parts, binDir)
		newVal := strings.Join(parts, ";")
		if err := setRegistryPathValue(key, newVal, valType); err != nil {
			return false, pathLocation, err
		}
		if state != nil {
			state.AddedPathDir = binDir
		}
		broadcastEnvironmentChange()
		return true, pathLocation, nil
	}

	targetToRemove := binDir
	if state != nil && strings.TrimSpace(state.AddedPathDir) != "" {
		targetToRemove = state.AddedPathDir
	}
	if state == nil || strings.TrimSpace(state.AddedPathDir) == "" {
		return pathListContains(val, binDir), pathLocation, nil
	}
	if !pathListContains(val, targetToRemove) {
		state.AddedPathDir = ""
		return false, pathLocation, nil
	}
	var kept []string
	for _, part := range splitPathList(val) {
		if !samePathDir(part, targetToRemove) {
			kept = append(kept, part)
		}
	}
	newVal := strings.Join(kept, ";")
	if err := setRegistryPathValue(key, newVal, valType); err != nil {
		return true, pathLocation, err
	}
	state.AddedPathDir = ""
	broadcastEnvironmentChange()
	return false, pathLocation, nil
}

func setRegistryPathValue(key registry.Key, value string, valType uint32) error {
	if valType == registry.SZ {
		return key.SetStringValue("Path", value)
	}
	return key.SetExpandStringValue("Path", value)
}

func splitPathList(raw string) []string {
	var out []string
	for _, p := range strings.Split(raw, ";") {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func pathListContains(raw, dir string) bool {
	for _, p := range splitPathList(raw) {
		if samePathDir(p, dir) {
			return true
		}
	}
	return false
}

func samePathDir(a, b string) bool {
	ca := strings.TrimRight(filepath.Clean(strings.Trim(a, `"`)), `\/`)
	cb := strings.TrimRight(filepath.Clean(strings.Trim(b, `"`)), `\/`)
	return strings.EqualFold(ca, cb)
}

func broadcastEnvironmentChange() {
	envPtr, err := syscall.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	var result uintptr
	_, _, _ = user32SendMessageTimeoutW.Call(
		uintptr(hwndBroadcast),
		uintptr(wmSettingChange),
		0,
		uintptr(unsafe.Pointer(envPtr)),
		uintptr(smtoAbortIfHung),
		100,
		uintptr(unsafe.Pointer(&result)),
	)
}
