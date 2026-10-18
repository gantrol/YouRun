package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"

	"commandmanager.local/core/internal/engine"
	"commandmanager.local/core/internal/projectmeta"
	"commandmanager.local/core/internal/updater"
	"commandmanager.local/core/internal/workspace"
)

type includeImagesKey struct{}

// WithIncludeImages controls whether CLI/MCP resource queries retain base64 process icons.
func WithIncludeImages(ctx context.Context, include bool) context.Context {
	return context.WithValue(ctx, includeImagesKey{}, include)
}

// ShouldIncludeImages reports whether base64 process icons were requested.
func ShouldIncludeImages(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	v, _ := ctx.Value(includeImagesKey{}).(bool)
	return v
}

// StructuredError preserves the canonical error code in the "error" JSON field
// while providing actionable candidates and recovery guidance for AI Agents.
type StructuredError struct {
	Code       string      `json:"error"`
	Message    string      `json:"message,omitempty"`
	Candidates any         `json:"candidates,omitempty"`
	Run        *engine.Run `json:"run,omitempty"`
	Recovery   string      `json:"recovery,omitempty"`
}

func (e *StructuredError) Error() string {
	return e.Code
}

type ProjectCandidate struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Directory string `json:"directory,omitempty"`
}

type CommandCandidate struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Text        string `json:"text"`
	ProjectID   string `json:"projectId,omitempty"`
	ProjectName string `json:"projectName,omitempty"`
	Directory   string `json:"directory,omitempty"`
	Kind        string `json:"kind,omitempty"`
}

