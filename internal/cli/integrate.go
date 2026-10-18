package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type AgentIntegrationTarget struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"` // "skill" | "mcp" | "path"
	Path       string `json:"path"`
	Configured bool   `json:"configured"`
	Note       string `json:"note,omitempty"`
}

type AgentIntegrationSettings struct {
	Enabled    bool                     `json:"enabled"`
	Supported  bool                     `json:"supported"`
	Reason     string                   `json:"reason,omitempty"`
	Executable string                   `json:"executable,omitempty"`
	CLIWrapper string                   `json:"cliWrapper,omitempty"`
	Targets    []AgentIntegrationTarget `json:"targets,omitempty"`
}

type integrationReceipt struct {
	Enabled      bool   `json:"enabled"`
	Executable   string `json:"executable,omitempty"`
	AddedPathDir string `json:"addedPathDir,omitempty"`
}

var integrationMu sync.Mutex

// ResolveIntegrationPaths determines the stable YouRun executable and CLI wrapper
// paths suitable for registering in global Agent skills, MCP configs, and user PATH.
// It refuses temporary go-build/debug/dist-builds paths unless a stable installation exists.
func ResolveIntegrationPaths(executableOverride string) (exePath string, cliPath string, binDir string, supported bool, reason string) {
	candidate := strings.TrimSpace(executableOverride)
	if candidate == "" {
		if raw, err := os.Executable(); err == nil {
			candidate = raw
		}
	}
	if resolved, err := filepath.EvalSymlinks(candidate); err == nil && filepath.IsAbs(resolved) {
		candidate = resolved
	}
	if candidate == "" || !filepath.IsAbs(candidate) || isEphemeralBuildExecutable(candidate) {
		if runtime.GOOS == "windows" {
			if home, err := os.UserHomeDir(); err == nil {
				fixed := filepath.Join(home, "Desktop", "YouRun", "YouRun.exe")
				if st, statErr := os.Stat(fixed); statErr == nil && !st.IsDir() {
					candidate = fixed
				}
			}
		}
	}
	if candidate == "" || !filepath.IsAbs(candidate) || isEphemeralBuildExecutable(candidate) {
		return "", "", "", false, "agentIntegrationDevelopment"
	}
	binDir = filepath.Dir(candidate)
	if runtime.GOOS == "windows" {
		cliPath = filepath.Join(binDir, "yourun.cmd")
	} else {
		wrapper := filepath.Join(binDir, "yourun")
		if st, err := os.Stat(wrapper); err == nil && !st.IsDir() {
			cliPath = wrapper
		} else {
			cliPath = candidate
		}
	}
	return candidate, cliPath, binDir, true, ""
}

func isEphemeralBuildExecutable(executable string) bool {
	lower := strings.ToLower(filepath.Clean(executable))
	if strings.HasSuffix(lower, ".test") || strings.HasSuffix(lower, ".test.exe") {
		return true
	}
	parts := strings.FieldsFunc(lower, func(r rune) bool { return r == '/' || r == '\\' })
	for i, part := range parts {
		if strings.HasPrefix(part, "go-build") || strings.HasPrefix(part, "__debug_bin") {
			return true
		}
		if i > 0 && parts[i-1] == "dist" && part == "builds" {
			return true
		}
	}
	return false
}

func ReadAgentIntegration(ctx context.Context, dataDir string) (AgentIntegrationSettings, error) {
	integrationMu.Lock()
	defer integrationMu.Unlock()
	return readAgentIntegrationLocked(ctx, dataDir)
}

