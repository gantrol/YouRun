package discovery

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// Maven discovery reads POMs, not the effective model. Repository parents,
// profiles, extensions, settings and executable build logic are never loaded.
func (s *scan) mavenCommands(path string, data []byte, rule Rule) error {
	models := map[string]*mavenModel{}
	model, err := s.mavenModel(path, data, models, map[string]bool{}, 0)
	if err != nil || model == nil {
		return err
	}
	tool, dir, platform := s.mavenLauncher(path)
	pomFile, err := filepath.Rel(dir, path)
	if err != nil {
		return err
	}
	if strings.HasPrefix(pomFile, "-") {
		pomFile = "./" + filepath.ToSlash(pomFile)
	}
	variables := map[string]string{"mavenTool": tool, "pomFile": filepath.ToSlash(pomFile)}
	s.addRuleCommand(rule, "package", path, dir, variables, platform, true)
	// Aggregator/plugin projects are buildable, but are not application targets.
	if model.packaging != "jar" && model.packaging != "war" {
		return nil
	}
	for _, key := range []string{"org.springframework.boot:spring-boot-maven-plugin", "org.codehaus.mojo:exec-maven-plugin"} {
		plugin, exists := model.plugins[key]
		if !exists {
			continue
		}
		if defaults, exists := model.managed[key]; exists {
			plugin = mergeMavenPlugin(defaults, plugin)
		}
		if plugin.unknown {
			s.issue(path, "scanPartial")
			continue
		}
		config := map[string]string{}
		for name, value := range plugin.config {
			config[name] = value
		}
		for name, value := range plugin.cli {
			config[name] = value
		}
		parameterProperties := map[string]string{"mainClass": "exec.mainClass", "skip": "exec.skip"}
		if key == "org.springframework.boot:spring-boot-maven-plugin" {
			parameterProperties = map[string]string{"mainClass": "spring-boot.run.main-class", "skip": "spring-boot.run.skip"}
		}
		for parameter, property := range parameterProperties {
			if _, configured := config[parameter]; !configured {
				if value, declared := model.properties[property]; declared {
					config[parameter] = value
				}
			}
		}
		if skipped, declared := config["skip"]; declared {
			value, known := resolveMavenValue(skipped, model.properties)
			if !known || value != "false" && value != "true" {
				s.issue(path, "scanPartial")
				continue
			}
			if value == "true" {
				continue
			}
		}
		mainClass := ""
		if declared, exists := config["mainClass"]; exists {
			value, known := resolveMavenValue(declared, model.properties)
			if !known || !mavenMainClass.MatchString(value) {
				s.issue(path, "scanPartial")
				continue
			}
			mainClass = value
		}
		if key == "org.springframework.boot:spring-boot-maven-plugin" {
			s.addRuleCommand(rule, "spring-boot-run", path, dir, variables, platform, true)
		} else if mainClass != "" {
			s.addRuleCommand(rule, "exec-java", path, dir, variables, platform, true)
		} else {
			s.issue(path, "scanPartial")
		}
	}
	return nil
}

func (s *scan) mavenLauncher(path string) (tool, dir, platform string) {
	name, platform := "mvnw", "macOS"
	if runtime.GOOS == "windows" {
		name, platform = "mvnw.cmd", "Windows"
	}
	for directory := filepath.Dir(path); ; directory = filepath.Dir(directory) {
		if s.entries[filepath.Join(directory, name)] {
			return "./" + name, directory, platform
		}
		if directory == "." {
			break
		}
	}
	return "mvn", filepath.Dir(path), ""
}

type mavenNode struct {
	name     string
	text     []byte
	attrs    map[string]string
	children []*mavenNode
}

func (n *mavenNode) child(name string) *mavenNode {
	if n != nil {
		for _, child := range n.children {
			if child.name == name {
				return child
			}
		}
	}
	return nil
}

func (n *mavenNode) value(name string) string {
	if child := n.child(name); child != nil {
		return strings.TrimSpace(string(child.text))
	}
	return ""
}

