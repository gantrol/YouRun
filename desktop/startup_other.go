//go:build !windows && !darwin

package desktop

import (
	"context"
	"errors"
)

func readStartup(context.Context) (StartupSettings, error) {
	return StartupSettings{Reason: "startupUnsupported"}, nil
}

func writeStartup(context.Context, bool, string) error {
	return errors.New("startupUnsupported")
}
