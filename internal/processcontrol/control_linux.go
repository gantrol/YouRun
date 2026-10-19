package processcontrol

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"golang.org/x/sys/unix"
)

type process struct{ fd int }

func Observe(pid int) (Target, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return Target{}, linuxError(err)
	}
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 {
		return Target{}, identityError()
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) < 20 {
		return Target{}, identityError()
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return Target{}, linuxError(err)
	}
	return Target{PID: pid, StartedAt: strings.TrimSpace(string(boot)) + ":" + fields[19]}, nil
}
func linuxError(err error) error {
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, unix.ESRCH) {
		return errors.New("processGone")
	}
	if errors.Is(err, os.ErrPermission) || errors.Is(err, unix.EPERM) {
		return errors.New("processAccessDenied")
	}
	return errors.New("processControlFailed")
}
func open(target Target, mode string) (*process, error) {
	if target.PID <= 1 || target.PID == os.Getpid() {
		return nil, errors.New("processProtected")
	}
	if target.StartedAt == "" {
		return nil, errors.New("processIdentityUnavailable")
	}
	fd, err := unix.PidfdOpen(target.PID, 0)
	if err != nil {
		return nil, linuxError(err)
	}
	current, err := Observe(target.PID)
	if err != nil || current.StartedAt != target.StartedAt {
		unix.Close(fd)
		if err != nil {
			return nil, err
		}
		return nil, identityError()
	}
	return &process{fd: fd}, nil
}
func (p *process) close() { unix.Close(p.fd) }
func (p *process) exited() (bool, error) {
	fds := []unix.PollFd{{Fd: int32(p.fd), Events: unix.POLLIN}}
	_, err := unix.Poll(fds, 0)
	return fds[0].Revents&unix.POLLIN != 0, err
}
func (p *process) signal(mode string) error {
	sig := unix.SIGTERM
	if mode == "force" {
		sig = unix.SIGKILL
	}
	if err := unix.PidfdSendSignal(p.fd, sig, nil, 0); err != nil {
		return linuxError(err)
	}
	return nil
}
func JobMembers(string) ([]Target, error) { return nil, errors.New("runNotOwned") }
