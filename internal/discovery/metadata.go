package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

type InspectOptions struct {
	Name        string
	Platform    string
	Sources     []string
	Directories []string
	Registry    *Registry
	SkipIcons   bool
}

type Inspection struct {
	Root         string          `json:"root"`
	Units        []ProjectUnit   `json:"units"`
	Icons        []IconCandidate `json:"icons"`
	Names        []NameCandidate `json:"names"`
	Issues       []Issue         `json:"issues"`
	Dependencies []string        `json:"dependencies"`
	RuleDigest   string          `json:"ruleDigest"`
}

type NameCandidate struct {
	Name     string `json:"name"`
	Source   string `json:"source"`
	priority int
}

type ProjectUnit struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Directory    string   `json:"directory"`
	Technologies []string `json:"technologies"`
	Sources      []string `json:"sources"`
}

type IconCandidate struct {
	ID       string   `json:"id"`
	UnitID   string   `json:"unitId"`
	Name     string   `json:"name"`
	Source   string   `json:"source"`
	Data     string   `json:"data"`
	RuleID   string   `json:"ruleId"`
	Priority int      `json:"priority"`
	Evidence []string `json:"evidence"`
}

type inspectTarget struct {
	path       string
	depth      int
	referenced bool
}
type iconProposal struct {
	path, unit, rule string
	priority         int
	evidence         []string
	declared         bool
	platform         bool
	directory        bool
	recursive        bool
	extensions       []string
}
type inspector struct {
	ctx                                        context.Context
	root                                       *os.Root
	registry                                   *Registry
	options                                    InspectOptions
	result                                     Inspection
	cache                                      map[string][]byte
	directoryEntries                           map[string][]os.DirEntry
	dependencies                               map[string]bool
	visited                                    map[string]bool
	processed                                  map[string]bool
	units                                      map[string]*ProjectUnit
	proposals                                  []iconProposal
	queue                                      []inspectTarget
	references                                 map[string][]string
	namePriority                               map[string]int
	noIcons                                    map[string]bool
	terminalDirs                               map[string]bool
	configUnits                                map[string]string
	explicitConfigs                            map[string]bool
	defaultConfigs                             map[string]string
	packageValues                              map[string]map[string]any
	pythonValues                               map[string]map[string]any
	scoped                                     bool
	reads, bytes, probes, directories, entries int
	scope                                      directoryScope
}

// Inspect only reads bounded local configuration and image data. It never runs
// project scripts, evaluates dynamic configuration, or imports project rules.
func Inspect(ctx context.Context, directory string, options InspectOptions) (Inspection, error) {
	if !filepath.IsAbs(directory) || forbidden(directory) {
		return Inspection{}, errors.New("invalidDirectory")
	}
	directory = filepath.Clean(directory)
	if info, err := os.Lstat(directory); err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Inspection{}, errors.New("invalidDirectory")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return Inspection{}, errors.New("invalidDirectory")
	}
	defer root.Close()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if options.Registry == nil {
		options.Registry = DefaultRegistry()
	}
	if options.Platform == "" {
		options.Platform = runtime.GOOS
	}
	if options.Platform == "win32" {
		options.Platform = "windows"
	}
	s := &inspector{ctx: ctx, root: root, registry: options.Registry, options: options,
		result: Inspection{Root: directory, Units: []ProjectUnit{}, Icons: []IconCandidate{}, Issues: []Issue{}, Dependencies: []string{}, RuleDigest: options.Registry.Digest()},
		cache:  map[string][]byte{}, dependencies: map[string]bool{}, visited: map[string]bool{}, processed: map[string]bool{}, units: map[string]*ProjectUnit{}, references: map[string][]string{}, namePriority: map[string]int{}, noIcons: map[string]bool{}, terminalDirs: map[string]bool{}, configUnits: map[string]string{}, explicitConfigs: map[string]bool{}, defaultConfigs: map[string]string{}}
	for _, issue := range s.registry.Diagnostics() {
		s.issue(issue.Path, issue.Code)
	}
	s.enqueue(".", 0, false)
	for _, source := range options.Sources {
		s.hint(source, true)
	}
	for _, dir := range options.Directories {
		s.hint(dir, false)
	}
	for len(s.queue) > 0 && ctx.Err() == nil {
		target := s.queue[0]
		s.queue = s.queue[1:]
		s.walk(target)
	}
	s.finish()
	if err := ctx.Err(); err != nil {
		return s.result, err
	}
	return s.result, nil
}

