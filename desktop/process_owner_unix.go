//go:build !windows

package desktop

import (
	"os"
	"syscall"
	"time"
)

func ownsCurrentProcess(rootPID int, _ time.Time) (bool, error) {
	group, err := syscall.Getpgid(os.Getpid())
	return rootPID > 0 && group == rootPID, err
}
