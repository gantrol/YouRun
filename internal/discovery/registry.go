package discovery

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

//go:embed rules/*.json rule.schema.json
var ruleFiles embed.FS

// RulePack is the versioned, data-only discovery extension boundary. Project
// repositories are never searched for these packs; extraDir is a host setting.
type RulePack struct {
	SchemaVersion int    `json:"schemaVersion"`
	ID            string `json:"id"`
	RuleVersion   int    `json:"ruleVersion"`
	Replaces      string `json:"replaces,omitempty"`
	Rules         []Rule `json:"rules"`
}

type Rule struct {
	ID       string                     `json:"id"`
	Match    RuleMatch                  `json:"match"`
	Resolver string                     `json:"resolve"`
	Emit     RuleEmit                   `json:"emit,omitempty"`
	Commands map[string]CommandTemplate `json:"commands,omitempty"`
	Options  RuleOptions                `json:"options,omitempty"`
	// The loader supplies provenance; packs cannot claim another pack's identity.
	PackID      string `json:"-"`
	RuleVersion int    `json:"-"`
}

type RuleMatch struct {
	Files           []string          `json:"files"`
	DependenciesAny []string          `json:"dependenciesAny,omitempty"`
	PropertyEquals  map[string]string `json:"propertyEquals,omitempty"`
	PluginsAny      []string          `json:"pluginsAny,omitempty"`
}

// MatchesDependencies evaluates the package condition after Match has selected
// a file by path. Package names are exact and case-sensitive, never globs.
func (r Rule) MatchesDependencies(values map[string]any) bool {
	if len(r.Match.DependenciesAny) == 0 {
		return true
	}
	for _, field := range []string{"dependencies", "devDependencies", "peerDependencies", "optionalDependencies"} {
		dependencies, _ := values[field].(map[string]any)
		for _, name := range r.Match.DependenciesAny {
			if version, ok := dependencies[name].(string); ok && strings.TrimSpace(version) != "" {
				return true
			}
		}
	}
	return false
}

type RuleEmit struct {
	Frameworks []string   `json:"frameworks,omitempty"`
	Names      []NameRule `json:"names,omitempty"`
	Icons      []IconRule `json:"icons,omitempty"`
}
type NameRule struct {
	Field  []string `json:"field"`
	Origin string   `json:"origin"`
}
type IconRule struct {
	Field       []string `json:"field,omitempty"`
	Paths       []string `json:"paths,omitempty"`
	Directories []string `json:"directories,omitempty"`
	Recursive   bool     `json:"recursive,omitempty"`
	Extensions  []string `json:"extensions,omitempty"`
	Base        string   `json:"base"`
	Platform    string   `json:"platform,omitempty"`
	Purpose     string   `json:"purpose"`
	Origin      string   `json:"origin"`
	Precedence  int      `json:"precedence,omitempty"`
}
type CommandTemplate struct {
	Tool     string   `json:"tool"`
	Args     []string `json:"args"`
	Origin   string   `json:"origin"`
	Platform string   `json:"platform,omitempty"`
	Review   bool     `json:"review,omitempty"`
}
type RuleOptions struct {
	PackageManagers       []string            `json:"packageManagers,omitempty"`
	Lockfiles             map[string]string   `json:"lockfiles,omitempty"`
	DefaultManager        string              `json:"defaultManager,omitempty"`
	ConfigFiles           []string            `json:"configFiles,omitempty"`
	BuildResourcesField   []string            `json:"buildResourcesField,omitempty"`
	DefaultBuildResources string              `json:"defaultBuildResources,omitempty"`
	WebRoots              []string            `json:"webRoots,omitempty"`
	PlatformConfigFiles   map[string][]string `json:"platformConfigFiles,omitempty"`
	ValueRoot             []string            `json:"valueRoot,omitempty"`
	DynamicConfigFiles    []string            `json:"dynamicConfigFiles,omitempty"`
	EntryFiles            []string            `json:"entryFiles,omitempty"`
	PlatformFiles         map[string][]string `json:"platformFiles,omitempty"`
}
type indexedRule struct {
	rule     Rule
	patterns []*regexp.Regexp
}
type Registry struct {
	rules       []indexedRule
	digest      string
	diagnostics []Issue
}

