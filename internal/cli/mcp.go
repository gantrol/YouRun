package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"commandmanager.local/core/internal/discovery"
	"commandmanager.local/core/internal/engine"
	"commandmanager.local/core/internal/updater"
	"commandmanager.local/core/internal/workspace"
)

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  any             `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type mcpToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpCallToolResult struct {
	Content []mcpToolContent `json:"content"`
	IsError bool             `json:"isError,omitempty"`
}

func runMCPServer(dataDir string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	reader := bufio.NewReaderSize(os.Stdin, 256<<10)
	writer := bufio.NewWriterSize(os.Stdout, 256<<10)

	for {
		payload, framing, err := readMCPMessage(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		if len(bytes.TrimSpace(payload)) == 0 {
			continue
		}
		var req jsonRPCRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			resp := jsonRPCResponse{
				JSONRPC: "2.0",
				ID:      json.RawMessage("null"),
				Error:   &jsonRPCError{Code: -32700, Message: "Parse error"},
			}
			_ = writeMCPMessage(writer, resp, framing)
			continue
		}
		// Notifications do not have an ID and must not receive a response.
		if len(req.ID) == 0 || string(req.ID) == "null" && strings.HasPrefix(req.Method, "notifications/") {
			continue
		}
		resp := handleMCPRequest(context.Background(), dataDir, exe, req)
		if err := writeMCPMessage(writer, resp, framing); err != nil {
			return err
		}
	}
}

// readMCPMessage supports both newline-delimited JSON-RPC (MCP 2024-11-05 / 2025-03-26 standard)
// and Content-Length HTTP-header framed JSON-RPC.
func readMCPMessage(r *bufio.Reader) ([]byte, string, error) {
	for {
		b, err := r.Peek(1)
		if err != nil {
			return nil, "", err
		}
		if b[0] == '\r' || b[0] == '\n' || b[0] == ' ' || b[0] == '\t' {
			_, _ = r.ReadByte()
			continue
		}
		break
	}
	peek, _ := r.Peek(15)
	if strings.HasPrefix(strings.ToLower(string(peek)), "content-length:") {
		contentLength := -1
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return nil, "header", err
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break
			}
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 && strings.EqualFold(strings.TrimSpace(parts[0]), "content-length") {
				if n, nErr := strconv.Atoi(strings.TrimSpace(parts[1])); nErr == nil && n >= 0 && n <= 16<<20 {
					contentLength = n
				}
			}
		}
		if contentLength < 0 {
			return nil, "header", errors.New("invalid Content-Length header")
		}
		buf := make([]byte, contentLength)
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, "header", err
		}
		return buf, "header", nil
	}
	line, err := r.ReadBytes('\n')
	if err != nil && (!errors.Is(err, io.EOF) || len(line) == 0) {
		return nil, "line", err
	}
	return bytes.TrimSpace(line), "line", nil
}

func writeMCPMessage(w *bufio.Writer, resp jsonRPCResponse, framing string) error {
	data, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if framing == "header" {
		if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(data)); err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
	} else {
		if _, err := w.Write(data); err != nil {
			return err
		}
		if err := w.WriteByte('\n'); err != nil {
			return err
		}
	}
	return w.Flush()
}

func handleMCPRequest(ctx context.Context, dataDir, exe string, req jsonRPCRequest) jsonRPCResponse {
	switch req.Method {
	case "initialize":
		var params struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		_ = json.Unmarshal(req.Params, &params)
		proto := "2024-11-05"
		if params.ProtocolVersion == "2025-03-26" || params.ProtocolVersion == "2024-11-05" {
			proto = params.ProtocolVersion
		}
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": proto,
				"capabilities": map[string]any{
					"tools":     map[string]any{},
					"resources": map[string]any{},
					"prompts":   map[string]any{},
				},
				"serverInfo": map[string]any{
					"name":    "yourun",
					"version": updater.Version,
				},
				"instructions": "YouRun manages project commands, detached executions, logs, and OS resource attribution (ports, processes, open files). Start with `yourun_context` to inspect a directory or project's saved commands, active runs, live Git source drift (`restartRecommended`), and held ports in one call. YouRun launches only saved commands (`run`, `dev`, `start`) and only stops/restarts owned runs.",
			},
		}
	case "ping":
		return jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{}}
	case "tools/list":
		return jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"tools": mcpToolsSpec()}}
	case "tools/call":
		res, rpcErr := handleMCPToolCall(ctx, dataDir, exe, req.Params)
		if rpcErr != nil {
			return jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: rpcErr}
		}
		return jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: res}
	case "resources/list":
		return jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"resources": mcpStaticResources()}}
	case "resources/templates/list":
		return jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"resourceTemplates": mcpResourceTemplates()}}
	case "resources/read":
		res, rpcErr := handleMCPResourceRead(ctx, dataDir, exe, req.Params)
		if rpcErr != nil {
			return jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: rpcErr}
		}
		return jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: res}
	case "prompts/list":
		return jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: map[string]any{"prompts": mcpPromptsList()}}
	case "prompts/get":
		res, rpcErr := handleMCPPromptGet(req.Params)
		if rpcErr != nil {
			return jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Error: rpcErr}
		}
		return jsonRPCResponse{JSONRPC: "2.0", ID: req.ID, Result: res}
	default:
		return jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonRPCError{Code: -32601, Message: "Method not found: " + req.Method},
		}
	}
}

func mcpToolsSpec() []map[string]any {
	return []map[string]any{
		{
			"name":        "yourun_context",
			"description": "One-shot situational awareness for a project or directory: returns live Git source snapshot, matched YouRun projects & workspaces, resolved `run`/`dev`/`start` shortcut commands, all project commands, active runs with live code-drift comparison (`restartRecommended`) & held TCP/UDP ports, recent finished runs, and suggested next actions.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"directory": map[string]any{"type": "string", "description": "Target directory path (defaults to current working directory)."},
					"project":   map[string]any{"type": "string", "description": "Optional project ID or name filter."},
				},
			},
		},
		{
			"name":        "yourun_launch",
			"description": "Launch a saved command in YouRun (`run`, `dev`, or `start`). Executes only saved definitions in their configured working directory. Optionally set `waitTimeoutMs` > 0 to wait for short tasks/builds and return final status + log tail in a single call.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"mode":          map[string]any{"type": "string", "enum": []string{"run", "dev", "start"}, "default": "run", "description": "Launch mode: `run` (Play shortcut / run script), `dev` (Bolt shortcut / dev-watch script), or `start` (explicit `commandId` or single bound command)."},
					"commandId":     map[string]any{"type": "string", "description": "Explicit saved command ID."},
					"project":       map[string]any{"type": "string", "description": "Project ID or name used to select the project."},
					"directory":     map[string]any{"type": "string", "description": "Directory path used to match a project or command (never overrides the saved command's directory)."},
					"id":            map[string]any{"type": "string", "description": "Optional idempotency run ID."},
					"waitTimeoutMs": map[string]any{"type": "integer", "description": "Optional milliseconds to wait for completion (e.g. 15000 for builds). Leave 0 or omit for long-running dev servers."},
				},
			},
		},
		{
			"name":        "yourun_control",
			"description": "Control or query YouRun executions: `stop` an active owned run, `restart` (atomic stop + replay) an owned run, `stop-all` active runs, `wait` for a run to finish, or query `status` / `activity`.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"action"},
				"properties": map[string]any{
					"action":    map[string]any{"type": "string", "enum": []string{"stop", "restart", "stop-all", "wait", "status", "activity"}, "description": "Control action to perform."},
					"id":        map[string]any{"type": "string", "description": "Target run ID (auto-resolved from `commandId`/`project`/`directory` for `stop`, `restart`, and `wait` when omitted)."},
					"commandId": map[string]any{"type": "string", "description": "Saved command ID used to resolve the target run."},
					"project":   map[string]any{"type": "string", "description": "Project ID or name used to resolve the target run."},
					"directory": map[string]any{"type": "string", "description": "Directory path used to resolve the target run."},
					"requestId": map[string]any{"type": "string", "description": "Optional idempotency ID for `restart`."},
					"timeoutMs": map[string]any{"type": "integer", "description": "Timeout in milliseconds when `action` is `wait` (default 15000)."},
				},
			},
		},
		{
			"name":        "yourun_logs",
			"description": "Read execution output logs. Auto-resolves the active or most recent run in the project/directory when `id` is omitted, reads the latest tail by default, and omits base64 byte duplication to save context tokens.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":        map[string]any{"type": "string", "description": "Run ID (auto-resolved from `commandId`/`project`/`directory` if omitted)."},
					"commandId": map[string]any{"type": "string", "description": "Saved command ID to resolve the latest run."},
					"project":   map[string]any{"type": "string", "description": "Project ID or name to resolve the active/latest run."},
					"directory": map[string]any{"type": "string", "description": "Directory path to resolve the active/latest run."},
					"tail":      map[string]any{"type": "integer", "default": 16384, "description": "Read the last N bytes from the end of the log (1..1048576)."},
					"cursor":    map[string]any{"type": "integer", "description": "Explicit byte offset cursor for incremental pagination (`nextCursor`)."},
					"limit":     map[string]any{"type": "integer", "default": 16384, "description": "Maximum bytes to read when `cursor` is specified."},
				},
			},
		},
		{
			"name":        "yourun_search",
			"description": "Search saved projects, collections, and commands (`scope: \"workspace\"`) or historical execution records (`scope: \"runs\"`).",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"scope":         map[string]any{"type": "string", "enum": []string{"workspace", "runs"}, "default": "workspace", "description": "Search workspace definitions or execution history."},
					"query":         map[string]any{"type": "string", "description": "Search query terms."},
					"project":       map[string]any{"type": "string", "description": "Filter by project ID or name."},
					"includeHidden": map[string]any{"type": "boolean", "default": false, "description": "Include hidden projects and commands."},
					"limit":         map[string]any{"type": "integer", "default": 30, "description": "Maximum items to return."},
				},
			},
		},
		{
			"name":        "yourun_resources",
			"description": "Inspect OS processes, local TCP/UDP ports, or open file/directory handles with YouRun run/project/workspace attribution (`managed`, `unconfirmed`, `external`). Base64 process icons are stripped automatically.",
			"inputSchema": map[string]any{
				"type":     "object",
				"required": []string{"mode"},
				"properties": map[string]any{
					"mode":      map[string]any{"type": "string", "enum": []string{"ports", "processes", "files"}, "description": "Resource type to inspect."},
					"port":      map[string]any{"type": "integer", "description": "Filter `ports` or `processes` by port number (e.g. 3000)."},
					"pid":       map[string]any{"type": "integer", "description": "Filter by OS process ID."},
					"protocol":  map[string]any{"type": "string", "enum": []string{"tcp", "udp"}, "description": "Filter `ports` by protocol."},
					"state":     map[string]any{"type": "string", "description": "Filter `ports` by socket state (e.g. `listening`, `established`)."},
					"ownership": map[string]any{"type": "string", "enum": []string{"all", "managed", "unconfirmed", "external", "yourun"}, "description": "Filter by YouRun ownership (`yourun` matches `managed` + `unconfirmed`)."},
					"project":   map[string]any{"type": "string", "description": "Filter attributed items by project ID or name."},
					"directory": map[string]any{"type": "string", "description": "Target file or directory path when `mode` is `files` (defaults to cwd)."},
					"recursive": map[string]any{"type": "boolean", "default": false, "description": "Include subdirectories when `mode` is `files`."},
					"query":     map[string]any{"type": "string", "description": "Substring search across process name, command line, address, project, or file path."},
					"limit":     map[string]any{"type": "integer", "default": 50, "description": "Maximum items to return."},
				},
			},
		},
		{
			"name":        "yourun_discover",
			"description": "Read-only inspection of a project directory: `discover` candidate commands from manifests, `inspect` project display names/issues, or `source` read-only Git status.",
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"mode":      map[string]any{"type": "string", "enum": []string{"discover", "inspect", "source"}, "default": "discover", "description": "Inspection mode."},
					"directory": map[string]any{"type": "string", "description": "Target directory path (defaults to cwd)."},
					"name":      map[string]any{"type": "string", "description": "Optional application name hint for `inspect`."},
					"platform":  map[string]any{"type": "string", "enum": []string{"windows", "darwin", "linux"}, "description": "Optional platform filter for `inspect`."},
				},
			},
		},
	}
}

