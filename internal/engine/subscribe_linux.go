package engine

import (
	"context"
	"golang.org/x/sys/unix"
)

func waitWorkerExit(ctx context.Context, pid int) bool {
	process, err := unix.PidfdOpen(pid, 0)
	if err != nil {
		return err == unix.ESRCH
	}
	defer unix.Close(process)
	pipe := make([]int, 2)
	if unix.Pipe(pipe) != nil {
		return false
	}
	defer unix.Close(pipe[0])
	defer unix.Close(pipe[1])
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { unix.Write(pipe[1], []byte{1}); close(done) })
	defer func() {
		if !stop() {
			<-done
		}
	}()
	fds := []unix.PollFd{{Fd: int32(process), Events: unix.POLLIN}, {Fd: int32(pipe[0]), Events: unix.POLLIN}}
	for {
		_, err := unix.Poll(fds, -1)
		if err == unix.EINTR {
			continue
		}
		return err == nil && fds[0].Revents != 0
	}
}