// DefaultRegistry fails loudly for an invalid built-in pack, which is a release
// error. Invalid optional user packs are isolated by LoadRegistry instead.
var builtinRegistry = sync.OnceValues(func() (*Registry, error) {
	return loadRegistry("")
})

func DefaultRegistry() *Registry {
	r, err := builtinRegistry()
	if err != nil {
		panic(err)
	}
	return r
}

func (r *Registry) Digest() string       { return r.digest }
func (r *Registry) Diagnostics() []Issue { return append([]Issue(nil), r.diagnostics...) }
func (r *Registry) Rules() []Rule {
	out := make([]Rule, 0, len(r.rules))
	for _, v := range r.rules {
		out = append(out, cloneRule(v.rule))
	}
	return out
}
func (r *Registry) Match(path string) []Rule {
	path = strings.ToLower(filepath.ToSlash(filepath.Clean(path)))
	var matches []Rule
	for _, v := range r.rules {
		for _, pattern := range v.patterns {
			if pattern.MatchString(path) {
				matches = append(matches, cloneRule(v.rule))
				break
			}
		}
	}
	return matches
}

func cloneRule(v Rule) Rule {
	// Rules are small and are copied only after a file actually matches. Callers
	// cannot mutate the registry midway through another discovery session.
	b, _ := json.Marshal(v)
	var copy Rule
	_ = json.Unmarshal(b, &copy)
	copy.PackID, copy.RuleVersion = v.PackID, v.RuleVersion
	return copy
}

// LoadRegistry loads embedded rules and optional, explicitly installed user
// rules. A failed replacement never removes the working embedded rule pack.
func LoadRegistry(extraDir string) (*Registry, error) {
	if extraDir == "" {
		return builtinRegistry()
	}
	if filepath.IsAbs(extraDir) && !forbidden(extraDir) {
		if _, err := os.Lstat(extraDir); os.IsNotExist(err) {
			return builtinRegistry()
		}
	}
	return loadRegistry(extraDir)
}

func loadRegistry(extraDir string) (*Registry, error) {
	packs := map[string]RulePack{}
	files, err := fs.Glob(ruleFiles, "rules/*.json")
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		data, err := ruleFiles.ReadFile(file)
		if err != nil {
			return nil, err
		}
		pack, err := decodeRulePack(data)
		if err != nil {
			return nil, fmt.Errorf("embedded discovery rule %s: %w", file, err)
		}
		if _, exists := packs[pack.ID]; exists {
			return nil, fmt.Errorf("duplicate embedded discovery pack %s", pack.ID)
		}
		packs[pack.ID] = pack
	}
	r := &Registry{}
	if extraDir != "" {
		if !filepath.IsAbs(extraDir) || forbidden(extraDir) {
			r.diagnostics = append(r.diagnostics, Issue{extraDir, "scanInvalidRule"})
		} else {
			r.loadExtra(extraDir, packs)
		}
	}
	ids := make([]string, 0, len(packs))
	for id := range packs {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	hash := sha256.New()
	for _, id := range ids {
		pack := packs[id]
		data, _ := json.Marshal(pack)
		hash.Write(data)
		hash.Write([]byte{0})
		for _, rule := range pack.Rules {
			rule.PackID, rule.RuleVersion = pack.ID, pack.RuleVersion
			entry := indexedRule{rule: rule}
			for _, glob := range rule.Match.Files {
				pattern, _ := compileRuleGlob(glob)
				entry.patterns = append(entry.patterns, pattern)
			}
			r.rules = append(r.rules, entry)
		}
	}
	r.digest = hex.EncodeToString(hash.Sum(nil))
	return r, nil
}