func parseMavenXML(data []byte) (*mavenNode, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var root *mavenNode
	stack := []*mavenNode{}
	nodes := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			if root == nil || len(stack) != 0 {
				return nil, errors.New("invalid Maven document")
			}
			return root, nil
		}
		if err != nil {
			return nil, err
		}
		switch token := token.(type) {
		case xml.Directive:
			return nil, errors.New("unsupported Maven XML directive")
		case xml.StartElement:
			nodes++
			if nodes > 16384 || len(stack) >= 64 {
				return nil, errors.New("Maven XML limit")
			}
			if token.Name.Space != "" && token.Name.Space != "http://maven.apache.org/POM/4.0.0" {
				return nil, errors.New("unsupported Maven namespace")
			}
			node := &mavenNode{name: token.Name.Local, attrs: map[string]string{}}
			for _, attr := range token.Attr {
				node.attrs[attr.Name.Local] = attr.Value
			}
			if len(stack) == 0 {
				if root != nil || node.name != "project" {
					return nil, errors.New("invalid Maven root")
				}
				root = node
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, node)
			}
			stack = append(stack, node)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, errors.New("invalid Maven nesting")
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				node := stack[len(stack)-1]
				node.text = append(node.text, token...)
			} else if strings.TrimSpace(string(token)) != "" {
				return nil, errors.New("invalid Maven document text")
			}
		}
	}
}

type mavenModel struct {
	group, artifact, version, packaging string
	properties                          map[string]string
	plugins, managed                    map[string]mavenPlugin
}

type mavenPlugin struct {
	config, cli                 map[string]string
	inherited, unknown          bool
	configOverride, cliOverride bool
	cliInherited                bool
}

var errMavenParentCycle = errors.New("cyclic Maven parent")

var mavenCoordinate = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,255}$`)
var mavenVersion = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.+~-]{0,255}$`)
var mavenMainClass = regexp.MustCompile(`^(?:[A-Za-z_$][A-Za-z0-9_$]*\.)*[A-Za-z_$][A-Za-z0-9_$]*(?:/[A-Za-z_$][A-Za-z0-9_$.]*)?$`)
var mavenExpression = regexp.MustCompile(`\$\{([^{}]+)\}`)

func resolveMavenValue(value string, properties map[string]string) (string, bool) {
	value = strings.TrimSpace(value)
	if len(value) > 8192 {
		return "", false
	}
	for round := 0; round < 8; round++ {
		if !strings.Contains(value, "${") {
			return value, len(value) <= 8192 && !strings.ContainsAny(value, "\x00\r\n")
		}
		known := true
		replacementBytes := 0
		next := mavenExpression.ReplaceAllStringFunc(value, func(expression string) string {
			replacement, exists := properties[expression[2:len(expression)-1]]
			known = known && exists
			replacementBytes += len(replacement)
			if replacementBytes > 8192 {
				known = false
				return ""
			}
			return replacement
		})
		if !known || next == value || len(next) > 8192 {
			return "", false
		}
		value = next
	}
	return "", false
}

