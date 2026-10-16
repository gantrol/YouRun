package discovery

import (
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Plugin-provided tasks and module paths are resolved without running Gradle.
// https://docs.gradle.org/current/userguide/multi_project_builds.html
// https://docs.gradle.org/current/userguide/version_catalogs.html
// https://docs.spring.io/spring-boot/gradle-plugin/reacting.html
type gradleProject struct {
	root, module, tool, platform string
	aliases                      map[string]string
}

var gradleName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]*$`)
var gradleClass = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)+$`)
var gradlePluginID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)

func (s *scan) gradleCommands(path string, data []byte, rule Rule) error {
	if s.ctx.Err() != nil {
		return s.ctx.Err()
	}
	if s.entries[filepath.Join(filepath.Dir(path), "build.gradle")] && s.entries[filepath.Join(filepath.Dir(path), "build.gradle.kts")] {
		s.issue(path, "scanPartial")
		return nil
	}
	nodes, err := gradleParse(data)
	if err != nil {
		s.issue(path, "scanPartial")
		return nil
	}
	project := s.gradleProject(path)
	if project == nil {
		return nil
	}
	plugins, partial := gradlePlugins(nodes, project.aliases)
	if partial {
		s.issue(path, "scanPartial")
	}
	matched := len(rule.Match.PluginsAny) == 0
	for _, plugin := range rule.Match.PluginsAny {
		matched = matched || plugins[plugin]
	}
	if !matched {
		return nil
	}
	disabled, unsafe := gradleTaskState(nodes)
	if unsafe {
		s.issue(path, "scanPartial")
		return nil
	}
	add := func(key, task string) {
		if disabled[task] {
			return
		}
		s.addRuleCommand(rule, key, path, project.root, map[string]string{"gradleTool": project.tool, "modulePath": project.module}, project.platform, true)
	}
	java := plugins["java"] || plugins["java-library"] || plugins["application"] || plugins["war"] || plugins["org.jetbrains.kotlin.jvm"]
	switch rule.Resolver {
	case "gradle-java-v1":
		if java {
			add("build", "build")
			add("test", "test")
		}
		if plugins["application"] {
			main, valid := gradleMainClass(nodes)
			if valid && gradleClass.MatchString(main) {
				add("run", "run")
			} else {
				s.issue(path, "scanPartial")
			}
		}
	case "spring-boot-gradle-v1":
		if plugins["org.springframework.boot"] && java {
			add("boot-run", "bootRun")
			add("boot-jar", "bootJar")
			if plugins["war"] {
				add("boot-war", "bootWar")
			}
		}
	case "android-gradle-v1":
		if plugins["com.android.application"] || plugins["com.android.library"] {
			s.gradleAndroid(path, nodes, project, plugins["com.android.application"], disabled, rule)
		}
	}
	return nil
}

func (s *scan) gradleProject(path string) *gradleProject {
	dir := filepath.Dir(path)
	project := &gradleProject{root: dir, tool: "gradle", aliases: map[string]string{}}
	settings := ""
	for current := dir; ; current = filepath.Dir(current) {
		groovy, kotlin := filepath.Join(current, "settings.gradle"), filepath.Join(current, "settings.gradle.kts")
		if s.entries[groovy] && s.entries[kotlin] {
			s.issue(path, "scanPartial")
			return nil
		}
		if s.entries[groovy] || s.entries[kotlin] {
			settings = groovy
			if s.entries[kotlin] {
				settings = kotlin
			}
			project.root = current
			break
		}
		if current == "." {
			break
		}
	}
	var settingsNodes []gradleNode
	if settings != "" {
		raw := s.read(settings)
		if raw == nil {
			s.issue(settings, "scanPartial")
			return nil
		}
		var err error
		settingsNodes, err = gradleParse(raw)
		if err != nil {
			s.issue(settings, "scanPartial")
			return nil
		}
		modules, valid := gradleModules(project.root, settingsNodes)
		if !valid {
			s.issue(settings, "scanPartial")
			return nil
		}
		if dir != project.root {
			found := false
			for name, moduleDir := range modules {
				if moduleDir == dir {
					if found {
						s.issue(settings, "scanPartial")
						return nil
					}
					project.module, found = name, true
				}
			}
			if !found {
				s.issue(path, "scanPartial")
				return nil
			}
		}
	}
	// Parent build logic can mutate child task/plugin configurations. Direct
	// repositories/dependencies blocks are harmless; executable configuration is
	// not evaluated to guess what it might apply to this module.
	if project.root != dir {
		for _, filename := range []string{"build.gradle", "build.gradle.kts"} {
			rootBuild := filepath.Join(project.root, filename)
			if !s.entries[rootBuild] {
				continue
			}
			raw := s.read(rootBuild)
			if raw == nil {
				s.issue(rootBuild, "scanPartial")
				return nil
			}
			parent, err := gradleParse(raw)
			if err != nil || gradleParentMutates(parent) {
				s.issue(rootBuild, "scanPartial")
				return nil
			}
		}
	}
	project.aliases = s.gradleAliases(project.root, settingsNodes)
	wrapper := "gradlew"
	project.platform = "macOS"
	if runtime.GOOS == "windows" {
		wrapper, project.platform = "gradlew.bat", "Windows"
	}
	if s.entries[filepath.Join(project.root, wrapper)] {
		project.tool = "./" + wrapper
	} else {
		project.platform = ""
	}
	return project
}

