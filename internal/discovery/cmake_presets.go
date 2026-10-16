package discovery

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

var errCMakeInvalid = errors.New("scanInvalidManifest")
var errCMakePartial = errors.New("scanPartial")
var errCMakeLimit = errors.New("scanLimit")

var cmakeSections = []struct {
	field, tool string
	prefix      []string
	version     int
}{
	{"configurePresets", "cmake", nil, 1},
	{"buildPresets", "cmake", []string{"--build"}, 2},
	{"testPresets", "ctest", nil, 2},
	{"packagePresets", "cpack", nil, 6},
}

type cmakeFile struct {
	path      string
	version   int
	loading   bool
	reachable map[string]bool
}

type cmakeField struct {
	value json.RawMessage
	file  *cmakeFile
}

type cmakePreset struct {
	name          string
	hidden        bool
	file          *cmakeFile
	section       int
	parents       []string
	fields        map[string]cmakeField
	environment   map[string]cmakeField
	condition     *cmakeCondition
	conditionSet  bool
	conditionFile *cmakeFile
	state         int
	invalid       bool
	resolved      *cmakePreset
}

type cmakeGraph struct {
	s         *scan
	directory string
	files     map[string]*cmakeFile
	presets   [4]map[string]*cmakePreset
	order     [4][]*cmakePreset
	count     int
	partial   bool
}

func (s *scan) cmake(path string, _ []byte) error {
	dir := filepath.Dir(path)
	projectFile, userFile := filepath.Join(dir, "CMakePresets.json"), filepath.Join(dir, "CMakeUserPresets.json")
	if filepath.Base(path) == "CMakeLists.txt" {
		if s.entries[projectFile] || s.entries[userFile] {
			return nil
		}
		s.add(path, dir, "cmake", "template", "", true, "-S", ".", "-B", "build")
		s.add(path, dir, "cmake", "template", "", true, "--build", "build", "--config", "Release")
		s.issue(path, "scanPartial")
		return nil
	}
	if s.cmakeDone[dir] || (s.cmakeIncluded[path] && !s.entries[filepath.Join(dir, "CMakeLists.txt")]) {
		return nil
	}
	if s.cmakeDone == nil {
		s.cmakeDone = map[string]bool{}
		s.cmakeIncluded = map[string]bool{}
	}
	s.cmakeDone[dir] = true
	g := &cmakeGraph{s: s, directory: dir, files: map[string]*cmakeFile{}}
	for i := range g.presets {
		g.presets[i] = map[string]*cmakePreset{}
	}
	for _, file := range []string{projectFile, userFile} {
		if s.entries[file] {
			g.load(file, 0)
		}
	}
	// Resolve every inheritance chain before emitting, so cycles or duplicates
	// cannot leave earlier candidates looking fully resolved.
	for _, section := range g.order {
		for _, p := range section {
			g.resolve(p, 0)
		}
	}
	for section, presets := range g.order {
		for _, p := range presets {
			if s.ctx.Err() != nil {
				return s.ctx.Err()
			}
			if p.hidden || p.invalid || p.resolved == nil {
				continue
			}
			enabled, err := g.enabled(p.resolved)
			if err != nil {
				s.issue(p.file.path, err.Error())
			}
			if err == nil && !enabled {
				continue
			}
			if errors.Is(err, errCMakeInvalid) {
				continue
			}
			rule := cmakeSections[section]
			args := append(append([]string{}, rule.prefix...), "--preset", p.name)
			s.add(p.file.path, dir, rule.tool, "configuration", "", g.partial || err != nil, args...)
		}
	}
	return nil
}

func (g *cmakeGraph) issue(file string, err error) {
	g.partial = true
	g.s.issue(file, err.Error())
}

// Includes are explicit file references, so they may enter ignored directories.
// Every path component still passes the existing root and symlink boundaries;
// bytes, reads and cancellation remain charged to the scan's shared budget.
func (g *cmakeGraph) read(file string) []byte {
	if !filepath.IsLocal(file) || forbidden(file) {
		return nil
	}
	parts := strings.Split(filepath.Clean(file), string(filepath.Separator))
	if len(parts) > 64 {
		g.issue(file, errCMakeLimit)
		return nil
	}
	for i := range parts {
		if g.s.ctx.Err() != nil {
			return nil
		}
		info, err := g.s.root.Lstat(filepath.Join(parts[:i+1]...))
		if err != nil {
			g.s.issue(file, "scanReadFailed")
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			g.s.issue(file, "scanLinkSkipped")
			return nil
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			g.s.issue(file, "scanReadFailed")
			return nil
		}
	}
	g.s.entries[file] = true
	return g.s.read(file)
}

