package engine

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
	"os/exec"
	"syscall"
	"time"
	"unsafe"
)

func replaceFile(from, to string) error {
	a, e := windows.UTF16PtrFromString(from)
	if e != nil {
		return e
	}
	b, e := windows.UTF16PtrFromString(to)
	if e != nil {
		return e
	}
	// Older controllers may briefly read without FILE_SHARE_DELETE. Retain
	// the complete temporary snapshot and retry that transient obstruction.
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := windows.MoveFileEx(a, b, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
		if err == nil || (!errors.Is(err, windows.ERROR_SHARING_VIOLATION) && !errors.Is(err, windows.ERROR_ACCESS_DENIED)) || time.Now().After(deadline) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func openSnapshot(path string) (*os.File, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	// Reading an immutable snapshot must not prevent its writer from
	// atomically publishing the next state, including an older helper.
	handle, err := windows.CreateFile(name, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(handle), path), nil
}
func prepareDetached(cmd *exec.Cmd) {
	flags := uint32(windows.DETACHED_PROCESS | windows.CREATE_NEW_PROCESS_GROUP)
	if _, mode := CanLaunchDetached(); mode != "nestedJobFallback" {
		flags |= windows.CREATE_BREAKAWAY_FROM_JOB
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: flags}
}

func prepareDetachedFallback(cmd *exec.Cmd, startErr error) bool {
	if cmd.SysProcAttr == nil || cmd.SysProcAttr.CreationFlags&windows.CREATE_BREAKAWAY_FROM_JOB == 0 {
		return false
	}
	if !errors.Is(startErr, windows.ERROR_ACCESS_DENIED) {
		return false
	}
	cmd.SysProcAttr.CreationFlags &^= windows.CREATE_BREAKAWAY_FROM_JOB
	return true
}

func prepareChild(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_SUSPENDED | windows.CREATE_NEW_PROCESS_GROUP}
}
func prepareHidden(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: windows.CREATE_NO_WINDOW}
}

// CanLaunchDetached reports whether the current process can spawn a worker and
// whether it uses full job breakaway or Windows nested job fallback.
func CanLaunchDetached() (bool, string) {
	var inJob int32
	isInJob := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob")
	r1, _, _ := isInJob.Call(uintptr(windows.CurrentProcess()), 0, uintptr(unsafe.Pointer(&inJob)))
	if r1 == 0 || inJob == 0 {
		return true, "notInJob"
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	err := windows.QueryInformationJobObject(
		0,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
		nil,
	)
	if err != nil {
		return true, "inJobUnknownLimits"
	}
	const breakawayFlags = windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK | windows.JOB_OBJECT_LIMIT_SILENT_BREAKAWAY_OK
	if info.BasicLimitInformation.LimitFlags&breakawayFlags == 0 {
		return true, "nestedJobFallback"
	}
	return true, "breakawayPermitted"
}

type processGroup struct{ job windows.Handle }

// Start suspended so no descendant can escape before assignment to the job.
func attachProcess(cmd *exec.Cmd) (*processGroup, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, err
	}
	p, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE|windows.PROCESS_SUSPEND_RESUME, false, uint32(cmd.Process.Pid))
	if err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	defer windows.CloseHandle(p)
	if err = windows.AssignProcessToJobObject(job, p); err != nil {
		windows.CloseHandle(job)
		return nil, err
	}
	resume := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtResumeProcess")
	status, _, _ := resume.Call(uintptr(p))
	if status != 0 {
		windows.TerminateJobObject(job, 1)
		windows.CloseHandle(job)
		return nil, syscall.Errno(status)
	}
	return &processGroup{job: job}, nil
}
func (g *processGroup) stop() error { return windows.TerminateJobObject(g.job, 1) }
func (g *processGroup) close()      { windows.CloseHandle(g.job) }
func (g *processGroup) alive() (bool, error) {
	var info struct {
		TotalUserTime, TotalKernelTime, ThisPeriodTotalUserTime, ThisPeriodTotalKernelTime int64
		TotalPageFaultCount, TotalProcesses, ActiveProcesses, TotalTerminatedProcesses     uint32
	}
	err := windows.QueryInformationJobObject(g.job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), nil)
	return info.ActiveProcesses > 0, err
}
