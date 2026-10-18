package cli

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"commandmanager.local/core/internal/engine"
	"commandmanager.local/core/internal/projectmeta"
	"commandmanager.local/core/internal/workspace"
)

var (
	compoundPattern   = regexp.MustCompile(`[;&|\r\n` + "`" + `]`)
	tokenPattern      = regexp.MustCompile(`"[^"]*"|'[^']*'|[^\s]+`)
	winEntryPattern   = regexp.MustCompile(`(?i)^\.[\\/](run|start|debug|build|release|package)\.(cmd|bat|ps1)$`)
	shEntryPattern    = regexp.MustCompile(`(?i)^\.\/(run|start|debug|build|release|package)\.sh$`)
	extStripPattern   = regexp.MustCompile(`(?i)\.(exe|cmd|bat|ps1)$`)
	camelSplitPattern = regexp.MustCompile(`([a-z])([A-Z])`)
	wordSplitPattern  = regexp.MustCompile(`[\s:._-]+`)
	configFlagPattern = regexp.MustCompile(`(?:^|\s)(?:--configuration[= ]|--config[= ]|-configuration |-c |[-/]p:configuration=)(debug|release)(?:\s|$)`)
	cmakeFlagPattern  = regexp.MustCompile(`(?:^|\s)--preset(?:=|\s)(debug|release)(?:\s|$)`)
	msbuildTarget     = regexp.MustCompile(`^[-/]t(arget)?:`)
	msbuildBuild      = regexp.MustCompile(`^[-/]t(arget)?:(re)?build$`)
	inspectFlag       = regexp.MustCompile(`^--inspect(?:-brk)?(?:=|$)`)
)

type rankedLaunch struct {
	command engine.Command
	kind    string
	score   int
}

