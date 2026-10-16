package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/gofrs/flock"
)

// Client is an in-process controller over private files, not a daemon client.
// ExecutionSchema must remain readable across application/database updates.
const ExecutionSchema = 1

type Client struct{ dir, executable string }
type executionManifest struct {
	Schema    int       `json:"schema"`
	Execution Execution `json:"execution"`
	Previous  string    `json:"previous,omitempty"`
	Executor  string    `json:"executor"`
}
type executionState struct {
	Schema    int    `json:"schema"`
	Run       Run    `json:"run"`
	WorkerPID int    `json:"workerPid,omitempty"`
	Phase     string `json:"phase"`
	Error     string `json:"error,omitempty"`
}

func Open(directory, executable string) (*Client, error) {
	directory, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	if strings.Contains(strings.ToLower(directory), "trash") {
		return nil, errors.New("invalidDirectory")
	}
	if err = secureDirectory(directory); err != nil {
		return nil, err
	}
	for _, name := range []string{"executions", "logs"} {
		if err = os.MkdirAll(filepath.Join(directory, name), 0700); err != nil {
			return nil, err
		}
	}
	return &Client{dir: directory, executable: executable}, nil
}
func fileLock(path string) (*flock.Flock, error) {
	lock := flock.New(path)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ok, err := lock.TryLockContext(ctx, 20*time.Millisecond)
	if err != nil || !ok {
		lock.Close()
		if err == nil {
			err = context.DeadlineExceeded
		}
		return nil, err
	}
	return lock, nil
}
func (c *Client) runPath(id, name string) string { return filepath.Join(c.dir, "executions", id, name) }
func activeStatus(status string) bool            { return status == "running" || status == "stopping" }
func terminalStatus(status string) bool {
	return oneOf(status, "success", "failed", "stopped", "handedOff")
}

func (c *Client) readState(id string) (executionState, error) {
	var state executionState
	if !validID.MatchString(id) {
		return state, errors.New("invalidRequest")
	}
	if err := readJSON(c.runPath(id, "state.json"), &state); err != nil {
		return state, err
	}
	if state.Schema != ExecutionSchema || state.Run.ID != id {
		return state, errors.New("executionSchemaUnsupported")
	}
	if activeStatus(state.Run.Status) {
		owner := flock.New(c.runPath(id, "owner.lock"))
		locked, err := owner.TryLock()
		owner.Close()
		if err != nil {
			return state, err
		}
		started, parseErr := time.Parse(time.RFC3339Nano, state.Run.Time)
		pending := state.Phase == "pending" && parseErr == nil && time.Since(started) < 15*time.Second
		if locked && !pending {
			// The helper may have committed its terminal result between our read
			// and lock acquisition. Re-read before declaring ownership lost.
			if err := readJSON(c.runPath(id, "state.json"), &state); err != nil {
				return state, err
			}
			if activeStatus(state.Run.Status) {
				state.Run.Status = "unknown"
				state.Error = "executorUnavailable"
			}
		}
		if activeStatus(state.Run.Status) {
			if _, err := os.Stat(c.runPath(id, "stop.json")); err == nil {
				state.Run.Status = "stopping"
			} else if !os.IsNotExist(err) {
				return state, err
			}
		}
	}
	state.Run.Error = state.Error
	return state, nil
}

// Legacy records are read-only. No signal is ever sent to an unowned PID.
func (c *Client) runs() ([]Run, error) {
	runs := []Run{}
	if err := readJSON(filepath.Join(c.dir, "runs.json"), &runs); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	seen := map[string]bool{}
	for i := range runs {
		seen[runs[i].ID] = true
		if activeStatus(runs[i].Status) {
			runs[i].Status = "unknown"
		}
	}
	entries, err := os.ReadDir(filepath.Join(c.dir, "executions"))
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !entry.IsDir() || !validID.MatchString(entry.Name()) {
			continue
		}
		state, err := c.readState(entry.Name())
		if err != nil {
			return nil, err
		}
		if seen[state.Run.ID] {
			return nil, errors.New("duplicateRunId")
		}
		runs = append(runs, state.Run)
	}
	sort.Slice(runs, func(i, j int) bool {
		a, _ := time.Parse(time.RFC3339Nano, runs[i].Time)
		b, _ := time.Parse(time.RFC3339Nano, runs[j].Time)
		if a.Equal(b) {
			return runs[i].ID < runs[j].ID
		}
		return a.After(b)
	})
	return runs, nil
}
func (c *Client) State() (RuntimeState, error) {
	lock, err := fileLock(filepath.Join(c.dir, "executions.lock"))
	if err != nil {
		return RuntimeState{}, err
	}
	defer lock.Close()
	runs, err := c.runs()
	return RuntimeState{Protocol: Protocol, Platform: runtime.GOOS, ObservedAt: time.Now().UnixNano(), Runs: runs}, err
}
func (c *Client) Run(id string) (Run, error) {
	if !validID.MatchString(id) {
		return Run{}, errors.New("invalidRequest")
	}
	state, err := c.State()
	if err != nil {
		return Run{}, err
	}
	for _, run := range state.Runs {
		if run.ID == id {
			return run, nil
		}
	}
	return Run{}, errors.New("runMissing")
}

