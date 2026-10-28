package engine

import (
	"context"
	"io"
	"os"
	"os/exec"
	"time"
)

// Execute owns only this process tree, independently of managed executions.
// Cancellation of a local build must never stop unrelated managed commands.
func Execute(ctx context.Context, cmd *exec.Cmd, output io.Writer) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if output == nil {
		output = io.Discard
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		return err
	}
	defer reader.Close()
	// Local builds use the same atomic process/Job creation as managed tasks.
	// Their combined output still streams into the bounded in-memory build log.
	process, group, err := startIndependent(cmd, writer)
	writer.Close()
	if err != nil {
		return err
	}
	defer process.close()
	defer group.close()
	defer group.stop()
	copied := make(chan error, 1)
	go func() { _, err := io.Copy(output, reader); copied <- err }()
	done := make(chan error, 1)
	go func() { _, err := process.wait(); done <- err }()
	select {
	case err = <-done:
	case <-ctx.Done():
		_ = group.stop()
		<-done
		err = ctx.Err()
	}
	// End local build descendants before draining output; a child retaining the
	// pipe must not keep a completed or cancelled build waiting indefinitely.
	_ = group.stop()
	select {
	case copyErr := <-copied:
		if err == nil {
			err = copyErr
		}
	case <-time.After(2 * time.Second):
		if err == nil {
			err = exec.ErrWaitDelay
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
