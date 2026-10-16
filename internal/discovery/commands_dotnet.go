package discovery

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Configured commands are separate from the original MSBuild candidates. They
// only describe static project targets and do not evaluate MSBuild or load SDKs.
func (s *scan) dotnetConfiguredCommands(path string, data []byte, rule Rule) error {
	dir := filepath.Dir(path)
	values := map[string]string{}
	unknown := map[string]bool{}
	fields := map[string]bool{"Sdk": true, "OutputType": true, "IsTestProject": true, "UseMaui": true, "TargetFramework": true, "TargetFrameworks": true, "LaunchSettingsFilePath": true, "AppDesignerFolder": true}
	for property := range rule.Match.PropertyEquals {
		fields[property] = true
	}
	readProperties := func(file string, raw []byte) bool {
		if err := s.dotnetCommandProperties(file, raw, fields, values, unknown); err != nil {
			s.issue(file, "scanInvalidManifest")
			return false
		}
		return true
	}
	if props := s.dotnetCommandAncestor(dir, "Directory.Build.props"); props != "" {
		if raw := s.read(props); raw == nil || !readProperties(props, raw) {
			return nil
		}
	}
	if err := s.dotnetCommandProperties(path, data, fields, values, unknown); err != nil {
		return err
	}
	if targets := s.dotnetCommandAncestor(dir, "Directory.Build.targets"); targets != "" {
		if raw := s.read(targets); raw == nil || !readProperties(targets, raw) {
			return nil
		}
	}
	// Explicit imports can replace any of these values. Do not present static
	// candidates as configured launches when their effective target is unknown.
	if unknown["_import"] {
		return nil
	}
	for property, expected := range rule.Match.PropertyEquals {
		if unknown[property] || !strings.EqualFold(values[property], expected) {
			return nil
		}
	}
	if unknown["OutputType"] || unknown["IsTestProject"] || strings.EqualFold(values["IsTestProject"], "true") || bytes.Contains(data, []byte("Microsoft.NET.Test.Sdk")) {
		return nil
	}
	web := strings.EqualFold(values["Sdk"], "Microsoft.NET.Sdk.Web")
	app := strings.EqualFold(values["OutputType"], "Exe") || strings.EqualFold(values["OutputType"], "WinExe") || values["OutputType"] == "" && web
	if !app {
		return nil
	}
	frameworks := s.dotnetCommandFrameworks(path, values, unknown)
	if len(frameworks) == 0 {
		return nil
	}
	variables := map[string]string{"file": filepath.Base(path)}
	if _, enabled := rule.Commands["run-target"]; enabled && strings.EqualFold(values["UseMaui"], "true") {
		for _, framework := range frameworks {
			platform, mobile := dotnetCommandPlatform(framework)
			if !mobile && platform != "Windows" {
				s.issue(path, "scanPartial")
				continue
			}
			variables["framework"] = framework
			s.addRuleCommand(rule, "run-target", path, dir, variables, platform, true)
		}
	}
	if _, enabled := rule.Commands["profile"]; !enabled || unknown["LaunchSettingsFilePath"] {
		return nil
	}
	configs := rule.Options.ConfigFiles
	if configured := values["LaunchSettingsFilePath"]; configured != "" {
		configs = []string{configured}
	} else if unknown["AppDesignerFolder"] {
		s.issue(path, "scanPartial")
		return nil
	} else if folder := values["AppDesignerFolder"]; folder != "" {
		configs = make([]string, 0, len(rule.Options.ConfigFiles))
		for _, config := range rule.Options.ConfigFiles {
			configs = append(configs, filepath.Join(folder, filepath.Base(config)))
		}
	}
	for _, config := range configs {
		file := localJoin(dir, config)
		if file == "" {
			s.issue(path, "scanPartial")
			continue
		}
		if !s.entries[file] {
			continue
		}
		raw := s.read(file)
		if raw == nil {
			return nil
		}
		profiles, err := dotnetLaunchProfiles(raw)
		if err != nil {
			s.issue(file, "scanInvalidManifest")
			return nil
		}
		names := make([]string, 0, len(profiles))
		counts := map[string]int{}
		for name := range profiles {
			names = append(names, name)
			counts[strings.ToLower(name)]++
		}
		sort.Strings(names)
		for index, name := range names {
			if index >= 64 {
				s.issue(file, "scanLimit")
				break
			}
			if counts[strings.ToLower(name)] > 1 || !dotnetProfileName(name) {
				s.issue(file, "scanPartial")
				continue
			}
			if profiles[name].CommandName != "Project" {
				continue
			}
			variables["profile"] = name
			for _, framework := range frameworks {
				platform, mobile := dotnetCommandPlatform(framework)
				if mobile {
					s.issue(file, "scanPartial")
					continue
				}
				variables["framework"] = framework
				s.addRuleCommand(rule, "profile", file, dir, variables, platform, true)
			}
		}
		// The SDK uses one launch settings file for this project.
		return nil
	}
	return nil
}

