package engine

import "context"

// SubscribeWorkerExit observes the helper's OS lifetime, without taking ownership
// or signaling it. A reused PID can only cause a fresh read, never a termination.
func (c *Client) SubscribeWorkerExit(ctx context.Context, id string, notify func()) bool {
	if !validID.MatchString(id) {
		return false
	}
	var state executionState
	if readJSON(c.runPath(id, "state.json"), &state) != nil || state.WorkerPID <= 0 {
		return false
	}
	go func() {
		if waitWorkerExit(ctx, state.WorkerPID) && ctx.Err() == nil {
			notify()
		}
	}()
	return true
}