type RunCandidate struct {
	ID        string `json:"id"`
	CommandID string `json:"commandId"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	PID       int    `json:"pid,omitempty"`
	Directory string `json:"directory"`
	Time      string `json:"time"`
}

func newStructuredError(code, message, recovery string, candidates any) *StructuredError {
	if message == "" || recovery == "" {
		defMsg, defRec := defaultErrorGuidance(code)
		if message == "" {
			message = defMsg
		}
		if recovery == "" {
			recovery = defRec
		}
	}
	return &StructuredError{
		Code:       code,
		Message:    message,
		Candidates: candidates,
		Recovery:   recovery,
	}
}

func defaultErrorGuidance(code string) (string, string) {
	switch code {
	case "ambiguousProject":
		return "Multiple saved projects matched the request.", "Pass --project <id> using one of the candidate project IDs, or run `yourun context`."
	case "projectMissing":
		return "No saved project matched the specified filter or directory.", "Run `yourun search` to list saved projects, or `yourun discover --directory <path>` to inspect an unimported project."
	case "ambiguousCommand":
		return "Multiple saved commands matched the launch mode in this scope.", "Pass --command <id> using one of the candidate command IDs, or run `yourun context` to inspect project commands."
	case "commandMissing":
		return "No saved command matched the requested mode or identifier.", "Run `yourun context` or `yourun search` to find valid command IDs, and use `yourun start --command <id>`."
	case "ambiguousRun":
		return "Multiple executions matched the target scope.", "Pass --id <runId> using one of the candidate run IDs, or narrow with --command <commandId>."
	case "noActiveRun":
		return "No active (running or stopping) execution matched the target scope.", "Run `yourun context` or `yourun list` to check execution states before stopping."
	case "runMissing":
		return "No matching execution record was found.", "Run `yourun list` or `yourun context` to find valid run IDs."
	case "alreadyRunning":
		return "The same saved command is already running in the same working directory.", "Use `yourun restart --id <runId>` to restart it, `yourun logs --id <runId> --tail 16384` to inspect its output, or `yourun stop --id <runId>` to stop it."
	case "runNotOwned":
		return "Execution ownership is unconfirmed (`unknown`); YouRun refuses to signal unowned OS processes.", "Launch a fresh owned execution via `yourun start`/`run`/`dev`, or inspect the process via `yourun processes`."
	case "replayUnavailable":
		return "This execution record cannot be replayed because its manifest is missing or its ownership is unknown.", "Start a new execution using `yourun start --command <commandId>` instead."
	case "jobBreakawayDenied":
		return "The current Windows Job Object disallows CREATE_BREAKAWAY_FROM_JOB, preventing detached worker creation.", "Invoke YouRun outside the restricted sandbox job, or run the command in a host process that permits job breakaway."
	case "invalidDirectory":
		return "The target directory is missing, inaccessible, or not an absolute path.", "Pass an existing directory path via `--directory <path>`."
	case "platformMismatch":
		return "The saved command is restricted to another operating system.", "Select a command compatible with the current platform (`" + runtime.GOOS + "`)."
	case "tooManyRuns":
		return "Maximum concurrent owned executions (16) reached.", "Stop finished or unused runs with `yourun stop --id <runId>` or `yourun stop-all`."
	case "waitTimeout":
		return "Timed out waiting for the execution to reach a terminal state; the execution continues running in the background.", "Call `yourun logs --id <runId> --tail 16384` to check progress, `yourun wait --id <runId>` to continue waiting, or `yourun stop --id <runId>` to terminate it."
	default:
		return "Command failed with code: " + code, "Run `yourun describe` or `yourun context` for usage and recovery instructions."
	}
}

func enrichErrorJSON(raw []byte) []byte {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return raw
	}
	code, ok := payload["error"].(string)
	if !ok || code == "" {
		return raw
	}
	msg, rec := defaultErrorGuidance(code)
	if _, hasMsg := payload["message"]; !hasMsg && msg != "" {
		payload["message"] = msg
	}
	if _, hasRec := payload["recovery"]; !hasRec && rec != "" {
		payload["recovery"] = rec
	}
	enriched, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return enriched
}

// SourceDriftReport compares a run's launch source snapshot with current disk Git status.
type SourceDriftReport struct {
	Status        string                `json:"status"`
	Reason        string                `json:"reason"`
	CurrentSource engine.SourceSnapshot `json:"currentSource"`
}

// CompareSourceDrift evaluates whether a running process's code has drifted since launch.
func CompareSourceDrift(launch *engine.SourceSnapshot, current engine.SourceSnapshot) SourceDriftReport {
	if launch == nil {
		return SourceDriftReport{
			Status:        "unrecorded",
			Reason:        "Launch source snapshot was not recorded for this run.",
			CurrentSource: current,
		}
	}
	if current.Inaccessible || launch.Inaccessible {
		return SourceDriftReport{
			Status:        "inaccessible",
			Reason:        "Working directory is inaccessible.",
			CurrentSource: current,
		}
	}
	if launch.NonGit || current.NonGit || (launch.Commit == "" && launch.Branch == "") {
		return SourceDriftReport{
			Status:        "unrecorded",
			Reason:        "Working directory is not tracked by Git.",
			CurrentSource: current,
		}
	}
	var diffs []string
	if launch.Branch != current.Branch {
		diffs = append(diffs, fmt.Sprintf("branch changed (%s -> %s)", launch.Branch, current.Branch))
	}
	if launch.Commit != current.Commit {
		diffs = append(diffs, fmt.Sprintf("HEAD changed (%s -> %s)", launch.ShortCommit, current.ShortCommit))
	}
	if launch.Dirty != current.Dirty {
		diffs = append(diffs, fmt.Sprintf("working tree dirty state changed (%v -> %v)", launch.Dirty, current.Dirty))
	}
	if len(diffs) > 0 {
		return SourceDriftReport{
			Status:        "changed",
			Reason:        "Code changed since launch: " + strings.Join(diffs, ", "),
			CurrentSource: current,
		}
	}
	return SourceDriftReport{
		Status:        "consistent",
		Reason:        "Working directory Git state matches launch snapshot.",
		CurrentSource: current,
	}
}

type ShortcutResolutionSummary struct {
	Status     string             `json:"status"`
	Command    *CommandCandidate  `json:"command,omitempty"`
	Candidates []CommandCandidate `json:"candidates,omitempty"`
	Error      string             `json:"error,omitempty"`
}

type WorkspaceLocationSummary struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Directory        string `json:"directory"`
	IsDefault        bool   `json:"isDefault,omitempty"`
	IsActive         bool   `json:"isActive,omitempty"`
	MatchesDirectory bool   `json:"matchesDirectory,omitempty"`
}

type ProjectCommandSummary struct {
	ID              string   `json:"id"`
	Title           string   `json:"title"`
	Text            string   `json:"text"`
	Directory       string   `json:"directory"`
	Shell           string   `json:"shell"`
	Platform        string   `json:"platform"`
	Kind            string   `json:"kind,omitempty"`
	ShortcutBound   bool     `json:"shortcutBound,omitempty"`
	ShortcutIcons   []string `json:"shortcutIcons,omitempty"`
	RequiresInputs  []string `json:"requiresInputs,omitempty"`
	AvailableOnHost bool     `json:"availableOnHost"`
}

type ContextProjectSummary struct {
	ID                 string                               `json:"id"`
	Name               string                               `json:"name"`
	ActiveWorkspaceID  string                               `json:"activeWorkspaceId,omitempty"`
	Workspaces         []WorkspaceLocationSummary           `json:"workspaces,omitempty"`
	ShortcutResolution map[string]ShortcutResolutionSummary `json:"shortcutResolution"`
	Commands           []ProjectCommandSummary              `json:"commands"`
}

type ActiveRunPortSummary struct {
	Protocol     string `json:"protocol"`
	LocalAddress string `json:"localAddress"`
	Port         int    `json:"port"`
	State        string `json:"state"`
	PID          int    `json:"pid"`
	ProcessName  string `json:"processName,omitempty"`
}

type EnrichedRunSummary struct {
	Run                engine.Run             `json:"run"`
	ProjectID          string                 `json:"projectId,omitempty"`
	ProjectName        string                 `json:"projectName,omitempty"`
	SourceDrift        *SourceDriftReport     `json:"sourceDrift,omitempty"`
	RestartRecommended bool                   `json:"restartRecommended,omitempty"`
	Ports              []ActiveRunPortSummary `json:"ports,omitempty"`
}

type ContextEnvironmentSummary struct {
	Platform          string `json:"platform"`
	Arch              string `json:"arch"`
	DataDir           string `json:"dataDir"`
	CanLaunchDetached bool   `json:"canLaunchDetached"`
	DetachedMode      string `json:"detachedMode"`
	ActiveRunsGlobal  int    `json:"activeRunsGlobal"`
	UnknownRunsGlobal int    `json:"unknownRunsGlobal"`
}

type ContextReport struct {
	Directory                   string                    `json:"directory"`
	Source                      engine.SourceSnapshot     `json:"source"`
	Environment                 ContextEnvironmentSummary `json:"environment"`
	ProjectMatchStatus          string                    `json:"projectMatchStatus"`
	MatchedProjects             []ContextProjectSummary   `json:"matchedProjects"`
	UnassignedDirectoryCommands []ProjectCommandSummary   `json:"unassignedDirectoryCommands,omitempty"`
	ActiveRuns                  []EnrichedRunSummary      `json:"activeRuns"`
	RecentRuns                  []EnrichedRunSummary      `json:"recentRuns"`
	SuggestedActions            []string                  `json:"suggestedActions"`
}

func summarizeCommand(cmd engine.Command, boundIcons map[string][]string) ProjectCommandSummary {
	kind, _, ok := classifyCommand(cmd)
	if !ok {
		kind = "custom"
	}
	icons := boundIcons[cmd.ID]
	return ProjectCommandSummary{
		ID:              cmd.ID,
		Title:           cmd.Title,
		Text:            cmd.Text,
		Directory:       cmd.Directory,
		Shell:           cmd.Shell,
		Platform:        cmd.Platform,
		Kind:            kind,
		ShortcutBound:   len(icons) > 0,
		ShortcutIcons:   icons,
		RequiresInputs:  cmd.Fields,
		AvailableOnHost: availableOnCurrentPlatform(cmd.Platform),
	}
}

func resolveShortcutSummary(mode string, state engine.Snapshot, dataDir, projectID, dirFlag string) ShortcutResolutionSummary {
	cmd, err := resolveLaunchCommand(mode, state, dataDir, "", projectID, dirFlag)
	if err == nil {
		kind, _, _ := classifyCommand(cmd)
		return ShortcutResolutionSummary{
			Status: "resolved",
			Command: &CommandCandidate{
				ID:        cmd.ID,
				Title:     cmd.Title,
				Text:      cmd.Text,
				Directory: cmd.Directory,
				Kind:      kind,
			},
		}
	}
	var se *StructuredError
	if ok := asStructuredError(err, &se); ok {
		var cands []CommandCandidate
		status := "missing"
		if se.Code == "ambiguousCommand" {
			status = "ambiguous"
			if rawCands, ok := se.Candidates.([]CommandCandidate); ok {
				cands = rawCands
			}
		}
		return ShortcutResolutionSummary{
			Status:     status,
			Candidates: cands,
			Error:      se.Code,
		}
	}
	return ShortcutResolutionSummary{
		Status: "missing",
		Error:  err.Error(),
	}
}

func asStructuredError(err error, target **StructuredError) bool {
	if err == nil {
		return false
	}
	if se, ok := err.(*StructuredError); ok {
		*target = se
		return true
	}
	return false
}

// BuildContextReport constructs a complete one-shot situational awareness snapshot for an Agent.
func BuildContextReport(ctx context.Context, dataDir string, client *engine.Client, service *workspace.Service, projectFlag, dirFlag string) (ContextReport, error) {
	targetDir := strings.TrimSpace(dirFlag)
	if targetDir == "" {
		if cwd, err := os.Getwd(); err == nil {
			targetDir = cwd
		}
	}
	if targetDir != "" {
		if abs, err := filepath.Abs(targetDir); err == nil {
			targetDir = filepath.Clean(abs)
		}
	}

	state, err := loadState(service)
	if err != nil {
		return ContextReport{}, err
	}
	activity, _ := client.Activity()
	canDetached, detachedMode := engine.CanLaunchDetached()
	meta, _ := projectmeta.LoadContext(ctx, dataDir)

	projectByID := map[string]engine.Project{}
	for _, p := range state.Definitions.Projects {
		projectByID[p.ID] = p
	}
	cmdByID := map[string]engine.Command{}
	for _, c := range state.Definitions.Commands {
		cmdByID[c.ID] = c
	}

	projectIDs, resolvedDir, resolveErr := resolveTargetProjectIDs(state, dataDir, projectFlag, targetDir)
	if resolvedDir != "" {
		targetDir = resolvedDir
	}
	matchStatus := "matched"
	if resolveErr != nil {
		var se *StructuredError
		if asStructuredError(resolveErr, &se) && se.Code == "ambiguousProject" {
			matchStatus = "ambiguous"
			if cands, ok := se.Candidates.([]ProjectCandidate); ok {
				for _, c := range cands {
					projectIDs = append(projectIDs, c.ID)
				}
			}
		} else {
			matchStatus = "unmatched"
		}
	} else if len(projectIDs) == 0 {
		matchStatus = "unmatched"
	} else if len(projectIDs) > 1 {
		matchStatus = "ambiguous"
	}

	matchedProjectSet := map[string]bool{}
	matchedProjects := make([]ContextProjectSummary, 0, len(projectIDs))
	for _, pid := range projectIDs {
		p, ok := projectByID[pid]
		if !ok || p.Hidden {
			continue
		}
		matchedProjectSet[pid] = true
		boundIcons := map[string][]string{}
		for _, b := range p.Buttons {
			if !b.Hidden && b.CommandID != "" {
				boundIcons[b.CommandID] = append(boundIcons[b.CommandID], b.Icon)
			}
		}
		var fallbackDirs []string
		var projCmds []ProjectCommandSummary
		for _, cmd := range state.Definitions.Commands {
			if cmd.Project != nil && *cmd.Project == pid && !cmd.Hidden {
				if cmd.Directory != "" {
					fallbackDirs = append(fallbackDirs, cmd.Directory)
				}
				projCmds = append(projCmds, summarizeCommand(cmd, boundIcons))
			}
		}
		locs, activeID := projectmeta.ResolveLocations(meta[pid], fallbackDirs)
		wsSummaries := make([]WorkspaceLocationSummary, 0, len(locs))
		for _, loc := range locs {
			_, matchesDir := pathDistance(loc.Directory, targetDir)
			wsSummaries = append(wsSummaries, WorkspaceLocationSummary{
				ID:               loc.ID,
				Name:             loc.Name,
				Directory:        loc.Directory,
				IsDefault:        loc.IsDefault,
				IsActive:         loc.ID == activeID,
				MatchesDirectory: matchesDir,
			})
		}
		// If targetDir wasn't explicitly passed and project was selected by --project,
		// use the project's active workspace directory for source inspection.
		if strings.TrimSpace(dirFlag) == "" && strings.TrimSpace(projectFlag) != "" && len(locs) > 0 {
			for _, loc := range locs {
				if loc.ID == activeID {
					targetDir = loc.Directory
					break
				}
			}
		}
		shortcuts := map[string]ShortcutResolutionSummary{
			"run":   resolveShortcutSummary("run", state, dataDir, pid, targetDir),
			"dev":   resolveShortcutSummary("dev", state, dataDir, pid, targetDir),
			"start": resolveShortcutSummary("start", state, dataDir, pid, targetDir),
		}
		matchedProjects = append(matchedProjects, ContextProjectSummary{
			ID:                 p.ID,
			Name:               p.Name,
			ActiveWorkspaceID:  activeID,
			Workspaces:         wsSummaries,
			ShortcutResolution: shortcuts,
			Commands:           projCmds,
		})
	}

	var unassignedCmds []ProjectCommandSummary
	if targetDir != "" {
		for _, cmd := range state.Definitions.Commands {
			if cmd.Hidden || cmd.Project != nil || strings.TrimSpace(cmd.Directory) == "" {
				continue
			}
			if normalizePath(cmd.Directory) == normalizePath(targetDir) {
				unassignedCmds = append(unassignedCmds, summarizeCommand(cmd, nil))
			}
		}
	}

	sourceSnap := engine.InspectSource(ctx, targetDir, "", "", false)

	// Collect listening/bound ports by runID if ResourceHandler is wired and there are active runs.
	portsByRunID := map[string][]ActiveRunPortSummary{}
	seenRunPort := map[string]bool{}
	hasActiveInScope := false
	for _, run := range state.Runs {
		if run.Status == "running" || run.Status == "stopping" {
			hasActiveInScope = true
			break
		}
	}
	if hasActiveInScope && ResourceHandler != nil {
		if rawPortsRes, pErr := ResourceHandler(WithIncludeImages(ctx, false), "ports", dataDir, service, "", false); pErr == nil {
			if b, mErr := json.Marshal(rawPortsRes); mErr == nil {
				var parsed struct {
					Items []struct {
						Protocol     string `json:"protocol"`
						LocalAddress string `json:"localAddress"`
						Port         int    `json:"port"`
						State        string `json:"state"`
						PID          int    `json:"pid"`
						ProcessName  string `json:"processName"`
						RunID        string `json:"runId"`
					} `json:"items"`
				}
				if json.Unmarshal(b, &parsed) == nil {
					for _, item := range parsed.Items {
						if item.RunID == "" {
							continue
						}
						if !strings.EqualFold(item.State, "listening") && !strings.EqualFold(item.Protocol, "UDP") {
							continue
						}
						dedupKey := item.RunID + "|" + strings.ToUpper(item.Protocol) + "|" + item.LocalAddress
						if seenRunPort[dedupKey] {
							continue
						}
						seenRunPort[dedupKey] = true
						portsByRunID[item.RunID] = append(portsByRunID[item.RunID], ActiveRunPortSummary{
							Protocol:     item.Protocol,
							LocalAddress: item.LocalAddress,
							Port:         item.Port,
							State:        item.State,
							PID:          item.PID,
							ProcessName:  item.ProcessName,
						})
					}
				}
			}
		}
	}

	activeRuns := []EnrichedRunSummary{}
	recentRuns := []EnrichedRunSummary{}
	for _, run := range state.Runs {
		cmd, hasCmd := cmdByID[run.CommandID]
		var projID, projName string
		if hasCmd && cmd.Project != nil {
			projID = *cmd.Project
			if p, ok := projectByID[projID]; ok {
				projName = p.Name
			}
		}
		inScope := false
		if len(matchedProjectSet) > 0 {
			if projID != "" {
				inScope = matchedProjectSet[projID]
			} else if targetDir != "" && run.Directory != "" {
				inScope = normalizePath(run.Directory) == normalizePath(targetDir)
			}
		} else if targetDir != "" && run.Directory != "" {
			if _, ok := pathDistance(targetDir, run.Directory); ok {
				inScope = true
			}
		}
		if !inScope {
			continue
		}
		isActive := run.Status == "running" || run.Status == "stopping"
		summary := EnrichedRunSummary{
			Run:         run,
			ProjectID:   projID,
			ProjectName: projName,
		}
		if isActive {
			curSrc := sourceSnap
			if run.Directory != "" && normalizePath(run.Directory) != normalizePath(targetDir) {
				curSrc = engine.InspectSource(ctx, run.Directory, "", "", false)
			}
			drift := CompareSourceDrift(run.LaunchSource, curSrc)
			summary.SourceDrift = &drift
			summary.RestartRecommended = drift.Status == "changed"
			summary.Ports = portsByRunID[run.ID]
			activeRuns = append(activeRuns, summary)
		} else if len(recentRuns) < 5 {
			recentRuns = append(recentRuns, summary)
		}
	}

	var suggestions []string
	if !canDetached {
		suggestions = append(suggestions, "Warning: current Windows Job Object disallows breakaway (`jobBreakawayDenied`); launch commands from a process outside the restricted sandbox.")
	}
	for _, ar := range activeRuns {
		if ar.RestartRecommended {
			suggestions = append(suggestions, fmt.Sprintf("Run `yourun restart --id %s` to apply recent code changes to active run %q.", ar.Run.ID, ar.Run.Title))
		}
		suggestions = append(suggestions, fmt.Sprintf("Run `yourun logs --id %s --tail 16384 --omit-data` to inspect latest output of %q.", ar.Run.ID, ar.Run.Title))
	}
	if len(activeRuns) == 0 && len(matchedProjects) == 1 {
		mp := matchedProjects[0]
		if devRes, ok := mp.ShortcutResolution["dev"]; ok && devRes.Status == "resolved" && devRes.Command != nil {
			suggestions = append(suggestions, fmt.Sprintf("Run `yourun dev --project %s` to launch %q (%s).", mp.ID, devRes.Command.Title, devRes.Command.Text))
		} else if runRes, ok := mp.ShortcutResolution["run"]; ok && runRes.Status == "resolved" && runRes.Command != nil {
			suggestions = append(suggestions, fmt.Sprintf("Run `yourun run --project %s` to launch %q (%s).", mp.ID, runRes.Command.Title, runRes.Command.Text))
		} else if len(mp.Commands) > 0 {
			suggestions = append(suggestions, fmt.Sprintf("Run `yourun start --command %s` to launch %q.", mp.Commands[0].ID, mp.Commands[0].Title))
		}
	}
	if len(recentRuns) > 0 && recentRuns[0].Run.Status == "failed" {
		suggestions = append(suggestions, fmt.Sprintf("Most recent run %q failed; inspect with `yourun logs --id %s --tail 16384 --omit-data`.", recentRuns[0].Run.Title, recentRuns[0].Run.ID))
	}
	if len(matchedProjects) == 0 && len(unassignedCmds) == 0 && targetDir != "" {
		suggestions = append(suggestions, fmt.Sprintf("Directory is not linked to a saved YouRun project; run `yourun discover --directory %q` to inspect candidate commands.", targetDir))
	}

	return ContextReport{
		Directory:                   targetDir,
		Source:                      sourceSnap,
		Environment:                 ContextEnvironmentSummary{Platform: runtime.GOOS, Arch: runtime.GOARCH, DataDir: dataDir, CanLaunchDetached: canDetached, DetachedMode: detachedMode, ActiveRunsGlobal: activity.Active, UnknownRunsGlobal: activity.Unknown},
		ProjectMatchStatus:          matchStatus,
		MatchedProjects:             matchedProjects,
		UnassignedDirectoryCommands: unassignedCmds,
		ActiveRuns:                  activeRuns,
		RecentRuns:                  recentRuns,
		SuggestedActions:            suggestions,
	}, nil
}

// ResourceFilterOptions defines optional CLI/MCP filters for processes, ports, and files.
type ResourceFilterOptions struct {
	Port          int
	PID           int
	Protocol      string
	State         string
	Ownership     string
	Project       string
	Query         string
	Limit         int
	IncludeImages bool
}

func matchesOwnershipFilter(ownership, filter string) bool {
	f := strings.ToLower(strings.TrimSpace(filter))
	if f == "" || f == "all" {
		return true
	}
	if f == "yourun" || f == "owned" {
		return ownership == "managed" || ownership == "unconfirmed"
	}
	return strings.EqualFold(ownership, f)
}

func matchesProjectSubstr(projID, projName, filter string) bool {
	f := strings.ToLower(strings.TrimSpace(filter))
	if f == "" {
		return true
	}
	return strings.EqualFold(projID, f) || strings.Contains(strings.ToLower(projName), f)
}

func matchesQueryTerms(query string, fields ...string) bool {
	terms := engine.SearchTerms(query)
	if len(terms) == 0 {
		return true
	}
	combined := strings.ToLower(strings.Join(fields, " "))
	for _, t := range terms {
		if !strings.Contains(combined, t) {
			return false
		}
	}
	return true
}

// QueryAndFilterResources executes the desktop resource query, strips base64 icons
// unless IncludeImages is set, and applies Agent filter flags.
func QueryAndFilterResources(ctx context.Context, mode, dataDir string, service *workspace.Service, targetPath string, recursive bool, opts ResourceFilterOptions) (map[string]any, error) {
	if ResourceHandler == nil {
		return nil, fmt.Errorf("resource handler unavailable")
	}
	raw, err := ResourceHandler(WithIncludeImages(ctx, opts.IncludeImages), mode, dataDir, service, targetPath, recursive)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}
	var envelope map[string]any
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		return nil, err
	}
	rawItems, ok := envelope["items"].([]any)
	if !ok {
		return envelope, nil
	}

	totalCount := len(rawItems)
	managedCount := 0
	filtered := make([]map[string]any, 0, len(rawItems))

	for _, entry := range rawItems {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		if !opts.IncludeImages {
			delete(item, "icon")
			delete(item, "processIcon")
		}
		ownership, _ := item["ownership"].(string)
		if ownership == "managed" {
			managedCount++
		}
		if !matchesOwnershipFilter(ownership, opts.Ownership) {
			continue
		}
		projID, _ := item["projectId"].(string)
		projName, _ := item["projectName"].(string)
		if !matchesProjectSubstr(projID, projName, opts.Project) {
			continue
		}
		if opts.PID > 0 {
			pidFloat, _ := item["pid"].(float64)
			if int(pidFloat) != opts.PID {
				continue
			}
		}
		if opts.Port > 0 {
			if mode == "ports" {
				portFloat, _ := item["port"].(float64)
				if int(portFloat) != opts.Port {
					continue
				}
			} else if mode == "processes" {
				matchedPort := false
				if portsSlice, ok := item["ports"].([]any); ok {
					for _, pVal := range portsSlice {
						if pf, ok := pVal.(float64); ok && int(pf) == opts.Port {
							matchedPort = true
							break
						}
					}
				}
				if !matchedPort {
					continue
				}
			}
		}
		if opts.Protocol != "" && mode == "ports" {
			proto, _ := item["protocol"].(string)
			if !strings.EqualFold(proto, strings.TrimSpace(opts.Protocol)) {
				continue
			}
		}
		if opts.State != "" && mode == "ports" {
			st, _ := item["state"].(string)
			if !strings.EqualFold(st, strings.TrimSpace(opts.State)) {
				continue
			}
		}
		if strings.TrimSpace(opts.Query) != "" {
			var searchFields []string
			for _, key := range []string{"name", "displayName", "processName", "processDisplayName", "executable", "commandLine", "localAddress", "remoteAddress", "state", "protocolLabel", "projectName", "projectId", "commandTitle", "commandId", "workspaceName", "directory", "targetFile", "ownership", "runId"} {
				if s, ok := item[key].(string); ok && s != "" {
					searchFields = append(searchFields, s)
				}
			}
			if pidFloat, ok := item["pid"].(float64); ok && pidFloat > 0 {
				searchFields = append(searchFields, strconv.Itoa(int(pidFloat)))
			}
			if portFloat, ok := item["port"].(float64); ok && portFloat > 0 {
				searchFields = append(searchFields, strconv.Itoa(int(portFloat)))
			}
			if !matchesQueryTerms(opts.Query, searchFields...) {
				continue
			}
		}
		filtered = append(filtered, item)
	}

	filteredCount := len(filtered)
	if opts.Limit > 0 && len(filtered) > opts.Limit {
		filtered = filtered[:opts.Limit]
	}
	envelope["totalCount"] = totalCount
	envelope["filteredCount"] = filteredCount
	envelope["managedCount"] = managedCount
	envelope["items"] = filtered
	return envelope, nil
}

type CLIFlagSpec struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Default     string `json:"default,omitempty"`
	Description string `json:"description"`
}

type CLICommandSpec struct {
	Name       string        `json:"name"`
	Aliases    []string      `json:"aliases,omitempty"`
	Category   string        `json:"category"`
	Mutates    bool          `json:"mutates"`
	Summary    string        `json:"summary"`
	Usage      string        `json:"usage"`
	Flags      []CLIFlagSpec `json:"flags,omitempty"`
	Examples   []string      `json:"examples"`
	ErrorCodes []string      `json:"errorCodes,omitempty"`
}

func commandCatalog() []CLICommandSpec {
	return []CLICommandSpec{
		{
			Name:     "context",
			Category: "agent-discovery",
			Mutates:  false,
			Summary:  "One-shot situational awareness for a directory or project: Git source snapshot, matched YouRun projects/workspaces, resolved run/dev/start commands, active runs with live code-drift & held ports, recent runs, and recommended next actions.",
			Usage:    "yourun context [--directory PATH] [--project ID|NAME]",
			Flags: []CLIFlagSpec{
				{Name: "--directory", Type: "string", Default: "cwd", Description: "Target directory to inspect"},
				{Name: "--project", Type: "string", Description: "Filter or select by project ID or name"},
			},
			Examples: []string{
				"yourun context",
				"yourun context --directory D:/repo/app",
				"yourun context --project my-app",
			},
		},
		{
			Name:     "capabilities",
			Aliases:  []string{"agent-info"},
			Category: "agent-discovery",
			Mutates:  false,
			Summary:  "Return YouRun version, runtime environment (including Windows Job breakaway capability), security model, recommended Agent workflows, CLI command catalog, and embedded MCP server info.",
			Usage:    "yourun capabilities",
			Examples: []string{"yourun capabilities"},
		},
		{
			Name:     "describe",
			Aliases:  []string{"schema"},
			Category: "agent-discovery",
			Mutates:  false,
			Summary:  "Return detailed specification, flags, examples, exit codes, and error recovery guidance for one or all YouRun commands.",
			Usage:    "yourun describe [SUBCOMMAND]",
			Examples: []string{
				"yourun describe",
				"yourun describe context",
				"yourun describe logs",
				"yourun describe ports",
			},
		},
		{
			Name:     "run",
			Category: "lifecycle",
			Mutates:  true,
			Summary:  "Launch the saved run command for a project or directory (prefers Play shortcut binding, then highest-ranked run script).",
			Usage:    "yourun run [--project ID|NAME] [--directory PATH] [--command ID] [--id RUN_ID]",
			Flags: []CLIFlagSpec{
				{Name: "--project", Type: "string", Description: "Project ID or name"},
				{Name: "--directory", Type: "string", Default: "cwd", Description: "Directory used to match a project or command"},
				{Name: "--command", Type: "string", Description: "Explicit saved command ID"},
				{Name: "--id", Type: "string", Description: "Optional idempotency run ID"},
			},
			Examples:   []string{"yourun run", "yourun run --project my-app"},
			ErrorCodes: []string{"ambiguousProject", "ambiguousCommand", "commandMissing", "alreadyRunning", "jobBreakawayDenied"},
		},
		{
			Name:     "dev",
			Category: "lifecycle",
			Mutates:  true,
			Summary:  "Launch the saved dev/debug/watch command for a project or directory (prefers Bolt shortcut binding, then highest-ranked dev/debug script).",
			Usage:    "yourun dev [--project ID|NAME] [--directory PATH] [--command ID] [--id RUN_ID]",
			Flags: []CLIFlagSpec{
				{Name: "--project", Type: "string", Description: "Project ID or name"},
				{Name: "--directory", Type: "string", Default: "cwd", Description: "Directory used to match a project or command"},
				{Name: "--command", Type: "string", Description: "Explicit saved command ID"},
				{Name: "--id", Type: "string", Description: "Optional idempotency run ID"},
			},
			Examples:   []string{"yourun dev", "yourun dev --project my-app"},
			ErrorCodes: []string{"ambiguousProject", "ambiguousCommand", "commandMissing", "alreadyRunning", "jobBreakawayDenied"},
		},
		{
			Name:     "start",
			Category: "lifecycle",
			Mutates:  true,
			Summary:  "Launch a specific saved command by --command ID, or the single bound/available command in a project or directory.",
			Usage:    "yourun start [--command ID] [--project ID|NAME] [--directory PATH] [--id RUN_ID]",
			Flags: []CLIFlagSpec{
				{Name: "--command", Type: "string", Description: "Saved command ID"},
				{Name: "--project", Type: "string", Description: "Project ID or name"},
				{Name: "--directory", Type: "string", Default: "cwd", Description: "Directory used to match a project or command"},
				{Name: "--id", Type: "string", Description: "Optional idempotency run ID"},
			},
			Examples:   []string{"yourun start --command cmd-123", "yourun start --project my-app"},
			ErrorCodes: []string{"ambiguousProject", "ambiguousCommand", "commandMissing", "alreadyRunning", "jobBreakawayDenied"},
		},
		{
			Name:       "stop",
			Category:   "lifecycle",
			Mutates:    true,
			Summary:    "Stop an active owned execution by --id, or auto-resolve the single active run by --command, --project, or --directory.",
			Usage:      "yourun stop [--id RUN_ID] [--command ID] [--project ID|NAME] [--directory PATH]",
			Examples:   []string{"yourun stop", "yourun stop --id <runId>", "yourun stop --project my-app"},
			ErrorCodes: []string{"noActiveRun", "ambiguousRun", "runNotOwned"},
		},
		{
			Name:       "restart",
			Aliases:    []string{"replay"},
			Category:   "lifecycle",
			Mutates:    true,
			Summary:    "Atomically stop (if active) and replay an owned execution using its saved snapshot, or auto-resolve the target run by --command, --project, or --directory.",
			Usage:      "yourun restart [--id ORIGINAL_RUN_ID] [--request-id NEW_RUN_ID] [--command ID] [--project ID|NAME] [--directory PATH]",
			Examples:   []string{"yourun restart", "yourun restart --id <runId>", "yourun restart --project my-app"},
			ErrorCodes: []string{"ambiguousRun", "runMissing", "replayUnavailable", "jobBreakawayDenied"},
		},
		{
			Name:     "stop-all",
			Category: "lifecycle",
			Mutates:  true,
			Summary:  "Request termination of all active owned executions and wait up to 10s for completion.",
			Usage:    "yourun stop-all",
			Examples: []string{"yourun stop-all"},
		},
		{
			Name:     "logs",
			Category: "observability",
			Mutates:  false,
			Summary:  "Read output logs of an execution. Auto-resolves the active or latest run in scope when --id is omitted, and reads the latest tail by default.",
			Usage:    "yourun logs [--id RUN_ID] [--project ID|NAME] [--directory PATH] [--command ID] [--tail BYTES] [--cursor BYTES] [--limit BYTES] [--omit-data]",
			Flags: []CLIFlagSpec{
				{Name: "--id", Type: "string", Description: "Run ID (auto-resolved from project/directory/command if omitted)"},
				{Name: "--tail", Type: "int", Default: "131072", Description: "Read the last N bytes from the end of the log"},
				{Name: "--cursor", Type: "int", Description: "Explicit starting byte offset (overrides tail default)"},
				{Name: "--limit", Type: "int", Default: "131072", Description: "Maximum bytes to return (1..1048576)"},
				{Name: "--omit-data", Type: "bool", Default: "false", Description: "Omit base64 byte array to save context tokens"},
			},
			Examples: []string{
				"yourun logs --tail 16384 --omit-data",
				"yourun logs --id <runId> --tail 32768 --omit-data",
				"yourun logs --id <runId> --cursor 0 --limit 65536",
			},
		},
		{
			Name:     "wait",
			Category: "observability",
			Mutates:  false,
			Summary:  "Wait for an execution to reach a terminal state without owning or cancelling it on timeout.",
			Usage:    "yourun wait [--id RUN_ID] [--project ID|NAME] [--directory PATH] [--command ID] [--timeout DURATION]",
			Flags: []CLIFlagSpec{
				{Name: "--id", Type: "string", Description: "Run ID (or auto-resolved active run)"},
				{Name: "--timeout", Type: "duration", Default: "0s", Description: "Maximum wait duration, e.g. 10s or 2m (exit code 124 on timeout)"},
			},
			Examples: []string{"yourun wait --id <runId> --timeout 15s"},
		},
		{
			Name:     "status",
			Category: "observability",
			Mutates:  false,
			Summary:  "Return the full workspace snapshot (definitions + runs), or a single execution record when --id is passed.",
			Usage:    "yourun status [--id RUN_ID]",
			Examples: []string{"yourun status --id <runId>", "yourun status"},
		},
		{
			Name:     "list",
			Category: "observability",
			Mutates:  false,
			Summary:  "List execution records, optionally filtered by --query.",
			Usage:    "yourun list [--query TEXT]",
			Examples: []string{"yourun list", "yourun list --query vite"},
		},
		{
			Name:     "search",
			Category: "observability",
			Mutates:  false,
			Summary:  "Search saved projects, collections, and commands.",
			Usage:    "yourun search [QUERY] [--query TEXT] [--project ID|NAME] [--hidden]",
			Examples: []string{"yourun search dev", "yourun search --project my-app"},
		},
		{
			Name:     "activity",
			Category: "observability",
			Mutates:  false,
			Summary:  "Return counts of active and unknown executions.",
			Usage:    "yourun activity",
			Examples: []string{"yourun activity"},
		},
		{
			Name:     "ports",
			Category: "os-resources",
			Mutates:  false,
			Summary:  "Inspect active local TCP/UDP ports with YouRun run/project/workspace attribution (base64 icons stripped by default).",
			Usage:    "yourun ports [--port PORT] [--protocol tcp|udp] [--ownership managed|unconfirmed|external|yourun] [--project ID|NAME] [--pid PID] [--query TEXT] [--limit N] [--include-images]",
			Flags: []CLIFlagSpec{
				{Name: "--port", Type: "int", Description: "Filter by local port number (e.g. 3000)"},
				{Name: "--protocol", Type: "string", Description: "Filter by tcp or udp"},
				{Name: "--ownership", Type: "string", Description: "Filter by managed, unconfirmed, external, or yourun"},
				{Name: "--project", Type: "string", Description: "Filter by attributed project ID or name"},
				{Name: "--pid", Type: "int", Description: "Filter by owning PID"},
				{Name: "--query", Type: "string", Description: "Filter by process name, command line, address, or project"},
				{Name: "--limit", Type: "int", Description: "Maximum items to return"},
				{Name: "--include-images", Type: "bool", Default: "false", Description: "Include base64 process icons"},
			},
			Examples: []string{
				"yourun ports --port 3000",
				"yourun ports --ownership managed",
				"yourun ports --query node",
			},
		},
		{
			Name:     "processes",
			Category: "os-resources",
			Mutates:  false,
			Summary:  "Inspect running OS processes with YouRun run/project/workspace attribution and listening ports (base64 icons stripped by default).",
			Usage:    "yourun processes [--pid PID] [--port PORT] [--ownership managed|unconfirmed|external|yourun] [--project ID|NAME] [--query TEXT] [--limit N] [--include-images]",
			Flags: []CLIFlagSpec{
				{Name: "--pid", Type: "int", Description: "Filter by PID"},
				{Name: "--port", Type: "int", Description: "Filter by listening port number"},
				{Name: "--ownership", Type: "string", Description: "Filter by managed, unconfirmed, external, or yourun"},
				{Name: "--project", Type: "string", Description: "Filter by attributed project ID or name"},
				{Name: "--query", Type: "string", Description: "Filter by process name, command line, executable, or project"},
				{Name: "--limit", Type: "int", Description: "Maximum items to return"},
				{Name: "--include-images", Type: "bool", Default: "false", Description: "Include base64 process icons"},
			},
			Examples: []string{
				"yourun processes --ownership managed",
				"yourun processes --pid 1234",
				"yourun processes --query vite --limit 20",
			},
		},
		{
			Name:     "files",
			Category: "os-resources",
			Mutates:  false,
			Summary:  "Inspect processes holding open handles on a file or directory (defaults to current working directory).",
			Usage:    "yourun files [PATH] [--directory PATH] [--recursive] [--ownership managed|unconfirmed|external|yourun] [--pid PID] [--query TEXT] [--limit N]",
			Flags: []CLIFlagSpec{
				{Name: "--directory", Type: "string", Default: "cwd", Description: "Target file or directory path"},
				{Name: "--recursive", Type: "bool", Default: "false", Description: "Include subdirectories when target is a directory"},
				{Name: "--ownership", Type: "string", Description: "Filter by managed, unconfirmed, external, or yourun"},
				{Name: "--pid", Type: "int", Description: "Filter by PID"},
				{Name: "--query", Type: "string", Description: "Filter by process name, command line, or target file"},
				{Name: "--limit", Type: "int", Description: "Maximum items to return"},
			},
			Examples: []string{
				"yourun files --directory ./dist --recursive",
				"yourun files D:/repo/app/build",
			},
		},
		{
			Name:     "discover",
			Category: "project-inspection",
			Mutates:  false,
			Summary:  "Scan a project directory using read-only manifest rules to discover candidate commands without modifying workspace definitions.",
			Usage:    "yourun discover [--directory PATH]",
			Examples: []string{"yourun discover", "yourun discover --directory D:/repo/app"},
		},
		{
			Name:     "inspect",
			Category: "project-inspection",
			Mutates:  false,
			Summary:  "Inspect project candidate display names, icons, and discovery issues.",
			Usage:    "yourun inspect [--directory PATH] [--name TEXT] [--platform windows|darwin|linux] [--include-images]",
			Examples: []string{"yourun inspect --directory D:/repo/app"},
		},
		{
			Name:     "source",
			Category: "project-inspection",
			Mutates:  false,
			Summary:  "Inspect read-only Git repository status (branch, commit, dirty state) of a directory without taking Git index locks.",
			Usage:    "yourun source [--directory PATH]",
			Examples: []string{"yourun source", "yourun source --directory D:/repo/app"},
		},
		{
			Name:     "mcp",
			Category: "agent-protocol",
			Mutates:  false,
			Summary:  "Start the embedded Model Context Protocol (MCP) JSON-RPC 2.0 server over stdio.",
			Usage:    "yourun mcp [--data PATH]",
			Examples: []string{"yourun mcp", "YouRun.exe mcp"},
		},
		{
			Name:     "integrate",
			Category: "agent-protocol",
			Mutates:  true,
			Summary:  "Inspect, selectively enable, or revoke individual AI Agent integrations (user PATH, global Agent Skills, and MCP client configs).",
			Usage:    "yourun integrate [--status] | [--enable --target ID[,ID...]|all] | [--disable [--target ID[,ID...]|all]]",
			Flags: []CLIFlagSpec{
				{Name: "--status", Type: "bool", Default: "true", Description: "Inspect current Agent integration targets without modifying files"},
				{Name: "--enable", Type: "bool", Default: "false", Description: "Enable the specified --target integration(s)"},
				{Name: "--disable", Type: "bool", Default: "false", Description: "Revoke the specified --target integration(s) (or all when --target is omitted)"},
				{Name: "--target", Type: "string", Description: "Target ID(s): user-path, skill-gemini, skill-agents, skill-claude, skill-codex, mcp-cursor, mcp-claude-code, mcp-claude-desktop, mcp-windsurf, or all"},
			},
			Examples: []string{
				"yourun integrate --status",
				"yourun integrate --enable --target user-path,skill-gemini",
				"yourun integrate --disable --target mcp-cursor",
				"yourun integrate --disable",
			},
		},
	}
}

// BuildCapabilitiesReport returns self-discovery metadata for AI Agents.
func BuildCapabilitiesReport(ctx context.Context, dataDir string) (map[string]any, error) {
	exe, _ := os.Executable()
	canDetached, detachedMode := engine.CanLaunchDetached()

	activeRuns, unknownRuns, projectCount, commandCount := 0, 0, 0, 0
	if c, err := engine.Open(dataDir, exe); err == nil {
		if act, aErr := c.Activity(); aErr == nil {
			activeRuns = act.Active
			unknownRuns = act.Unknown
		}
		if ws, wErr := workspace.New(dataDir, c.Request); wErr == nil {
			if defs, dErr := ws.Definitions(ctx); dErr == nil {
				for _, p := range defs.Projects {
					if !p.Hidden {
						projectCount++
					}
				}
				for _, cmd := range defs.Commands {
					if !cmd.Hidden {
						commandCount++
					}
				}
			}
		}
	}

	catalog := commandCatalog()
	sort.SliceStable(catalog, func(i, j int) bool {
		if catalog[i].Category != catalog[j].Category {
			return catalog[i].Category < catalog[j].Category
		}
		return catalog[i].Name < catalog[j].Name
	})

	return map[string]any{
		"product":      "YouRun",
		"version":      updater.Version,
		"build":        updater.Build,
		"distribution": updater.Distribution,
		"protocol":     engine.Protocol,
		"platform":     runtime.GOOS,
		"arch":         runtime.GOARCH,
		"dataDir":      dataDir,
		"executable":   exe,
		"environment": map[string]any{
			"canLaunchDetached":        canDetached,
			"detachedMode":             detachedMode,
			"resourceHandlerAvailable": ResourceHandler != nil,
			"activeRuns":               activeRuns,
			"unknownRuns":              unknownRuns,
			"projectCount":             projectCount,
			"commandCount":             commandCount,
		},
		"securityModel": map[string]any{
			"savedDefinitionsOnly":        true,
			"arbitraryExecAllowed":        false,
			"directoryOverrideAllowed":    false,
			"unownedProcessSignalAllowed": false,
			"notes": []string{
				"YouRun only launches commands already saved in the user's workspace definitions (`run`, `dev`, `start`).",
				"`--directory` is used solely to match/select a saved project or command; it never overrides a command's configured working directory.",
				"`stop`, `restart`, and `stop-all` only signal executions actively owned by YouRun (`running`/`stopping`), never `unknown` or external OS processes.",
			},
		},
		"recommendedWorkflows": map[string][]string{
			"enterProjectDirectory": {
				"1. Run `yourun context` (or MCP tool `yourun_context`) to inspect matched projects, shortcut commands, active runs, code drift, and ports in one call.",
				"2. If an active run has `restartRecommended: true` (`sourceDrift.status == \"changed\"`), run `yourun restart --id <runId>`.",
				"3. Otherwise launch with `yourun dev` / `yourun run` / `yourun start --command <id>`.",
			},
			"diagnosePortConflict": {
				"1. Run `yourun ports --port <PORT>` (or MCP tool `yourun_resources` with `mode: \"ports\", port: <PORT>`).",
				"2. Check `ownership`: if `managed`, YouRun owns the process via `runId` (`projectName`, `commandTitle`, `launchSource`).",
				"3. Inspect output with `yourun logs --id <runId> --tail 16384 --omit-data`, or stop/restart it with `yourun stop --id <runId>` / `yourun restart --id <runId>`.",
			},
			"diagnoseFileOrDirectoryLock": {
				"1. Run `yourun files --directory <PATH> --recursive` (or MCP tool `yourun_resources` with `mode: \"files\"`).",
				"2. If a locking process has `ownership: \"managed\"`, stop the owning run cleanly via `yourun stop --id <runId>`.",
			},
			"inspectLogsWithoutFloodingContext": {
				"1. Run `yourun logs --tail 16384 --omit-data` (auto-resolves active/latest run in current directory and omits redundant base64 bytes).",
				"2. Use `nextCursor` with `--cursor <nextCursor>` to tail incremental output.",
			},
		},
		"mcp": map[string]any{
			"subcommand": "mcp",
			"transport":  "stdio",
			"tools": []string{
				"yourun_context",
				"yourun_launch",
				"yourun_control",
				"yourun_logs",
				"yourun_search",
				"yourun_resources",
				"yourun_discover",
			},
		},
		"commands": catalog,
	}, nil
}