func (s *inspector) hint(value string, source bool) {
	if filepath.IsAbs(value) {
		rel, err := filepath.Rel(s.result.Root, value)
		if err != nil || !filepath.IsLocal(rel) {
			if rootReal, err1 := filepath.EvalSymlinks(s.result.Root); err1 == nil {
				if valReal, err2 := filepath.EvalSymlinks(value); err2 == nil {
					if rel2, err3 := filepath.Rel(rootReal, valReal); err3 == nil {
						rel = rel2
						err = nil
					}
				}
			}
		}
		if err != nil {
			return
		}
		value = rel
	}
	value = filepath.Clean(value)
	if !filepath.IsLocal(value) || forbidden(value) {
		return
	}
	s.references[value] = appendUnique(s.references[value], "saved-command")
	if source {
		s.process(value)
	}
	s.enqueue(filepath.Dir(value), 0, true)
	if !source {
		s.enqueue(value, 0, true)
	}
}

func (s *inspector) enqueue(path string, depth int, referenced bool) {
	path = filepath.Clean(path)
	if !filepath.IsLocal(path) || forbidden(path) {
		return
	}
	if !referenced && s.scope.excluded(path) {
		return
	}
	if depth > maxDepth {
		s.issue(path, "scanDepthLimit")
		return
	}
	if len(s.queue) >= 512 {
		s.issue(".", "scanDirectoryLimit")
		return
	}
	target := inspectTarget{path, depth, referenced}
	if referenced {
		s.queue = append([]inspectTarget{target}, s.queue...)
	} else {
		s.queue = append(s.queue, target)
	}
}

func (s *inspector) safeInfo(path string) os.FileInfo {
	path = filepath.Clean(path)
	if s.ctx.Err() != nil || !filepath.IsLocal(path) || forbidden(path) || s.probes >= 32768 {
		return nil
	}
	parts := strings.Split(path, string(filepath.Separator))
	var info os.FileInfo
	for i := range parts {
		s.probes++
		var err error
		info, err = s.root.Lstat(filepath.Join(parts[:i+1]...))
		if err != nil {
			if !os.IsNotExist(err) && (strings.Contains(err.Error(), "escapes") || strings.Contains(err.Error(), "symlink") || strings.Contains(err.Error(), "reparse")) {
				s.issue(path, "scanLinkSkipped")
			}
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			s.issue(path, "scanLinkSkipped")
			return nil
		}
	}
	return info
}

func (s *inspector) read(path string) []byte {
	path = filepath.Clean(path)
	if !filepath.IsLocal(path) || forbidden(path) {
		return nil
	}
	s.dependencies[path] = true
	if data, ok := s.cache[path]; ok {
		return data
	}
	s.cache[path] = nil
	if s.reads >= 512 || s.bytes >= maxTotalBytes {
		s.issue(path, "scanLimit")
		return nil
	}
	info := s.safeInfo(path)
	if info == nil || !info.Mode().IsRegular() {
		return nil
	}
	if info.Size() > 2<<20 || info.Size() > int64(maxTotalBytes-s.bytes) {
		s.issue(path, "scanLimit")
		return nil
	}
	file, err := s.root.Open(path)
	if err != nil {
		s.issue(path, "scanReadFailed")
		return nil
	}
	s.reads++
	data, err := io.ReadAll(io.LimitReader(file, int64(min(2<<20, maxTotalBytes-s.bytes))+1))
	file.Close()
	s.bytes += len(data)
	if err != nil || len(data) > 2<<20 || s.bytes > maxTotalBytes {
		s.issue(path, "scanLimit")
		return nil
	}
	s.cache[path] = data
	return data
}

