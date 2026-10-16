package desktop

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// Icons are optional and cached with the detected applications. One hidden
// helper reads installed executables; a failed icon never removes an opener.
func loadProjectOpenerIcons(ctx context.Context, openers []ProjectOpener) {
	type iconSource struct {
		ID   string `json:"id"`
		Path string `json:"path"`
	}
	sources := make([]iconSource, 0, len(openers))
	for _, opener := range openers {
		if strings.EqualFold(filepath.Ext(opener.path), ".exe") && !strings.Contains(strings.ToLower(opener.path), "trash") {
			sources = append(sources, iconSource{ID: opener.ID, Path: opener.path})
		}
	}
	if len(sources) == 0 {
		return
	}
	input, err := json.Marshal(sources)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	// Paths travel as JSON on stdin, never as PowerShell source or arguments.
	const script = `
$ErrorActionPreference = 'Stop'
[Console]::InputEncoding = [System.Text.UTF8Encoding]::new($false)
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
Add-Type -AssemblyName System.Drawing
$sources = @([Console]::In.ReadToEnd() | ConvertFrom-Json)
$icons = @{}
foreach ($source in $sources) {
  $icon = $null; $bitmap = $null; $stream = $null
  try {
    $icon = [System.Drawing.Icon]::ExtractAssociatedIcon($source.path)
    if ($null -eq $icon) { continue }
    $bitmap = $icon.ToBitmap()
    $stream = [System.IO.MemoryStream]::new()
    $bitmap.Save($stream, [System.Drawing.Imaging.ImageFormat]::Png)
    $icons[$source.id] = [Convert]::ToBase64String($stream.ToArray())
  } catch {} finally {
    if ($null -ne $stream) { $stream.Dispose() }
    if ($null -ne $bitmap) { $bitmap.Dispose() }
    if ($null -ne $icon) { $icon.Dispose() }
  }
}
[Console]::Out.Write(($icons | ConvertTo-Json -Compress))
`
	powershell := filepath.Join(os.Getenv("SystemRoot"), "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	cmd := exec.CommandContext(ctx, powershell, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.Stdin = bytes.NewReader(input)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
	output, err := cmd.Output()
	if err != nil {
		return
	}
	var icons map[string]string
	if json.Unmarshal(output, &icons) != nil {
		return
	}
	for index := range openers {
		encoded := icons[openers[index].ID]
		if len(encoded) == 0 || len(encoded) > 128*1024 {
			continue
		}
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			continue
		}
		config, err := png.DecodeConfig(bytes.NewReader(data))
		if err == nil && config.Width > 0 && config.Width <= 256 && config.Height > 0 && config.Height <= 256 {
			openers[index].Icon = "data:image/png;base64," + encoded
		}
	}
}