func (g *cmakeGraph) load(file string, depth int) *cmakeFile {
	file = filepath.Clean(file)
	if previous := g.files[file]; previous != nil {
		if previous.loading {
			g.issue(file, errCMakeInvalid)
			return nil
		}
		return previous
	}
	if depth > 16 || len(g.files) >= 32 {
		g.issue(file, errCMakeLimit)
		return nil
	}
	f := &cmakeFile{path: file, loading: true, reachable: map[string]bool{file: true}}
	g.files[file] = f
	defer func() { f.loading = false }()
	data := g.read(file)
	if data == nil {
		g.partial = true
		return f
	}
	var document map[string]json.RawMessage
	if json.Unmarshal(data, &document) != nil || document == nil || json.Unmarshal(document["version"], &f.version) != nil || f.version < 1 {
		g.issue(file, errCMakeInvalid)
		return f
	}
	if f.version > 12 {
		g.issue(file, errCMakePartial)
		return f
	}
	for section, spec := range cmakeSections {
		raw, exists := document[spec.field]
		if !exists {
			continue
		}
		var presets []json.RawMessage
		if f.version < spec.version || json.Unmarshal(raw, &presets) != nil || cmakeNull(raw) {
			g.issue(file, errCMakeInvalid)
			continue
		}
		for _, rawPreset := range presets {
			if g.count >= 512 {
				g.issue(file, errCMakeLimit)
				break
			}
			g.count++
			p, err := parseCMakePreset(rawPreset, f, section)
			if err != nil {
				g.issue(file, err)
				continue
			}
			if previous := g.presets[section][p.name]; previous != nil {
				previous.invalid = true
				g.issue(file, errCMakeInvalid)
				continue
			}
			g.presets[section][p.name] = p
			g.order[section] = append(g.order[section], p)
		}
	}
	// User presets implicitly include the project file, including in v1-v3.
	if file == filepath.Join(g.directory, "CMakeUserPresets.json") {
		project := filepath.Join(g.directory, "CMakePresets.json")
		if g.s.entries[project] {
			if included := g.load(project, depth+1); included != nil {
				maps.Copy(f.reachable, included.reachable)
			}
		}
	}
	if raw, exists := document["include"]; exists {
		if f.version < 4 {
			g.issue(file, errCMakeInvalid)
			return f
		}
		includes, err := cmakeStrings(raw, 32)
		if err != nil {
			g.issue(file, err)
			return f
		}
		for _, name := range includes {
			context := newCMakeContext(g, nil)
			expanded, err := context.expand(name, f, true, 0)
			if err != nil {
				g.issue(file, err)
				continue
			}
			target, ok := g.includePath(file, expanded)
			if !ok {
				g.issue(file, errCMakePartial)
				continue
			}
			g.s.cmakeIncluded[target] = true
			if included := g.load(target, depth+1); included != nil {
				maps.Copy(f.reachable, included.reachable)
			}
		}
	}
	return f
}

func (g *cmakeGraph) includePath(file, value string) (string, bool) {
	if value == "" || strings.ContainsAny(value, "\x00\r\n") {
		return "", false
	}
	value = filepath.FromSlash(strings.ReplaceAll(value, "\\", "/"))
	if filepath.IsAbs(value) {
		var err error
		value, err = filepath.Rel(g.s.result.Directory, value)
		if err != nil {
			return "", false
		}
	} else {
		value = filepath.Join(filepath.Dir(file), value)
	}
	value = filepath.Clean(value)
	return value, filepath.IsLocal(value) && !forbidden(value)
}

func cmakeNull(raw json.RawMessage) bool { return strings.TrimSpace(string(raw)) == "null" }

// encoding/json accepts null as a zero-valued string; preset string arrays do not.
func cmakeStrings(raw json.RawMessage, limit int) ([]string, error) {
	var values []json.RawMessage
	if cmakeNull(raw) || json.Unmarshal(raw, &values) != nil {
		return nil, errCMakeInvalid
	}
	if len(values) > limit {
		return nil, errCMakeLimit
	}
	result := make([]string, len(values))
	for i, value := range values {
		if cmakeNull(value) || json.Unmarshal(value, &result[i]) != nil {
			return nil, errCMakeInvalid
		}
	}
	return result, nil
}

