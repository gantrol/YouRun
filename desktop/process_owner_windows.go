package desktop

import (
	"errors"
	"os"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Follow live ancestors only. Creation times reject recycled parent PIDs.
// An incomplete chain is inconclusive, including after an update handoff;
// callers must not treat it as proof that stopping the run is safe.
// No process is opened with termination permissions.
func ownsCurrentProcess(rootPID int, startedAt time.Time) (bool, error) {
	if rootPID <= 0 {
		return false, errors.New("missing run PID")
	}
	snapshot, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(snapshot)
	parents := map[uint32]uint32{}
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for err = windows.Process32First(snapshot, &entry); err == nil; err = windows.Process32Next(snapshot, &entry) {
		parents[entry.ProcessID] = entry.ParentProcessID
	}
	if !errors.Is(err, windows.ERROR_NO_MORE_FILES) {
		return false, err
	}
	childTime := ^uint64(0)
	pid := uint32(os.Getpid())
	for depth := 0; pid != 0 && depth < 128; depth++ {
		process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return false, errors.New("process ancestor exited")
		}
		if err != nil {
			return false, err
		}
		var created, exited, kernel, user windows.Filetime
		err = windows.GetProcessTimes(process, &created, &exited, &kernel, &user)
		windows.CloseHandle(process)
		if err != nil {
			return false, err
		}
		started := uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime)
		if started > childTime {
			return false, errors.New("process ancestor PID reused")
		}
		// An ancestor older than the run cannot be in that run's process tree.
		// This also avoids walking into unrelated protected system processes.
		if time.Unix(0, created.Nanoseconds()).Before(startedAt) {
			return false, nil
		}
		if pid == uint32(rootPID) {
			return true, nil
		}
		childTime = started
		next, found := parents[pid]
		if !found || next == pid {
			return false, errors.New("incomplete process ancestry")
		}
		pid = next
	}
	if pid != 0 {
		return false, errors.New("process ancestry too deep")
	}
	return false, nil
}
