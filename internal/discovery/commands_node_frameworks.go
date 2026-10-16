package discovery

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// Supplemental commands never replace package scripts. CLI arguments live in
// rule packs; these processors only establish entrypoints, projects and profiles.
// CLI references: https://docs.nestjs.com/cli/workspaces
// https://docs.expo.dev/more/expo-cli/ https://docs.expo.dev/build/eas-json/
// https://github.com/react-native-community/cli/blob/main/docs/commands.md
type frameworkPackage struct {
	path    string
	values  map[string]any
	pkg     nodePackage
	manager string
	review  bool
}

func (s *scan) frameworkPackage(path string, rule Rule) *frameworkPackage {
	pkgPath := filepath.Join(filepath.Dir(path), "package.json")
	data := s.read(pkgPath)
	if data == nil {
		return nil
	}
	p := &frameworkPackage{path: pkgPath}
	if json.Unmarshal(data, &p.values) != nil || p.values == nil || json.Unmarshal(data, &p.pkg) != nil {
		s.issue(pkgPath, "scanInvalidManifest")
		return nil
	}
	if !rule.MatchesDependencies(p.values) {
		return nil
	}
	for _, nodeRule := range s.registry.Match(pkgPath) {
		if nodeRule.Resolver == "node-v1" {
			p.manager, p.review = s.nodePackageManager(pkgPath, p.pkg, nodeRule)
			break
		}
	}
	if p.manager == "" {
		s.issue(path, "scanManagerUnknown")
		return nil
	}
	return p
}

func frameworkDependency(values map[string]any, name string) bool {
	return (Rule{Match: RuleMatch{DependenciesAny: []string{name}}}).MatchesDependencies(values)
}

func (s *scan) nestCommands(path string, data []byte, rule Rule) error {
	var config map[string]any
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	p := s.frameworkPackage(path, rule)
	if p == nil || config == nil {
		return nil
	}
	dir := filepath.Dir(path)
	projects, _ := config["projects"].(map[string]any)
	if len(projects) == 0 {
		s.nestProject(path, dir, "", "", config, p, rule)
		return nil
	}
	defaultProject := ""
	if len(projects) > 128 {
		s.issue(path, "scanLimit")
	}
	root := fieldString(config, []string{"root"})
	for _, name := range frameworkKeys(projects) {
		if s.ctx.Err() != nil {
			return s.ctx.Err()
		}
		project, ok := projects[name].(map[string]any)
		if ok && root != "" && fieldString(project, []string{"root"}) == root {
			defaultProject = name
			break
		}
	}
	for _, name := range frameworkKeys(projects) {
		if s.ctx.Err() != nil {
			return s.ctx.Err()
		}
		if len(s.result.Candidates) >= maxCommands {
			s.issue(path, "scanLimit")
			break
		}
		project, ok := projects[name].(map[string]any)
		if !ok || !frameworkName(name) {
			s.issue(path, "scanInvalidManifest")
			continue
		}
		s.nestProject(path, dir, name, defaultProject, project, p, rule)
	}
	return nil
}

func (s *scan) nestProject(path, dir, name, defaultProject string, project map[string]any, p *frameworkPackage, rule Rule) {
	kind := fieldString(project, []string{"type"})
	if name != "" && kind != "application" && kind != "library" {
		s.issue(path, "scanPartial")
		return
	}
	sourceRoot := fieldString(project, []string{"sourceRoot"})
	if sourceRoot == "" || localJoin(dir, sourceRoot) == "" {
		s.issue(path, "scanPartial")
		return
	}
	// The CLI reads its own builder and tsconfig settings. Verify declared files
	// without flattening them or substituting a guessed tsc/webpack invocation.
	for _, fields := range [][]string{{"compilerOptions", "tsConfigPath"}, {"compilerOptions", "webpackConfigPath"}, {"compilerOptions", "builder", "options", "configPath"}} {
		if value := fieldString(project, fields); value != "" {
			if target := localJoin(dir, value); target == "" || !s.entries[target] {
				s.issue(path, "scanPartial")
				return
			}
		}
	}
	entry := fieldString(project, []string{"entryFile"})
	if entry == "" {
		entry = "main"
	}
	entryPath := localJoin(localJoin(dir, sourceRoot), entry)
	entryExists := entryPath != "" && (s.entries[entryPath] || s.entries[entryPath+".ts"] || s.entries[entryPath+".js"])
	if !entryExists && kind != "library" {
		s.issue(path, "scanPartial")
		return
	}
	if kind == "library" {
		configPath := fieldString(project, []string{"compilerOptions", "tsConfigPath"})
		if configPath == "" {
			s.issue(path, "scanPartial")
			return
		}
	}
	for _, operation := range []string{"build", "start", "watch"} {
		if kind == "library" && operation != "build" {
			continue
		}
		verb := operation
		if operation == "watch" {
			verb = "start"
		}
		if frameworkScript(p.pkg.Scripts, "nest", verb, func(args []string) bool {
			target := ""
			if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
				target = args[0]
			}
			if target == "" {
				target = defaultProject
			}
			watch := frameworkFlag(args, "--watch", "-w") != ""
			return target == name && watch == (operation == "watch")
		}) {
			continue
		}
		key := operation
		if name != "" {
			key += "-project"
		}
		s.addRuleCommand(rule, key+"-"+p.manager, path, dir, map[string]string{"projectName": name}, "", p.review)
	}
}

