package desktop

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const startupLabel = "local.commandmanager.yourun"

func startupFile() (string, error) {
	home, err := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", startupLabel+".plist"), err
}

type startupPlistValue struct {
	XMLName xml.Name
	Text    string              `xml:",chardata"`
	Values  []startupPlistValue `xml:",any"`
}

func readStartup(ctx context.Context) (StartupSettings, error) {
	state := StartupSettings{Supported: true}
	path, err := startupFile()
	if err != nil {
		return state, err
	}
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	defer file.Close()
	var plist startupPlistValue
	if err := xml.NewDecoder(io.LimitReader(file, 16<<10)).Decode(&plist); err != nil {
		return state, err
	}
	if plist.XMLName.Local != "plist" || len(plist.Values) != 1 || plist.Values[0].XMLName.Local != "dict" {
		return state, errors.New("invalid startup plist")
	}
	values := plist.Values[0].Values
	entries := map[string]startupPlistValue{}
	for i := 0; i+1 < len(values); i += 2 {
		entries[values[i].Text] = values[i+1]
	}
	if entries["Label"].Text != startupLabel {
		return state, errors.New("invalid startup label")
	}
	state.Enabled = entries["RunAtLoad"].XMLName.Local == "true" && entries["Disabled"].XMLName.Local != "true"
	if state.Enabled {
		blocked, err := startupBlocked(ctx)
		if err != nil {
			return state, err
		}
		if blocked {
			state.Enabled, state.Reason = false, "startupDisabledBySystem"
		}
	}
	return state, nil
}

func startupBlocked(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "/bin/launchctl", "print-disabled", "gui/"+strconv.Itoa(os.Getuid())).Output()
	if err != nil {
		return false, err
	}
	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), "=>")
		if ok && strings.TrimSpace(key) == `"`+startupLabel+`"` {
			return strings.TrimSpace(value) == "true", nil
		}
	}
	return false, nil
}

func writeStartup(ctx context.Context, enabled bool, executable string) error {
	path, err := startupFile()
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !enabled {
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	blocked, err := startupBlocked(ctx)
	if err != nil {
		return err
	}
	if blocked {
		return errors.New("startupDisabledBySystem")
	}
	var escaped bytes.Buffer
	if err := xml.EscapeText(&escaped, []byte(executable)); err != nil {
		return err
	}
	content := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>Label</key><string>` + startupLabel + `</string>
<key>AssociatedBundleIdentifiers</key><array><string>local.commandmanager.yourun</string><string>local.commandmanager.mygo</string><string>local.commandmanager.mygo-native</string><string>local.commandmanager.wails</string></array>
<key>ProgramArguments</key><array><string>` + escaped.String() + `</string></array>
<key>RunAtLoad</key><true/>
<key>LimitLoadToSessionType</key><string>Aqua</string>
</dict></plist>
`
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".yourun-startup-*.plist")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.WriteString(content)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}
	// Install for the next login without loading a second GUI in this session.
	return os.Rename(file.Name(), path)
}