func (s *scan) mavenModel(path string, data []byte, cache map[string]*mavenModel, visiting map[string]bool, depth int) (*mavenModel, error) {
	if visiting[path] {
		return nil, errMavenParentCycle
	}
	if depth >= 8 {
		s.issue(path, "scanLimit")
		return nil, nil
	}
	if model, loaded := cache[path]; loaded {
		return model, nil
	}
	visiting[path] = true
	defer delete(visiting, path)
	root, err := parseMavenXML(data)
	if err != nil {
		return nil, err
	}
	if root.value("modelVersion") != "4.0.0" || root.value("artifactId") == "" {
		return nil, errors.New("invalid Maven model")
	}
	for _, name := range []string{"modelVersion", "groupId", "artifactId", "version", "packaging", "parent", "properties", "build", "modules"} {
		count := 0
		for _, child := range root.children {
			if child.name == name {
				count++
			}
		}
		if count > 1 {
			return nil, errors.New("duplicate Maven model field")
		}
	}
	model := &mavenModel{properties: map[string]string{}, plugins: map[string]mavenPlugin{}, managed: map[string]mavenPlugin{}}
	localProperties := map[string]string{}
	if properties := root.child("properties"); properties != nil {
		for index, property := range properties.children {
			if index >= 256 {
				s.issue(path, "scanLimit")
				break
			}
			if len(property.children) != 0 {
				s.issue(path, "scanPartial")
				continue
			}
			localProperties[property.name] = strings.TrimSpace(string(property.text))
		}
	}
	parentNode := root.child("parent")
	if parentNode != nil {
		relative := "../pom.xml"
		if parentNode.child("relativePath") != nil {
			relative = parentNode.value("relativePath")
		}
		relative, known := resolveMavenValue(relative, localProperties)
		parentPath := ""
		if known && relative != "" {
			parentPath = localJoin(filepath.Dir(path), relative)
			if parentPath != "" && !s.entries[parentPath] {
				parentPath = filepath.Join(parentPath, "pom.xml")
			}
		}
		var parent *mavenModel
		if parentPath != "" {
			if raw := s.read(parentPath); raw != nil {
				parent, err = s.mavenModel(parentPath, raw, cache, visiting, depth+1)
				if err != nil {
					s.issue(parentPath, "scanInvalidManifest")
					if errors.Is(err, errMavenParentCycle) {
						return nil, err
					}
				}
			}
		}
		if parent != nil {
			properties := map[string]string{}
			for name, value := range parent.properties {
				properties[name] = value
			}
			for name, value := range localProperties {
				properties[name] = value
			}
			group, g := resolveMavenValue(parentNode.value("groupId"), properties)
			artifact, a := resolveMavenValue(parentNode.value("artifactId"), properties)
			version, v := resolveMavenValue(parentNode.value("version"), properties)
			if !g || !a || !v || parent.group != group || parent.artifact != artifact || parent.version != version || parent.packaging != "pom" {
				parent = nil
			}
		}
		if parent == nil {
			s.issue(path, "scanPartial")
		} else {
			for name, value := range parent.properties {
				model.properties[name] = value
			}
			for key, plugin := range parent.plugins {
				if plugin.inherited {
					if !plugin.cliInherited {
						plugin.cli = nil
						plugin.cliOverride = false
					}
					model.plugins[key] = plugin
				}
			}
			for key, plugin := range parent.managed {
				if plugin.inherited {
					if !plugin.cliInherited {
						plugin.cli = nil
						plugin.cliOverride = false
					}
					model.managed[key] = plugin
				}
			}
		}
	}
	for name, value := range localProperties {
		model.properties[name] = value
	}
	group, version := root.value("groupId"), root.value("version")
	if group == "" {
		group = parentNode.value("groupId")
	}
	if version == "" {
		version = parentNode.value("version")
	}
	model.properties["project.groupId"] = group
	model.properties["project.artifactId"] = root.value("artifactId")
	model.properties["project.version"] = version
	packaging := root.value("packaging")
	if packaging == "" {
		packaging = "jar"
	}
	model.properties["project.packaging"] = packaging
	for _, key := range []string{"groupId", "artifactId", "version", "packaging"} {
		model.properties["pom."+key] = model.properties["project."+key]
	}
	var known bool
	model.group, known = resolveMavenValue(group, model.properties)
	if !known || !mavenCoordinate.MatchString(model.group) {
		s.issue(path, "scanPartial")
		return nil, nil
	}
	model.artifact, known = resolveMavenValue(root.value("artifactId"), model.properties)
	if !known || !mavenCoordinate.MatchString(model.artifact) {
		s.issue(path, "scanPartial")
		return nil, nil
	}
	model.version, known = resolveMavenValue(version, model.properties)
	if !known || !mavenVersion.MatchString(model.version) {
		s.issue(path, "scanPartial")
		return nil, nil
	}
	model.packaging, known = resolveMavenValue(packaging, model.properties)
	if !known || !mavenCoordinate.MatchString(model.packaging) {
		s.issue(path, "scanPartial")
		return nil, nil
	}
	if root.child("profiles") != nil || root.child("build").child("extensions") != nil {
		s.issue(path, "scanPartial")
	}
	if modules := root.child("modules"); modules != nil {
		if model.packaging != "pom" {
			return nil, errors.New("Maven modules require pom packaging")
		}
		for index, module := range modules.children {
			if index >= 64 {
				s.issue(path, "scanLimit")
				break
			}
			value, known := resolveMavenValue(string(module.text), model.properties)
			modulePath := localJoin(filepath.Dir(path), value)
			if modulePath != "" && !s.entries[modulePath] {
				modulePath = filepath.Join(modulePath, "pom.xml")
			}
			if module.name != "module" || !known || modulePath == "" || !s.entries[modulePath] || modulePath == path {
				s.issue(path, "scanPartial")
			}
		}
	}
	build := root.child("build")
	s.mavenPlugins(path, build.child("pluginManagement").child("plugins"), model, model.managed)
	s.mavenPlugins(path, build.child("plugins"), model, model.plugins)
	cache[path] = model
	return model, nil
}

