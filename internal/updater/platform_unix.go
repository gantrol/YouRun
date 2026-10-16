//go:build !windows

package updater

import (
	"os"
	"os/exec"
	"syscall"
)

func Detached(cmd *exec.Cmd)        { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
func replace(from, to string) error { return os.Rename(from, to) }
