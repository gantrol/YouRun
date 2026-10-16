package engine

import (
	"context"
	"golang.org/x/sys/windows"
)

func waitWorkerExit(ctx context.Context, pid int) bool {
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return err == windows.ERROR_INVALID_PARAMETER
	}
	defer windows.CloseHandle(process)
	cancelled, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(cancelled)
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { windows.SetEvent(cancelled); close(done) })
	defer func() {
		if !stop() {
			<-done
		}
	}()
	result, err := windows.WaitForMultipleObjects([]windows.Handle{process, cancelled}, false, windows.INFINITE)
	return err == nil && result == windows.WAIT_OBJECT_0
}
