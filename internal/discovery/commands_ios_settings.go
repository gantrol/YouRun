package discovery

import (
	"bytes"
	"encoding/xml"
	"io"
	"regexp"
	"strings"
)

var iosRelevantSettings = []string{"SDKROOT", "SUPPORTED_PLATFORMS", "PRODUCT_BUNDLE_IDENTIFIER", "INFOPLIST_FILE", "GENERATE_INFOPLIST_FILE", "INFOPLIST_PREPROCESS", "INFOPLIST_KEY_CFBundleIdentifier"}

type iosBuildSettings struct {
	values  map[string]string
	unknown map[string]bool
}

func newIOSSettings() *iosBuildSettings {
	return &iosBuildSettings{values: map[string]string{}, unknown: map[string]bool{}}
}

func (v *iosBuildSettings) invalidate(key string) {
	delete(v.values, key)
	v.unknown[key] = true
}

func (v *iosBuildSettings) invalidateAll() {
	for _, key := range iosRelevantSettings {
		v.invalidate(key)
	}
}

func (s *scan) iosTargetSettings(project *iosProject, target map[string]any, configuration string) *iosBuildSettings {
	result := newIOSSettings()
	projectConfig := project.configurations(project.root)[configuration]
	targetConfig := project.configurations(target)[configuration]
	if projectConfig == nil || targetConfig == nil {
		s.issue(project.path, "scanPartial")
		result.invalidateAll()
		return result
	}
	for _, config := range []map[string]any{projectConfig, targetConfig} {
		if base := iosString(config, "baseConfigurationReference"); base != "" {
			file := project.fileReferencePath(base)
			if file == "" {
				s.issue(project.path, "scanPartial")
				result.invalidateAll()
			} else if data := s.read(file); data == nil {
				s.issue(file, "scanPartial")
				result.invalidateAll()
			} else {
				s.iosXCConfigSettings(file, data, result)
			}
		}
		if iosString(config, "baseConfigurationReferenceRelativePath") != "" || iosString(config, "baseConfigurationReferenceAnchor") != "" {
			s.issue(project.path, "scanPartial")
			result.invalidateAll()
		}
		values, _ := config["buildSettings"].(map[string]any)
		conditional := map[string]bool{}
		for field, raw := range values {
			key, _, condition := strings.Cut(field, "[")
			if !containsString(iosRelevantSettings, key) {
				continue
			}
			if condition {
				conditional[key] = true
				continue
			}
			value, ok := raw.(string)
			if !ok && key == "SUPPORTED_PLATFORMS" {
				items := iosStrings(raw)
				if array, isArray := raw.([]any); isArray && len(items) == len(array) {
					value, ok = strings.Join(items, " "), true
				}
			}
			if !ok || !iosStaticSetting(value) {
				result.invalidate(key)
				s.issue(project.path, "scanPartial")
				continue
			}
			result.values[key] = strings.TrimSpace(value)
			delete(result.unknown, key)
		}
		// Conditional settings have their own specificity. Map iteration order
		// must not make an unconditional sibling appear to resolve them.
		for key := range conditional {
			result.invalidate(key)
			s.issue(project.path, "scanPartial")
		}
	}
	return result
}

func iosStaticSetting(value string) bool {
	return len(value) <= 4096 && !strings.ContainsAny(value, "$\x00\r\n`\\")
}

var iosSDKName = regexp.MustCompile(`^(iphoneos|iphonesimulator)(?:[0-9]+(?:\.[0-9]+)*)?$`)

func iosSupportedPlatforms(settings *iosBuildSettings) (simulator, device bool) {
	if settings == nil || settings.unknown["SDKROOT"] || settings.unknown["SUPPORTED_PLATFORMS"] {
		return false, false
	}
	sdk := settings.values["SDKROOT"]
	if sdk != "" && sdk != "auto" && !iosSDKName.MatchString(sdk) {
		return false, false
	}
	if platforms, declared := settings.values["SUPPORTED_PLATFORMS"]; declared {
		for _, platform := range strings.Fields(platforms) {
			simulator = simulator || platform == "iphonesimulator"
			device = device || platform == "iphoneos"
		}
		return simulator, device
	}
	// Xcode's iOS base SDK supports device and simulator destinations. The
	// -sdk arguments choose the member of that family; explicit restrictions
	// above take precedence, and unresolved settings never get this default.
	if strings.HasPrefix(sdk, "iphoneos") {
		return true, true
	}
	return strings.HasPrefix(sdk, "iphonesimulator"), false
}