func gradleModules(root string, nodes []gradleNode) (map[string]string, bool) {
	modules := map[string]string{}
	overrides := map[string]string{}
	for _, node := range nodes {
		head := gradleHead(node)
		if containsString([]string{"rootProject.projectDir", "rootProject.buildFileName", "rootProject.setProjectDir", "rootProject.setBuildFileName", "rootProject"}, head) {
			return nil, false
		}
		if head == "include" && !node.block {
			names, valid := gradleStringList(node.head[1:])
			if !valid {
				return nil, false
			}
			for _, name := range names {
				parts := strings.Split(strings.TrimPrefix(name, ":"), ":")
				prefix := ""
				for _, part := range parts {
					if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "/\\$\x00\r\n") {
						return nil, false
					}
					prefix += ":" + part
					modules[prefix] = filepath.Join(root, filepath.FromSlash(strings.ReplaceAll(strings.TrimPrefix(prefix, ":"), ":", "/")))
					if len(modules) > 128 {
						return nil, false
					}
				}
			}
			continue
		}
		if head == "project" {
			h := node.head
			if node.block || len(h) < 9 || h[1].text != "(" || h[2].kind != 's' || h[3].text != ")" || h[4].text != ".projectDir" || h[5].text != "=" {
				return nil, false
			}
			name := ":" + strings.TrimPrefix(h[2].text, ":")
			value, valid := gradleFileLiteral(h[6:])
			if !valid {
				return nil, false
			}
			target := localJoin(root, value)
			if target == "" {
				return nil, false
			}
			overrides[name] = target
			continue
		}
		if node.block && gradleContains(node.body, []string{"include", "project", "projectDir", ".projectDir", "buildFileName"}) {
			return nil, false
		}
	}
	for name, target := range overrides {
		if _, included := modules[name]; !included {
			return nil, false
		}
		modules[name] = target
		// Parent-directory replacement can change descriptors created before or
		// after it. Require explicit child mappings instead of assuming ordering.
		for child := range modules {
			if strings.HasPrefix(child, name+":") && overrides[child] == "" {
				return nil, false
			}
		}
	}
	return modules, true
}

func gradleFileLiteral(tokens []gradleToken) (string, bool) {
	if len(tokens) > 0 && tokens[0].text == "new" {
		tokens = tokens[1:]
	}
	if len(tokens) < 2 {
		return "", false
	}
	function := tokens[0].text
	args, parens := gradleArguments(tokens[1:])
	if !parens {
		return "", false
	}
	if function == "file" || function == "files" {
		return gradleLiteral(args)
	}
	if function == "File" && len(args) == 3 && (args[0].text == "rootDir" || args[0].text == "settingsDir") && args[1].text == "," {
		return gradleLiteral(args[2:])
	}
	return "", false
}

