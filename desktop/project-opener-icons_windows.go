package desktop

import (
	"context"
	"path/filepath"
	"strings"
	"time"
)

// Reuse the native icon reader; metadata lookup must not launch a shell.
// Icons remain optional and share the existing application discovery cache.
func loadProjectOpenerIcons(ctx context.Context, openers []ProjectOpener) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	for i := range openers {
		if ctx.Err() != nil {
			return
		}
		path := openers[i].path
		if !strings.EqualFold(filepath.Ext(path), ".exe") || strings.Contains(strings.ToLower(path), "trash") {
			continue
		}
		if icon := extractExecutableIconDataURL(path); icon != "" {
			openers[i].Icon = icon
		}
	}
}
