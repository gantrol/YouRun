//go:build !windows

package engine

import (
	"commandmanager.local/core/internal/processcontrol"
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func replaceFile(from, to string) error                 { return os.Rename(from, to) }
func openSnapshot(path string) (*os.File, error)        { return os.Open(path) }
func prepareDetached(cmd *exec.Cmd)                     { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
func prepareDetachedFallback(_ *exec.Cmd, _ error) bool { return false }
func prepareChild(cmd *exec.Cmd)                        { cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func prepareHidden(cmd *exec.Cmd)                       {}
func CanLaunchDetached() (bool, string)                 { return true, "setsid" }

type processGroup struct{ pid int }

func attachProcess(cmd *exec.Cmd) (*processGroup, error) {
	return &processGroup{pid: cmd.Process.Pid}, nil
}
func (g *processGroup) stop() error {
	err := syscall.Kill(-g.pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
func (g *processGroup) close() {}
func (g *processGroup) graceful() error {
	err := syscall.Kill(-g.pid, syscall.SIGTERM)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
func groupContains(state executionState, target processcontrol.Target) bool {
	root, err := processcontrol.Observe(state.Run.PID)
	if err != nil || root.StartedAt != state.ProcessIdentity {
		return false
	}
	group, err := syscall.Getpgid(target.PID)
	return err == nil && group == state.Run.PID
}
func (g *processGroup) alive() (bool, error) {
	err := syscall.Kill(-g.pid, 0)
	if errors.Is(err, syscall.ESRCH) {
		return false, nil
	}
	if errors.Is(err, syscall.EPERM) {
		return true, nil
	}
	return err == nil, err
}
