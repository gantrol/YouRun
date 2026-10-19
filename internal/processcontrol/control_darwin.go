package processcontrol

import (
	"errors"
	"os"
	"strconv"
	"time"

	"golang.org/x/sys/unix"
)

type process struct {
	target Target
	queue  int
}

func Observe(pid int) (Target, error) {
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil {
		return Target{}, errors.New("processGone")
	}
	if int(info.Proc.P_pid) != pid {
		return Target{}, errors.New("processGone")
	}
	started := info.Proc.P_starttime
	return Target{PID: pid, StartedAt: time.Unix(started.Sec, int64(started.Usec)*1000).UTC().Format(time.RFC3339Nano), Name: strconv.Itoa(pid)}, nil
}
func open(target Target, mode string) (*process, error) {
	if target.PID <= 1 || target.PID == os.Getpid() {
		return nil, errors.New("processProtected")
	}
	if target.StartedAt == "" {
		return nil, errors.New("processIdentityUnavailable")
	}
	fd, err := unix.Kqueue()
	if err != nil {
		return nil, errors.New("processControlFailed")
	}
	change := unix.Kevent_t{Ident: uint64(target.PID), Filter: unix.EVFILT_PROC, Flags: unix.EV_ADD | unix.EV_ENABLE, Fflags: unix.NOTE_EXIT}
	if _, err = unix.Kevent(fd, []unix.Kevent_t{change}, nil, nil); err != nil {
		unix.Close(fd)
		return nil, errors.New("processAccessDenied")
	}
	current, err := Observe(target.PID)
	if err != nil || current.StartedAt != target.StartedAt {
		unix.Close(fd)
		if err != nil {
			return nil, err
		}
		return nil, identityError()
	}
	return &process{target, fd}, nil
}
func (p *process) close() { unix.Close(p.queue) }
func (p *process) exited() (bool, error) {
	events := make([]unix.Kevent_t, 1)
	n, err := unix.Kevent(p.queue, nil, events, &unix.Timespec{})
	if err != nil {
		return false, errors.New("processControlFailed")
	}
	if n > 0 {
		return true, nil
	}
	current, err := Observe(p.target.PID)
	return err != nil || current.StartedAt != p.target.StartedAt, nil
}
func (p *process) signal(mode string) error {
	if gone, err := p.exited(); err != nil || gone {
		return err
	}
	sig := unix.SIGTERM
	if mode == "force" {
		sig = unix.SIGKILL
	}
	if err := unix.Kill(p.target.PID, sig); err != nil {
		if errors.Is(err, unix.ESRCH) {
			return nil
		}
		return errors.New("processAccessDenied")
	}
	return nil
}
func JobMembers(string) ([]Target, error) { return nil, errors.New("runNotOwned") }