func (p *iosProject) fileReferencePath(id string) string {
	if p.pathsUnknown {
		return ""
	}
	object, _ := p.objects[id].(map[string]any)
	if iosString(object, "isa") != "PBXFileReference" {
		return ""
	}
	parents := map[string]string{}
	for parentID, raw := range p.objects {
		parent, _ := raw.(map[string]any)
		if iosString(parent, "isa") != "PBXGroup" {
			continue
		}
		for _, child := range iosStrings(parent["children"]) {
			if _, exists := parents[child]; exists {
				parents[child] = ""
			} else {
				parents[child] = parentID
			}
		}
	}
	parts := []string{}
	seen := map[string]bool{}
	for len(seen) < 48 {
		if seen[id] {
			return ""
		}
		seen[id] = true
		object, _ = p.objects[id].(map[string]any)
		path := iosString(object, "path")
		if path != "" {
			if !iosStaticSetting(path) {
				return ""
			}
			parts = append(parts, path)
		}
		source := iosString(object, "sourceTree")
		if source == "SOURCE_ROOT" || (source == "<group>" || source == "") && id == iosString(p.root, "mainGroup") {
			path := p.dir
			for i := len(parts) - 1; i >= 0; i-- {
				path = localJoin(path, parts[i])
				if path == "" {
					return ""
				}
			}
			return path
		}
		if source != "<group>" && source != "" {
			return ""
		}
		id = parents[id]
		if id == "" {
			return ""
		}
	}
	return ""
}