func WriteAgentIntegration(ctx context.Context, dataDir string, enabled bool) (AgentIntegrationSettings, error) {
	integrationMu.Lock()
	defer integrationMu.Unlock()

	exePath, cliPath, binDir, supported, reason := ResolveIntegrationPaths("")
	if enabled && !supported {
		return AgentIntegrationSettings{
			Enabled:   false,
			Supported: false,
			Reason:    reason,
		}, errors.New(reason)
	}

	receipt := loadIntegrationReceipt(dataDir)
	home, err := os.UserHomeDir()
	if err != nil {
		return AgentIntegrationSettings{}, err
	}

	if enabled {
		if runtime.GOOS == "windows" && strings.HasSuffix(strings.ToLower(cliPath), ".cmd") {
			if _, statErr := os.Stat(cliPath); errors.Is(statErr, os.ErrNotExist) {
				script := fmt.Sprintf("@echo off\r\nsetlocal DisableDelayedExpansion\r\n\"%%~dp0%s\" cli %%*\r\nexit /b %%errorlevel%%\r\n", filepath.Base(exePath))
				_ = os.WriteFile(cliPath, []byte(script), 0o644)
			}
		}
		skillMD := renderEmbeddedSkillMD(cliPath, exePath)
		openAIYAML := renderEmbeddedOpenAIYAML(cliPath)
		for _, spec := range skillTargetSpecs(home) {
			if !spec.alwaysInstall && !dirExists(spec.parentCheck) {
				continue
			}
			if err := writeSkillFiles(spec.dir, skillMD, openAIYAML); err != nil {
				return AgentIntegrationSettings{}, err
			}
		}
		for _, mcpSpec := range mcpTargetSpecs(home) {
			if !mcpSpec.shouldManage() {
				continue
			}
			_ = upsertMCPServerConfig(mcpSpec.path, exePath)
		}
		_, _, _ = updateUserPath(true, binDir, &receipt)
		receipt.Enabled = true
		receipt.Executable = exePath
		saveIntegrationReceipt(dataDir, receipt)
	} else {
		for _, spec := range skillTargetSpecs(home) {
			_ = os.RemoveAll(spec.dir)
		}
		for _, mcpSpec := range mcpTargetSpecs(home) {
			_ = removeMCPServerConfig(mcpSpec.path)
		}
		_, _, _ = updateUserPath(false, binDir, &receipt)
		receipt.Enabled = false
		saveIntegrationReceipt(dataDir, receipt)
	}

	return readAgentIntegrationLocked(ctx, dataDir)
}

func readAgentIntegrationLocked(ctx context.Context, dataDir string) (AgentIntegrationSettings, error) {
	exePath, cliPath, binDir, supported, reason := ResolveIntegrationPaths("")
	state := AgentIntegrationSettings{
		Supported:  supported,
		Reason:     reason,
		Executable: exePath,
		CLIWrapper: cliPath,
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return state, err
	}

	var targets []AgentIntegrationTarget
	anySkillConfigured := false

	for _, spec := range skillTargetSpecs(home) {
		skillFile := filepath.Join(spec.dir, "SKILL.md")
		configured := fileExists(skillFile)
		if configured {
			anySkillConfigured = true
		}
		if configured || spec.alwaysInstall || dirExists(spec.parentCheck) {
			targets = append(targets, AgentIntegrationTarget{
				ID:         spec.id,
				Name:       spec.name,
				Kind:       "skill",
				Path:       skillFile,
				Configured: configured,
			})
		}
	}

	for _, mcpSpec := range mcpTargetSpecs(home) {
		configured, note := inspectMCPServerConfig(mcpSpec.path)
		if configured || mcpSpec.shouldManage() {
			targets = append(targets, AgentIntegrationTarget{
				ID:         mcpSpec.id,
				Name:       mcpSpec.name,
				Kind:       "mcp",
				Path:       mcpSpec.path,
				Configured: configured,
				Note:       note,
			})
		}
	}

	pathConfigured, pathLocation := checkUserPath(binDir)
	targets = append(targets, AgentIntegrationTarget{
		ID:         "user-path",
		Name:       "PATH (yourun)",
		Kind:       "path",
		Path:       pathLocation,
		Configured: pathConfigured,
	})

	state.Targets = targets
	state.Enabled = supported && anySkillConfigured
	return state, ctx.Err()
}

type skillSpec struct {
	id            string
	name          string
	dir           string
	parentCheck   string
	alwaysInstall bool
}