func (s *scan) expoCommands(path string, data []byte, rule Rule) error {
	p := s.frameworkPackage(path, rule)
	if p == nil {
		return nil
	}
	dir := filepath.Dir(path)
	config, dynamic := s.mobileConfig(dir, true)
	if dynamic {
		s.issue(path, "scanPartial")
	}
	platforms := s.nativeMobilePlatforms(dir)
	// A dependency alone also appears in reusable modules. Require an app config
	// or an explicitly declared application entry, never execute app.config.*.
	if config == nil && !dynamic && !s.mobileEntry(dir, p.values) && len(platforms) == 0 {
		return nil
	}
	s.frameworkCommand(rule, "start", "expo", "start", path, p, "", dynamic)
	for _, platform := range []string{"android", "ios"} {
		if platforms[platform] {
			s.frameworkCommand(rule, platform, "expo", "run:"+platform, path, p, frameworkHostPlatform(platform), true)
		}
	}
	return nil
}

func (s *scan) reactNativeCommands(path string, data []byte, rule Rule) error {
	p := s.frameworkPackage(path, rule)
	if p == nil || frameworkDependency(p.values, "expo") {
		return nil
	}
	dir := filepath.Dir(path)
	platforms := s.nativeMobilePlatforms(dir)
	// The CLI may move platform roots in executable JavaScript. Keep existing
	// scripts, but do not claim conventional roots describe that configuration.
	if s.entries[filepath.Join(dir, "react-native.config.js")] || s.entries[filepath.Join(dir, "react-native.config.ts")] {
		s.issue(path, "scanPartial")
		return nil
	}
	config, _ := s.mobileConfig(dir, false)
	if len(platforms) == 0 && !(config != nil && s.mobileEntry(dir, p.values)) {
		return nil
	}
	s.frameworkCommand(rule, "start", "react-native", "start", path, p, "", false)
	for _, platform := range []string{"android", "ios"} {
		if platforms[platform] {
			s.frameworkCommand(rule, platform, "react-native", "run-"+platform, path, p, frameworkHostPlatform(platform), true)
			// Both build commands exist in CLI 12. Unknown tags, catalog
			// versions and older declarations remain script-only.
			if frameworkCLIMajor(p.values, "@react-native-community/cli-platform-"+platform) >= 12 {
				s.frameworkCommand(rule, "build-"+platform, "react-native", "build-"+platform, path, p, frameworkHostPlatform(platform), true)
			}
		}
	}
	return nil
}

func (s *scan) frameworkCommand(rule Rule, key, binary, operation, path string, p *frameworkPackage, platform string, review bool) {
	if frameworkScript(p.pkg.Scripts, binary, operation, nil) {
		return
	}
	s.addRuleCommand(rule, key+"-"+p.manager, path, filepath.Dir(path), nil, platform, review || p.review)
}

func frameworkHostPlatform(platform string) string {
	if platform == "ios" {
		return "macOS"
	}
	return ""
}

func (s *scan) mobileEntry(dir string, values map[string]any) bool {
	main := fieldString(values, []string{"main"})
	if main == "expo-router/entry" && frameworkDependency(values, "expo-router") {
		return true
	}
	if main == "node_modules/expo/AppEntry.js" && frameworkDependency(values, "expo") {
		return true
	}
	if main == "" {
		return false
	}
	entry := localJoin(dir, main)
	return entry != "" && (s.entries[entry] || s.entries[entry+".js"] || s.entries[entry+".ts"] || s.entries[entry+".tsx"])
}

