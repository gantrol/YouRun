package engine

import (
	"context"
	"golang.org/x/sys/unix"
)

func waitWorkerExit(ctx context.Context, pid int) bool {
	queue, err := unix.Kqueue()
	if err != nil {
		return false
	}
	defer unix.Close(queue)
	pipe := make([]int, 2)
	if unix.Pipe(pipe) != nil {
		return false
	}
	defer unix.Close(pipe[0])
	defer unix.Close(pipe[1])
	changes := []unix.Kevent_t{
		{Ident: uint64(pid), Filter: unix.EVFILT_PROC, Flags: unix.EV_ADD | unix.EV_ONESHOT, Fflags: unix.NOTE_EXIT},
		{Ident: uint64(pipe[0]), Filter: unix.EVFILT_READ, Flags: unix.EV_ADD | unix.EV_ONESHOT},
	}
	if _, err = unix.Kevent(queue, changes, nil, nil); err != nil {
		return err == unix.ESRCH
	}
	done := make(chan struct{})
	stop := context.AfterFunc(ctx, func() { unix.Write(pipe[1], []byte{1}); close(done) })
	defer func() {
		if !stop() {
			<-done
		}
	}()
	events := make([]unix.Kevent_t, 2)
	for {
		count, err := unix.Kevent(queue, nil, events, nil)
		if err == unix.EINTR {
			continue
		}
		if err != nil {
			return false
		}
		for _, event := range events[:count] {
			if event.Filter == unix.EVFILT_PROC {
				return true
			}
			if event.Filter == unix.EVFILT_READ {
				return false
			}
		}
	}
}
