package desktop

import (
	"os"
	"path/filepath"
	"runtime"

	"commandmanager.local/core/internal/engine"
)

func developmentSource(framework string) string {
	working, _ := os.Getwd()
	candidates := []string{working, filepath.Dir(working)}
	// go run and debugger builds retain source paths unless explicitly trimmed.
	if _, filename, _, ok := runtime.Caller(0); ok && filepath.IsAbs(filename) {
		candidates = append(candidates, filepath.Dir(filepath.Dir(filename)))
	}
	for _, candidate := range candidates {
		if source, err := validateLocalSource(candidate, framework); err == nil {
			return source
		}
	}
	return ""
}

// Every desktop entrypoint checks this before initializing its UI.
func ExecutionMode() (bool, error) { return engine.ExecutionMode() }