func (s *scan) mobileConfig(dir string, expo bool) (map[string]any, bool) {
	dynamic := expo && (s.entries[filepath.Join(dir, "app.config.ts")] || s.entries[filepath.Join(dir, "app.config.js")])
	for _, filename := range []string{"app.config.json", "app.json"} {
		path := filepath.Join(dir, filename)
		if !s.entries[path] {
			continue
		}
		var values map[string]any
		if raw := s.read(path); raw != nil && json.Unmarshal(raw, &values) == nil && values != nil {
			if nested, ok := values["expo"].(map[string]any); ok && expo {
				values = nested
			}
			if fieldString(values, []string{"name"}) != "" {
				return values, dynamic
			}
		} else {
			s.issue(path, "scanInvalidManifest")
		}
	}
	return nil, dynamic
}

func (s *scan) nativeMobilePlatforms(dir string) map[string]bool {
	platforms := map[string]bool{}
	android := filepath.Join(dir, "android")
	if (s.entries[filepath.Join(android, "settings.gradle")] || s.entries[filepath.Join(android, "settings.gradle.kts")]) &&
		(s.entries[filepath.Join(android, "app", "build.gradle")] || s.entries[filepath.Join(android, "app", "build.gradle.kts")]) {
		platforms["android"] = true
	}
	ios := filepath.Join(dir, "ios")
	for path := range s.entries {
		if filepath.Dir(filepath.Dir(path)) == ios && filepath.Base(path) == "project.pbxproj" && strings.HasSuffix(filepath.Dir(path), ".xcodeproj") {
			platforms["ios"] = true
			break
		}
	}
	return platforms
}

func (s *scan) easCommands(path string, data []byte, rule Rule) error {
	var values map[string]any
	if err := json.Unmarshal(data, &values); err != nil {
		return err
	}
	p := s.frameworkPackage(path, rule)
	if p == nil {
		return nil
	}
	profiles, _ := values["build"].(map[string]any)
	if len(profiles) > 128 {
		s.issue(path, "scanLimit")
	}
	dir := filepath.Dir(path)
	config, dynamic := s.mobileConfig(dir, true)
	if dynamic {
		s.issue(path, "scanPartial")
		config = nil
	}
	platforms := s.nativeMobilePlatforms(dir)
	if declared, ok := config["platforms"].([]any); ok {
		for _, value := range declared {
			if platform, ok := value.(string); ok && (platform == "android" || platform == "ios") {
				platforms[platform] = true
			}
		}
	}
	for _, name := range frameworkKeys(profiles) {
		if s.ctx.Err() != nil {
			return s.ctx.Err()
		}
		if len(s.result.Candidates) >= maxCommands {
			s.issue(path, "scanLimit")
			break
		}
		if !frameworkName(name) {
			s.issue(path, "scanInvalidManifest")
			continue
		}
		profile, valid := easProfile(profiles, name, map[string]bool{})
		if !valid {
			s.issue(path, "scanPartial")
			continue
		}
		targets := []string{}
		for _, platform := range []string{"android", "ios"} {
			_, configured := profile[platform].(map[string]any)
			if configured || platforms[platform] {
				targets = append(targets, platform)
			}
		}
		if len(targets) == 0 {
			targets = append(targets, "")
		}
		for _, target := range targets {
			if frameworkScript(p.pkg.Scripts, "eas", "build", func(args []string) bool {
				profile := frameworkFlag(args, "--profile", "-e")
				if profile == "" {
					profile = "production"
				}
				platform := frameworkFlag(args, "--platform", "-p")
				return profile == name && (platform == "" || platform == "all" || platform == target)
			}) {
				continue
			}
			key := "build"
			if target == "" {
				key = "build-profile"
			}
			s.addRuleCommand(rule, key+"-"+p.manager, path, dir, map[string]string{"buildProfile": name, "targetPlatform": target}, "", true)
		}
	}
	return nil
}

func easProfile(profiles map[string]any, name string, seen map[string]bool) (map[string]any, bool) {
	if seen[name] || len(seen) >= 5 {
		return nil, false
	}
	seen[name] = true
	profile, ok := profiles[name].(map[string]any)
	if !ok {
		return nil, false
	}
	result := map[string]any{}
	if parent := fieldString(profile, []string{"extends"}); parent != "" {
		inherited, valid := easProfile(profiles, parent, seen)
		if !valid {
			return nil, false
		}
		for key, value := range inherited {
			result[key] = value
		}
	}
	for key, value := range profile {
		result[key] = value
	}
	return result, true
}

func frameworkKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	if len(keys) > 128 {
		keys = keys[:128]
	}
	return keys
}

func frameworkName(name string) bool {
	return name != "" && len(name) <= 200 && !strings.HasPrefix(name, "-") && !strings.ContainsAny(name, "\x00\r\n")
}

func frameworkCLIMajor(values map[string]any, name string) int {
	for _, field := range []string{"dependencies", "devDependencies", "optionalDependencies"} {
		version := fieldString(values, []string{field, name})
		version = strings.TrimPrefix(strings.TrimPrefix(strings.TrimPrefix(version, "^"), "~"), "v")
		if strings.ContainsAny(version, " <>=|*") {
			continue
		}
		major, rest, ok := strings.Cut(version, ".")
		if !ok || rest == "" || strings.ContainsAny(rest, ":/") {
			continue
		}
		if number, err := strconv.Atoi(major); err == nil {
			return number
		}
	}
	return 0
}

func frameworkFlag(args []string, names ...string) string {
	for i, arg := range args {
		key, value, equal := strings.Cut(arg, "=")
		if !containsString(names, key) {
			continue
		}
		if equal {
			return value
		}
		if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			return args[i+1]
		}
		return "true"
	}
	return ""
}

// Literal token inspection only: wrappers (cross-env, npm exec, yarn, etc.) are
// harmless here because scripts remain authoritative. No script is evaluated.
func frameworkScript(scripts map[string]string, binary, operation string, matches func([]string) bool) bool {
	names := make([]string, 0, len(scripts))
	for name := range scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) > 128 {
		names = names[:128]
	}
	for _, name := range names {
		text := scripts[name]
		if len(text) > 8000 || strings.ContainsAny(text, "$%<>\\\n") {
			continue
		}
		tokens := referenceTokens(text)
		for start := 0; start < len(tokens); {
			end := start
			for end < len(tokens) && tokens[end] != "&" && tokens[end] != ";" && tokens[end] != "|" {
				end++
			}
			segment := tokens[start:end]
			start = end + 1
			if len(segment) == 0 {
				continue
			}
			// A following command may run in another directory. Do not treat
			// that script as covering this package's local CLI command.
			if segment[0] == "cd" || segment[0] == "pushd" {
				break
			}
			args, invoked := frameworkInvocation(segment, binary)
			if !invoked {
				continue
			}
			// `expo` alone is the documented alias of `expo start`.
			if binary == "expo" && operation == "start" && (len(args) == 0 || strings.HasPrefix(args[0], "-")) {
				if matches == nil || matches(args) {
					return true
				}
			}
			if len(args) > 0 && args[0] == operation && (matches == nil || matches(args[1:])) {
				return true
			}
		}
	}
	return false
}

func frameworkInvocation(tokens []string, binary string) ([]string, bool) {
	i := 0
	if tokens[i] == "env" || tokens[i] == "cross-env" {
		i++
	}
	for i < len(tokens) && strings.Contains(tokens[i], "=") && !strings.HasPrefix(tokens[i], "-") {
		i++
	}
	if i >= len(tokens) {
		return nil, false
	}
	packageAlias := ""
	switch tokens[i] {
	case "npx":
		i++
		packageAlias = map[string]string{"nest": "@nestjs/cli", "expo": "expo", "react-native": "react-native", "eas": "eas-cli"}[binary]
	case "npm":
		i++
		if i >= len(tokens) || (tokens[i] != "exec" && tokens[i] != "x") {
			return nil, false
		}
		i++
	case "pnpm", "yarn":
		i++
		if i < len(tokens) && tokens[i] == "exec" {
			i++
		}
	case "bun":
		i++
		if i >= len(tokens) || (tokens[i] != "x" && tokens[i] != "run") {
			return nil, false
		}
		i++
	}
	for i < len(tokens) && strings.HasPrefix(tokens[i], "-") {
		flag := tokens[i]
		if flag == "--package" || flag == "-p" {
			i += 2
			continue
		}
		if flag != "--" && flag != "--no-install" && flag != "--no" && flag != "--offline" && flag != "--yes" && flag != "-y" && !strings.HasPrefix(flag, "--package=") {
			return nil, false
		}
		i++
	}
	if i >= len(tokens) {
		return nil, false
	}
	command := filepath.Base(tokens[i])
	if command != binary && (packageAlias == "" || tokens[i] != packageAlias) {
		return nil, false
	}
	return tokens[i+1:], true
}