func (r *Registry) loadExtra(directory string, packs map[string]RulePack) {
	root, err := os.OpenRoot(directory)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		r.diagnostics = append(r.diagnostics, Issue{directory, "scanRuleReadFailed"})
		return
	}
	defer root.Close()
	f, err := root.Open(".")
	if err != nil {
		r.diagnostics = append(r.diagnostics, Issue{directory, "scanRuleReadFailed"})
		return
	}
	entries, err := f.ReadDir(257)
	f.Close()
	if err != nil && err != io.EOF {
		r.diagnostics = append(r.diagnostics, Issue{directory, "scanRuleReadFailed"})
		return
	}
	if len(entries) > 256 {
		r.diagnostics = append(r.diagnostics, Issue{directory, "scanLimit"})
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	count, total := 0, 0
	installed, replaced := map[string]bool{}, map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.EqualFold(filepath.Ext(name), ".json") || forbidden(name) {
			continue
		}
		path := filepath.Join(directory, name)
		count++
		if count > 32 || total >= 4<<20 {
			r.diagnostics = append(r.diagnostics, Issue{path, "scanLimit"})
			break
		}
		info, err := root.Lstat(name)
		if err != nil || !info.Mode().IsRegular() || info.Size() > 256<<10 {
			r.diagnostics = append(r.diagnostics, Issue{path, "scanInvalidRule"})
			continue
		}
		f, err := root.Open(name)
		if err != nil {
			r.diagnostics = append(r.diagnostics, Issue{path, "scanRuleReadFailed"})
			continue
		}
		data, err := io.ReadAll(io.LimitReader(f, (256<<10)+1))
		f.Close()
		total += len(data)
		if err != nil || len(data) > 256<<10 {
			r.diagnostics = append(r.diagnostics, Issue{path, "scanRuleReadFailed"})
			continue
		}
		pack, err := decodeRulePack(data)
		_, sameID := packs[pack.ID]
		_, targetExists := packs[pack.Replaces]
		if err != nil || installed[pack.ID] || (sameID && pack.Replaces != pack.ID) || (pack.Replaces != "" && (!targetExists || replaced[pack.Replaces])) {
			r.diagnostics = append(r.diagnostics, Issue{path, "scanInvalidRule"})
			continue
		}
		if pack.Replaces != "" {
			delete(packs, pack.Replaces)
			replaced[pack.Replaces] = true
		}
		packs[pack.ID] = pack
		installed[pack.ID] = true
	}
}

var ruleIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
var packageNamePattern = regexp.MustCompile(`^(?:@[a-z0-9][a-z0-9._-]*/)?[a-z0-9][a-z0-9._-]*$`)
var iconExtensionPattern = regexp.MustCompile(`^\.[a-z0-9]{1,12}$`)
var propertyNamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,127}$`)
var moduleNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._~+-]*(?:/[A-Za-z0-9][A-Za-z0-9._~+-]*)*$`)
var pluginIDPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]{0,213}$`)
var templateVariable = regexp.MustCompile(`\$\{([A-Za-z][A-Za-z0-9]*)\}`)

func decodeRulePack(data []byte) (RulePack, error) {
	var pack RulePack
	// No field in the rule schema accepts null. In particular a null condition
	// must not silently become an omitted condition during an external replacement.
	tokens := json.NewDecoder(bytes.NewReader(data))
	for {
		token, err := tokens.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return pack, err
		}
		if token == nil {
			return pack, fmt.Errorf("rule fields cannot be null")
		}
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&pack); err != nil {
		return pack, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return pack, fmt.Errorf("expected one rule pack")
	}
	if pack.SchemaVersion != 1 || pack.RuleVersion < 1 || !ruleIDPattern.MatchString(pack.ID) || (pack.Replaces != "" && !ruleIDPattern.MatchString(pack.Replaces)) || len(pack.Rules) == 0 || len(pack.Rules) > 64 {
		return pack, fmt.Errorf("invalid rule pack identity or version")
	}
	seen := map[string]bool{}
	for _, rule := range pack.Rules {
		if !ruleIDPattern.MatchString(rule.ID) || seen[rule.ID] {
			return pack, fmt.Errorf("invalid or duplicate rule id %q", rule.ID)
		}
		seen[rule.ID] = true
		if err := validateRule(rule); err != nil {
			return pack, fmt.Errorf("rule %s: %w", rule.ID, err)
		}
	}
	return pack, nil
}

func validateRule(rule Rule) error {
	switch rule.Resolver {
	case "project-script-v1":
	case "node-v1", "electron-builder-static-v1", "msbuild-v1", "go-v1", "cargo-v1", "swift-v1", "cmake-v1", "xcode-v1", "conventional-assets-v1", "web-manifest-v1", "package-metadata-v1", "tauri-static-v1", "python-static-v1", "flutter-static-v1", "json-app-v1", "go-module-metadata-v1", "python-commands-v1", "flutter-commands-v1", "dotnet-config-commands-v1", "nest-cli-v1", "expo-commands-v1", "react-native-commands-v1", "eas-commands-v1", "maven-commands-v1", "gradle-java-v1", "spring-boot-gradle-v1", "android-gradle-v1", "ios-xcode-commands-v1":
	default:
		return fmt.Errorf("unknown resolver %q", rule.Resolver)
	}
	if len(rule.Match.Files) == 0 || len(rule.Match.Files) > 32 {
		return fmt.Errorf("invalid file patterns")
	}
	for _, glob := range rule.Match.Files {
		if _, err := compileRuleGlob(glob); err != nil {
			return err
		}
	}
	if rule.Match.PluginsAny != nil {
		if !containsString([]string{"gradle-java-v1", "spring-boot-gradle-v1", "android-gradle-v1"}, rule.Resolver) || len(rule.Match.PluginsAny) == 0 || len(rule.Match.PluginsAny) > 32 {
			return fmt.Errorf("plugin conditions require a Gradle command resolver and 1 to 32 IDs")
		}
		seen := map[string]bool{}
		for _, id := range rule.Match.PluginsAny {
			if !pluginIDPattern.MatchString(id) || seen[id] {
				return fmt.Errorf("invalid or duplicate plugin ID")
			}
			seen[id] = true
		}
		for _, glob := range rule.Match.Files {
			base := strings.ToLower(filepath.Base(filepath.FromSlash(glob)))
			if base != "build.gradle" && base != "build.gradle.kts" {
				return fmt.Errorf("plugin conditions require Gradle build files")
			}
		}
	}
	if rule.Match.DependenciesAny != nil && (len(rule.Match.DependenciesAny) == 0 || len(rule.Match.DependenciesAny) > 32) {
		return fmt.Errorf("dependency conditions require 1 to 32 package names")
	}
	if len(rule.Match.DependenciesAny) > 0 {
		for _, glob := range rule.Match.Files {
			base := strings.ToLower(filepath.Base(filepath.FromSlash(glob)))
			valid := false
			switch rule.Resolver {
			case "package-metadata-v1", "expo-commands-v1", "react-native-commands-v1":
				valid = base == "package.json"
			case "nest-cli-v1":
				valid = base == "nest-cli.json" || base == "nest.json"
			case "eas-commands-v1":
				valid = base == "eas.json"
			case "json-app-v1":
				valid = base == "app.json" || base == "app.config.json" || base == "app.config.ts" || base == "app.config.js"
			case "go-module-metadata-v1":
				valid = base == "go.mod"
			case "python-static-v1":
				valid = base == "pyproject.toml" || base == "requirements.txt"
			case "msbuild-v1":
				ext := filepath.Ext(base)
				valid = ext == ".csproj" || ext == ".fsproj" || ext == ".vbproj"
			}
			if !valid {
				return fmt.Errorf("dependency condition is unsupported for this resolver or file pattern")
			}
		}
	}
	if rule.Match.PropertyEquals != nil {
		if (rule.Resolver != "msbuild-v1" && rule.Resolver != "dotnet-config-commands-v1") || len(rule.Match.PropertyEquals) == 0 || len(rule.Match.PropertyEquals) > 16 {
			return fmt.Errorf("property conditions require a supported MSBuild resolver and 1 to 16 properties")
		}
		for name, value := range rule.Match.PropertyEquals {
			if !propertyNamePattern.MatchString(name) || len(value) == 0 || len(value) > 512 || strings.ContainsAny(value, "\x00\r\n") {
				return fmt.Errorf("invalid property condition")
			}
		}
	}
	if rule.Resolver == "msbuild-v1" && (len(rule.Match.PropertyEquals) > 0 || len(rule.Match.DependenciesAny) > 0) && len(rule.Commands) > 0 {
		return fmt.Errorf("conditional MSBuild rules are metadata-only")
	}
	dependencies := map[string]bool{}
	for _, name := range rule.Match.DependenciesAny {
		valid := len(name) <= 214 && packageNamePattern.MatchString(name)
		if rule.Resolver == "go-module-metadata-v1" {
			valid = len(name) <= 512 && moduleNamePattern.MatchString(name)
		}
		if !valid || dependencies[name] {
			return fmt.Errorf("invalid or duplicate package dependency %q", name)
		}
		dependencies[name] = true
	}
	if len(rule.Emit.Frameworks) > 32 || len(rule.Emit.Names) > 32 || len(rule.Emit.Icons) > 64 || len(rule.Commands) > 32 {
		return fmt.Errorf("rule exceeds budget")
	}
	for _, framework := range rule.Emit.Frameworks {
		if !ruleIDPattern.MatchString(framework) {
			return fmt.Errorf("invalid framework")
		}
	}
	for _, name := range rule.Emit.Names {
		if !validField(name.Field) || !validOrigin(name.Origin) {
			return fmt.Errorf("invalid name selector")
		}
	}
	for _, icon := range rule.Emit.Icons {
		if len(icon.Extensions) > 8 {
			return fmt.Errorf("too many icon extensions")
		}
		for _, extension := range icon.Extensions {
			if !iconExtensionPattern.MatchString(extension) {
				return fmt.Errorf("invalid icon extension")
			}
		}
		selectors := 0
		for _, values := range [][]string{icon.Field, icon.Paths, icon.Directories} {
			if values != nil {
				selectors++
				if len(values) == 0 {
					return fmt.Errorf("icon selector cannot be empty")
				}
			}
		}
		if selectors != 1 || (len(icon.Field) > 0 && !validField(icon.Field)) || len(icon.Paths) > 64 || len(icon.Directories) > 32 {
			return fmt.Errorf("icon requires exactly one of field, paths or directories")
		}
		if len(icon.Directories) > 0 && (rule.Resolver != "conventional-assets-v1" || icon.Origin != "convention") {
			return fmt.Errorf("icon directories require conventional-assets-v1 and convention origin")
		}
		if icon.Recursive && len(icon.Directories) == 0 {
			return fmt.Errorf("recursive icon search requires directories")
		}
		if icon.Base != "config" && icon.Base != "unit" && icon.Base != "buildResourcesRoot" {
			return fmt.Errorf("unknown icon path base")
		}
		if icon.Platform != "" && icon.Platform != "windows" && icon.Platform != "darwin" && icon.Platform != "linux" {
			return fmt.Errorf("invalid icon platform")
		}
		if icon.Purpose != "application" || !validOrigin(icon.Origin) || icon.Precedence < 0 || icon.Precedence > 1000 {
			return fmt.Errorf("invalid icon metadata")
		}
		for _, path := range append(append([]string{}, icon.Paths...), icon.Directories...) {
			if !validLocalRulePath(path) {
				return fmt.Errorf("invalid icon path")
			}
		}
	}
	keys, tools, variables := commandContract(rule.Resolver)
	for key, command := range rule.Commands {
		commandVariables := templateVariablesForKey(rule.Resolver, key, variables)
		emptyAllowed := (rule.Resolver == "python-commands-v1" && key == "script") || (rule.Resolver == "project-script-v1" && key == "windows")
		if !containsString(keys, key) || command.Args == nil || (len(command.Args) == 0 && !emptyAllowed) || len(command.Args) > 64 || (command.Origin != "configuration" && command.Origin != "template") {
			return fmt.Errorf("invalid command template %q", key)
		}
		if command.Platform != "" && command.Platform != "Windows" && command.Platform != "macOS" && command.Platform != "macOS / Windows" {
			return fmt.Errorf("invalid command platform")
		}
		if !containsString(tools, command.Tool) {
			return fmt.Errorf("invalid command tool")
		}
		for _, arg := range append(append([]string{}, command.Args...), command.Tool) {
			if len(arg) > 4000 || strings.ContainsAny(arg, "\x00\r\n") {
				return fmt.Errorf("invalid command argument")
			}
			for _, match := range templateVariable.FindAllStringSubmatch(arg, -1) {
				if !containsString(commandVariables, match[1]) {
					return fmt.Errorf("unknown template variable %q", match[1])
				}
			}
			if strings.Contains(templateVariable.ReplaceAllString(arg, ""), "${") {
				return fmt.Errorf("invalid template variable")
			}
		}
	}
	if rule.Resolver == "node-v1" {
		if len(rule.Options.PackageManagers) == 0 || len(rule.Options.PackageManagers) > 4 || !containsString(rule.Options.PackageManagers, rule.Options.DefaultManager) {
			return fmt.Errorf("invalid package managers")
		}
		for _, manager := range rule.Options.PackageManagers {
			if manager != "npm" && manager != "pnpm" && manager != "yarn" && manager != "bun" {
				return fmt.Errorf("unsupported package manager")
			}
		}
	}
	if len(rule.Options.Lockfiles) > 32 || len(rule.Options.ConfigFiles) > 32 || len(rule.Options.WebRoots) > 32 {
		return fmt.Errorf("options exceed budget")
	}
	for path, manager := range rule.Options.Lockfiles {
		if !validLocalRulePath(path) || strings.Contains(path, "/") || !containsString(rule.Options.PackageManagers, manager) {
			return fmt.Errorf("invalid lockfile")
		}
	}
	for _, path := range rule.Options.ConfigFiles {
		if !validLocalRulePath(path) {
			return fmt.Errorf("invalid config path")
		}
	}
	if rule.Options.ValueRoot != nil && (rule.Resolver != "json-app-v1" || !validField(rule.Options.ValueRoot)) {
		return fmt.Errorf("value root requires json-app-v1 and a valid field selector")
	}
	if len(rule.Options.DynamicConfigFiles) > 8 || (rule.Options.DynamicConfigFiles != nil && rule.Resolver != "json-app-v1") {
		return fmt.Errorf("dynamic config files require json-app-v1 and at most 8 paths")
	}
	for _, path := range rule.Options.DynamicConfigFiles {
		if !validLocalRulePath(path) {
			return fmt.Errorf("invalid dynamic config path")
		}
	}
	for _, path := range rule.Options.WebRoots {
		if !validLocalRulePath(path) || strings.Contains(path, "/") || path == "." || path == ".." {
			return fmt.Errorf("invalid web root directory")
		}
	}
	if rule.Options.PlatformConfigFiles != nil && rule.Resolver != "tauri-static-v1" {
		return fmt.Errorf("platform config files require tauri-static-v1")
	}
	for platform, files := range rule.Options.PlatformConfigFiles {
		if platform != "windows" && platform != "darwin" && platform != "linux" {
			return fmt.Errorf("invalid config platform")
		}
		if len(files) == 0 || len(files) > 8 {
			return fmt.Errorf("invalid platform config file count")
		}
		for _, path := range files {
			if !validLocalRulePath(path) || strings.Contains(path, "/") || path == "." || path == ".." {
				return fmt.Errorf("invalid platform config filename")
			}
		}
	}
	if len(rule.Options.BuildResourcesField) > 0 && !validField(rule.Options.BuildResourcesField) {
		return fmt.Errorf("invalid resources selector")
	}
	if rule.Options.DefaultBuildResources != "" && !validLocalRulePath(rule.Options.DefaultBuildResources) {
		return fmt.Errorf("invalid resources path")
	}
	if rule.Options.EntryFiles != nil || rule.Options.PlatformFiles != nil {
		if rule.Resolver != "flutter-commands-v1" || len(rule.Options.EntryFiles) > 16 || len(rule.Options.PlatformFiles) > 5 {
			return fmt.Errorf("invalid configured entry or platform files")
		}
		for _, path := range rule.Options.EntryFiles {
			if !validLocalRulePath(path) {
				return fmt.Errorf("invalid entry path")
			}
		}
		for platform, paths := range rule.Options.PlatformFiles {
			if !containsString([]string{"apk", "ios", "web", "windows", "macos"}, platform) || len(paths) == 0 || len(paths) > 8 {
				return fmt.Errorf("invalid configured platform files")
			}
			for _, path := range paths {
				if !validLocalRulePath(path) {
					return fmt.Errorf("invalid platform path")
				}
			}
		}
	}
	return nil
}

// Each processor exposes a finite template contract; rule packs supply the
// command line while processors supply only values proven by project files.
func commandContract(resolver string) (keys, tools, variables []string) {
	switch resolver {
	case "project-script-v1":
		return []string{"windows", "shell"}, []string{"${script}", "bash"}, []string{"script"}
	case "node-v1":
		keys = []string{"script"}
		variables = []string{"packageManager", "scriptName"}
	case "msbuild-v1":
		keys = []string{"solution-build", "native-build", "native-build-default", "legacy-build", "build", "test", "run", "publish"}
		variables = []string{"file", "configuration", "platform"}
	case "python-commands-v1":
		return []string{"script", "poetry-script", "uv-script", "build", "poetry-build", "django-runserver", "django-check", "fastapi-dev", "fastapi-run", "flask-run"}, []string{"python", "poetry", "uv", "fastapi", "${scriptName}"}, []string{"scriptName", "manageFile", "entrypoint"}
	case "flutter-commands-v1":
		return []string{"run", "build-apk", "build-ios", "build-web", "build-windows", "build-macos"}, []string{"flutter"}, []string{"entry"}
	case "dotnet-config-commands-v1":
		return []string{"profile", "run-target"}, []string{"dotnet"}, []string{"file", "profile", "framework"}
	case "maven-commands-v1":
		return []string{"package", "exec-java", "spring-boot-run"}, []string{"mvn", "./mvnw.cmd", "./mvnw", "${mavenTool}"}, []string{"mavenTool", "pomFile"}
	case "gradle-java-v1", "spring-boot-gradle-v1", "android-gradle-v1":
		switch resolver {
		case "gradle-java-v1":
			keys = []string{"build", "test", "run"}
		case "spring-boot-gradle-v1":
			keys = []string{"boot-run", "boot-jar", "boot-war"}
		case "android-gradle-v1":
			keys = []string{"assemble", "bundle", "install", "start"}
		}
		return keys, []string{"gradle", "./gradlew.bat", "./gradlew", "${gradleTool}", "adb"}, []string{"gradleTool", "modulePath", "variant", "component"}
	case "ios-xcode-commands-v1":
		return []string{"project-simulator", "project-device", "workspace-simulator", "workspace-device", "target-simulator", "target-device", "simulator-launch"}, []string{"xcodebuild", "xcrun"}, []string{"container", "scheme", "target", "configuration", "bundleID"}
	case "nest-cli-v1", "expo-commands-v1", "react-native-commands-v1", "eas-commands-v1":
		operations := []string{"start", "android", "ios"}
		switch resolver {
		case "nest-cli-v1":
			operations = []string{"build", "start", "watch", "build-project", "start-project", "watch-project"}
			variables = []string{"projectName"}
		case "eas-commands-v1":
			operations = []string{"build", "build-profile"}
			variables = []string{"buildProfile", "targetPlatform"}
		case "react-native-commands-v1":
			operations = append(operations, "build-android", "build-ios")
		}
		for _, operation := range operations {
			for _, manager := range []string{"npm", "pnpm", "yarn", "bun"} {
				keys = append(keys, operation+"-"+manager)
			}
		}
		return keys, []string{"npm", "pnpm", "yarn", "bun"}, variables
	default:
		return nil, nil, nil
	}
	// Preserve the original external Node/MSBuild template tool contract.
	return keys, []string{"npm", "pnpm", "yarn", "bun", "dotnet", "MSBuild", "${packageManager}"}, variables
}

func templateVariablesForKey(resolver, key string, variables []string) []string {
	switch resolver {
	case "gradle-java-v1", "spring-boot-gradle-v1":
		return []string{"gradleTool", "modulePath"}
	case "android-gradle-v1":
		if key == "start" {
			return []string{"component"}
		}
		return []string{"gradleTool", "modulePath", "variant"}
	case "ios-xcode-commands-v1":
		switch key {
		case "simulator-launch":
			return []string{"bundleID"}
		case "target-simulator", "target-device":
			return []string{"container", "target", "configuration"}
		default:
			return []string{"container", "scheme", "configuration"}
		}
	case "python-commands-v1":
		switch key {
		case "script", "poetry-script", "uv-script":
			return []string{"scriptName"}
		case "django-runserver", "django-check":
			return []string{"manageFile"}
		case "fastapi-dev", "fastapi-run", "flask-run":
			return []string{"entrypoint"}
		default:
			return nil
		}
	case "dotnet-config-commands-v1":
		if key == "run-target" {
			return []string{"file", "framework"}
		}
	case "nest-cli-v1":
		if !strings.Contains(key, "-project-") {
			return nil
		}
	case "eas-commands-v1":
		if strings.HasPrefix(key, "build-profile-") {
			return []string{"buildProfile"}
		}
	}
	return variables
}

func containsString(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func validOrigin(origin string) bool { return origin == "declared" || origin == "convention" }
func validField(field []string) bool {
	if len(field) == 0 || len(field) > 16 {
		return false
	}
	for _, token := range field {
		if token == "" || len(token) > 128 || strings.ContainsAny(token, "\x00\r\n") {
			return false
		}
	}
	return true
}
func validLocalRulePath(path string) bool {
	return len(path) > 0 && len(path) <= 256 && !strings.ContainsAny(path, "\\:*?[]\x00\r\n") && filepath.IsLocal(filepath.FromSlash(path)) && !forbidden(path)
}

// Globs use forward slashes; ** is allowed only as a whole directory segment.
// Matching is case-insensitive on every host so cross-platform projects agree.
func compileRuleGlob(glob string) (*regexp.Regexp, error) {
	if len(glob) == 0 || len(glob) > 256 || strings.ContainsAny(glob, "\\:[]\x00\r\n") || strings.HasPrefix(glob, "/") || forbidden(glob) {
		return nil, fmt.Errorf("invalid file glob %q", glob)
	}
	var pattern strings.Builder
	pattern.WriteByte('^')
	parts := strings.Split(strings.ToLower(glob), "/")
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, fmt.Errorf("invalid file glob %q", glob)
		}
		if part == "**" {
			if i == len(parts)-1 {
				return nil, fmt.Errorf("glob ** requires a following file")
			}
			pattern.WriteString("(?:.*/)?")
			continue
		}
		if strings.Contains(part, "**") {
			return nil, fmt.Errorf("glob ** must be a directory segment")
		}
		for _, char := range part {
			switch char {
			case '*':
				pattern.WriteString("[^/]*")
			case '?':
				pattern.WriteString("[^/]")
			default:
				pattern.WriteString(regexp.QuoteMeta(string(char)))
			}
		}
		if i < len(parts)-1 {
			pattern.WriteByte('/')
		}
	}
	pattern.WriteByte('$')
	return regexp.Compile(pattern.String())
}

func (s *scan) addRuleCommand(rule Rule, key, path, dir string, variables map[string]string, platform string, review bool, extraArgs ...string) {
	template, ok := rule.Commands[key]
	if !ok {
		return
	}
	valid := true
	expand := func(value string) string {
		return templateVariable.ReplaceAllStringFunc(value, func(token string) string {
			value, ok := variables[token[2:len(token)-1]]
			if !ok {
				valid = false
			}
			return value
		})
	}
	tool := expand(template.Tool)
	args := make([]string, 0, len(template.Args)+len(extraArgs))
	for _, arg := range template.Args {
		args = append(args, expand(arg))
	}
	args = append(args, extraArgs...)
	if !valid || !safeArg.MatchString(tool) || strings.HasPrefix(tool, "-") {
		s.issue(path, "scanInvalidRule")
		return
	}
	if template.Platform != "" {
		platform = template.Platform
	}
	s.add(path, dir, tool, template.Origin, platform, review || template.Review, args...)
}