func classifyCommand(cmd engine.Command) (string, int, bool) {
	if compoundPattern.MatchString(cmd.Text) {
		return "", 0, false
	}
	rawTokens := tokenPattern.FindAllString(cmd.Text, -1)
	tokens := make([]string, 0, len(rawTokens))
	for _, t := range rawTokens {
		if len(t) >= 2 && ((t[0] == '"' && t[len(t)-1] == '"') || (t[0] == '\'' && t[len(t)-1] == '\'')) {
			t = t[1 : len(t)-1]
		}
		tokens = append(tokens, t)
	}
	if len(tokens) == 0 {
		return "", 0, false
	}
	executable := func(token string) string {
		parts := strings.FieldsFunc(token, func(r rune) bool { return r == '/' || r == '\\' })
		last := token
		if len(parts) > 0 {
			last = parts[len(parts)-1]
		}
		return strings.ToLower(extStripPattern.ReplaceAllString(last, ""))
	}
	invoked := tokens[0]
	tokens = tokens[1:]
	tool := executable(invoked)
	if tool == "npx" || (oneOfString(tool, "npm", "pnpm", "yarn", "bun") && len(tokens) > 0 && oneOfString(strings.ToLower(tokens[0]), "exec", "x")) {
		if tool != "npx" {
			tokens = tokens[1:]
		}
		sep := -1
		for i, tok := range tokens {
			if tok == "--" {
				sep = i
				break
			}
		}
		if sep >= 0 {
			tokens = tokens[sep+1:]
		} else {
			for len(tokens) > 0 && strings.HasPrefix(tokens[0], "-") {
				flag := tokens[0]
				tokens = tokens[1:]
				if flag == "--package" || flag == "-p" {
					if len(tokens) > 0 {
						tokens = tokens[1:]
					}
				} else if flag != "--no-install" && flag != "--offline" && flag != "--no" && !strings.HasPrefix(flag, "--package=") {
					return "", 0, false
				}
			}
		}
		if len(tokens) == 0 {
			return "", 0, false
		}
		tool = executable(tokens[0])
		tokens = tokens[1:]
	}
	args := make([]string, len(tokens))
	for i, tok := range tokens {
		args[i] = strings.ToLower(tok)
	}
	action := ""
	if len(tokens) > 0 {
		action = tokens[0]
	}
	script := false
	entrypoint := false
	switch {
	case winEntryPattern.MatchString(invoked) && len(tokens) == 0:
		action = tool
		entrypoint = true
	case tool == "bash" && len(tokens) == 1 && shEntryPattern.MatchString(tokens[0]):
		action = strings.TrimSuffix(executable(tokens[0]), ".sh")
		entrypoint = true
	case oneOfString(tool, "npm", "pnpm", "yarn", "bun"):
		script = true
		if len(args) > 0 && oneOfString(args[0], "run", "run-script") {
			if len(tokens) > 1 {
				action = tokens[1]
			} else {
				action = ""
			}
		} else if len(args) > 0 && oneOfString(args[0], "exec", "dlx") {
			return "", 0, false
		}
	case oneOfString(tool, "go", "cargo", "swift", "dotnet", "flutter", "wails", "tauri", "vite", "next", "nuxt", "astro", "nest", "expo", "react-native", "ng"):
		if oneOfString(tool, "vite", "next", "nuxt", "astro", "expo") && action == "" {
			action = "dev"
		}
		if tool == "dotnet" && len(args) > 0 && args[0] == "watch" && containsString(args, "test") {
			return "", 0, false
		}
	case oneOfString(tool, "gradle", "gradlew", "mvn", "mvnw"):
		var tasks []string
		for _, tok := range tokens {
			if !strings.HasPrefix(tok, "-") {
				tasks = append(tasks, tok)
			}
		}
		if len(tasks) != 1 {
			return "", 0, false
		}
		action = tasks[0]
	case tool == "cmake" && len(args) > 0 && args[0] == "--build":
		action = "build"
	case tool == "msbuild":
		for _, arg := range args {
			if msbuildTarget.MatchString(arg) && !msbuildBuild.MatchString(arg) {
				return "", 0, false
			}
		}
		action = "build"
	case tool == "xcodebuild" && containsString(args, "build") && !containsString(args, "test"):
		action = "build"
	default:
		return "", 0, false
	}

	spaced := camelSplitPattern.ReplaceAllString(action, "$1 $2")
	words := wordSplitPattern.Split(strings.ToLower(spaced), -1)
	for _, w := range words {
		if oneOfString(w, "test", "lint", "check", "typecheck", "format", "clean", "install", "deploy", "publish", "upload", "migrate", "seed", "delete", "remove") {
			return "", 0, false
		}
	}
	has := func(targets ...string) bool {
		for _, t := range targets {
			if containsString(words, t) {
				return true
			}
		}
		return false
	}
	isBuild := has("build", "rebuild", "assemble", "bundle", "package", "pack")
	isRun := has("run", "start", "serve")
	isDebug := has("debug", "dev", "develop", "development", "watch")
	isRelease := has("release", "prod", "production") || (entrypoint && has("package"))
	if !isBuild && !isRun && !isDebug && !isRelease {
		return "", 0, false
	}
	flags := strings.Join(args, " ")
	configuration := ""
	if m := configFlagPattern.FindStringSubmatch(flags); len(m) > 1 {
		configuration = m[1]
	} else if tool == "cmake" {
		if m := cmakeFlagPattern.FindStringSubmatch(flags); len(m) > 1 {
			configuration = m[1]
		}
	}
	exact := 0
	if len(words) == 1 && words[0] != "" {
		exact = 20
	}
	configured := 0
	if entrypoint {
		configured = 30
	} else if script {
		configured = 10
	}
	if isBuild && !script && !entrypoint {
		return "build", 50 + exact + configured, true
	}
	if isRelease || containsString(args, "--release") || configuration == "release" {
		return "release", 150 + exact + configured, true
	}
	hasInspect := false
	for _, arg := range args {
		if inspectFlag.MatchString(arg) {
			hasInspect = true
			break
		}
	}
	if isDebug || containsString(args, "--debug") || containsString(args, "--watch") || hasInspect || configuration == "debug" {
		bonus := 0
		if has("debug") {
			bonus = 10
		} else if has("dev") {
			bonus = 5
		}
		return "debug", 100 + exact + configured + bonus, true
	}
	if isRun {
		bonus := 0
		if has("run") {
			bonus = 10
		} else if has("start") {
			bonus = 5
		}
		if len(args) > 1 && args[1] == "." {
			bonus += 5
		}
		return "run", 100 + exact + configured + bonus, true
	}
	if isBuild {
		return "build", 50 + exact + configured, true
	}
	return "", 0, false
}