var iosXCConfigAssignment = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)(.*?)=(.*)$`)

func (s *scan) iosXCConfigSettings(path string, data []byte, result *iosBuildSettings) {
	conditional := map[string]bool{}
	// xcconfig strings can contain //, so comments are stripped with a lexer.
	clean := append([]byte{}, data...)
	quoted := false
	for i := 0; i < len(clean); i++ {
		if clean[i] == '\\' && quoted {
			i++
			continue
		}
		if clean[i] == '"' {
			quoted = !quoted
		}
		if quoted || clean[i] != '/' || i+1 >= len(clean) {
			continue
		}
		if clean[i+1] == '/' {
			for i < len(clean) && clean[i] != '\n' && clean[i] != '\r' {
				clean[i] = ' '
				i++
			}
		} else if clean[i+1] == '*' {
			end := bytes.Index(clean[i+2:], []byte("*/"))
			if end < 0 {
				result.invalidateAll()
				s.issue(path, "scanPartial")
				return
			}
			end += i + 4
			for i < end {
				if clean[i] != '\n' && clean[i] != '\r' {
					clean[i] = ' '
				}
				i++
			}
			i--
		}
	}
	if quoted {
		result.invalidateAll()
		s.issue(path, "scanPartial")
		return
	}
	for index, line := range strings.Split(string(clean), "\n") {
		if index >= 8192 {
			result.invalidateAll()
			s.issue(path, "scanLimit")
			return
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") || strings.HasSuffix(line, "\\") {
			result.invalidateAll()
			s.issue(path, "scanPartial")
			continue
		}
		match := iosXCConfigAssignment.FindStringSubmatch(line)
		if match == nil {
			result.invalidateAll()
			s.issue(path, "scanPartial")
			continue
		}
		if !containsString(iosRelevantSettings, match[1]) {
			continue
		}
		key, qualifier, value := match[1], strings.TrimSpace(match[2]), strings.TrimSpace(match[3])
		if qualifier != "" {
			conditional[key] = true
			continue
		}
		if strings.HasPrefix(value, "\"") {
			parser := iosPBXParser{data: []byte(value)}
			decoded, err := parser.string()
			if err != nil || parser.index != len(parser.data) {
				result.invalidate(key)
				s.issue(path, "scanPartial")
				continue
			}
			value = decoded
		}
		if !iosStaticSetting(value) {
			result.invalidate(key)
			s.issue(path, "scanPartial")
			continue
		}
		result.values[key] = value
		delete(result.unknown, key)
	}
	for key := range conditional {
		result.invalidate(key)
		s.issue(path, "scanPartial")
	}
}

func (s *scan) iosBundleIdentifier(project *iosProject, settings *iosBuildSettings) string {
	if settings == nil || settings.unknown["GENERATE_INFOPLIST_FILE"] || settings.unknown["INFOPLIST_FILE"] {
		return ""
	}
	if settings.unknown["INFOPLIST_PREPROCESS"] || settings.values["INFOPLIST_PREPROCESS"] == "YES" || settings.unknown["INFOPLIST_KEY_CFBundleIdentifier"] || settings.values["INFOPLIST_KEY_CFBundleIdentifier"] != "" {
		// Plist preprocessing and explicit generated-key overrides need the
		// effective build pipeline to resolve their precedence and values.
		s.issue(project.path, "scanPartial")
		return ""
	}
	bundle := settings.values["PRODUCT_BUNDLE_IDENTIFIER"]
	generated := settings.values["GENERATE_INFOPLIST_FILE"] == "YES"
	if plist := settings.values["INFOPLIST_FILE"]; plist != "" {
		if project.pathsUnknown {
			return ""
		}
		path := localJoin(project.dir, plist)
		if path == "" {
			s.issue(project.path, "scanPartial")
			return ""
		}
		data := s.read(path)
		if data == nil {
			s.issue(path, "scanPartial")
			return ""
		}
		value, found, valid := iosPlistBundleIdentifier(data)
		if !valid {
			s.issue(path, "scanPartial")
			return ""
		}
		if found {
			if value != "$(PRODUCT_BUNDLE_IDENTIFIER)" && value != "${PRODUCT_BUNDLE_IDENTIFIER}" {
				bundle = value
			}
		} else if !generated {
			s.issue(path, "scanPartial")
			return ""
		}
	} else if !generated {
		s.issue(project.path, "scanPartial")
		return ""
	}
	if len(bundle) > 255 || !iosBundleID.MatchString(bundle) {
		s.issue(project.path, "scanPartial")
		return ""
	}
	return bundle
}

// Only a top-level CFBundleIdentifier string participates in app identity.
// Plist DOCTYPE is accepted as text; encoding/xml never fetches external DTDs.
func iosPlistBundleIdentifier(data []byte) (value string, found, valid bool) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	stack := []string{}
	pending := ""
	nodes, roots, dictionaries := 0, 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return value, found, roots == 1 && dictionaries == 1 && len(stack) == 0
		}
		if err != nil {
			return "", false, false
		}
		switch element := token.(type) {
		case xml.StartElement:
			nodes++
			if nodes > 8192 || len(stack) > 48 {
				return "", false, false
			}
			if len(stack) == 0 {
				roots++
				if element.Name.Local != "plist" || roots > 1 {
					return "", false, false
				}
			}
			if len(stack) == 1 {
				dictionaries++
				if element.Name.Local != "dict" || dictionaries != 1 {
					return "", false, false
				}
			}
			if strings.Join(stack, "/") == "plist/dict" {
				if element.Name.Local == "key" {
					if decoder.DecodeElement(&pending, &element) != nil {
						return "", false, false
					}
					continue
				}
				if pending == "CFBundleIdentifier" {
					if found || element.Name.Local != "string" || decoder.DecodeElement(&value, &element) != nil {
						return "", false, false
					}
					found, pending = true, ""
					continue
				}
				pending = ""
			}
			stack = append(stack, element.Name.Local)
		case xml.EndElement:
			if len(stack) == 0 {
				return "", false, false
			}
			stack = stack[:len(stack)-1]
		}
	}
}
