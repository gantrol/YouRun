package discovery

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

type iosBuildReference struct {
	Target    string
	Container string
}

type iosSharedScheme struct {
	Build               []iosBuildReference
	Configurations      []string
	Launch              *iosBuildReference
	LaunchConfiguration string
}

type iosProject struct {
	path, dir    string
	objects      map[string]any
	root         map[string]any
	pathsUnknown bool
}

// Only shared scheme references and PBX object identities connect a command to
// a target. The old generic Xcode build/archive discovery remains independent.
func (s *scan) iosXcodeCommands(path string, data []byte, rule Rule) error {
	if filepath.Base(path) == "project.pbxproj" {
		return s.iosTargetCommands(path, data, rule)
	}
	scheme, err := parseIOSSharedScheme(data)
	if err != nil {
		if errors.Is(err, errIOSLimit) {
			s.issue(path, "scanLimit")
			return nil
		}
		return err
	}
	container := iosSchemeContainer(path)
	if container == "" || len(scheme.Build) == 0 {
		s.issue(path, "scanPartial")
		return nil
	}
	dir := filepath.Dir(container)
	name := strings.TrimSuffix(filepath.Base(path), ".xcscheme")
	if !iosLiteralName(name) {
		s.issue(path, "scanPartial")
		return nil
	}
	kind := "project"
	var workspace map[string]bool
	if strings.HasSuffix(strings.ToLower(container), ".xcworkspace") {
		kind = "workspace"
		workspace = s.iosWorkspaceProjects(container)
		if len(workspace) == 0 {
			s.issue(path, "scanPartial")
			return nil
		}
	}
	projects := map[string]*iosProject{}
	resolve := func(reference iosBuildReference) (*iosProject, map[string]any) {
		projectPath := iosContainerReference(dir, reference.Container)
		if projectPath == "" || kind == "workspace" && !workspace[projectPath] {
			s.issue(path, "scanPartial")
			return nil, nil
		}
		if kind == "project" && projectPath != container {
			// A scheme may build a subproject, but resolving its project graph
			// requires more than trusting an arbitrary ReferencedContainer.
			s.issue(path, "scanPartial")
			return nil, nil
		}
		project, loaded := projects[projectPath]
		if !loaded {
			if len(projects) >= 16 {
				s.issue(path, "scanLimit")
				return nil, nil
			}
			manifest := filepath.Join(projectPath, "project.pbxproj")
			if raw := s.read(manifest); raw != nil {
				project = s.iosReadProject(manifest, raw)
			} else {
				s.issue(manifest, "scanPartial")
			}
			projects[projectPath] = project
		}
		if project == nil {
			return nil, nil
		}
		target := project.target(reference.Target)
		if target == nil {
			s.issue(path, "scanPartial")
			return nil, nil
		}
		return project, target
	}
	variables := map[string]string{"container": filepath.Base(container), "scheme": name}
	for _, configuration := range scheme.Configurations {
		if s.ctx.Err() != nil {
			return s.ctx.Err()
		}
		if !iosLiteralName(configuration) {
			s.issue(path, "scanPartial")
			continue
		}
		simulator, device := true, true
		for _, reference := range scheme.Build {
			project, target := resolve(reference)
			if project == nil {
				simulator, device = false, false
				break
			}
			settings := s.iosTargetSettings(project, target, configuration)
			sim, dev := iosSupportedPlatforms(settings)
			simulator, device = simulator && sim, device && dev
		}
		variables["configuration"] = configuration
		if simulator {
			s.addRuleCommand(rule, kind+"-simulator", path, dir, variables, "macOS", true)
		}
		if device {
			s.addRuleCommand(rule, kind+"-device", path, dir, variables, "macOS", true)
		}
	}
	if scheme.Launch == nil || !iosLiteralName(scheme.LaunchConfiguration) {
		return nil
	}
	built := false
	for _, reference := range scheme.Build {
		built = built || reference == *scheme.Launch
	}
	if !built {
		s.issue(path, "scanPartial")
		return nil
	}
	project, target := resolve(*scheme.Launch)
	if project == nil || iosString(target, "productType") != "com.apple.product-type.application" {
		return nil
	}
	settings := s.iosTargetSettings(project, target, scheme.LaunchConfiguration)
	if simulator, _ := iosSupportedPlatforms(settings); !simulator {
		return nil
	}
	if bundleID := s.iosBundleIdentifier(project, settings); bundleID != "" {
		variables["bundleID"] = bundleID
		// This starts an already installed app on an already booted simulator.
		// Discovery neither starts a device nor assumes a DerivedData path.
		s.addRuleCommand(rule, "simulator-launch", path, dir, variables, "macOS", true)
	}
	return nil
}