func (s *scan) dotnetCommandAncestor(dir, name string) string {
	for {
		file := filepath.Join(dir, name)
		if s.entries[file] {
			return file
		}
		if dir == "." {
			return ""
		}
		dir = filepath.Dir(dir)
	}
}

func (s *scan) dotnetCommandProperties(path string, data []byte, fields map[string]bool, values map[string]string, unknown map[string]bool) error {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	type element struct {
		name        string
		conditional bool
	}
	stack := []element{}
	nodes, roots := 0, 0
	setValue := func(name, value string, conditional bool) {
		value = strings.TrimSpace(value)
		if conditional || dynamicMSBuildValue(value) {
			s.issue(path, "scanPartial")
			// Typical MAUI projects append Windows only on Windows hosts. A
			// self-append leaves the existing unconditional targets available;
			// the conditional additions themselves are not claimed as targets.
			prefix := "$(" + name + ");"
			if (name == "TargetFrameworks" || name == "TargetFramework") && strings.HasPrefix(value, prefix) && values[name] != "" && !unknown[name] {
				suffix := strings.TrimSpace(strings.TrimPrefix(value, prefix))
				if !dynamicMSBuildValue(suffix) {
					if !conditional {
						values[name] += ";" + suffix
					}
					return
				}
			}
			delete(values, name)
			unknown[name] = true
			return
		}
		values[name] = value
		delete(unknown, name)
	}
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			if len(stack) != 0 || roots != 1 {
				return errors.New("invalid project document")
			}
			return nil
		}
		if err != nil {
			return err
		}
		switch item := token.(type) {
		case xml.Directive:
			s.issue(path, "scanPartial")
			unknown["_import"] = true
		case xml.StartElement:
			nodes++
			if nodes > 16384 || len(stack) > 64 {
				s.issue(path, "scanLimit")
				return errors.New("project document limit")
			}
			if len(stack) == 0 {
				roots++
				if item.Name.Local != "Project" || roots != 1 {
					return errors.New("invalid project root")
				}
			}
			conditional := len(stack) > 0 && stack[len(stack)-1].conditional
			switch item.Name.Local {
			case "Choose", "When", "Otherwise", "Target":
				conditional = true
			}
			attributes := map[string]string{}
			for _, attr := range item.Attr {
				attributes[attr.Name.Local] = attr.Value
				if strings.EqualFold(attr.Name.Local, "Condition") {
					conditional = true
				}
			}
			if item.Name.Local == "Project" && attributes["Sdk"] != "" {
				setValue("Sdk", attributes["Sdk"], conditional)
			}
			if item.Name.Local == "Sdk" && attributes["Name"] != "" {
				setValue("Sdk", attributes["Name"], conditional)
			}
			if item.Name.Local == "Import" {
				s.issue(path, "scanPartial")
				unknown["_import"] = true
			}
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1].name
			}
			if fields[item.Name.Local] && parent == "PropertyGroup" {
				var value string
				if err := decoder.DecodeElement(&value, &item); err != nil {
					return err
				}
				setValue(item.Name.Local, value, conditional)
			} else {
				stack = append(stack, element{item.Name.Local, conditional})
			}
		case xml.EndElement:
			if len(stack) == 0 {
				return errors.New("invalid project document")
			}
			stack = stack[:len(stack)-1]
		}
	}
}

var dotnetFrameworkName = regexp.MustCompile(`^(?:net|netcoreapp)[0-9]+(?:\.[0-9]+)*(?:-[a-z]+[0-9]*(?:\.[0-9]+)*)?$`)

