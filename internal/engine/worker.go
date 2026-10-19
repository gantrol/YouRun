package engine

import (
	"commandmanager.local/core/internal/processcontrol"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ExecutionMode runs before any GUI, database or update initialization. Both
// packaged and go-run desktop hosts can execute commands using their own file.
func ExecutionMode() (bool, error) {
	if len(os.Args) < 2 || os.Args[1] != "_execute" {
		return false, nil
	}
	flags := flag.NewFlagSet("_execute", flag.ContinueOnError)
	directory := flags.String("data", "", "Data directory")
	id := flags.String("id", "", "Run identifier")
	if err := flags.Parse(os.Args[2:]); err != nil {
		return true, err
	}
	if *directory == "" || flags.NArg() != 0 {
		return true, errors.New("invalidRequest")
	}
	return true, ExecuteRun(*directory, *id)
}

// ExecuteRun is the private, per-execution entrypoint. It never opens the
// application's database, starts a server, or depends on a live requesting GUI.
func ExecuteRun(directory, id string) error {
	if !validID.MatchString(id) {
		return errors.New("invalidRequest")
	}
	c, err := Open(directory, "")
	if err != nil {
		return err
	}
	owner, err := fileLock(c.runPath(id, "owner.lock"))
	if err != nil {
		return err
	}
	defer owner.Close()
	var manifest executionManifest
	var state executionState
	if err = readJSON(c.runPath(id, "manifest.json"), &manifest); err != nil {
		return err
	}
	if err = readJSON(c.runPath(id, "state.json"), &state); err != nil {
		return err
	}
	if manifest.Schema != ExecutionSchema || state.Schema != ExecutionSchema {
		return errors.New("executionSchemaUnsupported")
	}
	if manifest.Execution.ID != id || state.Run.ID != id || manifest.Execution.Validate() != nil {
		return errors.New("invalidRequest")
	}
	// Never re-execute an uncertain intent. Only the first helper can claim it.
	if state.Phase != "pending" || state.WorkerPID != 0 {
		return errors.New("executionAlreadyClaimed")
	}
	started, err := time.Parse(time.RFC3339Nano, state.Run.Time)
	if err != nil || time.Since(started) >= 15*time.Second {
		return errors.New("executionIntentExpired")
	}
	state.WorkerPID, state.Phase = os.Getpid(), "preparing"
	save := func() error { return atomicJSON(c.runPath(id, "state.json"), state) }
	finish := func(status string, code *int, cause error) error {
		state.Phase, state.Run.Status, state.Run.ExitCode = "finished", status, code
		if cause != nil {
			state.Error = cause.Error()
			state.Run.Error = state.Error
		}
		return save()
	}
	if err = save(); err != nil {
		return err
	}
	stopRequested := func() (bool, error) {
		var request struct {
			Schema int `json:"schema"`
		}
		err := readJSON(c.runPath(id, "stop.json"), &request)
		if os.IsNotExist(err) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if request.Schema != ExecutionSchema {
			return false, errors.New("executionSchemaUnsupported")
		}
		return true, nil
	}
	if manifest.Previous != "" {
		state.Phase = "waiting"
		if err = save(); err != nil {
			return err
		}
		if err = c.Stop(manifest.Previous); err != nil {
			return finish("failed", nil, err)
		}
		for deadline := time.Now().Add(10 * time.Second); ; {
			stop, err := stopRequested()
			if err != nil {
				return finish("failed", nil, err)
			}
			if stop {
				return finish("stopped", nil, nil)
			}
			previous, err := c.readState(manifest.Previous)
			if err != nil {
				return finish("failed", nil, err)
			}
			if terminalStatus(previous.Run.Status) {
				break
			}
			if previous.Run.Status == "unknown" {
				return finish("failed", nil, errors.New("runNotOwned"))
			}
			if time.Now().After(deadline) {
				return finish("failed", nil, errors.New("restartTimeout"))
			}
			time.Sleep(100 * time.Millisecond)
		}
	}
	stop, err := stopRequested()
	if err != nil {
		return finish("failed", nil, err)
	}
	if stop {
		return finish("stopped", nil, nil)
	}
	e := manifest.Execution
	cmd, _, err := shellCommand(Command{Text: e.Text, Shell: e.Shell, Platform: e.Platform, Fields: e.Fields}, Launch{Directory: e.Directory, Values: e.Values})
	if err != nil {
		return finish("failed", nil, err)
	}
	log, err := openOutput(filepath.Join(c.dir, "logs", id+".log"), e.OmitOutput, os.O_CREATE|os.O_EXCL|os.O_WRONLY|os.O_APPEND)
	if err != nil {
		return finish("failed", nil, err)
	}
	defer log.Close()
	// Real file handles go directly to the child. No Go Writer/pipe is allowed:
	// killing this helper must not sever stdout/stderr or stop the command.
	state.GroupName = "Local\\YouRun-" + ID()
	process, group, err := startIndependent(cmd, log, state.GroupName)
	if err != nil {
		return finish("failed", nil, err)
	}
	defer process.close()
	defer group.close() // deliberately no kill-on-close or deferred stop
	state.Run.PID, state.Phase = process.pid, "executing"
	state.ControlVersion = 1
	if target, err := processcontrol.Observe(process.pid); err == nil {
		state.ProcessIdentity = target.StartedAt
	}
	if err = save(); err != nil {
		fmt.Fprintln(os.Stderr, "persist PID:", err)
	}
	type outcome struct {
		code int
		err  error
	}
	finished := make(chan outcome, 1)
	go func() { code, err := process.wait(); finished <- outcome{code, err} }()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	var result outcome
	var rootExitedAt time.Time
	rootFinished, stopping := false, false
	var gracefulDeadline time.Time
	holdGroup := false
	for {
		select {
		case result = <-finished:
			rootFinished = true
			rootExitedAt = time.Now()
		case <-ticker.C:
		}
		if rootFinished {
			active, err := group.alive()
			if err != nil {
				return finish("unknown", nil, err)
			}
			if !active || (!stopping && !holdGroup && (result.err != nil || time.Since(rootExitedAt) >= 500*time.Millisecond)) {
				break
			}
		}
		var graceful struct {
			ID string `json:"id"`
		}
		if err := readJSON(c.runPath(id, "graceful.json"), &graceful); err == nil && validID.MatchString(graceful.ID) && graceful.ID != state.ControlRequestID && !stopping {
			state.ControlRequestID, state.ControlError = graceful.ID, ""
			if err := group.graceful(); err != nil {
				state.ControlError = "processGracefulUnsupported"
			} else {
				holdGroup = true
				gracefulDeadline = time.Now().Add(8 * time.Second)
				state.Run.Status = "stopping"
			}
			_ = save()
		}
		if !gracefulDeadline.IsZero() && time.Now().After(gracefulDeadline) {
			gracefulDeadline = time.Time{}
			state.Run.Status, state.ControlError = "running", "processExitTimeout"
			_ = save()
		}
		if !stopping {
			requested, err := stopRequested()
			if err != nil {
				fmt.Fprintln(os.Stderr, "read stop request:", err)
				continue
			}
			if requested {
				if err = group.stop(); err != nil {
					fmt.Fprintln(os.Stderr, "stop process group:", err)
					continue
				}
				stopping, state.Run.Status = true, "stopping"
				holdGroup = false
				gracefulDeadline = time.Time{}
				if err = save(); err != nil {
					fmt.Fprintln(os.Stderr, "persist stopping:", err)
				}
			}
		}
	}
	code, status := result.code, "success"
	if stopping || !gracefulDeadline.IsZero() {
		status = "stopped"
	} else if result.err != nil {
		status = "failed"
	}
	return finish(status, &code, nil)
}
