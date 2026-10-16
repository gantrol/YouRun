package discovery

import (
	"bytes"
	"encoding/xml"
	"io"
	"path/filepath"
	"strings"
)

type xmlProperty struct {
	name, value string
	conditional bool
}

func properties(data []byte) ([]xmlProperty, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	props := []xmlProperty{}
	stack := []bool{}
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return props, nil
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			conditional := len(stack) > 0 && stack[len(stack)-1]
			for _, a := range t.Attr {
				if strings.EqualFold(a.Name.Local, "Condition") {
					conditional = true
				}
				if t.Name.Local == "Project" && a.Name.Local == "Sdk" {
					props = append(props, xmlProperty{"Sdk", a.Value, conditional})
				}
				if t.Name.Local == "Sdk" && a.Name.Local == "Name" {
					props = append(props, xmlProperty{"Sdk", a.Value, conditional})
				}
				if t.Name.Local == "ProjectConfiguration" && a.Name.Local == "Include" {
					props = append(props, xmlProperty{"ProjectConfiguration", a.Value, conditional})
				}
			}
			stack = append(stack, conditional)
			switch t.Name.Local {
			case "OutputType", "TargetFramework", "TargetFrameworks", "UseWPF", "UseWindowsForms", "UseWinUI", "IsTestProject":
				var value string
				if err := d.DecodeElement(&value, &t); err != nil {
					return nil, err
				}
				stack = stack[:len(stack)-1]
				props = append(props, xmlProperty{t.Name.Local, strings.TrimSpace(value), conditional})
			}
		case xml.EndElement:
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
}
func (s *scan) msbuild(path string, data []byte, rule Rule) error {
	dir := filepath.Dir(path)
	file := filepath.Base(path)
	variables := map[string]string{"file": file}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".sln" || ext == ".slnx" {
		// Solution-wide build uses MSBuild to also cover native/mixed solutions.
		if ext == ".slnx" {
			if _, err := properties(data); err != nil {
				return err
			}
		}
		s.addRuleCommand(rule, "solution-build", path, dir, variables, "", false)
		return nil
	}
	props, err := properties(data)
	if err != nil {
		return err
	}
	if ext == ".vcxproj" {
		found := false
		for _, p := range props {
			if p.name != "ProjectConfiguration" {
				continue
			}
			configuration, platform, ok := strings.Cut(p.value, "|")
			if !ok || configuration == "" || platform == "" {
				continue
			}
			found = true
			variables["configuration"], variables["platform"] = configuration, platform
			s.addRuleCommand(rule, "native-build", path, dir, variables, "", p.conditional)
		}
		if !found {
			s.addRuleCommand(rule, "native-build-default", path, dir, variables, "", false)
		}
		return nil
	}
	values := map[string]string{}
	conditional := false
	for _, p := range props {
		if p.conditional || strings.Contains(p.value, "$(") {
			conditional = true
			continue
		}
		values[p.name] = p.value
	}
	sdk := values["Sdk"]
	if sdk == "" {
		s.addRuleCommand(rule, "legacy-build", path, dir, variables, "", false)
		s.issue(path, "scanPartial")
		return nil
	}
	frameworks := strings.Split(values["TargetFrameworks"], ";")
	if values["TargetFrameworks"] == "" {
		frameworks = []string{values["TargetFramework"]}
	}
	app := strings.EqualFold(values["OutputType"], "Exe") || strings.EqualFold(values["OutputType"], "WinExe") || strings.Contains(sdk, "Microsoft.NET.Sdk.Web")
	isTest := strings.EqualFold(values["IsTestProject"], "true") || bytes.Contains(data, []byte("Microsoft.NET.Test.Sdk"))
	for _, framework := range frameworks {
		framework = strings.TrimSpace(framework)
		platform := ""
		if strings.HasPrefix(framework, "net4") || strings.Contains(strings.ToLower(framework), "-windows") || strings.EqualFold(values["UseWPF"], "true") || strings.EqualFold(values["UseWindowsForms"], "true") || strings.EqualFold(values["UseWinUI"], "true") {
			platform = "Windows"
		}
		// Mobile targets need a device/runtime; do not turn them into desktop launches.
		mobile := strings.Contains(framework, "-android") || strings.Contains(framework, "-ios") || strings.Contains(framework, "-tvos") || strings.Contains(framework, "-maccatalyst")
		if strings.Contains(framework, "-macos") || strings.Contains(framework, "-maccatalyst") || strings.Contains(framework, "-ios") || strings.Contains(framework, "-tvos") {
			platform = "macOS"
		}
		args := []string{}
		if framework != "" {
			args = []string{"--framework", framework}
		}
		review := conditional || framework == "" || mobile
		s.addRuleCommand(rule, "build", path, dir, variables, platform, review, args...)
		if isTest {
			s.addRuleCommand(rule, "test", path, dir, variables, platform, review, args...)
		}
		if app && !isTest && !mobile {
			s.addRuleCommand(rule, "run", path, dir, variables, platform, review, args...)
			s.addRuleCommand(rule, "publish", path, dir, variables, platform, false, args...)
		}
	}
	if conditional {
		s.issue(path, "scanPartial")
	}
	return nil
}
