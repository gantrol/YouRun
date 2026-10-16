package discovery

import (
	"bufio"
	"bytes"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Python metadata describes declared dependencies, not an evaluated environment.
// Entry points are Python object references and are never turned into commands.
func (s *inspector) pythonMetadata(path string, data []byte, rule Rule) {
	if s.pythonValues == nil {
		s.pythonValues = map[string]map[string]any{}
	}
	values, cached := s.pythonValues[path]
	if !cached {
		switch strings.ToLower(filepath.Base(path)) {
		case "pyproject.toml":
			values = s.pythonProjectValues(path, data)
		case "requirements.txt":
			values = s.pythonRequirementsValues(path, data)
		default:
			return
		}
		s.pythonValues[path] = values
	}
	if values == nil || !rule.MatchesDependencies(values) {
		return
	}
	unit := s.unit(path, rule)
	s.applyMetadata(rule, path, unit, values, "")
}

func (s *inspector) pythonProjectValues(path string, data []byte) map[string]any {
	var document struct {
		Project struct {
			Name         string
			Dependencies []string
			Dynamic      []string
		}
		Tool struct {
			Poetry struct {
				Name         string
				Dependencies map[string]any
			}
		}
	}
	if err := toml.Unmarshal(data, &document); err != nil {
		s.issue(path, "scanInvalidManifest")
		return nil
	}
	name := document.Project.Name
	if name == "" {
		name = document.Tool.Poetry.Name
	}
	dependencies := map[string]any{}
	for i, value := range document.Project.Dependencies {
		if i >= 1024 {
			s.issue(path, "scanLimit")
			break
		}
		s.pythonDependency(path, value, dependencies)
	}
	for _, dynamic := range document.Project.Dynamic {
		if dynamic == "dependencies" || dynamic == "name" {
			s.issue(path, "scanPartial")
		}
	}
	// Only the ordinary Poetry dependency table is included. Development groups
	// and extras are separate declarations and are not inferred transitively.
	names := make([]string, 0, len(document.Tool.Poetry.Dependencies))
	for name := range document.Tool.Poetry.Dependencies {
		names = append(names, name)
	}
	sort.Strings(names)
	for i, name := range names {
		if i >= 1024 {
			s.issue(path, "scanLimit")
			break
		}
		normalized := normalizePythonName(name)
		if normalized == "python" {
			continue
		}
		if !pythonName.MatchString(name) {
			s.issue(path, "scanPartial")
			continue
		}
		switch value := document.Tool.Poetry.Dependencies[name].(type) {
		case string:
			if strings.TrimSpace(value) != "" {
				dependencies[normalized] = value
			}
		case map[string]any:
			if len(value) == 0 {
				s.issue(path, "scanPartial")
				continue
			}
			// The package name remains statically declared even for path/URL
			// sources. No source is opened and no dependency is installed.
			dependencies[normalized] = "declared"
			if value["markers"] != nil || value["python"] != nil || value["platform"] != nil || value["optional"] == true {
				s.issue(path, "scanPartial")
			}
		default:
			s.issue(path, "scanPartial")
		}
	}
	return map[string]any{"name": name, "dependencies": dependencies}
}

func (s *inspector) pythonRequirementsValues(path string, data []byte) map[string]any {
	dependencies := map[string]any{}
	scanner := bufio.NewScanner(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))))
	scanner.Buffer(make([]byte, 4096), 64<<10)
	continuation := false
	lines := 0
	for scanner.Scan() {
		lines++
		if lines > 4096 || len(dependencies) >= 1024 {
			s.issue(path, "scanLimit")
			break
		}
		line := strings.TrimSpace(scanner.Text())
		continued := strings.HasSuffix(line, "\\")
		if continuation || continued {
			s.issue(path, "scanPartial")
			continuation = continued
			continue
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		for i := 1; i < len(line); i++ {
			if line[i] == '#' && (line[i-1] == ' ' || line[i-1] == '\t') {
				line = strings.TrimSpace(line[:i])
				break
			}
		}
		// This rejects -r/-c/-e, pip flags, local paths, URLs without a
		// declared name, and environment expansion. It never follows includes.
		s.pythonDependency(path, line, dependencies)
	}
	if scanner.Err() != nil {
		s.issue(path, "scanLimit")
	}
	return map[string]any{"dependencies": dependencies}
}

var pythonName = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$`)
var pythonNameSeparators = regexp.MustCompile(`[-_.]+`)
var pythonRequirementHead = regexp.MustCompile(`^([A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?)(?:\[[A-Za-z0-9._-]+(?:\s*,\s*[A-Za-z0-9._-]+)*\])?\s*(.*)$`)
var pythonVersionSpecifier = regexp.MustCompile(`^(===|==|!=|<=|>=|~=|<|>)\s*[A-Za-z0-9][A-Za-z0-9.*+!_-]*$`)

func normalizePythonName(name string) string {
	return pythonNameSeparators.ReplaceAllString(strings.ToLower(name), "-")
}

func (s *inspector) pythonDependency(path, declaration string, dependencies map[string]any) {
	declaration = strings.TrimSpace(declaration)
	if len(declaration) == 0 || len(declaration) > 8000 || strings.ContainsAny(declaration, "$`\x00\r\n\\") {
		s.issue(path, "scanPartial")
		return
	}
	match := pythonRequirementHead.FindStringSubmatch(declaration)
	if match == nil {
		s.issue(path, "scanPartial")
		return
	}
	name, suffix := match[1], strings.TrimSpace(match[2])
	if strings.HasSuffix(name, ".whl") || strings.HasSuffix(name, ".zip") || strings.HasSuffix(name, ".tar.gz") {
		s.issue(path, "scanPartial")
		return
	}
	if requirement, marker, ok := strings.Cut(suffix, ";"); ok {
		if strings.TrimSpace(marker) == "" {
			s.issue(path, "scanPartial")
			return
		}
		suffix = strings.TrimSpace(requirement)
		s.issue(path, "scanPartial")
	}
	if strings.HasPrefix(suffix, "@") {
		if strings.TrimSpace(strings.TrimPrefix(suffix, "@")) == "" {
			s.issue(path, "scanPartial")
			return
		}
	} else if suffix != "" {
		if strings.HasPrefix(suffix, "(") && strings.HasSuffix(suffix, ")") {
			suffix = strings.TrimSpace(suffix[1 : len(suffix)-1])
		}
		for _, specifier := range strings.Split(suffix, ",") {
			if !pythonVersionSpecifier.MatchString(strings.TrimSpace(specifier)) {
				s.issue(path, "scanPartial")
				return
			}
		}
	}
	dependencies[normalizePythonName(name)] = declaration
}