func (c *Client) start(execution Execution, previous string) (Run, error) {
	if execution.Fields == nil {
		execution.Fields = []string{}
	}
	if execution.Values == nil {
		execution.Values = map[string]string{}
	}
	if err := execution.Validate(); err != nil {
		return Run{}, err
	}
	if execution.Platform == "Windows" && runtime.GOOS != "windows" || execution.Platform == "macOS" && runtime.GOOS != "darwin" {
		return Run{}, errors.New("platformMismatch")
	}
	lock, err := fileLock(filepath.Join(c.dir, "executions.lock"))
	if err != nil {
		return Run{}, err
	}
	defer lock.Close()
	var existing executionManifest
	if err = readJSON(c.runPath(execution.ID, "manifest.json"), &existing); err == nil {
		if existing.Schema != ExecutionSchema || !reflect.DeepEqual(existing.Execution, execution) || existing.Previous != previous {
			return Run{}, errors.New("requestIdConflict")
		}
		state, err := c.readState(execution.ID)
		return state.Run, err
	} else if !os.IsNotExist(err) {
		return Run{}, err
	}
	runs, err := c.runs()
	if err != nil {
		return Run{}, err
	}
	count := 0
	for _, run := range runs {
		if run.ID == execution.ID {
			return Run{}, errors.New("requestIdConflict")
		}
		// Unknown records retain their history but do not reserve a command
		// forever. A new explicit start is a separate execution, never recovery
		// or replay of the unowned process.
		if activeStatus(run.Status) {
			if run.CommandID == execution.CommandID && run.ID != previous {
				return run, errors.New("alreadyRunning")
			}
			count++
		}
	}
	if count >= 16 {
		return Run{}, errors.New("tooManyRuns")
	}
	info, err := os.Stat(execution.Directory)
	if err != nil || !info.IsDir() {
		return Run{}, errors.New("invalidDirectory")
	}
	_, expanded, err := shellCommand(Command{Text: execution.Text, Shell: execution.Shell, Fields: execution.Fields}, Launch{Directory: execution.Directory, Values: execution.Values})
	if err != nil {
		return Run{}, err
	}
	binary, err := resolveExecutor(c.executable)
	if err != nil {
		return Run{}, err
	}
	// Environment values travel only through process inheritance, never the journal.
	manifest := executionManifest{Schema: ExecutionSchema, Execution: execution, Previous: previous, Executor: binary}
	state := executionState{Schema: ExecutionSchema, Phase: "pending", Run: Run{ID: execution.ID, CommandID: execution.CommandID, Title: execution.Title, Text: expanded, Template: execution.Text, Directory: execution.Directory, Values: execution.Values, Shell: execution.Shell, Time: time.Now().UTC().Format(time.RFC3339Nano), Status: "running", OmitOutput: execution.OmitOutput}}
	// Publish a complete directory in one rename, before starting any process.
	temp, err := os.MkdirTemp(filepath.Join(c.dir, "executions"), ".pending-")
	if err != nil {
		return Run{}, err
	}
	defer func() {
		os.Remove(filepath.Join(temp, "manifest.json"))
		os.Remove(filepath.Join(temp, "state.json"))
		os.Remove(temp)
	}()
	if err = atomicJSON(filepath.Join(temp, "manifest.json"), manifest); err != nil {
		return Run{}, err
	}
	if err = atomicJSON(filepath.Join(temp, "state.json"), state); err != nil {
		return Run{}, err
	}
	if err = os.Rename(temp, c.runPath(execution.ID, "")); err != nil {
		return Run{}, err
	}
	launchLog, err := openOutput(c.runPath(execution.ID, "executor.log"), execution.OmitOutput, os.O_CREATE|os.O_APPEND|os.O_WRONLY)
	if err == nil {
		defer launchLog.Close()
		cmd := exec.Command(binary, "_execute", "--data", c.dir, "--id", execution.ID)
		cmd.Dir = filepath.Dir(binary)
		cmd.Env = os.Environ()
		cmd.Stdout, cmd.Stderr = launchLog, launchLog
		prepareDetached(cmd)
		err = cmd.Start()
		if err == nil {
			go func() { _ = cmd.Wait() }()
		}
	}
	if err != nil {
		state.Phase, state.Run.Status, state.Error = "finished", "failed", err.Error()
		if saveErr := atomicJSON(c.runPath(execution.ID, "state.json"), state); saveErr != nil {
			return state.Run, saveErr
		}
		return state.Run, err
	}
	// Do not rerun if acknowledgement is lost. Worker writes never need this lock.
	for deadline := time.Now().Add(8 * time.Second); time.Now().Before(deadline); {
		if err = readJSON(c.runPath(execution.ID, "state.json"), &state); err != nil {
			return Run{}, err
		}
		if state.WorkerPID != 0 || terminalStatus(state.Run.Status) {
			return state.Run, nil
		}
		time.Sleep(25 * time.Millisecond)
	}
	return state.Run, errors.New("executionAcknowledgementTimeout")
}
func (c *Client) Stop(id string) error {
	state, err := c.readState(id)
	if err != nil {
		return err
	}
	if terminalStatus(state.Run.Status) {
		return nil
	}
	if state.Run.Status == "unknown" {
		return errors.New("runNotOwned")
	}
	return atomicJSON(c.runPath(id, "stop.json"), map[string]int{"schema": ExecutionSchema})
}
func (c *Client) Replay(id, previous string) (Run, error) {
	if !validID.MatchString(id) || !validID.MatchString(previous) || id == previous {
		return Run{}, errors.New("invalidRequest")
	}
	var accepted executionManifest
	if err := readJSON(c.runPath(id, "manifest.json"), &accepted); err == nil {
		if accepted.Schema != ExecutionSchema || accepted.Previous != previous {
			return Run{}, errors.New("requestIdConflict")
		}
		state, err := c.readState(id)
		return state.Run, err
	} else if !os.IsNotExist(err) {
		return Run{}, err
	}
	var original executionManifest
	if err := readJSON(c.runPath(previous, "manifest.json"), &original); err != nil {
		return Run{}, errors.New("replayUnavailable")
	}
	state, err := c.readState(previous)
	if err != nil || original.Schema != ExecutionSchema || state.Run.Status == "unknown" {
		return Run{}, errors.New("replayUnavailable")
	}
	original.Execution.ID = id
	return c.start(original.Execution, previous)
}
func (c *Client) Activity() (Activity, error) {
	state, err := c.State()
	result := Activity{ReplayDetached: true}
	for _, run := range state.Runs {
		if activeStatus(run.Status) {
			result.Active++
		}
		if run.Status == "unknown" {
			result.Unknown++
		}
	}
	return result, err
}
func (c *Client) StopAll() error {
	state, err := c.State()
	if err != nil {
		return err
	}
	var failures []error
	ids := []string{}
	for _, run := range state.Runs {
		if activeStatus(run.Status) {
			failures = append(failures, c.Stop(run.ID))
			ids = append(ids, run.ID)
		}
	}
	if err := errors.Join(failures...); err != nil {
		return err
	}
	for deadline := time.Now().Add(10 * time.Second); len(ids) > 0; {
		pending := []string{}
		for _, id := range ids {
			state, err := c.readState(id)
			if err != nil {
				return err
			}
			if state.Run.Status == "unknown" {
				return errors.New("runNotOwned")
			}
			if !terminalStatus(state.Run.Status) {
				pending = append(pending, id)
			}
		}
		ids = pending
		if len(ids) == 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return errors.New("stopTimeout")
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}
func decodeRequest(data []byte, value any) error {
	if len(data) > 2<<20 {
		return errors.New("invalidRequest")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if d.Decode(value) != nil || d.Decode(&struct{}{}) != io.EOF {
		return errors.New("invalidRequest")
	}
	return nil
}
func (c *Client) Request(method, path string, body []byte) (int, []byte, error) {
	var value any
	var err error
	switch method + " " + strings.SplitN(path, "?", 2)[0] {
	case "GET /api/state":
		value, err = c.State()
	case "GET /api/activity":
		value, err = c.Activity()
	case "GET /api/logs":
		var u *url.URL
		u, err = url.Parse(path)
		if err == nil {
			value, err = c.readLog(u.Query())
		}
	case "POST /api/run":
		var execution Execution
		if err = decodeRequest(body, &execution); err == nil {
			value, err = c.start(execution, "")
		}
	case "POST /api/stop":
		var request struct {
			ID string `json:"id"`
		}
		if err = decodeRequest(body, &request); err == nil {
			err = c.Stop(request.ID)
			value = map[string]bool{"ok": err == nil}
		}
	case "POST /api/stop-all":
		err = c.StopAll()
		value = map[string]bool{"ok": err == nil}
	case "POST /api/replay":
		var request struct {
			ID    string `json:"id"`
			RunID string `json:"runId"`
		}
		if err = decodeRequest(body, &request); err == nil {
			value, err = c.Replay(request.ID, request.RunID)
		}
	default:
		err = errors.New("invalidRequest")
	}
	status := 200
	if err != nil {
		status = 409
		failure := map[string]any{"error": err.Error()}
		if run, ok := value.(Run); ok && run.ID != "" {
			failure["run"] = run
		}
		value = failure
	}
	data, marshalErr := json.Marshal(value)
	return status, data, marshalErr
}