// Explicit app targets stay discoverable alongside shared schemes. A scheme
// may be damaged, test-only, or describe a different target in this project.
func (s *scan) iosTargetCommands(path string, data []byte, rule Rule) error {
	container := filepath.Dir(path)
	project := s.iosReadProject(path, data)
	if project == nil {
		return nil
	}
	ids := iosStrings(project.root["targets"])
	if len(ids) > 128 {
		s.issue(path, "scanLimit")
		return nil
	}
	nameCounts := map[string]int{}
	for _, id := range ids {
		nameCounts[iosString(project.target(id), "name")]++
	}
	for _, id := range ids {
		if s.ctx.Err() != nil {
			return s.ctx.Err()
		}
		target := project.target(id)
		name := iosString(target, "name")
		if iosString(target, "productType") != "com.apple.product-type.application" || !iosLiteralName(name) || nameCounts[name] != 1 {
			continue
		}
		configs := project.configurations(target)
		names := make([]string, 0, len(configs))
		for configuration := range configs {
			names = append(names, configuration)
		}
		sort.Strings(names)
		if len(names) > 32 {
			s.issue(path, "scanLimit")
			continue
		}
		for _, configuration := range names {
			if s.ctx.Err() != nil {
				return s.ctx.Err()
			}
			settings := s.iosTargetSettings(project, target, configuration)
			simulator, device := iosSupportedPlatforms(settings)
			variables := map[string]string{"container": filepath.Base(container), "target": name, "configuration": configuration}
			if simulator {
				s.addRuleCommand(rule, "target-simulator", path, project.dir, variables, "macOS", true)
			}
			if device {
				s.addRuleCommand(rule, "target-device", path, project.dir, variables, "macOS", true)
			}
		}
	}
	return nil
}

func parseIOSSharedScheme(data []byte) (iosSharedScheme, error) {
	result := iosSharedScheme{}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	stack := []string{}
	nodes, roots, launches := 0, 0, 0
	entryEnabled, runnable := false, false
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			if roots != 1 || len(stack) != 0 {
				return result, errors.New("invalid shared scheme")
			}
			return result, nil
		}
		if err != nil {
			return result, err
		}
		switch element := token.(type) {
		case xml.Directive:
			return result, errors.New("unsupported scheme directive")
		case xml.StartElement:
			nodes++
			if nodes > 8192 || len(stack) > 48 {
				return result, errIOSLimit
			}
			if len(stack) == 0 {
				roots++
				if element.Name.Local != "Scheme" || roots > 1 {
					return result, errors.New("invalid scheme root")
				}
			}
			attributes := iosXMLAttributes(element)
			parent := strings.Join(stack, "/")
			switch {
			case parent == "Scheme/BuildAction/BuildActionEntries" && element.Name.Local == "BuildActionEntry":
				entryEnabled = attributes["buildForRunning"] == "YES"
			case parent == "Scheme/BuildAction/BuildActionEntries/BuildActionEntry" && element.Name.Local == "BuildableReference" && entryEnabled:
				if len(result.Build) >= 64 {
					return result, errIOSLimit
				}
				result.Build = append(result.Build, iosBuildReference{attributes["BlueprintIdentifier"], attributes["ReferencedContainer"]})
			case parent == "Scheme" && strings.HasSuffix(element.Name.Local, "Action"):
				if configuration := attributes["buildConfiguration"]; configuration != "" {
					result.Configurations = appendUnique(result.Configurations, configuration)
					if len(result.Configurations) > 32 {
						return result, errIOSLimit
					}
				}
				if element.Name.Local == "LaunchAction" {
					launches++
					if launches > 1 {
						return result, errors.New("ambiguous scheme launch action")
					}
					result.LaunchConfiguration = attributes["buildConfiguration"]
				}
			case parent == "Scheme/LaunchAction" && element.Name.Local == "BuildableProductRunnable":
				runnable = attributes["runnableDebuggingMode"] == "0"
			case parent == "Scheme/LaunchAction/BuildableProductRunnable" && element.Name.Local == "BuildableReference" && runnable:
				if result.Launch != nil {
					return result, errors.New("ambiguous scheme runnable")
				}
				result.Launch = &iosBuildReference{attributes["BlueprintIdentifier"], attributes["ReferencedContainer"]}
			}
			stack = append(stack, element.Name.Local)
		case xml.EndElement:
			if len(stack) == 0 {
				return result, errors.New("invalid scheme structure")
			}
			stack = stack[:len(stack)-1]
		}
	}
}

func iosXMLAttributes(element xml.StartElement) map[string]string {
	result := map[string]string{}
	for _, attr := range element.Attr {
		result[attr.Name.Local] = attr.Value
	}
	return result
}

func iosLiteralName(value string) bool {
	return value != "" && len(value) <= 256 && utf8.ValidString(value) && strings.TrimSpace(value) == value && !strings.HasPrefix(value, "-") && !strings.ContainsAny(value, "$`\x00") && strings.IndexFunc(value, unicode.IsControl) < 0
}

