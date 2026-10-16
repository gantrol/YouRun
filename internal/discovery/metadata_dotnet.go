package discovery

import (
	"bytes"
	"encoding/xml"
	"io"
	"path/filepath"
	"strings"
)

func (s *inspector) dotnetMetadata(path string, data []byte, rule Rule) {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".sln" || ext == ".slnx" {
		return
	}
	fields := map[string]bool{"Sdk": true, "OutputType": true, "IsTestProject": true}
	for property := range rule.Match.PropertyEquals {
		fields[property] = true
	}
	for _, name := range rule.Emit.Names {
		if len(name.Field) == 1 {
			fields[name.Field[0]] = true
		}
	}
	for _, icon := range rule.Emit.Icons {
		if len(icon.Field) == 1 {
			fields[icon.Field[0]] = true
		}
	}
	values := map[string]any{}
	// MSBuild searches upward for the nearest Directory.Build.props. Literal
	// inherited paths remain relative to the project, as in the actual build.
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		props := filepath.Join(dir, "Directory.Build.props")
		if raw := s.read(props); raw != nil {
			s.dotnetProperties(props, raw, fields, values)
			break
		}
		if dir == "." {
			break
		}
	}
	if !s.dotnetProperties(path, data, fields, values) {
		return
	}
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		targets := filepath.Join(dir, "Directory.Build.targets")
		if raw := s.read(targets); raw != nil {
			s.dotnetProperties(targets, raw, fields, values)
			break
		}
		if dir == "." {
			break
		}
	}
	if !rule.MatchesDependencies(values) {
		return
	}
	for property, expected := range rule.Match.PropertyEquals {
		if !strings.EqualFold(fieldString(values, []string{property}), expected) {
			return
		}
	}
	u := s.unit(path, rule)
	output := fieldString(values, []string{"OutputType"})
	web := strings.EqualFold(fieldString(values, []string{"Sdk"}), "Microsoft.NET.Sdk.Web")
	s.noIcons[u.ID] = (!strings.EqualFold(output, "Exe") && !strings.EqualFold(output, "WinExe") && !(output == "" && web)) || strings.EqualFold(fieldString(values, []string{"IsTestProject"}), "true")
	if fieldString(values, []string{"Sdk"}) != "" {
		u.Technologies = appendUnique(u.Technologies, "dotnet")
	}
	s.applyMetadata(rule, path, u, values, "")
}

func (s *inspector) dotnetProperties(path string, data []byte, fields map[string]bool, values map[string]any) bool {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	type element struct {
		name        string
		conditional bool
	}
	stack := []element{}
	nodes := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return len(stack) == 0
		}
		if err != nil {
			s.issue(path, "scanInvalidManifest")
			return false
		}
		switch item := token.(type) {
		case xml.Directive:
			s.issue(path, "scanPartial")
			return false
		case xml.StartElement:
			nodes++
			if nodes > 16384 || len(stack) > 64 {
				s.issue(path, "scanLimit")
				return false
			}
			conditional := len(stack) > 0 && stack[len(stack)-1].conditional
			attributes := map[string]string{}
			for _, attr := range item.Attr {
				attributes[attr.Name.Local] = strings.TrimSpace(attr.Value)
				if strings.EqualFold(attr.Name.Local, "Condition") {
					conditional = true
				}
			}
			if item.Name.Local == "Project" && attributes["Sdk"] != "" {
				if conditional || dynamicMSBuildValue(attributes["Sdk"]) {
					delete(values, "Sdk")
					s.issue(path, "scanPartial")
				} else {
					values["Sdk"] = attributes["Sdk"]
				}
			}
			if item.Name.Local == "Import" {
				s.issue(path, "scanPartial")
			}
			parent := ""
			if len(stack) > 0 {
				parent = stack[len(stack)-1].name
			}
			if parent == "ItemGroup" && item.Name.Local == "PackageReference" {
				s.dotnetPackageReference(path, attributes, conditional, values)
			}
			if parent == "ItemGroup" && item.Name.Local == "MauiIcon" && fields["MauiIcon"] {
				var body struct {
					Inner string `xml:",innerxml"`
				}
				if decoder.DecodeElement(&body, &item) != nil {
					s.issue(path, "scanInvalidManifest")
					return false
				}
				// Child metadata can compose or transform this icon too. It is
				// outside the literal Include subset and must not be ignored.
				s.dotnetMauiIcon(path, attributes, conditional || strings.TrimSpace(body.Inner) != "", values)
				continue
			}
			if fields[item.Name.Local] && parent == "PropertyGroup" {
				var value string
				if decoder.DecodeElement(&value, &item) != nil {
					s.issue(path, "scanInvalidManifest")
					return false
				}
				value = strings.TrimSpace(value)
				if conditional || dynamicMSBuildValue(value) {
					delete(values, item.Name.Local)
					s.issue(path, "scanPartial")
					continue
				}
				values[item.Name.Local] = value
			} else {
				stack = append(stack, element{item.Name.Local, conditional})
			}
		case xml.EndElement:
			if len(stack) == 0 {
				s.issue(path, "scanInvalidManifest")
				return false
			}
			stack = stack[:len(stack)-1]
		}
	}
}

func dynamicMSBuildValue(value string) bool {
	return strings.Contains(value, "$(") || strings.Contains(value, "@(") || strings.Contains(value, "%(")
}

func (s *inspector) dotnetPackageReference(path string, attributes map[string]string, conditional bool, values map[string]any) {
	name := attributes["Include"]
	remove := false
	if name == "" {
		name, remove = attributes["Remove"], true
	}
	if name == "" {
		// Update changes metadata of an existing package; it cannot introduce
		// a dependency that was never included in this static project graph.
		return
	}
	if conditional || dynamicMSBuildValue(name) || strings.ContainsAny(name, "*?;") {
		s.issue(path, "scanPartial")
		return
	}
	dependencies, _ := values["dependencies"].(map[string]any)
	if dependencies == nil {
		dependencies = map[string]any{}
		values["dependencies"] = dependencies
	}
	if len(dependencies) >= 256 {
		s.issue(path, "scanLimit")
		return
	}
	name = strings.ToLower(name)
	if remove {
		delete(dependencies, name)
	} else {
		version := attributes["Version"]
		if version == "" {
			// NuGet central package management keeps versions in a separate
			// file. Include still proves that this project has the dependency.
			version = "*"
		}
		dependencies[name] = version
	}
}

func (s *inspector) dotnetMauiIcon(path string, attributes map[string]string, conditional bool, values map[string]any) {
	if _, seen := values["_mauiIconSeen"]; seen {
		return
	}
	values["_mauiIconSeen"] = true
	include := attributes["Include"]
	if conditional || include == "" || dynamicMSBuildValue(include) || strings.ContainsAny(include, "*?;") {
		s.issue(path, "scanPartial")
		return
	}
	// The build pipeline composes these layers and applies colors. Returning
	// just the background would misrepresent the generated application icon.
	for _, attribute := range []string{"ForegroundFile", "Color", "TintColor", "ForegroundScale"} {
		if attributes[attribute] != "" {
			s.issue(path, "scanPartial")
			return
		}
	}
	values["MauiIcon"] = include
}