func oneOfString(value string, choices ...string) bool {
	for _, c := range choices {
		if value == c {
			return true
		}
	}
	return false
}

func containsString(slice []string, target string) bool {
	return oneOfString(target, slice...)
}

func normalizePath(p string) string {
	cleaned := filepath.Clean(p)
	if runtime.GOOS == "windows" {
		return strings.ToLower(cleaned)
	}
	return cleaned
}

func pathDistance(base, target string) (int, bool) {
	b := normalizePath(base)
	t := normalizePath(target)
	rel, err := filepath.Rel(b, t)
	if err != nil {
		return 0, false
	}
	if rel == "." {
		return 0, true
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return 0, false
	}
	return len(strings.Split(rel, string(filepath.Separator))), true
}

func availableOnCurrentPlatform(platform string) bool {
	switch platform {
	case "Windows":
		return runtime.GOOS == "windows"
	case "macOS":
		return runtime.GOOS != "windows"
	default:
		return true
	}
}

func loadState(service *workspace.Service) (engine.Snapshot, error) {
	status, data, err := service.Request(context.Background(), "GET", "/api/state", nil)
	if err != nil {
		return engine.Snapshot{}, err
	}
	if status >= 400 {
		return engine.Snapshot{}, errors.New("requestFailed")
	}
	var state engine.Snapshot
	if err = json.Unmarshal(data, &state); err != nil {
		return engine.Snapshot{}, err
	}
	return state, nil
}

func projectCandidatesForIDs(state engine.Snapshot, dataDir string, ids []string) []ProjectCandidate {
	meta, _ := projectmeta.Load(dataDir)
	byID := map[string]engine.Project{}
	for _, p := range state.Definitions.Projects {
		byID[p.ID] = p
	}
	out := make([]ProjectCandidate, 0, len(ids))
	for _, id := range ids {
		p, ok := byID[id]
		if !ok {
			continue
		}
		var fallback []string
		for _, c := range state.Definitions.Commands {
			if c.Project != nil && *c.Project == id && c.Directory != "" {
				fallback = append(fallback, c.Directory)
			}
		}
		locs, activeID := projectmeta.ResolveLocations(meta[id], fallback)
		dir := ""
		for _, loc := range locs {
			if loc.ID == activeID {
				dir = loc.Directory
				break
			}
		}
		if dir == "" && len(locs) > 0 {
			dir = locs[0].Directory
		}
		out = append(out, ProjectCandidate{ID: p.ID, Name: p.Name, Directory: dir})
	}
	return out
}

func commandCandidatesFromCommands(cmds []engine.Command, projectByID map[string]engine.Project) []CommandCandidate {
	out := make([]CommandCandidate, 0, len(cmds))
	for _, c := range cmds {
		var pid, pname string
		if c.Project != nil {
			pid = *c.Project
			if p, ok := projectByID[pid]; ok {
				pname = p.Name
			}
		}
		kind, _, _ := classifyCommand(c)
		out = append(out, CommandCandidate{
			ID:          c.ID,
			Title:       c.Title,
			Text:        c.Text,
			ProjectID:   pid,
			ProjectName: pname,
			Directory:   c.Directory,
			Kind:        kind,
		})
	}
	return out
}