// BuildDescribeReport returns detailed schema and error documentation for a command or all commands.
func BuildDescribeReport(target string) (any, error) {
	catalog := commandCatalog()
	exitCodes := map[string]string{
		"0":   "Success or launch/stop/restart accepted",
		"1":   "Waited execution finished with failed or stopped status",
		"2":   "Request validation, resolution, or conflict error (see `error`, `candidates`, and `recovery` in JSON)",
		"3":   "Waited execution ended in `unknown` ownership state",
		"124": "`wait` timed out (`waitTimeout`); execution continues running in background",
	}
	errorCatalog := map[string]map[string]string{}
	for _, code := range []string{
		"ambiguousProject", "projectMissing", "ambiguousCommand", "commandMissing",
		"ambiguousRun", "noActiveRun", "runMissing", "alreadyRunning", "runNotOwned",
		"replayUnavailable", "jobBreakawayDenied", "invalidDirectory", "platformMismatch",
		"tooManyRuns", "waitTimeout",
	} {
		msg, rec := defaultErrorGuidance(code)
		errorCatalog[code] = map[string]string{
			"message":  msg,
			"recovery": rec,
		}
	}

	needle := strings.ToLower(strings.TrimSpace(target))
	if needle == "" {
		return map[string]any{
			"product":      "YouRun",
			"protocol":     engine.Protocol,
			"commands":     catalog,
			"exitCodes":    exitCodes,
			"errorCatalog": errorCatalog,
		}, nil
	}
	for _, spec := range catalog {
		if spec.Name == needle || containsString(spec.Aliases, needle) {
			return map[string]any{
				"command":   spec,
				"exitCodes": exitCodes,
			}, nil
		}
	}
	return nil, newStructuredError(
		"unknownCommand",
		fmt.Sprintf("Unknown subcommand %q.", target),
		"Run `yourun describe` without arguments to list all valid commands.",
		nil,
	)
}