func (s *scan) gradleAliases(root string, settings []gradleNode) map[string]string {
	catalogs := map[string]string{"libs": filepath.Join(root, "gradle", "libs.versions.toml")}
	for _, resolution := range settings {
		if gradleHead(resolution) != "dependencyResolutionManagement" {
			continue
		}
		for _, block := range resolution.body {
			if gradleHead(block) == "defaultLibrariesExtensionName" {
				delete(catalogs, "libs")
				s.issue(root, "scanPartial")
			}
			if gradleHead(block) != "versionCatalogs" {
				continue
			}
			for _, catalog := range block.body {
				if len(catalogs) >= 32 {
					s.issue(root, "scanLimit")
					break
				}
				name, valid := gradleBlockName(catalog)
				if !valid || !gradleName.MatchString(name) {
					s.issue(root, "scanPartial")
					continue
				}
				for _, from := range catalog.body {
					if gradleHead(from) != "from" {
						continue
					}
					args, _ := gradleArguments(from.head[1:])
					value, valid := gradleFileLiteral(args)
					if !valid || localJoin(root, value) == "" {
						s.issue(root, "scanPartial")
						delete(catalogs, name)
						continue
					}
					catalogs[name] = localJoin(root, value)
				}
			}
		}
	}
	aliases := map[string]string{}
	names := make([]string, 0, len(catalogs))
	for name := range catalogs {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if s.ctx.Err() != nil || len(aliases) > 128 {
			break
		}
		path := catalogs[name]
		if !s.entries[path] {
			continue
		}
		var values map[string]any
		raw := s.read(path)
		if raw == nil || toml.Unmarshal(raw, &values) != nil {
			s.issue(path, "scanPartial")
			continue
		}
		plugins, _ := values["plugins"].(map[string]any)
		for _, alias := range frameworkKeys(plugins) {
			plugin := ""
			switch value := plugins[alias].(type) {
			case string:
				plugin, _, _ = strings.Cut(value, ":")
			case map[string]any:
				plugin, _ = value["id"].(string)
			}
			if gradlePluginID.MatchString(plugin) {
				accessor := strings.NewReplacer("-", ".", "_", ".").Replace(alias)
				key := name + ".plugins." + accessor
				if old, exists := aliases[key]; exists && old != plugin {
					aliases[key] = ""
					s.issue(path, "scanPartial")
				} else {
					aliases[key] = plugin
				}
			}
		}
	}
	return aliases
}

func gradlePlugins(nodes []gradleNode, aliases map[string]string) (map[string]bool, bool) {
	plugins, partial := map[string]bool{}, false
	for _, node := range nodes {
		if gradleHead(node) == "plugins" && node.block {
			for _, declaration := range node.body {
				h := declaration.head
				if len(h) == 0 || declaration.block {
					partial = true
					continue
				}
				plugin, end := "", 1
				switch gradleHead(declaration) {
				case "id", "kotlin":
					if len(h) >= 4 && h[1].text == "(" && h[2].kind == 's' && h[3].text == ")" {
						plugin, end = h[2].text, 4
					} else if len(h) >= 2 && h[1].kind == 's' {
						plugin, end = h[1].text, 2
					}
					if h[0].text == "kotlin" && plugin != "" {
						plugin = "org.jetbrains.kotlin." + plugin
					}
				case "alias":
					if len(h) >= 4 && h[1].text == "(" && h[2].kind == 'i' && h[3].text == ")" {
						plugin, end = aliases[h[2].text], 4
					}
				default:
					if len(h) == 1 {
						plugin = h[0].text
					}
				}
				applied := true
				for i := end; i < len(h); i++ {
					if h[i].kind == 'i' && (h[i].text == "apply" || h[i].text == ".apply") {
						args, _ := gradleArguments(h[i+1:])
						if len(args) != 1 || args[0].kind != 'i' || (args[0].text != "true" && args[0].text != "false") {
							partial, applied = true, false
						} else {
							applied = args[0].text == "true"
						}
						break
					}
				}
				if plugin == "" {
					partial = true
				} else if applied && gradlePluginID.MatchString(plugin) {
					plugins[plugin] = true
				}
			}
		} else if gradleHead(node) == "apply" && !node.block {
			args, _ := gradleArguments(node.head[1:])
			if len(args) == 3 && args[0].text == "plugin" && (args[1].text == ":" || args[1].text == "=") && args[2].kind == 's' {
				plugins[args[2].text] = true
			} else {
				partial = true
			}
		}
	}
	return plugins, partial
}

