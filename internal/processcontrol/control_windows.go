package processcontrol

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type process struct {
	handle windows.Handle
	pid    uint32
}

type closeWindowsRequest struct {
	process *process
	count   int
	denied  bool
	failed  bool
}

var closeWindowsRequests sync.Map
var closeWindowsSequence atomic.Uint64
var user32 = windows.NewLazySystemDLL("user32.dll")
var closeWindowCallback = syscall.NewCallback(func(hwnd, reference uintptr) uintptr {
	value, ok := closeWindowsRequests.Load(reference)
	if !ok {
		return 0
	}
	request := value.(*closeWindowsRequest)
	var pid uint32
	user32.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid != request.process.pid {
		return 1
	}
	if exited, err := request.process.exited(); err != nil || exited {
		return 0
	}
	okValue, _, err := user32.NewProc("PostMessageW").Call(hwnd, 0x0010, 0, 0)
	if okValue != 0 {
		request.count++
	} else if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		request.denied = true
	} else {
		request.failed = true
	}
	return 1
})

func failure(err error) error {
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return errors.New("processAccessDenied")
	}
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return errors.New("processGone")
	}
	return errors.New("processControlFailed")
}

func creation(h windows.Handle) (string, error) {
	var c, e, k, u windows.Filetime
	if err := windows.GetProcessTimes(h, &c, &e, &k, &u); err != nil {
		return "", failure(err)
	}
	return time.Unix(0, c.Nanoseconds()).UTC().Format(time.RFC3339Nano), nil
}

func Observe(pid int) (Target, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return Target{}, failure(err)
	}
	defer windows.CloseHandle(h)
	started, err := creation(h)
	return Target{PID: pid, StartedAt: started}, err
}

func open(target Target, mode string) (*process, error) {
	if target.PID <= 4 || target.PID == os.Getpid() {
		return nil, errors.New("processProtected")
	}
	if target.StartedAt == "" {
		return nil, errors.New("processIdentityUnavailable")
	}
	rights := uint32(windows.PROCESS_QUERY_LIMITED_INFORMATION | windows.SYNCHRONIZE)
	if mode == "force" {
		rights |= windows.PROCESS_TERMINATE
	}
	h, err := windows.OpenProcess(rights, false, uint32(target.PID))
	if err != nil {
		return nil, failure(err)
	}
	p := &process{h, uint32(target.PID)}
	started, err := creation(h)
	if err != nil || started != target.StartedAt {
		p.close()
		if err != nil {
			return nil, err
		}
		return nil, identityError()
	}
	var critical int32
	ok, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessCritical").Call(uintptr(h), uintptr(unsafe.Pointer(&critical)))
	if ok == 0 || critical != 0 {
		p.close()
		return nil, errors.New("processProtected")
	}
	var path [32768]uint16
	size := uint32(len(path))
	if err := windows.QueryFullProcessImageName(h, 0, &path[0], &size); err == nil {
		name := strings.ToLower(filepath.Base(windows.UTF16ToString(path[:size])))
		switch name {
		case "csrss.exe", "wininit.exe", "winlogon.exe", "lsass.exe", "services.exe", "smss.exe":
			p.close()
			return nil, errors.New("processProtected")
		}
	}
	return p, nil
}

func (p *process) close() { windows.CloseHandle(p.handle) }
func (p *process) exited() (bool, error) {
	status, err := windows.WaitForSingleObject(p.handle, 0)
	if err != nil {
		return false, failure(err)
	}
	return status == windows.WAIT_OBJECT_0, nil
}

func (p *process) signal(mode string) error {
	if exited, err := p.exited(); err != nil || exited {
		return err
	}
	if mode == "force" {
		if err := windows.TerminateProcess(p.handle, 1); err != nil {
			return failure(err)
		}
		return nil
	}
	// Only windows owned by this pinned process receive WM_CLOSE. Never send
	// a console broadcast to an arbitrary process's shared terminal.
	reference := uintptr(closeWindowsSequence.Add(1))
	request := &closeWindowsRequest{process: p}
	closeWindowsRequests.Store(reference, request)
	defer closeWindowsRequests.Delete(reference)
	user32.NewProc("EnumWindows").Call(closeWindowCallback, reference)
	if request.denied {
		return errors.New("processAccessDenied")
	}
	if request.failed {
		return errors.New("processControlFailed")
	}
	if request.count == 0 {
		return errors.New("processGracefulUnsupported")
	}
	return nil
}

// JobMembers is authoritative membership, unlike a parent-PID attribution.
func JobMembers(name string) ([]Target, error) {
	n, err := windows.UTF16PtrFromString(name)
	if err != nil || !strings.HasPrefix(name, "Local\\YouRun-") {
		return nil, errors.New("runNotOwned")
	}
	value, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("OpenJobObjectW").Call(0x0004, 0, uintptr(unsafe.Pointer(n)))
	h := windows.Handle(value)
	if h == 0 {
		return nil, errors.New("runNotOwned")
	}
	defer windows.CloseHandle(h)
	for size := 64; size <= 65536; size *= 2 {
		buf := make([]uintptr, size)
		err = windows.QueryInformationJobObject(h, windows.JobObjectBasicProcessIdList, uintptr(unsafe.Pointer(&buf[0])), uint32(len(buf))*uint32(unsafe.Sizeof(buf[0])), nil)
		if errors.Is(err, windows.ERROR_MORE_DATA) {
			continue
		}
		if err != nil {
			return nil, errors.New("runNotOwned")
		}
		count := *(*uint32)(unsafe.Add(unsafe.Pointer(&buf[0]), 4))
		offset := uintptr(8) / unsafe.Sizeof(buf[0])
		if int(count)+int(offset) > len(buf) {
			return nil, errors.New("runNotOwned")
		}
		items := []Target{}
		for i := uint32(0); i < count; i++ {
			pid := uint32(buf[int(offset)+int(i)])
			memberHandle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
			if err != nil {
				if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
					continue
				}
				return nil, failure(err)
			}
			var member int32
			ok, _, _ := windows.NewLazySystemDLL("kernel32.dll").NewProc("IsProcessInJob").Call(uintptr(memberHandle), uintptr(h), uintptr(unsafe.Pointer(&member)))
			started, creationErr := creation(memberHandle)
			windows.CloseHandle(memberHandle)
			if ok == 0 || member == 0 || creationErr != nil {
				continue
			}
			items = append(items, Target{PID: int(pid), StartedAt: started})
		}
		return items, nil
	}
	return nil, errors.New("processTooManyTargets")
}
