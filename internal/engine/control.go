package engine

import (
	"commandmanager.local/core/internal/processcontrol"
	"errors"
)

// Graceful requests use a separate journal so old executors can never mistake
// a graceful request for their legacy force-only stop.json protocol.
func (c *Client) StopGracefully(id string) (string, error) {
	state, err := c.readState(id)
	if err != nil {
		return "", err
	}
	if terminalStatus(state.Run.Status) {
		return "", nil
	}
	if state.Run.Status == "unknown" {
		return "", errors.New("runNotOwned")
	}
	if state.ControlVersion != 1 {
		return "", errors.New("processGracefulUnsupported")
	}
	requestID := ID()
	err = atomicJSON(c.runPath(id, "graceful.json"), map[string]string{"id": requestID})
	return requestID, err
}

func (c *Client) GracefulError(id, requestID string) string {
	state, err := c.readState(id)
	if err != nil {
		return "runNotOwned"
	}
	if state.ControlRequestID == requestID {
		return state.ControlError
	}
	return ""
}

// ConfirmProcess requires executor-owned membership. Display attribution from
// command lines or ancestry alone is not sufficient for a stop operation.
func (c *Client) ConfirmProcess(id string, target processcontrol.Target) bool {
	state, err := c.readState(id)
	if err != nil || !activeStatus(state.Run.Status) || state.ControlVersion != 1 {
		return false
	}
	if processcontrol.Verify(target) != nil {
		return false
	}
	return groupContains(state, target)
}

// Resource inspection must not create directories or change their ACLs.
func ConfirmRunProcess(directory, id string, target processcontrol.Target) bool {
	return (&Client{dir: directory}).ConfirmProcess(id, target)
}