func (s *inspector) readDirectory(path string) []os.DirEntry {
	if entries, ok := s.directoryEntries[path]; ok {
		return entries
	}
	if s.directoryEntries == nil {
		s.directoryEntries = map[string][]os.DirEntry{}
	}
	s.directoryEntries[path] = nil
	s.dependencies[path] = true
	if s.directories >= 256 || s.entries >= maxEntries {
		if s.directories >= 256 {
			s.issue(".", "scanDirectoryLimit")
		}
		if s.entries >= maxEntries {
			s.issue(".", "scanEntryLimit")
		}
		return nil
	}
	info := s.safeInfo(path)
	if info == nil || !info.IsDir() {
		return nil
	}
	s.directories++
	f, err := s.root.Open(path)
	if err != nil {
		return nil
	}
	entries, err := f.ReadDir(min(1024, maxEntries-s.entries) + 1)
	f.Close()
	if err != nil && err != io.EOF {
		s.issue(path, "scanReadFailed")
		return nil
	}
	if len(entries) > min(1024, maxEntries-s.entries) {
		s.issue(path, "scanLimit")
		entries = entries[:min(1024, maxEntries-s.entries)]
	}
	s.entries += len(entries)
	for _, entry := range entries {
		if entry.Name() == ".gitignore" && entry.Type().IsRegular() {
			s.scope.load(path, s.read(filepath.Join(path, entry.Name())), s.issue)
			break
		}
	}
	s.directoryEntries[path] = entries
	return entries
}

func (s *inspector) walk(target inspectTarget) {
	path := target.path
	if s.visited[path] {
		return
	}
	s.visited[path] = true
	entries := s.readDirectory(path)
	sort.Slice(entries, func(i, j int) bool {
		firstPackage, secondPackage := strings.EqualFold(entries[i].Name(), "package.json"), strings.EqualFold(entries[j].Name(), "package.json")
		if firstPackage != secondPackage {
			return firstPackage
		}
		return entries[i].Name() < entries[j].Name()
	})
	// Process manifests before descending: literal workspace/script references
	// reserve the budget for active applications instead of documentation copies.
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			s.issue(filepath.Join(path, entry.Name()), "scanLinkSkipped")
			continue
		}
		if entry.Type()&os.ModeSymlink == 0 && entry.Type().IsRegular() && !forbidden(entry.Name()) {
			s.process(filepath.Join(path, entry.Name()))
		}
	}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || ignored(entry.Name()) || strings.EqualFold(entry.Name(), "bin") {
			continue
		}
		next := filepath.Join(path, entry.Name())
		if (path == "." && s.scoped) || s.terminalDirs[path] {
			continue
		}
		s.enqueue(next, target.depth+1, false)
	}
}

func (s *inspector) process(path string) {
	path = filepath.Clean(path)
	if s.processed[path] || forbidden(path) {
		return
	}
	rules := s.registry.Match(filepath.ToSlash(path))
	metadataRules := rules[:0]
	for _, rule := range rules {
		if !configuredCommands(rule) {
			metadataRules = append(metadataRules, rule)
		}
	}
	rules = metadataRules
	if len(rules) == 0 {
		return
	}
	rank := func(rule Rule) int {
		switch rule.Resolver {
		case "node-v1":
			return 0
		case "electron-builder-static-v1":
			return 2
		case "conventional-assets-v1":
			return 3
		default:
			return 1
		}
	}
	sort.SliceStable(rules, func(i, j int) bool { return rank(rules[i]) < rank(rules[j]) })
	s.processed[path] = true
	data := s.read(path)
	if data == nil {
		return
	}
	for _, rule := range rules {
		switch rule.Resolver {
		case "node-v1":
			s.nodeMetadata(path, data, rule)
		case "package-metadata-v1":
			s.packageMetadata(path, data, rule)
		case "electron-builder-static-v1":
			s.electronMetadata(path, data, rule)
		case "msbuild-v1":
			s.dotnetMetadata(path, data, rule)
		case "web-manifest-v1":
			s.webManifestMetadata(path, data, rule)
		case "tauri-static-v1":
			s.tauriMetadata(path, data, rule)
		case "python-static-v1":
			s.pythonMetadata(path, data, rule)
		case "flutter-static-v1":
			s.flutterMetadata(path, data, rule)
		case "json-app-v1":
			s.jsonAppMetadata(path, data, rule)
		case "go-module-metadata-v1":
			s.goModuleMetadata(path, data, rule)
		case "conventional-assets-v1":
			// A conventional rule augments a recognized unit. It cannot turn an
			// arbitrary asset directory into an application of its own.
			unit := s.units[s.unitKey(path)]
			if unit == nil {
				unit = s.unit(path, rule)
			}
			s.applyMetadata(rule, path, unit, nil, "")
		}
	}
}