func iosString(object map[string]any, key string) string {
	value, _ := object[key].(string)
	return value
}

func iosStrings(value any) []string {
	items, _ := value.([]any)
	result := []string{}
	for _, item := range items {
		if text, ok := item.(string); ok {
			result = append(result, text)
		}
	}
	return result
}

func iosSchemeContainer(path string) string {
	for dir := filepath.Dir(path); dir != "."; dir = filepath.Dir(dir) {
		if strings.HasSuffix(strings.ToLower(dir), ".xcodeproj") || strings.HasSuffix(strings.ToLower(dir), ".xcworkspace") {
			return dir
		}
	}
	return ""
}

func iosContainerReference(base, reference string) string {
	prefix, value, ok := strings.Cut(reference, ":")
	if !ok || prefix != "container" || !strings.HasSuffix(strings.ToLower(value), ".xcodeproj") {
		return ""
	}
	return localJoin(base, value)
}

func (s *scan) iosReadProject(path string, data []byte) *iosProject {
	root, err := parseIOSPBX(data)
	if err != nil {
		code := "scanInvalidManifest"
		if errors.Is(err, errIOSLimit) {
			code = "scanLimit"
		}
		s.issue(path, code)
		return nil
	}
	objects, _ := root["objects"].(map[string]any)
	project, _ := objects[iosString(root, "rootObject")].(map[string]any)
	if iosString(project, "isa") != "PBXProject" {
		s.issue(path, "scanInvalidManifest")
		return nil
	}
	pathsUnknown := iosString(project, "projectDirPath") != "" || iosString(project, "projectRoot") != ""
	if pathsUnknown {
		s.issue(path, "scanPartial")
	}
	return &iosProject{path: path, dir: filepath.Dir(filepath.Dir(path)), objects: objects, root: project, pathsUnknown: pathsUnknown}
}

func (p *iosProject) target(id string) map[string]any {
	if !containsString(iosStrings(p.root["targets"]), id) {
		return nil
	}
	object, _ := p.objects[id].(map[string]any)
	if iosString(object, "isa") != "PBXNativeTarget" {
		return nil
	}
	return object
}

func (p *iosProject) configurations(object map[string]any) map[string]map[string]any {
	result := map[string]map[string]any{}
	list, _ := p.objects[iosString(object, "buildConfigurationList")].(map[string]any)
	if iosString(list, "isa") != "XCConfigurationList" {
		return result
	}
	for _, id := range iosStrings(list["buildConfigurations"]) {
		config, _ := p.objects[id].(map[string]any)
		name := iosString(config, "name")
		if iosString(config, "isa") != "XCBuildConfiguration" || !iosLiteralName(name) {
			continue
		}
		if _, duplicate := result[name]; duplicate {
			result[name] = nil
		} else {
			result[name] = config
		}
	}
	return result
}

func (s *scan) iosWorkspaceProjects(container string) map[string]bool {
	path := filepath.Join(container, "contents.xcworkspacedata")
	data := s.read(path)
	projects := map[string]bool{}
	if data == nil {
		s.issue(path, "scanPartial")
		return projects
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	base := filepath.Dir(container)
	stack := []string{}
	nodes, roots := 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			if roots != 1 || len(stack) != 0 {
				s.issue(path, "scanInvalidManifest")
				return nil
			}
			return projects
		}
		if err != nil {
			s.issue(path, "scanInvalidManifest")
			return nil
		}
		switch element := token.(type) {
		case xml.Directive:
			s.issue(path, "scanPartial")
			return nil
		case xml.StartElement:
			nodes++
			if nodes > 8192 || len(stack) > 48 {
				s.issue(path, "scanLimit")
				return nil
			}
			current := base
			if len(stack) > 0 {
				current = stack[len(stack)-1]
			} else {
				roots++
				if element.Name.Local != "Workspace" || roots != 1 {
					s.issue(path, "scanInvalidManifest")
					return nil
				}
			}
			location := iosXMLAttributes(element)["location"]
			if location != "" {
				kind, value, valid := strings.Cut(location, ":")
				if !valid || kind != "group" && kind != "container" {
					current = ""
				} else if kind == "container" {
					current = base
					if value != "" {
						current = localJoin(base, value)
					}
				} else if current != "" && value != "" {
					current = localJoin(current, value)
				}
			}
			if current == "" {
				s.issue(path, "scanPartial")
			} else if element.Name.Local == "FileRef" && strings.HasSuffix(strings.ToLower(current), ".xcodeproj") {
				projects[current] = true
			}
			stack = append(stack, current)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil
			}
			stack = stack[:len(stack)-1]
		}
	}
}

var iosBundleID = regexp.MustCompile(`^[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+$`)
