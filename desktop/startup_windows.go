package desktop

import (
	"context"
	"encoding/binary"
	"errors"
	"strings"
	"unicode/utf16"

	"commandmanager.local/core/internal/updater"
	"golang.org/x/sys/windows/registry"
)

const startupRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`
const startupApprovalKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
const startupName = "YouRun"

func readStartup(ctx context.Context) (StartupSettings, error) {
	state := StartupSettings{Supported: updater.Distribution != "msix"}
	if !state.Supported {
		state.Reason = "startupUnsupported"
		return state, nil
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, startupRunKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	defer key.Close()
	command, _, err := key.GetStringValue(startupName)
	if errors.Is(err, registry.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	state.Enabled = strings.TrimSpace(command) != ""
	if state.Enabled {
		blocked, err := startupBlocked()
		if err != nil {
			return state, err
		}
		if blocked {
			state.Enabled, state.Reason = false, "startupDisabledBySystem"
		}
	}
	return state, ctx.Err()
}

func startupBlocked() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, startupApprovalKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer key.Close()
	value, _, err := key.GetBinaryValue(startupName)
	if errors.Is(err, registry.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	// Windows uses 0, 2 or 6 for approved Run items. Unknown
	// records are treated as blocked; never rewrite the OS approval record.
	if len(value) < 4 {
		return true, nil
	}
	state := binary.LittleEndian.Uint32(value)
	return state != 0 && state != 2 && state != 6, nil
}

func writeStartup(ctx context.Context, enabled bool, executable string) error {
	if updater.Distribution == "msix" {
		return errors.New("startupUnsupported")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !enabled {
		key, err := registry.OpenKey(registry.CURRENT_USER, startupRunKey, registry.SET_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		defer key.Close()
		err = key.DeleteValue(startupName)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	blocked, err := startupBlocked()
	if err != nil {
		return err
	}
	if blocked {
		return errors.New("startupDisabledBySystem")
	}
	// Quote even paths without spaces; register only the GUI, without inherited
	// arguments, environment, command execution or a shell wrapper.
	command := `"` + executable + `"`
	if strings.ContainsAny(executable, "\"\r\n\x00") || len(utf16.Encode([]rune(command))) > 260 {
		return errors.New("invalid startup executable")
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, startupRunKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(startupName, command)
}