func runCandidatesFromRuns(runs []engine.Run) []RunCandidate {
	out := make([]RunCandidate, 0, len(runs))
	for _, r := range runs {
		out = append(out, RunCandidate{
			ID:        r.ID,
			CommandID: r.CommandID,
			Title:     r.Title,
			Status:    r.Status,
			PID:       r.PID,
			Directory: r.Directory,
			Time:      r.Time,
		})
	}
	return out
}

func resolveTargetProjectIDs(state engine.Snapshot, dataDir, projectFlag, dirFlag string) ([]string, string, error) {
	projects := state.Definitions.Projects
	if strings.TrimSpace(projectFlag) != "" {
		needle := strings.TrimSpace(projectFlag)
		var exactID, exactName, partial []string
		for _, p := range projects {
			if p.Hidden {
				continue
			}
			if strings.EqualFold(p.ID, needle) {
				exactID = append(exactID, p.ID)
			} else if strings.EqualFold(p.Name, needle) {
				exactName = append(exactName, p.ID)
			} else if strings.Contains(strings.ToLower(p.Name), strings.ToLower(needle)) {
				partial = append(partial, p.ID)
			}
		}
		switch {
		case len(exactID) == 1:
			return exactID, "", nil
		case len(exactName) == 1:
			return exactName, "", nil
		case len(exactName) > 1:
			return nil, "", newStructuredError("ambiguousProject", "", "", projectCandidatesForIDs(state, dataDir, exactName))
		case len(partial) == 1:
			return partial, "", nil
		case len(partial) > 1:
			return nil, "", newStructuredError("ambiguousProject", "", "", projectCandidatesForIDs(state, dataDir, partial))
		default:
			var visibleIDs []string
			for _, p := range projects {
				if !p.Hidden {
					visibleIDs = append(visibleIDs, p.ID)
				}
			}
			return nil, "", newStructuredError("projectMissing", "", "", projectCandidatesForIDs(state, dataDir, visibleIDs))
		}
	}

	targetDir := strings.TrimSpace(dirFlag)
	if targetDir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, "", err
		}
		targetDir = cwd
	}
	absDir, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, "", newStructuredError("invalidDirectory", "", "", nil)
	}
	absDir = filepath.Clean(absDir)

	meta, _ := projectmeta.Load(dataDir)
	hiddenProject := map[string]bool{}
	for _, p := range projects {
		if p.Hidden {
			hiddenProject[p.ID] = true
		}
	}

	bestDist := -1
	matched := map[string]bool{}
	recordMatch := func(projectID string, dist int) {
		if projectID == "" || hiddenProject[projectID] {
			return
		}
		if bestDist < 0 || dist < bestDist {
			bestDist = dist
			matched = map[string]bool{projectID: true}
		} else if dist == bestDist {
			matched[projectID] = true
		}
	}

	for projectID, entry := range meta {
		for _, root := range entry.Roots {
			if dist, ok := pathDistance(root, absDir); ok {
				recordMatch(projectID, dist)
			}
		}
		for _, loc := range entry.Locations {
			if dist, ok := pathDistance(loc.Directory, absDir); ok {
				recordMatch(projectID, dist)
			}
		}
	}
	for _, cmd := range state.Definitions.Commands {
		if cmd.Hidden || cmd.Project == nil || strings.TrimSpace(cmd.Directory) == "" {
			continue
		}
		if dist, ok := pathDistance(cmd.Directory, absDir); ok {
			recordMatch(*cmd.Project, dist)
		}
	}

	var ids []string
	for id := range matched {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, absDir, nil
}