func skillTargetSpecs(home string) []skillSpec {
	return []skillSpec{
		{
			id:            "skill-agents",
			name:          "Agent Skills (~/.agents)",
			dir:           filepath.Join(home, ".agents", "skills", "yourun"),
			parentCheck:   filepath.Join(home, ".agents"),
			alwaysInstall: true,
		},
		{
			id:            "skill-gemini",
			name:          "Antigravity / Gemini (~/.gemini)",
			dir:           filepath.Join(home, ".gemini", "config", "skills", "yourun"),
			parentCheck:   filepath.Join(home, ".gemini"),
			alwaysInstall: true,
		},
		{
			id:          "skill-claude",
			name:        "Claude Code (~/.claude)",
			dir:         filepath.Join(home, ".claude", "skills", "yourun"),
			parentCheck: filepath.Join(home, ".claude"),
		},
		{
			id:          "skill-codex",
			name:        "Codex (~/.codex)",
			dir:         filepath.Join(home, ".codex", "skills", "yourun"),
			parentCheck: filepath.Join(home, ".codex"),
		},
	}
}

type mcpSpec struct {
	id           string
	name         string
	path         string
	shouldManage func() bool
}

func mcpTargetSpecs(home string) []mcpSpec {
	specs := []mcpSpec{
		{
			id:   "mcp-cursor",
			name: "Cursor MCP",
			path: filepath.Join(home, ".cursor", "mcp.json"),
			shouldManage: func() bool {
				return dirExists(filepath.Join(home, ".cursor"))
			},
		},
		{
			id:   "mcp-claude-code",
			name: "Claude Code MCP",
			path: filepath.Join(home, ".claude.json"),
			shouldManage: func() bool {
				return fileExists(filepath.Join(home, ".claude.json")) || dirExists(filepath.Join(home, ".claude"))
			},
		},
		{
			id:   "mcp-windsurf",
			name: "Windsurf MCP",
			path: filepath.Join(home, ".codeium", "windsurf", "mcp_config.json"),
			shouldManage: func() bool {
				return dirExists(filepath.Join(home, ".codeium", "windsurf"))
			},
		},
	}
	if claudeDesktop := claudeDesktopConfigPath(home); claudeDesktop != "" {
		specs = append(specs, mcpSpec{
			id:   "mcp-claude-desktop",
			name: "Claude Desktop MCP",
			path: claudeDesktop,
			shouldManage: func() bool {
				return dirExists(filepath.Dir(claudeDesktop))
			},
		})
	}
	return specs
}

func claudeDesktopConfigPath(home string) string {
	switch runtime.GOOS {
	case "windows":
		if appData := strings.TrimSpace(os.Getenv("APPDATA")); appData != "" {
			return filepath.Join(appData, "Claude", "claude_desktop_config.json")
		}
		return filepath.Join(home, "AppData", "Roaming", "Claude", "claude_desktop_config.json")
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	default:
		return filepath.Join(home, ".config", "Claude", "claude_desktop_config.json")
	}
}

func writeSkillFiles(skillDir, skillMD, openAIYAML string) error {
	agentsDir := filepath.Join(skillDir, "agents")
	if err := os.MkdirAll(agentsDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(agentsDir, "openai.yaml"), []byte(openAIYAML), 0o644)
}

func inspectMCPServerConfig(path string) (bool, string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, ""
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return false, ""
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		return false, "skipped_malformed_json"
	}
	servers, ok := root["mcpServers"].(map[string]any)
	if !ok {
		return false, ""
	}
	entry, ok := servers["yourun"].(map[string]any)
	if !ok {
		return false, ""
	}
	cmd, _ := entry["command"].(string)
	return strings.TrimSpace(cmd) != "", ""
}

func upsertMCPServerConfig(path, exePath string) error {
	var root map[string]any
	if data, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		if err := json.Unmarshal(data, &root); err != nil {
			// Never overwrite a user's malformed JSON file.
			return nil
		}
	}
	if root == nil {
		root = map[string]any{}
	}
	servers, ok := root["mcpServers"].(map[string]any)
	if !ok || servers == nil {
		servers = map[string]any{}
		root["mcpServers"] = servers
	}
	servers["yourun"] = map[string]any{
		"command": exePath,
		"args":    []string{"mcp"},
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(encoded, '\n'), 0o644)
}

func removeMCPServerConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil || root == nil {
		return nil
	}
	servers, ok := root["mcpServers"].(map[string]any)
	if !ok || servers == nil {
		return nil
	}
	if _, exists := servers["yourun"]; !exists {
		return nil
	}
	delete(servers, "yourun")
	encoded, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(encoded, '\n'), 0o644)
}

func loadIntegrationReceipt(dataDir string) integrationReceipt {
	if strings.TrimSpace(dataDir) == "" {
		return integrationReceipt{}
	}
	var rec integrationReceipt
	if data, err := os.ReadFile(filepath.Join(dataDir, "agent-integration.json")); err == nil {
		_ = json.Unmarshal(data, &rec)
	}
	return rec
}

func saveIntegrationReceipt(dataDir string, rec integrationReceipt) {
	if strings.TrimSpace(dataDir) == "" {
		return
	}
	if encoded, err := json.MarshalIndent(rec, "", "  "); err == nil {
		_ = os.WriteFile(filepath.Join(dataDir, "agent-integration.json"), append(encoded, '\n'), 0o644)
	}
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func renderEmbeddedSkillMD(cliPath, exePath string) string {
	return fmt.Sprintf(`---
name: yourun
description: Manage project commands, detached executions, live code-drift detection, logs, and OS resource attribution (ports, processes, open file handles) with YouRun. Use when starting, stopping, or restarting saved project commands/dev servers, checking whether a running service has drifted from current Git code, reading task logs, diagnosing local port conflicts, or finding which process locks a file or directory.
---

# YouRun Agent Guide

YouRun (`+"`YouRun.exe` / `yourun`"+`) is a local command and runtime manager. Its GUI, CLI, and embedded `+"`stdio`"+` MCP server share the same execution journal (`+"`executions/<id>/`"+`), log store (`+"`logs/<id>.log`"+`), and SQLite workspace database—requiring **no background daemon, no GUI window, and no connection approval**.

## Entrypoint Resolution

1. **Embedded MCP Server (Preferred when registered)**:
   - Command: `+"`%s mcp`"+` (or `+"`yourun mcp`"+`)
   - Primary tools: `+"`yourun_context`, `yourun_launch`, `yourun_control`, `yourun_logs`, `yourun_resources`, `yourun_search`, `yourun_discover`"+`.
2. **CLI Wrapper**:
   - Path: `+"`%s`"+` (or `+"`yourun`"+` when in `+"`PATH`"+`)
   - All CLI responses are structured JSON on `+"`stdout`"+`.

## Golden Workflows for Agents

### 1. First Step in Any Repository: One-Shot Situational Awareness
Before launching a dev server, build, or guessing project commands, run:
`+"```powershell\n%s context\n```"+`
(Or call MCP tool `+"`yourun_context`"+`.)
In a single call, `+"`context`"+` returns:
- `+"`source`"+`: Current directory Git branch, commit, and dirty status.
- `+"`matchedProjects`"+`: Saved YouRun projects matching the directory, their workspace locations, all saved commands, and what `+"`yourun run`, `yourun dev`, and `yourun start`"+` resolve to (`+"`shortcutResolution`"+`).
- `+"`activeRuns`"+`: Currently running executions for this project, including:
  - `+"`sourceDrift`"+`: Live comparison between `+"`launchSource`"+` and current disk Git state (`+"`consistent`"+` vs `+"`changed`"+`).
  - `+"`restartRecommended`"+`: `+"`true`"+` when code has changed since the process was launched.
  - `+"`ports`"+`: Active listening TCP/UDP ports owned by each run.
- `+"`recentRuns`"+`: Most recent completed/failed runs so you can immediately spot recent failures.
- `+"`suggestedActions`"+`: Ready-to-run commands tailored to the current state.

### 2. Launching, Stopping, and Restarting Saved Commands
YouRun executes **only** saved command definitions in their configured working directory (`+"`--directory`"+` only selects/matches a project or command; it never overrides a command's working directory).
- **Launch dev/watch command**:
  `+"```powershell\n  %s dev [--project ID|NAME] [--directory PATH] [--wait]\n  ```"+`
- **Launch primary run command**:
  `+"```powershell\n  %s run [--project ID|NAME] [--directory PATH] [--wait]\n  ```"+`
- **Launch specific command by ID**:
  `+"```powershell\n  %s start --command <COMMAND_ID> [--wait]\n  ```"+`
- **Apply code changes to an active run (atomic stop + replay)**:
  `+"```powershell\n  %s restart [--id <RUN_ID>] [--project ID|NAME] [--wait]\n  ```"+`
- **Stop an owned active run**:
  `+"```powershell\n  %s stop [--id <RUN_ID>] [--project ID|NAME]\n  ```"+`
- **Wait for a build/task to finish** (never cancels the task on timeout; exit code `+"`124`"+` on timeout):
  `+"```powershell\n  %s wait [--id <RUN_ID>] --timeout 30s\n  ```"+`

### 3. Reading Logs Without Flooding Context
`+"`logs`"+` automatically resolves the active (or most recent) run in the current project/directory when `+"`--id`"+` is omitted, reads from the **tail** by default, and supports `+"`--omit-data`"+` to skip redundant base64 byte arrays:
`+"```powershell\n%s logs --tail 16384 --omit-data\n```"+`
For incremental polling, pass `+"`--cursor <nextCursor>`"+` from the previous response.

### 4. Diagnosing Port Conflicts (`+"`EADDRINUSE`"+`)
Instead of running raw `+"`netstat` + `tasklist`"+` and guessing which project owns a process, query YouRun's attributed port table (base64 icons are stripped automatically):
`+"```powershell\n%s ports --port 3000\n```"+`
- Check `+"`ownership`"+`:
  - `+"`\"managed\"`"+`: Owned by an active YouRun run (`+"`runId`, `projectName`, `commandTitle`, `workspaceName`, `launchSource`"+`). Stop or restart it cleanly via `+"`yourun stop --id <runId>`"+` or `+"`yourun restart --id <runId>`"+`.
  - `+"`\"unconfirmed\"`"+`: Historical `+"`unknown`"+` run.
  - `+"`\"external\"`"+`: Started outside YouRun (`+"`pid`, `processName`, `commandLine`, `executable`"+` provided).

### 5. Diagnosing Locked Files or Directories (`+"`EBUSY` / `EPERM`"+`)
When a build or file operation fails because a file or folder is locked:
`+"```powershell\n%s files --directory ./dist --recursive\n```"+`
If any item has `+"`ownership: \"managed\"`"+`, stop its `+"`runId`"+` via `+"`yourun stop --id <runId>`"+`.

### 6. Self-Discovery, Global Registration & Error Recovery
- Inspect environment capabilities & workflows: `+"`yourun capabilities`"+`
- Inspect detailed command flags, exit codes, and error catalog: `+"`yourun describe [subcommand]`"+`
- Refresh global Agent skills, MCP configs & `+"`PATH`"+` registration: `+"`yourun integrate [--enable | --disable | --status]`"+`
- Whenever a command fails with exit code `+"`2`"+` (e.g., `+"`ambiguousProject`, `ambiguousCommand`, `ambiguousRun`, `commandMissing`, `alreadyRunning`"+`), read the `+"`candidates`"+` array and `+"`recovery`"+` hint in the JSON response to retry with the exact `+"`--project`, `--command`, or `--id`"+`.
`, exePath, cliPath, cliPath, cliPath, cliPath, cliPath, cliPath, cliPath, cliPath, cliPath, cliPath, cliPath)
}

func renderEmbeddedOpenAIYAML(cliPath string) string {
	return fmt.Sprintf(`interface:
  display_name: "YouRun"
  short_description: "Manage project commands, detached runs, logs, ports, and file locks"
  default_prompt: "Use $yourun (%s context) to inspect the current project's commands, active runs, listening ports, and source drift."
`, cliPath)
}
