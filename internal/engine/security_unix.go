//go:build !windows

package engine

import (
	"errors"
	"os"
)

func secureDirectory(directory string) error {
	if os.Geteuid() == 0 {
		return errors.New("YouRun must not run as root")
	}
	if err := os.MkdirAll(directory, 0700); err != nil {
		return err
	}
	return os.Chmod(directory, 0700)
}