func gradleContains(nodes []gradleNode, names []string) bool {
	for _, node := range nodes {
		for _, token := range node.head {
			member := token.text[strings.LastIndex(token.text, ".")+1:]
			if token.kind == 'i' && (containsString(names, token.text) || containsString(names, member)) {
				return true
			}
		}
		if gradleContains(node.body, names) {
			return true
		}
	}
	return false
}

func gradleParentMutates(nodes []gradleNode) bool {
	for _, node := range nodes {
		head := gradleHead(node)
		if head == "project" && !node.block {
			return true
		}
		if head == "apply" && gradleContains([]gradleNode{node}, []string{"from"}) {
			return true
		}
		if strings.HasPrefix(head, "gradle.") && gradleContains(node.body, []string{"apply", "plugins", "tasks", "enabled", "setEnabled"}) {
			return true
		}
		if containsString([]string{"subprojects", "allprojects", "project", "configure", "afterEvaluate"}, head) && gradleContains(node.body, []string{"apply", "plugins", "tasks", "tasks.named", "tasks.configureEach", "android", "enabled", "setEnabled"}) {
			return true
		}
	}
	return false
}

func gradleTaskState(nodes []gradleNode) (map[string]bool, bool) {
	disabled := map[string]bool{}
	for _, node := range nodes {
		head := gradleHead(node)
		if head == "apply" && gradleContains([]gradleNode{node}, []string{"from"}) {
			return disabled, true
		}
		if containsString([]string{"if", "for", "afterEvaluate", "allprojects", "subprojects"}, head) && gradleContains(node.body, []string{"plugins", "apply", "tasks", "enabled", "setEnabled"}) {
			return disabled, true
		}
		if head == "tasks" && node.block {
			children, unsafe := gradleTaskState(node.body)
			if unsafe {
				return disabled, true
			}
			for task := range children {
				disabled[task] = true
			}
			continue
		}
		hasEnabled := gradleContains(node.body, []string{"enabled", "isEnabled", "setEnabled", "onlyIf"})
		if !hasEnabled {
			// Assignments outside a task block may address a provider/getter.
			for _, token := range node.head {
				if token.kind == 'i' && (strings.HasSuffix(token.text, ".enabled") || strings.HasSuffix(token.text, ".isEnabled")) {
					return disabled, true
				}
			}
			continue
		}
		task := ""
		if strings.HasPrefix(head, "tasks.") && !strings.Contains(strings.TrimPrefix(head, "tasks."), ".") && len(node.head) == 1 {
			task = strings.TrimPrefix(head, "tasks.")
		} else if containsString([]string{"tasks.named", "named", "tasks.getByName", "getByName", "tasks.register", "register"}, head) {
			args, _ := gradleArguments(node.head[1:])
			task, _ = gradleLiteral(args)
		} else if len(node.head) == 1 && (containsString([]string{"build", "test", "run", "bootJar", "bootWar", "bootRun"}, head) || strings.HasPrefix(head, "assemble") || strings.HasPrefix(head, "bundle") || strings.HasPrefix(head, "install")) {
			task = head
		}
		if task == "" {
			return disabled, true
		}
		for _, setting := range node.body {
			if args, exists := gradleProperty(setting, "enabled"); exists {
				if len(args) != 1 || args[0].text != "true" {
					disabled[task] = true
				}
			} else if gradleContains([]gradleNode{setting}, []string{"enabled", "isEnabled", "setEnabled", "onlyIf"}) {
				disabled[task] = true
			}
		}
	}
	return disabled, false
}

func gradleMainClass(nodes []gradleNode) (string, bool) {
	main, known := "", false
	for _, node := range nodes {
		for _, name := range []string{"mainClassName", "application.mainClass", "application.mainClassName"} {
			if args, exists := gradleProperty(node, name); exists {
				main, known = gradleLiteral(args)
			}
		}
		if gradleHead(node) == "application" && node.block {
			for _, property := range node.body {
				for _, name := range []string{"mainClass", "mainClassName"} {
					if args, exists := gradleProperty(property, name); exists {
						main, known = gradleLiteral(args)
					}
				}
			}
		}
	}
	return main, known
}
