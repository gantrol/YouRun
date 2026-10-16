package engine

import (
	"context"
	"os/exec"
	"time"
)

// Execute owns only this process tree, independently of managed executions.
// Cancellation of a local build must never stop unrelated managed commands.
func Execute(ctx context.Context, cmd *exec.Cmd) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	prepareChild(cmd)
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Start(); err != nil {
		return err
	}
	group, err := attachProcess(cmd)
	if err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return err
	}
	defer group.close()
	defer group.stop()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return err
	case <-ctx.Done():
		_ = group.stop()
		_ = cmd.Process.Kill()
		<-done
		return ctx.Err()
	}
}
