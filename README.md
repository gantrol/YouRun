![YouRun: a running yellow rabbit and parallel command tracks](docs/images/yourun-cover.png)

# YouRun

[![中文 | 点我](docs/images/badges/zh-CN.svg)](README.zh-CN.md)[![License: Apache 2.0](https://img.shields.io/badge/license-Apache%202.0-8A9E77?style=flat-square)](LICENSE)[![Platform: Windows x64](https://img.shields.io/badge/platform-Windows%20x64-F2CA52?style=flat-square)](#current-support)

Built with [MyGo](https://github.com/egoist/mygo), YouRun brings development, run, and packaging commands from different projects together. Users and agents can start commands, stop them, and inspect their output in one place. Even if YouRun is closed or crashes, commands it has already started can keep running independently.

![YouRun project list](docs/images/yourun-projects-screenshot.png)

- I used to rely heavily on IDEs and editors, where running project commands was convenient. With AI-assisted coding, I need them less, and keeping multiple windows open feels cumbersome.
- Running commands directly through an agent is another option, but agent updates, computer restarts, and similar events can disrupt that workflow.

The Windows MyGo WebView edition is currently the primary distribution target.

## Close the window, keep commands running

![Concept illustration: the YouRun control window closes while three commands keep running independently](docs/images/yourun-independent-runs.png)

Each running command is managed by an independent execution process. When closing YouRun, you can choose to keep commands running or stop them before exiting. Reopen the window to check on existing runs.

## Getting started

1. Open YouRun, import a project directory, and select the commands to save. You can also add projects and commands manually.
2. Review the command text, working directory, and environment variables. Directory scanning uses static discovery and does not automatically execute project scripts.
3. Start a command and view its status and output in the run records. Stop or restart it as needed.
4. Organize frequently used commands into collections, or search for projects and commands directly.

Install the tools your projects require, such as Node.js, Python, or Go, separately on your computer. YouRun's output panel displays logs; it currently does not provide an interactive terminal or standard input.

## CLI for scripts and agents

The GUI and CLI share `YouRun.exe`. Windows builds include `yourun.cmd`, which internally calls `YouRun.exe cli <subcommand>`, waits for the process, and returns its exit code. Keep both files in the same directory. To call `yourun` from any directory, copy both files to a permanent directory and add that directory to `PATH`.

Run the following example in PowerShell from the build output directory. It assumes you have already saved a project named `Demo` and its `npm run dev` command in the GUI:

```powershell
# Find saved projects and commands (shorthand: .\yourun.cmd search Demo)
.\yourun.cmd search --query Demo

# Assign a unique run ID for subsequent queries and stop requests
$runId = "demo-dev-" + [guid]::NewGuid().ToString("N")
.\yourun.cmd dev --project Demo --id $runId

# Check status and output
.\yourun.cmd status --id $runId
.\yourun.cmd logs --id $runId

# Stop this run
.\yourun.cmd stop --id $runId
```

Common commands:

| Command | Purpose |
| --- | --- |
| `search` | Find saved projects, collections, and commands using `--query` or positional arguments |
| `run` / `dev` / `start` | Select and launch a saved command using `--project`, `--directory`, or `--command` |
| `status` / `list` / `activity` | Query status, execution records, and activity |
| `logs` | Read a run's logs, with support for a cursor and read limit |
| `stop` / `stop-all` | Stop a specific run using `--id`, `--command`, `--project`, or `--directory`, or stop all manageable runs |
| `restart` | Start a new run based on an existing one |
| `wait` | Wait for a run to finish, with an optional `--timeout` |
| `discover` / `inspect` | Scan a directory and inspect discovered commands without saving configuration |

CLI queries and operations return JSON. Exit codes: `0` for success or an accepted request, `1` for a failed or stopped task, `2` for a request or argument error, `3` for an unknown execution outcome, and `124` when `wait` times out.

Save command definitions through the GUI. The CLI does not provide an interface for executing arbitrary command text, writing project configuration, or opening an interactive shell. For the full list of options:

```powershell
.\yourun.cmd --help
```

## Building from source

These steps target the Windows x64 MyGo WebView edition. Install Node.js, npm, and a Go toolchain. The build script sets `GOTOOLCHAIN=go1.27.2+auto` to select a compatible toolchain with Go `1.27.2` as its default. Running the GUI also requires the Microsoft Edge WebView2 Runtime.

From this application's directory, run:

```powershell
npm --prefix frontend ci --ignore-scripts --no-audit --no-fund
node build.mjs --framework=mygo
```

The build checks frontend types, bundles the frontend, and compiles the Go application. Output files are written to:

```text
dist/builds/<build-number>/windows-amd64/
├── YouRun.exe
├── yourun.cmd
├── LICENSE
├── THIRD-PARTY-NOTICES.txt
└── THIRD-PARTY-LICENSES.txt
```

For the 0.1.1 release candidate, run `node build.mjs --framework=mygo --release=true`. Its output is archived under `dist/releases/0.1.1/portable/windows-amd64/`. Adding `--package=msix --msix-config=packaging/windows/msix.store.json` creates the Store candidate under `dist/releases/0.1.1/msix/windows-amd64/msix-mygo/`; this requires the Windows SDK. Existing binaries are not overwritten. A `PUBLISHED` file in the version directory prevents further release builds for that version.

Double-click `YouRun.exe` to open the window. For development, run `.\run.ps1` from the application's directory. It installs frontend dependencies as needed, builds the frontend, and launches the MyGo WebView edition by default.

## Current support

- **Windows**: The primary distribution target. The upcoming Microsoft Store package targets Windows 11 (build 22000 or later) on x64 devices and requires the WebView2 Runtime.
- **Launch at login**: Unpackaged Windows builds provide a setting to enable or disable launch at login. The current Microsoft Store package does not yet include this capability.
- **Other platforms**: The source includes build entry points for other platforms and window frameworks, but these are not yet verified distributions.

## Data and privacy

Projects, command definitions, run records, and logs are stored locally. Unpackaged Windows builds use `%APPDATA%\command-manager` by default. Set `COMMAND_MANAGER_HOME` to override the data directory. For MSIX packages, Windows application data management determines the actual location.

YouRun does not require an account or provide cloud sync. Tools launched through YouRun may access the network according to their own configuration. Saved environment variables and output logs may contain sensitive information; review them before sharing.

## License

The code is licensed under the [Apache License 2.0](LICENSE). See [THIRD-PARTY-NOTICES.txt](THIRD-PARTY-NOTICES.txt) for third-party dependency notices.