func (s *scan) dotnetCommandFrameworks(path string, values map[string]string, unknown map[string]bool) []string {
	property := "TargetFrameworks"
	if values[property] == "" && !unknown[property] {
		property = "TargetFramework"
	}
	if unknown[property] || values[property] == "" {
		s.issue(path, "scanPartial")
		return nil
	}
	frameworks := []string{}
	for index, framework := range strings.Split(values[property], ";") {
		if index >= 32 {
			s.issue(path, "scanLimit")
			break
		}
		framework = strings.TrimSpace(framework)
		if len(framework) > 96 || !dotnetFrameworkName.MatchString(framework) {
			s.issue(path, "scanPartial")
			continue
		}
		frameworks = appendUnique(frameworks, framework)
	}
	return frameworks
}

func dotnetCommandPlatform(framework string) (platform string, mobile bool) {
	switch {
	case strings.Contains(framework, "-windows"), strings.HasPrefix(framework, "net4"):
		return "Windows", false
	case strings.Contains(framework, "-ios"), strings.Contains(framework, "-maccatalyst"), strings.Contains(framework, "-tvos"):
		return "macOS", true
	case strings.Contains(framework, "-android"):
		return "", true
	case strings.Contains(framework, "-macos"):
		return "macOS", false
	default:
		return "", false
	}
}

type dotnetLaunchProfile struct {
	CommandName string `json:"commandName"`
}

func dotnetLaunchProfiles(data []byte) (map[string]dotnetLaunchProfile, error) {
	var settings struct {
		Profiles json.RawMessage `json:"profiles"`
	}
	clean, err := dotnetLaunchJSON(data)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(clean, &settings); err != nil {
		return nil, err
	}
	profiles := map[string]dotnetLaunchProfile{}
	if len(settings.Profiles) == 0 || bytes.Equal(bytes.TrimSpace(settings.Profiles), []byte("null")) {
		return profiles, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(settings.Profiles))
	if token, err := decoder.Token(); err != nil || token != json.Delim('{') {
		return nil, errors.New("invalid launch profiles")
	}
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := token.(string)
		if _, duplicate := profiles[name]; !ok || duplicate {
			return nil, errors.New("ambiguous launch profile")
		}
		var profile dotnetLaunchProfile
		if err := decoder.Decode(&profile); err != nil {
			return nil, err
		}
		profiles[name] = profile
	}
	return profiles, nil
}

func dotnetProfileName(name string) bool {
	return name != "" && len(name) <= 256 && !strings.HasPrefix(name, "-") && strings.TrimSpace(name) == name && strings.IndexFunc(name, unicode.IsControl) < 0 && !dynamicMSBuildValue(name)
}

// The SDK accepts JSON comments and trailing commas in launchSettings.json.
// Preserve strings verbatim, including URLs and escaped quotes; no environment
// values are copied into the command model or diagnostics.
func dotnetLaunchJSON(data []byte) ([]byte, error) {
	clean := append([]byte{}, bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})...)
	for i := 0; i < len(clean); i++ {
		if clean[i] == '"' {
			i++
			for i < len(clean) && clean[i] != '"' {
				if clean[i] == '\\' {
					i++
				}
				i++
			}
			continue
		}
		if clean[i] != '/' || i+1 >= len(clean) {
			continue
		}
		if clean[i+1] == '/' {
			for i < len(clean) && clean[i] != '\n' && clean[i] != '\r' {
				clean[i] = ' '
				i++
			}
		} else if clean[i+1] == '*' {
			clean[i], clean[i+1] = ' ', ' '
			i += 2
			for i+1 < len(clean) && !(clean[i] == '*' && clean[i+1] == '/') {
				clean[i] = ' '
				i++
			}
			if i+1 >= len(clean) {
				return nil, errors.New("unterminated JSON comment")
			}
			clean[i], clean[i+1] = ' ', ' '
			i++
		}
	}
	for i := 0; i < len(clean); i++ {
		if clean[i] == '"' {
			i++
			for i < len(clean) && clean[i] != '"' {
				if clean[i] == '\\' {
					i++
				}
				i++
			}
			continue
		}
		if clean[i] == ',' {
			next := i + 1
			for next < len(clean) && (clean[next] == ' ' || clean[next] == '\t' || clean[next] == '\r' || clean[next] == '\n') {
				next++
			}
			if next < len(clean) && (clean[next] == '}' || clean[next] == ']') {
				clean[i] = ' '
			}
		}
	}
	return clean, nil
}