func (s *inspector) unitKey(path string) string {
	if key := s.configUnits[path]; key != "" {
		return key
	}
	if strings.HasSuffix(strings.ToLower(path), "proj") {
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(filepath.Dir(path))
}

func (s *inspector) unit(path string, rule Rule) *ProjectUnit {
	key := s.unitKey(path)
	u := s.units[key]
	if u == nil {
		name := filepath.Base(filepath.Dir(path))
		if strings.HasSuffix(strings.ToLower(path), "proj") {
			name = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		}
		if name == "." {
			name = filepath.Base(s.result.Root)
		}
		u = &ProjectUnit{ID: stableID("unit", key), Name: name, Directory: filepath.ToSlash(filepath.Dir(path)), Technologies: []string{}, Sources: []string{}}
		s.units[key] = u
	}
	u.Sources = appendUnique(u.Sources, filepath.ToSlash(path))
	for _, framework := range rule.Emit.Frameworks {
		u.Technologies = appendUnique(u.Technologies, framework)
	}
	return u
}

func (s *inspector) applyMetadata(rule Rule, path string, unit *ProjectUnit, values map[string]any, resources string) {
	namePriority := 10
	if rule.Resolver == "msbuild-v1" {
		namePriority = 20
	}
	if rule.Resolver == "tauri-static-v1" {
		namePriority = 40
	}
	if rule.Resolver == "json-app-v1" || rule.Resolver == "flutter-static-v1" {
		namePriority = 30
	}
	if rule.Resolver == "electron-builder-static-v1" {
		namePriority = 30
		if !strings.EqualFold(filepath.Base(path), "package.json") {
			namePriority = 40
		}
	}
	chosen := false
	for _, name := range rule.Emit.Names {
		if value := fieldString(values, name.Field); value != "" && len(value) <= 200 {
			s.addName(value, filepath.ToSlash(path)+"#"+strings.Join(name.Field, "."), namePriority)
			if !chosen && namePriority >= s.namePriority[unit.ID] {
				unit.Name = value
				s.namePriority[unit.ID] = namePriority
				chosen = true
			}
		}
	}
	for _, icon := range rule.Emit.Icons {
		if icon.Platform != "" && icon.Platform != s.options.Platform {
			continue
		}
		base := filepath.Dir(path)
		if icon.Base == "unit" {
			base = filepath.FromSlash(unit.Directory)
		}
		if icon.Base == "buildResourcesRoot" {
			base = resources
			if base == "" {
				continue
			}
		}
		paths := append([]string{}, icon.Paths...)
		if len(icon.Directories) > 0 {
			paths = icon.Directories
		}
		if value := fieldString(values, icon.Field); value != "" {
			paths = append([]string{value}, paths...)
		}
		for _, value := range paths {
			if len(icon.Directories) == 0 && len(icon.Extensions) > 0 && !containsString(icon.Extensions, strings.ToLower(filepath.Ext(value))) {
				continue
			}
			asset := localJoin(base, value)
			if asset == "" {
				if value != "" {
					s.issue(path, "scanPartial")
				}
				continue
			}
			s.proposals = append(s.proposals, iconProposal{path: asset, unit: unit.ID, rule: rule.PackID + "/" + rule.ID, priority: icon.Precedence, evidence: []string{filepath.ToSlash(path)}, declared: icon.Origin == "declared", platform: icon.Platform != "", directory: len(icon.Directories) > 0, recursive: icon.Recursive, extensions: icon.Extensions})
		}
	}
}

func (s *inspector) issue(path, code string) {
	issue := Issue{Path: filepath.ToSlash(path), Code: code}
	for _, old := range s.result.Issues {
		if old == issue {
			return
		}
	}
	if len(s.result.Issues) < 128 {
		s.result.Issues = append(s.result.Issues, issue)
	}
}

func (s *inspector) addName(name, source string, priority int) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return
	}
	for _, candidate := range s.result.Names {
		if candidate.Name == name && candidate.Source == source {
			return
		}
	}
	if len(s.result.Names) < 128 {
		s.result.Names = append(s.result.Names, NameCandidate{Name: name, Source: source, priority: priority})
	}
}

