package updater

import (
	"golang.org/x/sys/windows"
	"os/exec"
	"syscall"
)

func Detached(cmd *exec.Cmd) {
	// A GUI launched by an execution helper belongs to a job that disallows breakaway.
	// Its successor can stay in that job: the execution helper owns the job, not the old
	// window, and continues tracking it after the old GUI exits. Runner startup
	// uses its separate engine detachment path.
	// Detach the console without setting STARTF_USESHOWWINDOW/SW_HIDE: that
	// startup hint also suppresses the GUI's first ShowWindow call.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP}
}
func replace(from, to string) error {
	a, err := windows.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	b, err := windows.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(a, b, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}