func formatMCPResult(value any, isError bool) mcpCallToolResult {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		data = []byte(fmt.Sprintf(`{"error": %q}`, err.Error()))
		isError = true
	}
	return mcpCallToolResult{
		Content: []mcpToolContent{{Type: "text", Text: string(data)}},
		IsError: isError,
	}
}

func formatMCPError(err error) mcpCallToolResult {
	var se *StructuredError
	if asStructuredError(err, &se) {
		return formatMCPResult(se, true)
	}
	msg, rec := defaultErrorGuidance(err.Error())
	return formatMCPResult(&StructuredError{
		Code:     err.Error(),
		Message:  msg,
		Recovery: rec,
	}, true)
}

func handleMCPToolCall(ctx context.Context, dataDir, exe string, rawParams json.RawMessage) (mcpCallToolResult, *jsonRPCError) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(rawParams, &params); err != nil || params.Name == "" {
		return mcpCallToolResult{}, &jsonRPCError{Code: -32602, Message: "Invalid tool call parameters"}
	}

	c, err := engine.Open(dataDir, exe)
	if err != nil {
		return formatMCPError(err), nil
	}
	service, err := workspace.New(dataDir, c.Request)
	if err != nil {
		return formatMCPError(err), nil
	}

	switch params.Name {
	case "yourun_context":
		var args struct {
			Directory string `json:"directory"`
			Project   string `json:"project"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		report, cErr := BuildContextReport(ctx, dataDir, c, service, args.Project, args.Directory)
		if cErr != nil {
			return formatMCPError(cErr), nil
		}
		return formatMCPResult(report, false), nil

	case "yourun_launch":
		var args struct {
			Mode          string `json:"mode"`
			CommandID     string `json:"commandId"`
			Project       string `json:"project"`
			Directory     string `json:"directory"`
			ID            string `json:"id"`
			WaitTimeoutMs int64  `json:"waitTimeoutMs"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		mode := strings.ToLower(strings.TrimSpace(args.Mode))
		if mode == "" {
			mode = "run"
		}
		if mode != "run" && mode != "dev" && mode != "start" {
			return formatMCPError(errors.New("invalidRequest")), nil
		}
		state, stateErr := loadState(service)
		if stateErr != nil {
			return formatMCPError(stateErr), nil
		}
		cmd, resolveErr := resolveLaunchCommand(mode, state, dataDir, args.CommandID, args.Project, args.Directory)
		if resolveErr != nil {
			return formatMCPError(resolveErr), nil
		}
		runID := strings.TrimSpace(args.ID)
		if runID == "" {
			runID = engine.ID()
		}
		launch := engine.Launch{ID: runID, CommandID: cmd.ID, Directory: "", Values: map[string]string{}}
		body, _ := json.Marshal(launch)
		status, data, reqErr := service.Request(ctx, "POST", "/api/run", body)
		if reqErr != nil {
			return formatMCPError(reqErr), nil
		}
		if status >= 400 {
			return mcpCallToolResult{
				Content: []mcpToolContent{{Type: "text", Text: string(enrichErrorJSON(data))}},
				IsError: true,
			}, nil
		}
		var launched engine.Run
		_ = json.Unmarshal(data, &launched)
		if args.WaitTimeoutMs > 0 {
			finalRun, timedOut, wErr := waitRunResult(c, launched.ID, time.Duration(args.WaitTimeoutMs)*time.Millisecond)
			if wErr != nil {
				return formatMCPError(wErr), nil
			}
			logChunk, _ := readLogClean(c, finalRun.ID, 16384, -1, 16384, true)
			return formatMCPResult(map[string]any{
				"run":      finalRun,
				"timedOut": timedOut,
				"logs":     logChunk,
			}, finalRun.Status == "failed" || finalRun.Status == "unknown"), nil
		}
		return formatMCPResult(launched, false), nil

	case "yourun_control":
		var args struct {
			Action    string `json:"action"`
			ID        string `json:"id"`
			CommandID string `json:"commandId"`
			Project   string `json:"project"`
			Directory string `json:"directory"`
			RequestID string `json:"requestId"`
			TimeoutMs int64  `json:"timeoutMs"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		action := strings.ToLower(strings.TrimSpace(args.Action))
		switch action {
		case "activity":
			act, aErr := c.Activity()
			if aErr != nil {
				return formatMCPError(aErr), nil
			}
			return formatMCPResult(act, false), nil
		case "status":
			if strings.TrimSpace(args.ID) != "" {
				r, rErr := c.Run(strings.TrimSpace(args.ID))
				if rErr != nil {
					return formatMCPError(rErr), nil
				}
				return formatMCPResult(r, false), nil
			}
			st, sErr := loadState(service)
			if sErr != nil {
				return formatMCPError(sErr), nil
			}
			return formatMCPResult(st, false), nil
		case "stop-all":
			if sErr := c.StopAll(); sErr != nil {
				return formatMCPError(sErr), nil
			}
			act, _ := c.Activity()
			return formatMCPResult(map[string]any{"ok": true, "active": act.Active, "unknown": act.Unknown}, false), nil
		case "stop":
			targetID := strings.TrimSpace(args.ID)
			if targetID == "" {
				st, sErr := loadState(service)
				if sErr != nil {
					return formatMCPError(sErr), nil
				}
				var rErr error
				targetID, rErr = resolveTargetRunID("stop", st, dataDir, args.ID, args.CommandID, args.Project, args.Directory)
				if rErr != nil {
					return formatMCPError(rErr), nil
				}
			}
			body, _ := json.Marshal(map[string]string{"id": targetID})
			status, data, reqErr := c.Request("POST", "/api/stop", body)
			if reqErr != nil {
				return formatMCPError(reqErr), nil
			}
			if status >= 400 {
				return mcpCallToolResult{Content: []mcpToolContent{{Type: "text", Text: string(enrichErrorJSON(data))}}, IsError: true}, nil
			}
			return formatMCPResult(map[string]any{"ok": true, "id": targetID}, false), nil
		case "restart":
			targetID := strings.TrimSpace(args.ID)
			if targetID == "" {
				st, sErr := loadState(service)
				if sErr != nil {
					return formatMCPError(sErr), nil
				}
				var rErr error
				targetID, rErr = resolveTargetRunID("restart", st, dataDir, args.ID, args.CommandID, args.Project, args.Directory)
				if rErr != nil {
					return formatMCPError(rErr), nil
				}
			}
			reqID := strings.TrimSpace(args.RequestID)
			if reqID == "" {
				reqID = engine.ID()
			}
			body, _ := json.Marshal(map[string]string{"id": reqID, "runId": targetID})
			status, data, reqErr := c.Request("POST", "/api/replay", body)
			if reqErr != nil {
				return formatMCPError(reqErr), nil
			}
			if status >= 400 {
				return mcpCallToolResult{Content: []mcpToolContent{{Type: "text", Text: string(enrichErrorJSON(data))}}, IsError: true}, nil
			}
			var replayed engine.Run
			_ = json.Unmarshal(data, &replayed)
			return formatMCPResult(replayed, false), nil
		case "wait":
			targetID := strings.TrimSpace(args.ID)
			if targetID == "" {
				st, sErr := loadState(service)
				if sErr != nil {
					return formatMCPError(sErr), nil
				}
				var rErr error
				targetID, rErr = resolveTargetRunID("logs", st, dataDir, args.ID, args.CommandID, args.Project, args.Directory)
				if rErr != nil {
					return formatMCPError(rErr), nil
				}
			}
			timeout := 15 * time.Second
			if args.TimeoutMs > 0 {
				timeout = time.Duration(args.TimeoutMs) * time.Millisecond
			}
			finalRun, timedOut, wErr := waitRunResult(c, targetID, timeout)
			if wErr != nil {
				return formatMCPError(wErr), nil
			}
			return formatMCPResult(map[string]any{
				"run":      finalRun,
				"timedOut": timedOut,
			}, timedOut || finalRun.Status == "failed" || finalRun.Status == "unknown"), nil
		default:
			return formatMCPError(errors.New("invalidRequest")), nil
		}

	case "yourun_logs":
		var args struct {
			ID        string `json:"id"`
			CommandID string `json:"commandId"`
			Project   string `json:"project"`
			Directory string `json:"directory"`
			Tail      int64  `json:"tail"`
			Cursor    *int64 `json:"cursor"`
			Limit     int64  `json:"limit"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		targetID := strings.TrimSpace(args.ID)
		if targetID == "" {
			st, sErr := loadState(service)
			if sErr != nil {
				return formatMCPError(sErr), nil
			}
			var rErr error
			targetID, rErr = resolveTargetRunID("logs", st, dataDir, args.ID, args.CommandID, args.Project, args.Directory)
			if rErr != nil {
				return formatMCPError(rErr), nil
			}
		}
		cursorVal := int64(-1)
		if args.Cursor != nil {
			cursorVal = *args.Cursor
		}
		tail := args.Tail
		if tail <= 0 && cursorVal < 0 {
			tail = 16384
		}
		limit := args.Limit
		if limit <= 0 {
			limit = 16384
		}
		chunk, lErr := readLogClean(c, targetID, tail, cursorVal, limit, true)
		if lErr != nil {
			return formatMCPError(lErr), nil
		}
		return formatMCPResult(chunk, false), nil

	case "yourun_search":
		var args struct {
			Scope         string `json:"scope"`
			Query         string `json:"query"`
			Project       string `json:"project"`
			IncludeHidden bool   `json:"includeHidden"`
			Limit         int    `json:"limit"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		if strings.EqualFold(args.Scope, "runs") {
			st, sErr := c.State()
			if sErr != nil {
				return formatMCPError(sErr), nil
			}
			runs := filterRuns(st.Runs, args.Query)
			if args.Limit > 0 && len(runs) > args.Limit {
				runs = runs[:args.Limit]
			}
			return formatMCPResult(map[string]any{"runs": runs, "count": len(runs)}, false), nil
		}
		res, sErr := buildSearchResponse(service, args.Query, args.Project, args.IncludeHidden)
		if sErr != nil {
			return formatMCPError(sErr), nil
		}
		if args.Limit > 0 && len(res.Commands) > args.Limit {
			res.Commands = res.Commands[:args.Limit]
		}
		return formatMCPResult(res, false), nil

	case "yourun_resources":
		var args struct {
			Mode      string `json:"mode"`
			Port      int    `json:"port"`
			PID       int    `json:"pid"`
			Protocol  string `json:"protocol"`
			State     string `json:"state"`
			Ownership string `json:"ownership"`
			Project   string `json:"project"`
			Directory string `json:"directory"`
			Recursive bool   `json:"recursive"`
			Query     string `json:"query"`
			Limit     int    `json:"limit"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		mode := strings.ToLower(strings.TrimSpace(args.Mode))
		if mode != "ports" && mode != "processes" && mode != "files" {
			return formatMCPError(errors.New("invalidRequest")), nil
		}
		targetPath := strings.TrimSpace(args.Directory)
		if mode == "files" && targetPath == "" {
			targetPath, _ = os.Getwd()
		}
		limit := args.Limit
		if limit <= 0 {
			limit = 50
		}
		res, rErr := QueryAndFilterResources(ctx, mode, dataDir, service, targetPath, args.Recursive, ResourceFilterOptions{
			Port:          args.Port,
			PID:           args.PID,
			Protocol:      args.Protocol,
			State:         args.State,
			Ownership:     args.Ownership,
			Project:       args.Project,
			Query:         args.Query,
			Limit:         limit,
			IncludeImages: false,
		})
		if rErr != nil {
			return formatMCPError(rErr), nil
		}
		return formatMCPResult(res, false), nil

	case "yourun_discover":
		var args struct {
			Mode      string `json:"mode"`
			Directory string `json:"directory"`
			Name      string `json:"name"`
			Platform  string `json:"platform"`
		}
		_ = json.Unmarshal(params.Arguments, &args)
		mode := strings.ToLower(strings.TrimSpace(args.Mode))
		if mode == "" {
			mode = "discover"
		}
		targetDir := strings.TrimSpace(args.Directory)
		if targetDir == "" {
			targetDir, _ = os.Getwd()
		} else if abs, absErr := filepath.Abs(targetDir); absErr == nil {
			targetDir = abs
		}
		switch mode {
		case "source":
			return formatMCPResult(engine.InspectSource(ctx, targetDir, "", "", true), false), nil
		case "inspect":
			rules, rErr := discovery.LoadRegistry(filepath.Join(dataDir, "discovery-rules"))
			if rErr != nil {
				return formatMCPError(rErr), nil
			}
			res, iErr := discovery.Inspect(ctx, targetDir, discovery.InspectOptions{Name: args.Name, Platform: args.Platform, Registry: rules})
			if iErr != nil {
				return formatMCPError(iErr), nil
			}
			for i := range res.Icons {
				res.Icons[i].Data = ""
			}
			return formatMCPResult(res, false), nil
		case "discover":
			rules, rErr := discovery.LoadRegistry(filepath.Join(dataDir, "discovery-rules"))
			if rErr != nil {
				return formatMCPError(rErr), nil
			}
			res, dErr := discovery.DiscoverWithRegistry(ctx, targetDir, rules)
			if dErr != nil {
				return formatMCPError(dErr), nil
			}
			return formatMCPResult(res, false), nil
		default:
			return formatMCPError(errors.New("invalidRequest")), nil
		}

	default:
		return mcpCallToolResult{}, &jsonRPCError{Code: -32601, Message: "Unknown tool: " + params.Name}
	}
}

func mcpStaticResources() []map[string]any {
	return []map[string]any{
		{
			"uri":         "yourun://capabilities",
			"name":        "YouRun Capabilities & Workflows",
			"description": "Self-discovery metadata, runtime environment, security boundaries, and recommended Agent workflows.",
			"mimeType":    "application/json",
		},
		{
			"uri":         "yourun://context",
			"name":        "Current Directory Situational Context",
			"description": "Matched projects, workspaces, resolved run/dev/start commands, active runs with code drift, and recent runs for the current working directory.",
			"mimeType":    "application/json",
		},
		{
			"uri":         "yourun://ports/managed",
			"name":        "Managed Local Ports",
			"description": "Local TCP/UDP ports currently held by active YouRun executions.",
			"mimeType":    "application/json",
		},
	}
}

func mcpResourceTemplates() []map[string]any {
	return []map[string]any{
		{
			"uriTemplate": "yourun://runs/{id}",
			"name":        "Execution Record & Source Drift",
			"description": "Execution state and live Git source drift comparison for a specific run ID.",
			"mimeType":    "application/json",
		},
		{
			"uriTemplate": "yourun://runs/{id}/logs",
			"name":        "Execution Log Tail",
			"description": "Latest 16 KiB UTF-8 log output for a specific run ID.",
			"mimeType":    "application/json",
		},
		{
			"uriTemplate": "yourun://projects/{id}/context",
			"name":        "Project Context",
			"description": "Full situational awareness report for a specific project ID or name.",
			"mimeType":    "application/json",
		},
	}
}

func handleMCPResourceRead(ctx context.Context, dataDir, exe string, rawParams json.RawMessage) (any, *jsonRPCError) {
	var params struct {
		URI string `json:"uri"`
	}
	if err := json.Unmarshal(rawParams, &params); err != nil || params.URI == "" {
		return nil, &jsonRPCError{Code: -32602, Message: "Invalid resource URI"}
	}
	c, err := engine.Open(dataDir, exe)
	if err != nil {
		return nil, &jsonRPCError{Code: -32603, Message: err.Error()}
	}
	service, err := workspace.New(dataDir, c.Request)
	if err != nil {
		return nil, &jsonRPCError{Code: -32603, Message: err.Error()}
	}

	var value any
	switch {
	case params.URI == "yourun://capabilities":
		value, err = BuildCapabilitiesReport(ctx, dataDir)
	case params.URI == "yourun://context":
		value, err = BuildContextReport(ctx, dataDir, c, service, "", "")
	case params.URI == "yourun://ports/managed":
		value, err = QueryAndFilterResources(ctx, "ports", dataDir, service, "", false, ResourceFilterOptions{Ownership: "yourun", Limit: 100})
	case strings.HasPrefix(params.URI, "yourun://runs/") && strings.HasSuffix(params.URI, "/logs"):
		runID := strings.TrimSuffix(strings.TrimPrefix(params.URI, "yourun://runs/"), "/logs")
		value, err = readLogClean(c, runID, 16384, -1, 16384, true)
	case strings.HasPrefix(params.URI, "yourun://runs/"):
		runID := strings.TrimPrefix(params.URI, "yourun://runs/")
		var r engine.Run
		r, err = c.Run(runID)
		if err == nil {
			var drift *SourceDriftReport
			if r.Directory != "" {
				cur := engine.InspectSource(ctx, r.Directory, "", "", false)
				d := CompareSourceDrift(r.LaunchSource, cur)
				drift = &d
			}
			value = map[string]any{"run": r, "sourceDrift": drift}
		}
	case strings.HasPrefix(params.URI, "yourun://projects/") && strings.HasSuffix(params.URI, "/context"):
		proj := strings.TrimSuffix(strings.TrimPrefix(params.URI, "yourun://projects/"), "/context")
		proj, _ = url.PathUnescape(proj)
		value, err = BuildContextReport(ctx, dataDir, c, service, proj, "")
	default:
		return nil, &jsonRPCError{Code: -32602, Message: "Unknown resource URI: " + params.URI}
	}
	if err != nil {
		return nil, &jsonRPCError{Code: -32603, Message: err.Error()}
	}
	encoded, _ := json.MarshalIndent(value, "", "  ")
	return map[string]any{
		"contents": []map[string]any{
			{
				"uri":      params.URI,
				"mimeType": "application/json",
				"text":     string(encoded),
			},
		},
	}, nil
}

func mcpPromptsList() []map[string]any {
	return []map[string]any{
		{
			"name":        "diagnose-port",
			"description": "Check which process holds a local port, determine if YouRun owns it, inspect its logs/source drift, and stop or restart it safely.",
			"arguments": []map[string]any{
				{"name": "port", "description": "Local TCP/UDP port number (e.g. 3000)", "required": true},
			},
		},
		{
			"name":        "diagnose-project",
			"description": "Inspect a project directory's saved YouRun commands, active runs, Git source drift, ports, and recent failures.",
			"arguments": []map[string]any{
				{"name": "directory", "description": "Project directory path", "required": false},
			},
		},
		{
			"name":        "diagnose-file-lock",
			"description": "Find processes holding open handles on a file or directory and cleanly stop the owning YouRun run if managed.",
			"arguments": []map[string]any{
				{"name": "path", "description": "File or directory path", "required": true},
			},
		},
	}
}

func handleMCPPromptGet(rawParams json.RawMessage) (any, *jsonRPCError) {
	var params struct {
		Name      string            `json:"name"`
		Arguments map[string]string `json:"arguments"`
	}
	if err := json.Unmarshal(rawParams, &params); err != nil || params.Name == "" {
		return nil, &jsonRPCError{Code: -32602, Message: "Invalid prompt parameters"}
	}
	var promptText string
	switch params.Name {
	case "diagnose-port":
		port := params.Arguments["port"]
		promptText = fmt.Sprintf("Use `yourun_resources` with `mode: \"ports\"` and `port: %s` to inspect the process holding port %s. If `ownership` is `managed`, check its logs with `yourun_logs` (`id: <runId>`) and either `stop` or `restart` it via `yourun_control`.", port, port)
	case "diagnose-project":
		dir := params.Arguments["directory"]
		promptText = fmt.Sprintf("Call `yourun_context` (with `directory: %q`) to inspect matched projects, shortcut commands (`run`/`dev`/`start`), active runs, code drift (`restartRecommended`), and recent runs. If an active run has `restartRecommended: true`, restart it with `yourun_control` (`action: \"restart\"`).", dir)
	case "diagnose-file-lock":
		p := params.Arguments["path"]
		promptText = fmt.Sprintf("Call `yourun_resources` with `mode: \"files\"`, `directory: %q`, and `recursive: true`. If any locking process has `ownership: \"managed\"`, use `yourun_control` (`action: \"stop\"`, `id: <runId>`) to release the handle cleanly.", p)
	default:
		return nil, &jsonRPCError{Code: -32601, Message: "Unknown prompt: " + params.Name}
	}
	return map[string]any{
		"description": "YouRun diagnostic workflow",
		"messages": []map[string]any{
			{
				"role": "user",
				"content": map[string]any{
					"type": "text",
					"text": promptText,
				},
			},
		},
	}, nil
}

func waitRunResult(c *engine.Client, id string, timeout time.Duration) (engine.Run, bool, error) {
	started := time.Now()
	for {
		run, err := c.Run(id)
		if err != nil {
			return engine.Run{}, false, err
		}
		switch run.Status {
		case "success", "handedOff", "failed", "stopped", "unknown":
			return run, false, nil
		}
		if timeout > 0 && time.Since(started) >= timeout {
			return run, true, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func readLogClean(c *engine.Client, id string, tail, cursor, limit int64, omitData bool) (map[string]any, error) {
	effLimit := limit
	if effLimit <= 0 {
		effLimit = 128 << 10
	}
	if tail > 0 && cursor < 0 {
		effLimit = tail
	}
	if effLimit > 1<<20 {
		effLimit = 1 << 20
	}
	var path string
	if cursor >= 0 {
		path = fmt.Sprintf("/api/logs?id=%s&cursor=%d&limit=%d", url.QueryEscape(id), cursor, effLimit)
	} else {
		path = fmt.Sprintf("/api/logs?id=%s&limit=%d", url.QueryEscape(id), effLimit)
	}
	status, data, err := c.Request("GET", path, nil)
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	if status >= 400 {
		if code, ok := payload["error"].(string); ok && code != "" {
			return nil, errors.New(code)
		}
		return nil, errors.New("requestFailed")
	}
	payload["id"] = id
	if omitData {
		delete(payload, "data")
	}
	return payload, nil
}