func (s *inspector) finish() {
	for _, unit := range s.units {
		s.result.Units = append(s.result.Units, *unit)
	}
	sort.Slice(s.result.Units, func(i, j int) bool { return s.result.Units[i].ID < s.result.Units[j].ID })
	for _, unit := range s.result.Units {
		s.addName(unit.Name, unit.Directory, 0)
	}
	sort.SliceStable(s.result.Names, func(i, j int) bool {
		a, b := s.result.Names[i], s.result.Names[j]
		if a.priority != b.priority {
			return a.priority > b.priority
		}
		return strings.Count(a.Source, "/") < strings.Count(b.Source, "/")
	})
	sort.SliceStable(s.proposals, func(i, j int) bool {
		if s.proposals[i].recursive != s.proposals[j].recursive {
			return !s.proposals[i].recursive
		}
		// Filename-independent directory searches never displace a valid
		// declared icon or an established conventional path.
		if s.proposals[i].directory != s.proposals[j].directory {
			return !s.proposals[i].directory
		}
		if s.proposals[i].declared != s.proposals[j].declared {
			return s.proposals[i].declared
		}
		if s.proposals[i].platform != s.proposals[j].platform {
			return s.proposals[i].platform
		}
		return s.proposals[i].priority > s.proposals[j].priority
	})
	selected := map[string]bool{}
	encoded := map[string]string{}
	for _, proposal := range s.proposals {
		if s.options.SkipIcons || s.ctx.Err() != nil {
			break
		}
		if selected[proposal.unit] || s.noIcons[proposal.unit] {
			continue
		}
		if s.explicitConfigs[proposal.unit] && strings.HasPrefix(proposal.rule, "electron-builder/") {
			explicit := false
			for _, source := range proposal.evidence {
				if s.configUnits[filepath.FromSlash(source)] != "" {
					explicit = true
				}
			}
			if !explicit {
				continue
			}
		}
		paths := []string{proposal.path}
		if proposal.recursive {
			paths = s.iconSearchPaths(proposal)
		} else if proposal.directory {
			paths = s.iconDirectoryPaths(proposal)
		}
		for _, path := range paths {
			if s.ctx.Err() != nil {
				break
			}
			data, ok := encoded[path]
			if !ok {
				if raw := s.read(path); raw != nil {
					data = encodeLogo(raw, strings.ToLower(filepath.Ext(path)))
					if data == "" {
						s.issue(path, "scanPartial")
					}
				}
				encoded[path] = data
			}
			if data == "" {
				continue
			}
			name := ""
			for _, unit := range s.result.Units {
				if unit.ID == proposal.unit {
					name = unit.Name
					break
				}
			}
			evidence := append([]string{}, proposal.evidence...)
			for _, source := range proposal.evidence {
				evidence = append(evidence, s.references[filepath.FromSlash(source)]...)
			}
			s.result.Icons = append(s.result.Icons, IconCandidate{ID: stableID("icon", proposal.unit, filepath.ToSlash(path)), UnitID: proposal.unit, Name: name, Source: filepath.ToSlash(path), Data: data, RuleID: proposal.rule, Priority: proposal.priority, Evidence: evidence})
			selected[proposal.unit] = true
			break
		}
	}
	// A configured project name can identify one application in a repository.
	// Never pick one of several equally named applications by traversal order.
	named := []IconCandidate{}
	for _, icon := range s.result.Icons {
		if s.options.Name != "" && strings.EqualFold(icon.Name, s.options.Name) {
			named = append(named, icon)
		}
	}
	if len(named) == 1 {
		s.result.Icons = named
	}
	sort.Slice(s.result.Icons, func(i, j int) bool { return s.result.Icons[i].Source < s.result.Icons[j].Source })
	for dependency := range s.dependencies {
		s.result.Dependencies = append(s.result.Dependencies, filepath.ToSlash(dependency))
	}
	sort.Strings(s.result.Dependencies)
}

func stableID(parts ...string) string {
	hash := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(hash[:16])
}
func appendUnique(values []string, value string) []string {
	for _, old := range values {
		if old == value {
			return values
		}
	}
	return append(values, value)
}
func localJoin(base, value string) string {
	if value == "" || strings.ContainsAny(value, "\x00:$%`\r\n") {
		return ""
	}
	value = filepath.FromSlash(strings.ReplaceAll(value, "\\", "/"))
	if filepath.IsAbs(value) {
		return ""
	}
	joined := filepath.Clean(filepath.Join(base, value))
	if !filepath.IsLocal(joined) || forbidden(joined) {
		return ""
	}
	return joined
}
func fieldString(values map[string]any, fields []string) string {
	if len(fields) == 0 {
		return ""
	}
	var current any = values
	for _, field := range fields {
		object, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = object[field]
	}
	value, _ := current.(string)
	return strings.TrimSpace(value)
}
