// Package processcontrol controls an observed process instance, never a bare PID.
package processcontrol

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
)

type Target struct {
	PID       int    `json:"pid"`
	StartedAt string `json:"startedAt"`
	Name      string `json:"name"`
	ParentPID int    `json:"parentPid,omitempty"`
}

type Result struct {
	Target
	Status string `json:"status"`
}

// Apply pins each instance before signalling it and retains that handle until
// exit has been observed. A graceful timeout never escalates to force.
func Apply(ctx context.Context, targets []Target, mode string) []Result {
	results := make([]Result, len(targets))
	type pending struct {
		index   int
		process *process
	}
	var waiting []pending
	for i, target := range targets {
		results[i] = Result{Target: target}
		if mode != "graceful" && mode != "force" {
			results[i].Status = "invalidRequest"
			continue
		}
		if ctx.Err() != nil {
			results[i].Status = "processExitTimeout"
			continue
		}
		p, err := open(target, mode)
		if err != nil {
			results[i].Status = err.Error()
			continue
		}
		if err = p.signal(mode); err != nil {
			results[i].Status = err.Error()
			p.close()
			continue
		}
		waiting = append(waiting, pending{i, p})
	}
	defer func() {
		for _, item := range waiting {
			item.process.close()
		}
	}()
	deadline := time.NewTimer(8 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		remaining := false
		for _, item := range waiting {
			if results[item.index].Status != "" {
				continue
			}
			exited, err := item.process.exited()
			if err != nil {
				results[item.index].Status = err.Error()
			} else if exited {
				results[item.index].Status = "processExited"
			} else {
				remaining = true
			}
		}
		if !remaining {
			return results
		}
		select {
		case <-ctx.Done():
		case <-deadline.C:
		case <-tick.C:
			continue
		}
		for _, item := range waiting {
			if results[item.index].Status == "" {
				results[item.index].Status = "processExitTimeout"
			}
		}
		return results
	}
}

func identityError() error { return errors.New("processIdentityChanged") }

func BornAfter(child, parent string) bool {
	c, ce := time.Parse(time.RFC3339Nano, child)
	p, pe := time.Parse(time.RFC3339Nano, parent)
	if ce == nil && pe == nil {
		return !c.Before(p)
	}
	// Linux identities contain the boot UUID and start ticks. Compare only
	// within the same boot; elapsed wall time is not a process identity.
	cb, ct, cok := strings.Cut(child, ":")
	pb, pt, pok := strings.Cut(parent, ":")
	cn, ce := strconv.ParseUint(ct, 10, 64)
	pn, pe := strconv.ParseUint(pt, 10, 64)
	return cok && pok && cb == pb && ce == nil && pe == nil && cn >= pn
}

// Verify is read-only. Force rights are requested only when applying a plan.
func Verify(target Target) error {
	p, err := open(target, "verify")
	if err != nil {
		return err
	}
	defer p.close()
	exited, err := p.exited()
	if err != nil {
		return err
	}
	if exited {
		return errors.New("processGone")
	}
	return nil
}

func RequestGraceful(targets []Target) error {
	var failures []error
	for _, target := range targets {
		p, err := open(target, "graceful")
		if err == nil {
			err = p.signal("graceful")
			p.close()
		}
		if err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
