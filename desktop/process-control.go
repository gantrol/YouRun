package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"sort"
	"sync"
	"time"

	"commandmanager.local/core/internal/engine"
	"commandmanager.local/core/internal/processcontrol"
)

type controlTarget struct {
	processcontrol.Target
	RunID string `json:"runId,omitempty"`
}
type controlResult struct {
	controlTarget
	Status string `json:"status"`
}
type controlPlan struct {
	ID         string          `json:"id"`
	Mode       string          `json:"mode"`
	Scope      string          `json:"scope"`
	Targets    []controlTarget `json:"targets"`
	ExpiresAt  time.Time       `json:"expiresAt"`
	Results    []controlResult `json:"results,omitempty"`
	CanElevate bool            `json:"canElevate"`
	running    bool
}
type processActions struct {
	mu    sync.Mutex
	plans map[string]*controlPlan
}

func processIdentity(p rawProcess) string {
	if p.Identity != "" {
		return p.Identity
	}
	if p.StartedAt.IsZero() {
		return ""
	}
	return p.StartedAt.UTC().Format(time.RFC3339Nano)
}

func (h *Host) prepareProcessControl(ctx context.Context, requested []controlTarget, mode, scope string) (*controlPlan, error) {
	if len(requested) == 0 || len(requested) > 128 || (mode != "graceful" && mode != "force") || (scope != "task" && scope != "process" && scope != "tree") {
		return nil, errors.New("invalidRequest")
	}
	procs, err := collectOSProcesses(WithSkipProcessIcons(ctx, true))
	if err != nil {
		return nil, errors.New("requestFailed")
	}
	protected := map[int]bool{0: true, 1: true, os.Getpid(): true}
	for pid := os.Getppid(); pid > 0 && !protected[pid]; {
		protected[pid] = true
		p, ok := procs[pid]
		if !ok {
			break
		}
		pid = p.PPID
	}
	selected := map[int]controlTarget{}
	h.ownerMu.Lock()
	ownerRunID := h.ownerRunID
	h.ownerMu.Unlock()
	for _, target := range requested {
		p, ok := procs[target.PID]
		if !ok {
			return nil, errors.New("processGone")
		}
		if target.StartedAt == "" {
			return nil, errors.New("processIdentityUnavailable")
		}
		if target.StartedAt != processIdentity(p) {
			return nil, errors.New("processIdentityChanged")
		}
		target.Name, target.ParentPID = p.Name, p.PPID
		if scope == "task" && target.RunID != "" {
			if !h.client.ConfirmProcess(target.RunID, target.Target) {
				return nil, errors.New("runNotOwned")
			}
			run, err := h.client.Run(target.RunID)
			if err != nil || protected[run.PID] || target.RunID == ownerRunID {
				return nil, errors.New("processProtected")
			}
		} else {
			target.RunID = ""
		}
		selected[target.PID] = target
	}
	if scope == "tree" {
		for changed := true; changed; {
			changed = false
			for _, p := range procs {
				if _, ok := selected[p.PID]; ok {
					continue
				}
				parent, ok := selected[p.PPID]
				if !ok {
					continue
				}
				if !processcontrol.BornAfter(processIdentity(p), processIdentity(procs[parent.PID])) {
					return nil, errors.New("processTreeUnconfirmed")
				}
				selected[p.PID] = controlTarget{Target: processcontrol.Target{PID: p.PID, ParentPID: p.PPID, StartedAt: processIdentity(p), Name: p.Name}}
				changed = true
				if len(selected) > 512 {
					return nil, errors.New("processTooManyTargets")
				}
			}
		}
	}
	plan := &controlPlan{ID: engine.ID(), Mode: mode, Scope: scope, ExpiresAt: time.Now().Add(2 * time.Minute), Targets: []controlTarget{}, CanElevate: canElevateProcessControl()}
	runs := map[string]bool{}
	for _, target := range selected {
		if protected[target.PID] {
			return nil, errors.New("processProtected")
		}
		if err := processcontrol.Verify(target.Target); err != nil && err.Error() != "processAccessDenied" {
			return nil, err
		}
		if target.RunID != "" {
			if runs[target.RunID] {
				continue
			}
			runs[target.RunID] = true
		}
		plan.Targets = append(plan.Targets, target)
	}
	// Children first; a parent exit must not be mistaken for complete tree exit.
	depth := func(pid int) int {
		n := 0
		for p, ok := selected[pid]; ok && n <= len(selected); p, ok = selected[p.ParentPID] {
			n++
		}
		return n
	}
	sort.Slice(plan.Targets, func(i, j int) bool {
		di, dj := depth(plan.Targets[i].PID), depth(plan.Targets[j].PID)
		if di != dj {
			return di > dj
		}
		return plan.Targets[i].PID < plan.Targets[j].PID
	})
	return plan, nil
}