func resolveLaunchCommand(mode string, state engine.Snapshot, dataDir, commandFlag, projectFlag, dirFlag string) (engine.Command, error) {
	commands := state.Definitions.Commands
	projectByID := map[string]engine.Project{}
	for _, p := range state.Definitions.Projects {
		projectByID[p.ID] = p
	}

	if strings.TrimSpace(commandFlag) != "" {
		needle := strings.TrimSpace(commandFlag)
		for _, cmd := range commands {
			if cmd.ID != needle {
				continue
			}
			if cmd.Hidden || (cmd.Project != nil && projectByID[*cmd.Project].Hidden) {
				return engine.Command{}, newStructuredError("commandMissing", "", "", nil)
			}
			if !availableOnCurrentPlatform(cmd.Platform) {
				return engine.Command{}, newStructuredError("platformMismatch", "", "", nil)
			}
			if strings.TrimSpace(cmd.Directory) == "" {
				return engine.Command{}, newStructuredError("invalidDirectory", "", "", nil)
			}
			return cmd, nil
		}
		return engine.Command{}, newStructuredError("commandMissing", "", "", nil)
	}

	projectIDs, absDir, err := resolveTargetProjectIDs(state, dataDir, projectFlag, dirFlag)
	if err != nil {
		return engine.Command{}, err
	}
	if len(projectIDs) > 1 {
		return engine.Command{}, newStructuredError("ambiguousProject", "", "", projectCandidatesForIDs(state, dataDir, projectIDs))
	}

	var scope []engine.Command
	var targetProject *engine.Project
	if len(projectIDs) == 1 {
		p, ok := projectByID[projectIDs[0]]
		if ok {
			targetProject = &p
		}
		for _, cmd := range commands {
			if cmd.Project != nil && *cmd.Project == projectIDs[0] && !cmd.Hidden && availableOnCurrentPlatform(cmd.Platform) && len(cmd.Fields) == 0 && strings.TrimSpace(cmd.Directory) != "" {
				scope = append(scope, cmd)
			}
		}
	} else if absDir != "" {
		for _, cmd := range commands {
			if cmd.Hidden || (cmd.Project != nil && projectByID[*cmd.Project].Hidden) || !availableOnCurrentPlatform(cmd.Platform) || len(cmd.Fields) > 0 || strings.TrimSpace(cmd.Directory) == "" {
				continue
			}
			if normalizePath(cmd.Directory) == normalizePath(absDir) {
				scope = append(scope, cmd)
			}
		}
	}
	if len(scope) == 0 {
		return engine.Command{}, newStructuredError("commandMissing", "", "", nil)
	}

	wantKind := "run"
	wantIcon := "play"
	if mode == "dev" {
		wantKind = "debug"
		wantIcon = "bolt"
	}

	cmdByID := map[string]engine.Command{}
	for _, cmd := range scope {
		cmdByID[cmd.ID] = cmd
	}

	if targetProject != nil {
		if mode == "start" {
			var bound []engine.Command
			for _, b := range targetProject.Buttons {
				if !b.Hidden && b.CommandID != "" {
					if c, ok := cmdByID[b.CommandID]; ok {
						bound = append(bound, c)
					}
				}
			}
			if len(bound) == 1 {
				return bound[0], nil
			}
			if len(bound) > 1 {
				return engine.Command{}, newStructuredError("ambiguousCommand", "", "", commandCandidatesFromCommands(bound, projectByID))
			}
		}

		var buttonMatches []engine.Command
		for _, b := range targetProject.Buttons {
			if b.Hidden || b.CommandID == "" {
				continue
			}
			c, ok := cmdByID[b.CommandID]
			if !ok {
				continue
			}
			kind, _, okRank := classifyCommand(c)
			if (okRank && kind == wantKind) || (!okRank && b.Icon == wantIcon) {
				buttonMatches = append(buttonMatches, c)
			}
		}
		if len(buttonMatches) == 1 {
			return buttonMatches[0], nil
		}
		if len(buttonMatches) > 1 {
			return engine.Command{}, newStructuredError("ambiguousCommand", "", "", commandCandidatesFromCommands(buttonMatches, projectByID))
		}
	}

	if mode == "start" {
		if len(scope) == 1 {
			return scope[0], nil
		}
		return engine.Command{}, newStructuredError("ambiguousCommand", "", "", commandCandidatesFromCommands(scope, projectByID))
	}

	var ranked []rankedLaunch
	for _, cmd := range scope {
		kind, score, ok := classifyCommand(cmd)
		if !ok || kind != wantKind {
			continue
		}
		if absDir != "" && normalizePath(cmd.Directory) == normalizePath(absDir) {
			score += 100
		}
		ranked = append(ranked, rankedLaunch{command: cmd, kind: kind, score: score})
	}
	if len(ranked) == 0 {
		return engine.Command{}, newStructuredError(
			"commandMissing",
			"No saved "+mode+" command was found in this scope; inspect available commands in `candidates` and run `yourun start --command <id>`.",
			"Use `yourun start --command <id>` with one of the candidate commands.",
			commandCandidatesFromCommands(scope, projectByID),
		)
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		return ranked[i].score > ranked[j].score
	})
	if len(ranked) > 1 && ranked[0].score == ranked[1].score && ranked[0].command.ID != ranked[1].command.ID {
		var tied []engine.Command
		topScore := ranked[0].score
		for _, r := range ranked {
			if r.score == topScore {
				tied = append(tied, r.command)
			}
		}
		return engine.Command{}, newStructuredError("ambiguousCommand", "", "", commandCandidatesFromCommands(tied, projectByID))
	}
	return ranked[0].command, nil
}

