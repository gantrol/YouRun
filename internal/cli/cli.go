// Package cli implements the public command line shared by desktop entrypoints.
package cli

import (
	"commandmanager.local/core/internal/discovery"
	"commandmanager.local/core/internal/engine"
	"commandmanager.local/core/internal/workspace"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Main leaves process exit to the caller and never initializes a GUI or host.
func Main(args []string) int {
	if len(args) > 0 && args[0] == "cli" {
		args = args[1:]
	}
	if len(args) == 0 || args[0] != "_execute" {
		prepareConsole()
	}
	if err := run(args); err != nil {
		var failure *cliExit
		if errors.As(err, &failure) {
			return failure.code
		}
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{"error": err.Error()})
		return 2
	}
	return 0
}

type cliExit struct{ code int }

func (e *cliExit) Error() string { return "command failed" }

func run(args []string) error {
	mode := "status"
	if len(args) > 0 {
		mode, args = args[0], args[1:]
	}
	if mode == "help" || mode == "--help" || mode == "-h" {
		fmt.Println(`YouRun — non-interactive command management
Desktop: no arguments open the window; cli without a subcommand returns status.
  run        Launch the saved run command [--project ID|NAME] [--directory PATH] [--command ID]
  dev        Launch the saved dev/debug command [--project ID|NAME] [--directory PATH] [--command ID]
  start      Launch a saved command [--command ID] [--project ID|NAME] [--directory PATH]
  stop       Stop an active run [--id ID] [--command ID] [--project ID|NAME] [--directory PATH]
  restart    Replay an owned run [--id ORIGINAL] [--request-id NEW] [--command ID] [--project ID|NAME]
  stop-all   Request termination of all owned executions
  status     Application snapshot, or one execution with --id
  list       Execution records [--query TEXT]
  search     Search projects, collections, and commands [--query TEXT] [--project ID|NAME] [--hidden]
  logs       --id ID [--cursor BYTES] [--limit BYTES]; text and lossless base64 data
  wait       --id ID [--timeout 30s]; waiting never owns or cancels the command
  activity   Counts of active and unknown executions
  discover / inspect [--directory PATH]
All commands accept --data PATH. Launch commands execute only saved definitions.
Exit codes: 0 success/accepted, 1 failed/stopped task, 2 request error,
3 unknown execution outcome, 124 wait timeout. Responses are JSON.
No GUI, resident service, connection approval, or interactive stdin is required.`)
		return nil
	}
	switch mode {
	case "_execute", "discover", "inspect", "wait", "status", "list", "search", "activity", "stop-all", "logs", "restart", "replay", "run", "dev", "start", "stop":
	default:
		return fmt.Errorf("unknown command %q", mode)
	}
	flags := flag.NewFlagSet(mode, flag.ContinueOnError)
	dirDefault, err := engine.DataDir()
	if err != nil {
		return err
	}
	dir := flags.String("data", dirDefault, "Local data directory")
	id := flags.String("id", "", "Run identifier")
	commandID := flags.String("command", "", "Saved command identifier")
	requestID := flags.String("request-id", "", "Idempotency identifier for a restart")
	cursor := flags.Int64("cursor", 0, "Log byte offset")
	limit := flags.Int64("limit", 128<<10, "Maximum log bytes, up to 1 MiB")
	timeout := flags.Duration("timeout", 0, "Wait timeout (0 waits indefinitely)")
	project := flags.String("directory", "", "Project directory")
	projectName := flags.String("name", "", "Display name used to match an application during inspection")
	platform := flags.String("platform", "", "Icon platform: windows, darwin or linux")
	includeImages := flags.Bool("include-images", false, "Include encoded icons in inspect output")
	query := flags.String("query", "", "Search query for search or list")
	projectFilter := flags.String("project", "", "Filter or select by project ID or name")
	showHidden := flags.Bool("hidden", false, "Include hidden projects and commands in search")
	if err = flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if mode == "search" && *query == "" && flags.NArg() > 0 {
		*query = strings.Join(flags.Args(), " ")
	} else if flags.NArg() != 0 {
		return fmt.Errorf("invalid arguments")
	}
	if *timeout < 0 {
		return fmt.Errorf("invalid arguments")
	}
	// This branch precedes opening workspace/SQLite. Old workers remain immune
	// to application schema migrations and do not initialize a GUI or listener.
	if mode == "_execute" {
		return engine.ExecuteRun(*dir, *id)
	}
	if mode == "discover" || mode == "inspect" {
		targetDir := strings.TrimSpace(*project)
		if targetDir == "" {
			targetDir, err = os.Getwd()
			if err != nil {
				return err
			}
		}
		rules, rulesErr := discovery.LoadRegistry(filepath.Join(*dir, "discovery-rules"))
		if rulesErr != nil {
			return rulesErr
		}
		if mode == "inspect" {
			if *platform != "" && *platform != "windows" && *platform != "darwin" && *platform != "linux" {
				return fmt.Errorf("invalid platform")
			}
			result, inspectErr := discovery.Inspect(context.Background(), targetDir, discovery.InspectOptions{Name: *projectName, Platform: *platform, Registry: rules})
			if inspectErr != nil {
				return inspectErr
			}
			if !*includeImages {
				for i := range result.Icons {
					result.Icons[i].Data = ""
				}
			}
			return json.NewEncoder(os.Stdout).Encode(result)
		}
		result, discoverErr := discovery.DiscoverWithRegistry(context.Background(), targetDir, rules)
		if discoverErr != nil {
			return discoverErr
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	c, err := engine.Open(*dir, exe)
	if err != nil {
		return err
	}
	if mode == "wait" {
		return waitRun(c, *id, *timeout)
	}
	if mode == "status" && *id != "" {
		run, err := c.Run(*id)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(run)
	}
	if mode == "list" {
		state, err := c.State()
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(filterRuns(state.Runs, *query))
	}
	if mode == "activity" {
		activity, activityErr := c.Activity()
		if activityErr != nil {
			return activityErr
		}
		return json.NewEncoder(os.Stdout).Encode(activity)
	}
	if mode == "stop-all" {
		if err = c.StopAll(); err != nil {
			return err
		}
		activity, err := c.Activity()
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]any{"ok": true, "active": activity.Active, "unknown": activity.Unknown})
	}
	if mode == "logs" {
		path := fmt.Sprintf("/api/logs?id=%s&cursor=%d&limit=%d", url.QueryEscape(*id), *cursor, *limit)
		status, data, err := c.Request("GET", path, nil)
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		if status >= 400 {
			return &cliExit{code: 2}
		}
		return nil
	}

	service, openErr := workspace.New(*dir, c.Request)
	if openErr != nil {
		return openErr
	}
	if mode == "search" {
		return searchWorkspace(service, *query, *projectFilter, *showHidden)
	}

	method, path := "GET", "/api/state"
	var body []byte
	useEngineDirect := false

	switch mode {
	case "status":
	case "run", "dev", "start":
		method, path = "POST", "/api/run"
		state, stateErr := loadState(service)
		if stateErr != nil {
			return stateErr
		}
		cmd, resolveErr := resolveLaunchCommand(mode, state, *dir, *commandID, *projectFilter, *project)
		if resolveErr != nil {
			return resolveErr
		}
		runID := strings.TrimSpace(*id)
		if runID == "" {
			runID = engine.ID()
		}
		// Never override a saved command's configured directory from the CLI.
		launch := engine.Launch{ID: runID, CommandID: cmd.ID, Directory: "", Values: map[string]string{}}
		body, err = json.Marshal(launch)
	case "stop":
		method, path = "POST", "/api/stop"
		useEngineDirect = true
		targetID := strings.TrimSpace(*id)
		if targetID == "" {
			state, stateErr := loadState(service)
			if stateErr != nil {
				return stateErr
			}
			targetID, err = resolveTargetRunID(mode, state, *dir, *id, *commandID, *projectFilter, *project)
			if err != nil {
				return err
			}
		}
		body, err = json.Marshal(map[string]string{"id": targetID})
	case "restart", "replay":
		method, path = "POST", "/api/replay"
		useEngineDirect = true
		targetID := strings.TrimSpace(*id)
		if targetID == "" {
			state, stateErr := loadState(service)
			if stateErr != nil {
				return stateErr
			}
			targetID, err = resolveTargetRunID(mode, state, *dir, *id, *commandID, *projectFilter, *project)
			if err != nil {
				return err
			}
		}
		reqID := strings.TrimSpace(*requestID)
		if reqID == "" {
			reqID = engine.ID()
		}
		body, err = json.Marshal(map[string]string{"id": reqID, "runId": targetID})
	}
	if err != nil {
		return err
	}
	var status int
	var data []byte
	if useEngineDirect {
		status, data, err = c.Request(method, path, body)
	} else {
		status, data, err = service.Request(context.Background(), method, path, body)
	}
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	if status >= 400 {
		return &cliExit{code: 2}
	}
	return nil
}

func waitRun(c *engine.Client, id string, timeout time.Duration) error {
	started := time.Now()
	for {
		run, err := c.Run(id)
		if err != nil {
			return err
		}
		switch run.Status {
		case "success", "handedOff":
			return json.NewEncoder(os.Stdout).Encode(run)
		case "failed", "stopped", "unknown":
			if err = json.NewEncoder(os.Stdout).Encode(run); err != nil {
				return err
			}
			code := 1
			if run.Status == "unknown" {
				code = 3
			}
			return &cliExit{code: code}
		}
		if timeout > 0 && time.Since(started) >= timeout {
			if err = json.NewEncoder(os.Stdout).Encode(map[string]any{"error": "waitTimeout", "run": run}); err != nil {
				return err
			}
			return &cliExit{code: 124}
		}
		time.Sleep(100 * time.Millisecond)
	}
}
