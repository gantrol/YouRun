package engine

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

type independentProcess struct {
	pid    int
	handle windows.Handle
}

// Assign the Job as part of CreateProcess, not in a suspended/resume sequence.
// A helper crash at any instruction after creation leaves a runnable command.
// Only stdin and the log are inherited, never the ownership lock or Job handle.
func startIndependent(cmd *exec.Cmd, log *os.File, groupName ...string) (*independentProcess, *processGroup, error) {
	if cmd.Err != nil {
		return nil, nil, cmd.Err
	}
	var name *uint16
	if len(groupName) > 0 {
		name, _ = windows.UTF16PtrFromString(groupName[0])
	}
	job, err := windows.CreateJobObject(nil, name)
	if err != nil {
		return nil, nil, err
	}
	started := false
	defer func() {
		if !started {
			windows.CloseHandle(job)
		}
	}()
	// A managed developer tool may itself invoke YouRun. Its new execution
	// helper must be able to leave this Job instead of depending on that tool.
	// Ordinary descendants still inherit the Job; explicit breakaway is opt-in.
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_BREAKAWAY_OK
	if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, nil, err
	}
	input, err := os.Open(os.DevNull)
	if err != nil {
		return nil, nil, err
	}
	defer input.Close()
	handles := make([]windows.Handle, 2)
	for i, file := range []*os.File{input, log} {
		if err = windows.DuplicateHandle(windows.CurrentProcess(), windows.Handle(file.Fd()), windows.CurrentProcess(), &handles[i], 0, true, windows.DUPLICATE_SAME_ACCESS); err != nil {
			return nil, nil, err
		}
		defer windows.CloseHandle(handles[i])
	}
	attributes, err := windows.NewProcThreadAttributeList(2)
	if err != nil {
		return nil, nil, err
	}
	defer attributes.Delete()
	if err = attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil {
		return nil, nil, err
	}
	// ProcThreadAttributeValue(JobList=13, false, true, false), Windows 10+.
	const jobListAttribute = 0x0002000d
	if err = attributes.Update(jobListAttribute, unsafe.Pointer(&job), unsafe.Sizeof(job)); err != nil {
		return nil, nil, err
	}
	startup := windows.StartupInfoEx{ProcThreadAttributeList: attributes.List()}
	startup.Cb = uint32(unsafe.Sizeof(startup))
	startup.Flags = windows.STARTF_USESTDHANDLES | windows.STARTF_USESHOWWINDOW
	startup.ShowWindow = windows.SW_HIDE
	startup.StdInput, startup.StdOutput, startup.StdErr = handles[0], handles[1], handles[1]
	application, err := windows.UTF16PtrFromString(cmd.Path)
	if err != nil {
		return nil, nil, err
	}
	commandLine, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(cmd.Args))
	if err != nil {
		return nil, nil, err
	}
	directory, err := windows.UTF16PtrFromString(cmd.Dir)
	if err != nil {
		return nil, nil, err
	}
	environment := cmd.Environ() // includes os/exec's case-insensitive deduplication
	sort.Slice(environment, func(i, j int) bool { return strings.ToUpper(environment[i]) < strings.ToUpper(environment[j]) })
	for _, entry := range environment {
		if strings.ContainsRune(entry, 0) {
			return nil, nil, errors.New("invalidEnvironment")
		}
	}
	block := utf16.Encode([]rune(strings.Join(environment, "\x00") + "\x00\x00"))
	var info windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT | windows.CREATE_NEW_CONSOLE | windows.CREATE_NEW_PROCESS_GROUP)
	if err = windows.CreateProcess(application, commandLine, nil, nil, true, flags, &block[0], directory, &startup.StartupInfo, &info); err != nil {
		return nil, nil, err
	}
	windows.CloseHandle(info.Thread)
	started = true
	return &independentProcess{pid: int(info.ProcessId), handle: info.Process}, &processGroup{job: job, pid: int(info.ProcessId), rootHandle: info.Process}, nil
}
func (p *independentProcess) wait() (int, error) {
	if _, err := windows.WaitForSingleObject(p.handle, windows.INFINITE); err != nil {
		return -1, err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(p.handle, &code); err != nil {
		return -1, err
	}
	if code != 0 {
		return int(code), fmt.Errorf("exit status %d", code)
	}
	return 0, nil
}
func (p *independentProcess) close() { windows.CloseHandle(p.handle) }