func resolveTargetRunID(mode string, state engine.Snapshot, dataDir, idFlag, commandFlag, projectFlag, dirFlag string) (string, error) {
	if strings.TrimSpace(idFlag) != "" {
		return strings.TrimSpace(idFlag), nil
	}
	onlyActive := mode == "stop"

	matchCommandIDs := map[string]bool{}
	var fallbackDir string
	if strings.TrimSpace(commandFlag) != "" {
		matchCommandIDs[strings.TrimSpace(commandFlag)] = true
	} else {
		projectIDs, absDir, err := resolveTargetProjectIDs(state, dataDir, projectFlag, dirFlag)
		if err != nil {
			return "", err
		}
		if len(projectIDs) > 1 {
			return "", newStructuredError("ambiguousProject", "", "", projectCandidatesForIDs(state, dataDir, projectIDs))
		}
		if len(projectIDs) == 1 {
			for _, cmd := range state.Definitions.Commands {
				if cmd.Project != nil && *cmd.Project == projectIDs[0] {
					matchCommandIDs[cmd.ID] = true
				}
			}
		} else if absDir != "" {
			fallbackDir = absDir
			for _, run := range state.Runs {
				if normalizePath(run.Directory) == normalizePath(absDir) {
					matchCommandIDs[run.CommandID] = true
				}
			}
		}
	}

	var activeMatches []engine.Run
	var replayableMatches []engine.Run
	for _, run := range state.Runs {
		matched := matchCommandIDs[run.CommandID]
		if !matched && fallbackDir != "" && normalizePath(run.Directory) == normalizePath(fallbackDir) {
			matched = true
		}
		if !matched {
			continue
		}
		if run.Status == "running" || run.Status == "stopping" {
			activeMatches = append(activeMatches, run)
		} else if !onlyActive && (run.Status != "unknown" || mode == "logs") {
			replayableMatches = append(replayableMatches, run)
		}
	}

	if len(activeMatches) == 1 {
		return activeMatches[0].ID, nil
	}
	if len(activeMatches) > 1 {
		return "", newStructuredError("ambiguousRun", "", "", runCandidatesFromRuns(activeMatches))
	}
	if onlyActive {
		return "", newStructuredError("noActiveRun", "", "", nil)
	}
	if mode == "logs" && len(replayableMatches) >= 1 {
		// state.Runs is ordered newest-first; for logs without an active run,
		// return the most recent run in scope so `yourun logs` works effortlessly.
		return replayableMatches[0].ID, nil
	}
	if len(replayableMatches) == 1 {
		return replayableMatches[0].ID, nil
	}
	if len(replayableMatches) > 1 {
		return "", newStructuredError("ambiguousRun", "", "", runCandidatesFromRuns(replayableMatches[:min(len(replayableMatches), 8)]))
	}
	return "", newStructuredError("runMissing", "", "", nil)
}
