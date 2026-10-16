//go:build !windows

package desktop

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"commandmanager.local/core/internal/discovery"
)

func loadProjectOpenerIcons(ctx context.Context, openers []ProjectOpener) {
	if runtime.GOOS != "darwin" {
		return
	}
	for i := range openers {
		if ctx.Err() != nil {
			return
		}
		bundle := darwinOpenerBundle(openers[i].path)
		if bundle == "" || strings.Contains(strings.ToLower(bundle), "trash") {
			continue
		}
		iconPath := darwinBundleIconPath(bundle)
		if iconPath == "" || strings.Contains(strings.ToLower(iconPath), "trash") {
			continue
		}
		if encoded := encodeDarwinBundleIcon(iconPath); encoded != "" {
			openers[i].Icon = encoded
		}
	}
}

func darwinOpenerBundle(path string) string {
	for _, candidate := range []string{path, func() string { p, _ := filepath.EvalSymlinks(path); return p }()} {
		for current := filepath.Clean(candidate); current != "/" && current != "."; current = filepath.Dir(current) {
			if strings.HasSuffix(strings.ToLower(current), ".app") {
				if info, err := os.Stat(current); err == nil && info.IsDir() {
					return current
				}
			}
		}
	}
	return ""
}

func darwinBundleIconPath(bundle string) string {
	resources := filepath.Join(bundle, "Contents", "Resources")
	if plist, err := os.Open(filepath.Join(bundle, "Contents", "Info.plist")); err == nil {
		data, readErr := io.ReadAll(io.LimitReader(plist, 64<<10))
		plist.Close()
		if readErr == nil {
			for _, name := range darwinPlistIconNames(data) {
				name = filepath.Base(name)
				candidates := []string{filepath.Join(resources, name)}
				if filepath.Ext(name) == "" {
					candidates = append(candidates, filepath.Join(resources, name+".icns"))
				}
				for _, c := range candidates {
					if info, statErr := os.Stat(c); statErr == nil && info.Mode().IsRegular() {
						return c
					}
				}
			}
		}
	}
	entries, err := os.ReadDir(resources)
	if err != nil {
		return ""
	}
	for _, preferred := range []string{"AppIcon.icns", "icon.icns", "app.icns"} {
		for _, entry := range entries {
			if entry.Type().IsRegular() && strings.EqualFold(entry.Name(), preferred) {
				return filepath.Join(resources, entry.Name())
			}
		}
	}
	for _, entry := range entries {
		if entry.Type().IsRegular() && strings.EqualFold(filepath.Ext(entry.Name()), ".icns") {
			return filepath.Join(resources, entry.Name())
		}
	}
	return ""
}

func darwinPlistIconNames(data []byte) []string {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var names []string
	lastKey := ""
	for {
		tok, err := decoder.Token()
		if err != nil {
			break
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "key":
			var key string
			if decoder.DecodeElement(&key, &start) == nil {
				lastKey = strings.TrimSpace(key)
			}
		case "string":
			var val string
			if decoder.DecodeElement(&val, &start) == nil {
				if (lastKey == "CFBundleIconFile" || lastKey == "CFBundleIconName") && strings.TrimSpace(val) != "" {
					names = append(names, strings.TrimSpace(val))
				}
				lastKey = ""
			}
		}
	}
	return names
}

func encodeDarwinBundleIcon(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, (8<<20)+1))
	if err != nil || len(data) == 0 || len(data) > 8<<20 {
		return ""
	}
	if len(data) <= 2<<20 {
		if encoded := discovery.EncodeLogo(data, filepath.Ext(path)); encoded != "" {
			return encoded
		}
	}
	// Large .icns bundles (> 2 MiB due to 1024x1024@2x chunks) still carry compact 128-512px PNG chunks.
	if len(data) >= 16 && string(data[:4]) == "icns" {
		limit := int(binary.BigEndian.Uint32(data[4:8]))
		if limit < 16 || limit > len(data) {
			limit = len(data)
		}
		type chunk struct {
			data []byte
			area int
		}
		var chunks []chunk
		for offset := 8; offset+8 <= limit; {
			size := int(binary.BigEndian.Uint32(data[offset+4 : offset+8]))
			if size < 8 || offset+size > limit {
				break
			}
			payload := data[offset+8 : offset+size]
			offset += size
			if len(payload) <= 2<<20 && bytes.HasPrefix(payload, []byte("\x89PNG\r\n\x1a\n")) {
				if cfg, cfgErr := png.DecodeConfig(bytes.NewReader(payload)); cfgErr == nil && cfg.Width >= 32 && cfg.Width <= 512 && cfg.Height >= 32 && cfg.Height <= 512 {
					chunks = append(chunks, chunk{data: payload, area: cfg.Width * cfg.Height})
				}
			}
		}
		sort.SliceStable(chunks, func(i, j int) bool { return chunks[i].area > chunks[j].area })
		for _, c := range chunks {
			if encoded := discovery.EncodeLogo(c.data, ".png"); encoded != "" {
				return encoded
			}
		}
	}
	return ""
}
