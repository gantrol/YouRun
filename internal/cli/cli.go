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
		var structured *StructuredError
		if asStructuredError(err, &structured) {
			_ = json.NewEncoder(os.Stdout).Encode(structured)
			return 2
		}
		msg, rec := defaultErrorGuidance(err.Error())
		_ = json.NewEncoder(os.Stdout).Encode(&StructuredError{
			Code:     err.Error(),
			Message:  msg,
			Recovery: rec,
		})
		return 2
	}
	return 0
}

type cliExit struct{ code int }

func (e *cliExit) Error() string { return "command failed" }

// ResourceHandler allows the desktop package to attach OS resource queries to the CLI.
var ResourceHandler func(ctx context.Context, mode string, dataDir string, ws *workspace.Service, targetPath string, recursive bool) (any, error)

func run(args []string) error {
	mode := "status"
	if len(args) > 0 {
		mode, args = args[0], args[1:]
	}
	if mode == "help" || mode == "--help" || mode == "-h" {
		fmt.Println(`YouRun — non-interactive command management & Agent runtime
Desktop: no arguments open the window; cli without a subcommand returns status.
  context       One-shot situational awareness [--directory PATH] [--project ID|NAME]
  capabilities  Self-discovery metadata, environment, workflows, and tool catalog
  describe      Detailed command/error schema [SUBCOMMAND]
  mcp           Start embedded Model Context Protocol (MCP) server over stdio
  integrate     Install/update or remove global AI Agent skills, MCP configs, and PATH [--enable|--disable|--status]
  run           Launch the saved run command [--project ID|NAME] [--directory PATH] [--command ID]
  dev           Launch the saved dev/debug command [--project ID|NAME] [--directory PATH] [--command ID]
  start         Launch a saved command [--command ID] [--project ID|NAME] [--directory PATH]
  stop          Stop an active run [--id ID] [--command ID] [--project ID|NAME] [--directory PATH]
  restart       Replay an owned run [--id ORIGINAL] [--request-id NEW] [--command ID] [--project ID|NAME]
  stop-all      Request termination of all owned executions
  status        Application snapshot, or one execution with --id
  list          Execution records [--query TEXT] [--limit N]
  search        Search projects, collections, and commands [--query TEXT] [--project ID|NAME] [--hidden]
  logs          [--id ID] [--project ID|NAME] [--directory PATH] [--tail BYTES] [--cursor BYTES] [--limit BYTES] [--omit-data]
  wait          [--id ID] [--project ID|NAME] [--directory PATH] [--timeout 30s]; waiting never owns or cancels the command
  activity      Counts of active and unknown executions
  processes     Inspect processes & YouRun attribution [--pid PID] [--port PORT] [--ownership TYPE] [--project ID|NAME] [--query TEXT] [--limit N]
  ports         Inspect local TCP/UDP ports & YouRun attribution [--port PORT] [--protocol tcp|udp] [--ownership TYPE] [--project ID|NAME] [--query TEXT] [--limit N]
  files         Inspect processes holding a file/dir [PATH] [--directory PATH] [--recursive] [--ownership TYPE] [--query TEXT] [--limit N]
  discover / inspect / source [--directory PATH]
All commands accept --data PATH. Launch commands execute only saved definitions.
Exit codes: 0 success/accepted, 1 failed/stopped task, 2 request error,
3 unknown execution outcome, 124 wait timeout. Responses are JSON.
No GUI, resident service, connection approval, or interactive stdin is required.`)
		return nil
	}
	switch mode {
	case "_execute", "context", "capabilities", "agent-info", "describe", "schema", "mcp", "integrate", "discover", "inspect", "source", "processes", "ports", "files", "wait", "status", "list", "search", "activity", "stop-all", "logs", "restart", "replay", "run", "dev", "start", "stop":
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
	tail := flags.Int64("tail", 0, "Read the last N bytes from the end of the log")
	limit := flags.Int64("limit", 128<<10, "Maximum log bytes (up to 1 MiB) or item count limit")
	omitData := flags.Bool("omit-data", false, "Omit base64 byte array from logs output")
	timeout := flags.Duration("timeout", 0, "Wait timeout (0 waits indefinitely)")
	project := flags.String("directory", "", "Project or target directory")
	projectName := flags.String("name", "", "Display name used to match an application during inspection")
	platform := flags.String("platform", "", "Icon platform: windows, darwin or linux")
	includeImages := flags.Bool("include-images", false, "Include encoded icons in inspect/resource output")
	query := flags.String("query", "", "Search query for search, list, ports, processes, or files")
	projectFilter := flags.String("project", "", "Filter or select by project ID or name")
	showHidden := flags.Bool("hidden", false, "Include hidden projects and commands in search")
	recursive := flags.Bool("recursive", false, "Include subdirectories when querying files")
	portFilter := flags.Int("port", 0, "Filter ports or processes by local port number")
	pidFilter := flags.Int("pid", 0, "Filter processes, ports, or files by PID")
	protocolFilter := flags.String("protocol", "", "Filter ports by tcp or udp")
	stateFilter := flags.String("state", "", "Filter ports by state (e.g. listening, established)")
	ownershipFilter := flags.String("ownership", "", "Filter OS resources by managed, unconfirmed, external, or yourun")
	enableFlag := flags.Bool("enable", false, "Enable selected AI Agent integration target(s)")
	disableFlag := flags.Bool("disable", false, "Disable and revoke selected AI Agent integration target(s)")
	statusFlag := flags.Bool("status", false, "Read AI Agent integration status without modifying files")
	targetFlag := flags.String("target", "", "Comma-separated AI Agent integration target IDs (e.g. user-path,skill-gemini,mcp-cursor or all)")
	if err = flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	visited := map[string]bool{}
	flags.Visit(func(f *flag.Flag) {
		visited[f.Name] = true
	})

	describeTarget := ""
	integrateAction := "status"
	switch mode {
	case "search", "list", "ports", "processes":
		if *query == "" && flags.NArg() > 0 {
			*query = strings.Join(flags.Args(), " ")
		} else if flags.NArg() != 0 {
			return fmt.Errorf("invalid arguments")
		}
	case "files":
		if *project == "" && flags.NArg() == 1 {
			*project = flags.Arg(0)
		} else if flags.NArg() != 0 {
			return fmt.Errorf("invalid arguments")
		}
	case "describe", "schema":
		if flags.NArg() == 1 {
			describeTarget = flags.Arg(0)
		} else if flags.NArg() == 0 {
			if *commandID != "" {
				describeTarget = *commandID
			} else if *query != "" {
				describeTarget = *query
			}
		} else {
			return fmt.Errorf("invalid arguments")
		}
	case "integrate":
		if *enableFlag && *disableFlag {
			return fmt.Errorf("invalid arguments")
		}
		if *statusFlag {
			integrateAction = "status"
		} else if *disableFlag {
			integrateAction = "disable"
		} else if *enableFlag {
			integrateAction = "enable"
		} else if flags.NArg() == 1 {
			switch strings.ToLower(strings.TrimSpace(flags.Arg(0))) {
			case "status":
				integrateAction = "status"
			case "disable", "off", "remove", "uninstall", "revoke":
				integrateAction = "disable"
			case "enable", "on", "install":
				integrateAction = "enable"
			default:
				return fmt.Errorf("invalid arguments")
			}
		} else if flags.NArg() != 0 {
			return fmt.Errorf("invalid arguments")
		}
	default:
		if flags.NArg() != 0 {
			return fmt.Errorf("invalid arguments")
		}
	}
	if *timeout < 0 {
		return fmt.Errorf("invalid arguments")
	}
	// This branch precedes opening workspace/SQLite. Old workers remain immune
	// to application schema migrations and do not initialize a GUI or listener.
	if mode == "_execute" {
		return engine.ExecuteRun(*dir, *id)
	}
	if mode == "describe" || mode == "schema" {
		res, dErr := BuildDescribeReport(describeTarget)
		if dErr != nil {
			return dErr
		}
		return json.NewEncoder(os.Stdout).Encode(res)
	}
	if mode == "capabilities" || mode == "agent-info" {
		res, cErr := BuildCapabilitiesReport(context.Background(), *dir)
		if cErr != nil {
			return cErr
		}
		return json.NewEncoder(os.Stdout).Encode(res)
	}
	if mode == "integrate" {
		var res AgentIntegrationSettings
		var iErr error
		if integrateAction == "status" {
			res, iErr = ReadAgentIntegration(context.Background(), *dir)
		} else {
			var targetIDs []string
			if strings.TrimSpace(*targetFlag) != "" {
				targetIDs = []string{*targetFlag}
			} else if integrateAction == "disable" {
				targetIDs = []string{"all"}
			}
			res, iErr = WriteAgentIntegrationTargets(context.Background(), *dir, targetIDs, integrateAction == "enable")
		}
		if iErr != nil {
			return iErr
		}
		return json.NewEncoder(os.Stdout).Encode(res)
	}
	if mode == "mcp" {
		return runMCPServer(*dir)
	}
	if mode == "discover" || mode == "inspect" || mode == "source" {
		targetDir := strings.TrimSpace(*project)
		if targetDir == "" {
			targetDir, err = os.Getwd()
			if err != nil {
				return err
			}
		} else {
			targetDir, err = filepath.Abs(targetDir)
			if err != nil {
				return errors.New("invalidDirectory")
			}
		}
		if mode == "source" {
			return json.NewEncoder(os.Stdout).Encode(engine.InspectSource(context.Background(), targetDir, "", "", true))
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
		runs := filterRuns(state.Runs, *query)
		if visited["limit"] && *limit > 0 && int(*limit) < len(runs) {
			runs = runs[:int(*limit)]
		}
		return json.NewEncoder(os.Stdout).Encode(runs)
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
	if mode == "logs" && *id != "" && !visited["tail"] && !*omitData {
		var path string
		if visited["cursor"] {
			path = fmt.Sprintf("/api/logs?id=%s&cursor=%d&limit=%d", url.QueryEscape(*id), *cursor, *limit)
		} else {
			path = fmt.Sprintf("/api/logs?id=%s&limit=%d", url.QueryEscape(*id), *limit)
		}
		status, data, err := c.Request("GET", path, nil)
		if err != nil {
			return err
		}
		if status >= 400 {
			fmt.Println(string(enrichErrorJSON(data)))
			return &cliExit{code: 2}
		}
		fmt.Println(string(data))
		return nil
	}

	service, openErr := workspace.New(*dir, c.Request)
	if openErr != nil {
		return openErr
	}
	if mode == "context" {
		report, ctxErr := BuildContextReport(context.Background(), *dir, c, service, *projectFilter, *project)
		if ctxErr != nil {
			return ctxErr
		}
		return json.NewEncoder(os.Stdout).Encode(report)
	}
	if mode == "wait" {
		targetID := strings.TrimSpace(*id)
		if targetID == "" {
			state, stateErr := loadState(service)
			if stateErr != nil {
				return stateErr
			}
			targetID, err = resolveTargetRunID("logs", state, *dir, *id, *commandID, *projectFilter, *project)
			if err != nil {
				return err
			}
		}
		return waitRun(c, targetID, *timeout)
	}
	if mode == "logs" {
		targetID := strings.TrimSpace(*id)
		if targetID == "" {
			state, stateErr := loadState(service)
			if stateErr != nil {
				return stateErr
			}
			targetID, err = resolveTargetRunID("logs", state, *dir, *id, *commandID, *projectFilter, *project)
			if err != nil {
				return err
			}
		}
		cursorVal := int64(-1)
		if visited["cursor"] {
			cursorVal = *cursor
		}
		chunk, logErr := readLogClean(c, targetID, *tail, cursorVal, *limit, *omitData)
		if logErr != nil {
			return logErr
		}
		return json.NewEncoder(os.Stdout).Encode(chunk)
	}
	if mode == "search" {
		return searchWorkspace(service, *query, *projectFilter, *showHidden)
	}
	if mode == "processes" || mode == "ports" || mode == "files" {
		targetPath := strings.TrimSpace(*project)
		if mode == "files" && targetPath == "" {
			targetPath, err = os.Getwd()
			if err != nil {
				return err
			}
		}
		itemLimit := 0
		if visited["limit"] && *limit > 0 {
			itemLimit = int(*limit)
		}
		res, resErr := QueryAndFilterResources(context.Background(), mode, *dir, service, targetPath, *recursive, ResourceFilterOptions{
			Port:          *portFilter,
			PID:           *pidFilter,
			Protocol:      *protocolFilter,
			State:         *stateFilter,
			Ownership:     *ownershipFilter,
			Project:       *projectFilter,
			Query:         *query,
			Limit:         itemLimit,
			IncludeImages: *includeImages,
		})
		if resErr != nil {
			return resErr
		}
		return json.NewEncoder(os.Stdout).Encode(res)
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
	if status >= 400 {
		fmt.Println(string(enrichErrorJSON(data)))
		return &cliExit{code: 2}
	}
	fmt.Println(string(data))
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
			msg, rec := defaultErrorGuidance("waitTimeout")
			if err = json.NewEncoder(os.Stdout).Encode(map[string]any{"error": "waitTimeout", "message": msg, "recovery": rec, "run": run}); err != nil {
				return err
			}
			return &cliExit{code: 124}
		}
		time.Sleep(100 * time.Millisecond)
	}
}