func (s *scan) mavenPlugins(path string, list *mavenNode, model *mavenModel, plugins map[string]mavenPlugin) {
	if list == nil {
		return
	}
	seen := map[string]bool{}
	for index, node := range list.children {
		if index >= 128 {
			s.issue(path, "scanLimit")
			return
		}
		if node.name != "plugin" {
			s.issue(path, "scanPartial")
			continue
		}
		group := node.value("groupId")
		if group == "" {
			group = "org.apache.maven.plugins"
		}
		group, g := resolveMavenValue(group, model.properties)
		artifact, a := resolveMavenValue(node.value("artifactId"), model.properties)
		if !g || !a || !mavenCoordinate.MatchString(group) || !mavenCoordinate.MatchString(artifact) {
			s.issue(path, "scanPartial")
			continue
		}
		key := group + ":" + artifact
		if key != "org.springframework.boot:spring-boot-maven-plugin" && key != "org.codehaus.mojo:exec-maven-plugin" {
			continue
		}
		plugin := mavenPlugin{config: map[string]string{}, cli: map[string]string{}, inherited: true, cliInherited: true}
		if inherited := node.value("inherited"); inherited != "" {
			value, known := resolveMavenValue(inherited, model.properties)
			plugin.inherited = known && value == "true"
			plugin.unknown = !known || value != "true" && value != "false"
		}
		plugin.config, plugin.configOverride, plugin.unknown = mavenPluginConfig(node.child("configuration"), plugin.unknown)
		if executions := node.child("executions"); executions != nil {
			if len(executions.attrs) > 0 {
				plugin.unknown = true
			}
			cliSeen := false
			for _, execution := range executions.children {
				if execution.value("id") == "default-cli" {
					plugin.unknown = plugin.unknown || cliSeen
					cliSeen = true
					if inherited := execution.value("inherited"); inherited != "" {
						value, known := resolveMavenValue(inherited, model.properties)
						plugin.cliInherited = known && value == "true"
						plugin.unknown = plugin.unknown || !known || value != "true" && value != "false"
					}
					plugin.cli, plugin.cliOverride, plugin.unknown = mavenPluginConfig(execution.child("configuration"), plugin.unknown)
				} else if execution.child("configuration").child("mainClass") != nil {
					// Execution-specific mainClass does not apply to exec:java.
					s.issue(path, "scanPartial")
				}
			}
		}
		if seen[key] {
			plugin.unknown = true
		}
		seen[key] = true
		plugins[key] = mergeMavenPlugin(plugins[key], plugin)
		if plugins[key].unknown {
			s.issue(path, "scanPartial")
		}
	}
}

func mavenPluginConfig(node *mavenNode, unknown bool) (map[string]string, bool, bool) {
	config := map[string]string{}
	if node == nil {
		return config, false, unknown
	}
	override := node.attrs["combine.self"] == "override"
	if node.attrs["combine.children"] != "" || node.attrs["combine.self"] != "" && !override {
		unknown = true
	}
	for _, field := range node.children {
		if field.name != "mainClass" && field.name != "skip" {
			continue
		}
		if _, duplicate := config[field.name]; duplicate || len(field.children) > 0 || len(field.attrs) > 0 {
			unknown = true
		}
		config[field.name] = strings.TrimSpace(string(field.text))
	}
	return config, override, unknown
}

func mergeMavenPlugin(parent, child mavenPlugin) mavenPlugin {
	merged := child
	merged.config, merged.cli = map[string]string{}, map[string]string{}
	if !child.configOverride {
		for key, value := range parent.config {
			merged.config[key] = value
		}
	}
	if !child.cliOverride {
		for key, value := range parent.cli {
			merged.cli[key] = value
		}
	}
	for key, value := range child.config {
		merged.config[key] = value
	}
	for key, value := range child.cli {
		merged.cli[key] = value
	}
	merged.unknown = parent.unknown || child.unknown
	return merged
}