func parseCMakePreset(raw json.RawMessage, file *cmakeFile, section int) (*cmakePreset, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, errCMakeInvalid
	}
	p := &cmakePreset{file: file, section: section, fields: map[string]cmakeField{}, environment: map[string]cmakeField{}}
	if json.Unmarshal(fields["name"], &p.name) != nil || p.name == "" || len(p.name) > 1024 || strings.ContainsAny(p.name, "\x00\r\n") {
		return nil, errCMakeInvalid
	}
	if value, ok := fields["hidden"]; ok {
		if cmakeNull(value) || json.Unmarshal(value, &p.hidden) != nil {
			return nil, errCMakeInvalid
		}
	}
	if value, ok := fields["inherits"]; ok {
		var parent string
		if json.Unmarshal(value, &parent) == nil && !cmakeNull(value) {
			p.parents = []string{parent}
		} else {
			var err error
			p.parents, err = cmakeStrings(value, 64)
			if err != nil {
				return nil, err
			}
		}
		for _, name := range p.parents {
			if name == "" {
				return nil, errCMakeInvalid
			}
		}
	}
	if value, ok := fields["condition"]; ok {
		if file.version < 3 {
			return nil, errCMakeInvalid
		}
		p.conditionSet, p.conditionFile = true, file
		if !cmakeNull(value) {
			var err error
			nodes := 0
			p.condition, err = parseCMakeCondition(value, 0, &nodes)
			if err != nil {
				return nil, err
			}
		}
	}
	if value, ok := fields["environment"]; ok {
		var environment map[string]json.RawMessage
		if cmakeNull(value) || json.Unmarshal(value, &environment) != nil {
			return nil, errCMakeInvalid
		}
		if len(environment) > 256 {
			return nil, errCMakeLimit
		}
		for name, rawValue := range environment {
			var text string
			if name == "" || (!cmakeNull(rawValue) && json.Unmarshal(rawValue, &text) != nil) {
				return nil, errCMakeInvalid
			}
			p.environment[name] = cmakeField{rawValue, file}
		}
	}
	for _, name := range []string{"generator", "binaryDir", "configurePreset"} {
		if value, ok := fields[name]; ok {
			var text string
			if cmakeNull(value) || json.Unmarshal(value, &text) != nil {
				return nil, errCMakeInvalid
			}
			p.fields[name] = cmakeField{value, file}
		}
	}
	if value, ok := fields["inheritConfigureEnvironment"]; ok {
		var enabled bool
		if cmakeNull(value) || json.Unmarshal(value, &enabled) != nil {
			return nil, errCMakeInvalid
		}
		p.fields["inheritConfigureEnvironment"] = cmakeField{value, file}
	}
	return p, nil
}

func (p *cmakePreset) text(name string) string {
	var value string
	_ = json.Unmarshal(p.fields[name].value, &value)
	return value
}

func (g *cmakeGraph) resolve(p *cmakePreset, depth int) *cmakePreset {
	if p.invalid {
		return nil
	}
	if p.state == 2 {
		return p.resolved
	}
	if p.state == 1 || depth > 32 {
		p.invalid = true
		err := errCMakeInvalid
		if depth > 32 {
			err = errCMakeLimit
		}
		g.issue(p.file.path, err)
		return nil
	}
	p.state = 1
	result := *p
	result.fields, result.environment = maps.Clone(p.fields), maps.Clone(p.environment)
	for _, name := range p.parents {
		parent := g.presets[p.section][name]
		if parent == nil || !p.file.reachable[parent.file.path] {
			p.invalid = true
			g.issue(p.file.path, errCMakeInvalid)
			break
		}
		inherited := g.resolve(parent, depth+1)
		if inherited == nil {
			p.invalid = true
			break
		}
		for key, value := range inherited.fields {
			if _, ok := result.fields[key]; !ok || (key != "inheritConfigureEnvironment" && result.text(key) == "") {
				result.fields[key] = value
			}
		}
		for key, value := range inherited.environment {
			if _, ok := result.environment[key]; !ok {
				result.environment[key] = value
			}
		}
		if !p.conditionSet && result.condition == nil {
			result.condition, result.conditionFile = inherited.condition, inherited.conditionFile
		}
	}
	p.state = 2
	if !p.invalid {
		if !p.hidden && ((p.section > 0 && result.text("configurePreset") == "") || (p.section == 0 && p.file.version < 3 && (result.text("generator") == "" || result.text("binaryDir") == ""))) {
			p.invalid = true
			g.issue(p.file.path, errCMakeInvalid)
		} else {
			p.resolved = &result
		}
	}
	return p.resolved
}

func (g *cmakeGraph) enabled(p *cmakePreset) (bool, error) {
	context := newCMakeContext(g, p)
	if p.section > 0 {
		configuration := g.presets[0][p.text("configurePreset")]
		if configuration == nil || !p.file.reachable[configuration.file.path] || configuration.resolved == nil {
			return false, errCMakeInvalid
		}
		if configuration.hidden {
			return false, nil
		}
		enabled, err := g.enabled(configuration.resolved)
		if err != nil || !enabled {
			return enabled, err
		}
		context.generator = configuration.resolved.text("generator")
		inherit := true
		if field, ok := p.fields["inheritConfigureEnvironment"]; ok {
			_ = json.Unmarshal(field.value, &inherit)
		}
		if inherit {
			for key, value := range configuration.resolved.environment {
				if _, ok := context.environment[key]; !ok {
					context.environment[key] = value
				}
			}
		}
	}
	// Validate environment cycles without leaking expanded values into candidates.
	for _, key := range slices.Sorted(maps.Keys(context.environment)) {
		value := context.environment[key]
		if !cmakeNull(value.value) {
			if _, err := context.env(key, 0); err != nil {
				return false, err
			}
		}
	}
	return context.condition(p.condition, p.conditionFile, 0)
}