func (h *Host) applyProcessControl(ctx context.Context, plan *controlPlan, elevate bool) []controlResult {
	results := make([]controlResult, len(plan.Targets))
	native := []processcontrol.Target{}
	indices := []int{}
	var wg sync.WaitGroup
	for i, target := range plan.Targets {
		results[i] = controlResult{controlTarget: target}
		if elevate && len(plan.Results) > i && plan.Results[i].Status != "processAccessDenied" {
			results[i] = plan.Results[i]
			continue
		}
		if target.RunID == "" {
			native = append(native, target.Target)
			indices = append(indices, i)
			continue
		}
		wg.Add(1)
		go func(i int, target controlTarget) {
			defer wg.Done()
			// Run IDs are immutable, and this association was proven when the
			// server created the plan. The original child may already have exited;
			// the executor still owns and controls the remaining Job members.
			var err error
			var requestID string
			if plan.Mode == "graceful" {
				requestID, err = h.client.StopGracefully(target.RunID)
			} else {
				err = h.client.Stop(target.RunID)
			}
			if err != nil {
				results[i].Status = err.Error()
				return
			}
			ticker := time.NewTicker(100 * time.Millisecond)
			defer ticker.Stop()
			for {
				run, err := h.client.Run(target.RunID)
				if err != nil || run.Status == "unknown" {
					results[i].Status = "runNotOwned"
					return
				}
				if run.Status == "stopped" || run.Status == "success" || run.Status == "failed" {
					results[i].Status = "processExited"
					return
				}
				if run.Status == "handedOff" {
					results[i].Status = "processTreeUnconfirmed"
					return
				}
				if requestID != "" {
					if code := h.client.GracefulError(target.RunID, requestID); code != "" {
						results[i].Status = code
						return
					}
				}
				select {
				case <-ctx.Done():
					results[i].Status = "processExitTimeout"
					return
				case <-ticker.C:
				}
			}
		}(i, target)
	}
	if len(native) > 0 {
		var items []processcontrol.Result
		if elevate {
			items = elevateProcessControl(ctx, native, plan.Mode)
		} else {
			items = processcontrol.Apply(ctx, native, plan.Mode)
		}
		for i, item := range items {
			results[indices[i]].Status = item.Status
		}
	}
	wg.Wait()
	return results
}

func (h *Host) serveProcessControl(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	fail := func(code string, status int) {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]string{"error": code})
	}
	if r.Method != http.MethodPost {
		fail("invalidRequest", 405)
		return
	}
	if h.updates.blocked() {
		fail("updateBusy", 409)
		return
	}
	var request struct {
		Action  string          `json:"action"`
		Targets []controlTarget `json:"targets,omitempty"`
		Mode    string          `json:"mode,omitempty"`
		Scope   string          `json:"scope,omitempty"`
		ID      string          `json:"id,omitempty"`
		Elevate bool            `json:"elevate,omitempty"`
	}
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	d.DisallowUnknownFields()
	if d.Decode(&request) != nil || d.Decode(&struct{}{}) != io.EOF {
		fail("invalidRequest", 400)
		return
	}
	a := &h.processActions
	if request.Action == "prepare" {
		ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		defer cancel()
		plan, err := h.prepareProcessControl(ctx, request.Targets, request.Mode, request.Scope)
		if err != nil {
			fail(err.Error(), 409)
			return
		}
		a.mu.Lock()
		if a.plans == nil {
			a.plans = map[string]*controlPlan{}
		}
		for id, p := range a.plans {
			if !p.running && time.Now().After(p.ExpiresAt) {
				delete(a.plans, id)
			}
		}
		if len(a.plans) >= 32 {
			a.mu.Unlock()
			fail("processBusy", 409)
			return
		}
		a.plans[plan.ID] = plan
		a.mu.Unlock()
		json.NewEncoder(w).Encode(plan)
		return
	}
	if request.Action == "force-preview" {
		a.mu.Lock()
		previous := a.plans[request.ID]
		if previous == nil || previous.running || previous.Mode != "graceful" || time.Now().After(previous.ExpiresAt) {
			a.mu.Unlock()
			fail("processPlanExpired", 409)
			return
		}
		next := &controlPlan{ID: engine.ID(), Mode: "force", Scope: previous.Scope, ExpiresAt: time.Now().Add(2 * time.Minute), CanElevate: previous.CanElevate, Targets: []controlTarget{}}
		for _, item := range previous.Results {
			if item.Status == "processExitTimeout" || item.Status == "processGracefulUnsupported" {
				next.Targets = append(next.Targets, item.controlTarget)
			}
		}
		if len(next.Targets) == 0 || len(a.plans) >= 32 {
			a.mu.Unlock()
			fail("processBusy", 409)
			return
		}
		a.plans[next.ID] = next
		a.mu.Unlock()
		json.NewEncoder(w).Encode(next)
		return
	}
	if request.Action != "execute" {
		fail("invalidRequest", 400)
		return
	}
	a.mu.Lock()
	plan := a.plans[request.ID]
	if plan == nil || time.Now().After(plan.ExpiresAt) {
		a.mu.Unlock()
		fail("processPlanExpired", 409)
		return
	}
	if plan.running {
		a.mu.Unlock()
		fail("processBusy", 409)
		return
	}
	if plan.Results != nil && !request.Elevate {
		snapshot := *plan
		a.mu.Unlock()
		json.NewEncoder(w).Encode(snapshot)
		return
	}
	if request.Elevate {
		denied := false
		for _, item := range plan.Results {
			denied = denied || item.Status == "processAccessDenied"
		}
		if !denied || !plan.CanElevate {
			a.mu.Unlock()
			fail("invalidRequest", 400)
			return
		}
	}
	plan.running = true
	snapshot := *plan
	a.mu.Unlock()
	// Once admitted, disconnecting the renderer does not abandon the result.
	timeout := 12 * time.Second
	if request.Elevate {
		timeout = 2 * time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	results := h.applyProcessControl(ctx, &snapshot, request.Elevate)
	a.mu.Lock()
	plan.Results = results
	plan.running = false
	plan.ExpiresAt = time.Now().Add(2 * time.Minute)
	snapshot = *plan
	a.mu.Unlock()
	json.NewEncoder(w).Encode(snapshot)
}
