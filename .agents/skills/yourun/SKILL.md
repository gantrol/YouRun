---
name: yourun
description: Manage project commands, detached executions, live code-drift detection, logs, and OS resource attribution (ports, processes, open file handles) with YouRun. Use when starting, stopping, or restarting saved project commands/dev servers, checking whether a running service has drifted from current Git code, reading task logs, diagnosing local port conflicts, or finding which process locks a file or directory.
---

# YouRun Agent Guide

YouRun (`YouRun.exe` / `yourun`) is a local command and runtime manager. Its GUI, CLI, and embedded `stdio` MCP server share the same execution journal (`executions/<id>/`), log store (`logs/<id>.log`), and SQLite workspace database—requiring **no background daemon, no GUI window, and no connection approval**.

## Entrypoint Resolution

1. **Embedded MCP Server (Preferred when registered)**:
   - Command: `C:\Users\gantrol\Desktop\YouRun\YouRun.exe mcp` (or `yourun mcp`)
   - Primary tools: `yourun_context`, `yourun_launch`, `yourun_control`, `yourun_logs`, `yourun_resources`, `yourun_search`, `yourun_discover`.
2. **CLI Wrapper**:
   - Fixed Windows path: `C:\Users\gantrol\Desktop\YouRun\yourun.cmd` (or `"C:\Users\gantrol\Desktop\YouRun\YouRun.exe" cli <subcommand>`)
   - All CLI responses are single-line or structured JSON on `stdout`.

## Golden Workflows for Agents

### 1. First Step in Any Repository: One-Shot Situational Awareness
Before launching a dev server, build, or guessing project commands, run:
```powershell
C:\Users\gantrol\Desktop\YouRun\yourun.cmd context
```
(Or call MCP tool `yourun_context`.)
In a single call, `context` returns:
- `source`: Current directory Git branch, commit, and dirty status.
- `matchedProjects`: Saved YouRun projects matching the directory, their workspace locations, all saved commands, and what `yourun run`, `yourun dev`, and `yourun start` resolve to (`shortcutResolution`).
- `activeRuns`: Currently running executions for this project, including:
  - `sourceDrift`: Live comparison between `launchSource` and current disk Git state (`consistent` vs `changed`).
  - `restartRecommended`: `true` when code has changed since the process was launched.
  - `ports`: Active listening TCP/UDP ports owned by each run.
- `recentRuns`: Most recent completed/failed runs so you can immediately spot recent failures.
- `suggestedActions`: Ready-to-run commands tailored to the current state.

### 2. Launching, Stopping, and Restarting Saved Commands
YouRun executes **only** saved command definitions in their configured working directory (`--directory` only selects/matches a project or command; it never overrides a command's working directory).
- **Launch dev/watch command**:
  ```powershell
  C:\Users\gantrol\Desktop\YouRun\yourun.cmd dev [--project ID|NAME] [--directory PATH]
  ```
- **Launch primary run command**:
  ```powershell
  C:\Users\gantrol\Desktop\YouRun\yourun.cmd run [--project ID|NAME] [--directory PATH]
  ```
- **Launch specific command by ID**:
  ```powershell
  C:\Users\gantrol\Desktop\YouRun\yourun.cmd start --command <COMMAND_ID>
  ```
- **Apply code changes to an active run (atomic stop + replay)**:
  ```powershell
  C:\Users\gantrol\Desktop\YouRun\yourun.cmd restart [--id <RUN_ID>] [--project ID|NAME]
  ```
- **Stop an owned active run**:
  ```powershell
  C:\Users\gantrol\Desktop\YouRun\yourun.cmd stop [--id <RUN_ID>] [--project ID|NAME]
  ```
- **Wait for a build/task to finish** (never cancels the task on timeout; exit code `124` on timeout):
  ```powershell
  C:\Users\gantrol\Desktop\YouRun\yourun.cmd wait [--id <RUN_ID>] --timeout 30s
  ```

### 3. Reading Logs Without Flooding Context
`logs` automatically resolves the active (or most recent) run in the current project/directory when `--id` is omitted, reads from the **tail** by default, and supports `--omit-data` to skip redundant base64 byte arrays:
```powershell
C:\Users\gantrol\Desktop\YouRun\yourun.cmd logs --tail 16384 --omit-data
```
For incremental polling, pass `--cursor <nextCursor>` from the previous response.

### 4. Diagnosing Port Conflicts (`EADDRINUSE`)
Instead of running raw `netstat` + `tasklist` and guessing which project owns a process, query YouRun's attributed port table (base64 icons are stripped automatically):
```powershell
C:\Users\gantrol\Desktop\YouRun\yourun.cmd ports --port 3000
```
- Check `ownership`:
  - `"managed"`: Owned by an active YouRun run (`runId`, `projectName`, `commandTitle`, `workspaceName`, `launchSource`). Stop or restart it cleanly via `yourun stop --id <runId>` or `yourun restart --id <runId>`.
  - `"unconfirmed"`: Historical `unknown` run.
  - `"external"`: Started outside YouRun (`pid`, `processName`, `commandLine`, `executable` provided).

### 5. Diagnosing Locked Files or Directories (`EBUSY` / `EPERM`)
When a build or file operation fails because a file or folder is locked:
```powershell
C:\Users\gantrol\Desktop\YouRun\yourun.cmd files --directory ./dist --recursive
```
If any item has `ownership: "managed"`, stop its `runId` via `yourun stop --id <runId>`.

### 6. Self-Discovery, Global Registration & Error Recovery
- Inspect environment capabilities & workflows: `yourun capabilities`
- Inspect detailed command flags, exit codes, and error catalog: `yourun describe [subcommand]`
- Inspect, selectively enable, or revoke Agent integrations: `yourun integrate [--status] | [--enable --target ID[,ID...]] | [--disable [--target ID[,ID...]]]`
- Whenever a command fails with exit code `2` (e.g., `ambiguousProject`, `ambiguousCommand`, `ambiguousRun`, `commandMissing`, `alreadyRunning`), read the `candidates` array and `recovery` hint in the JSON response to retry with the exact `--project`, `--command`, or `--id`.
