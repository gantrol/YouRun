// Package discovery reads project files without invoking a project toolchain.
package discovery

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
)

const (
	maxEntries     = 8192
	maxDirectories = 512
	maxDepth       = 6
	maxFileBytes   = 1 << 20
	maxTotalBytes  = 16 << 20
	maxCommands    = 256
)

type Candidate struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Text      string `json:"text"`
	Source    string `json:"source"`
	Directory string `json:"directory"`
	Tool      string `json:"tool"`
	Origin    string `json:"origin"`
	Review    bool   `json:"review"`
	Shell     string `json:"shell"`
	Platform  string `json:"platform"`
	Icon      string `json:"icon"`
}
type Issue struct {
	Path string `json:"path"`
	Code string `json:"code"`
}
type Result struct {
	Directory  string          `json:"directory"`
	Name       string          `json:"name"`
	Names      []NameCandidate `json:"names,omitempty"`
	Candidates []Candidate     `json:"candidates"`
	Issues     []Issue         `json:"issues"`
}
type scan struct {
	ctx                                   context.Context
	root                                  *os.Root
	result                                Result
	entries                               map[string]bool
	cache                                 map[string][]byte
	bytes, directories, entryCount, reads int
	seen                                  map[string]bool
	registry                              *Registry
	scope                                 directoryScope
	pending                               []scanDirectory
	projectDirs                           map[string]bool
	cmakeDone                             map[string]bool
	cmakeIncluded                         map[string]bool
}

type scanDirectory struct {
	path  string
	depth int
}

func Discover(ctx context.Context, directory string) (Result, error) {
	return DiscoverWithRegistry(ctx, directory, DefaultRegistry())
}

// DiscoverWithRegistry keeps each scan on one validated rules snapshot.
func DiscoverWithRegistry(ctx context.Context, directory string, registry *Registry) (Result, error) {
	if registry == nil {
		registry = DefaultRegistry()
	}
	if !filepath.IsAbs(directory) || forbidden(directory) {
		return Result{}, errors.New("invalidDirectory")
	}
	directory, err := filepath.EvalSymlinks(directory)
	if err != nil || forbidden(directory) {
		return Result{}, errors.New("invalidDirectory")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return Result{}, errors.New("invalidDirectory")
	}
	defer root.Close()
	parentContext := ctx
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	s := &scan{ctx: ctx, root: root, registry: registry, entries: map[string]bool{}, cache: map[string][]byte{}, seen: map[string]bool{}, result: Result{Directory: directory, Name: filepath.Base(directory), Candidates: []Candidate{}, Issues: registry.Diagnostics()}}
	if s.result.Issues == nil {
		s.result.Issues = []Issue{}
	}
	s.pending = []scanDirectory{{".", 0}}
	for len(s.pending) > 0 && ctx.Err() == nil {
		next := s.pending[0]
		s.pending = s.pending[1:]
		s.walk(next.path, next.depth)
	}
	paths := [4][]string{}
	s.projectDirs = map[string]bool{".": true}
	for path := range s.entries {
		matched := [4]bool{}
		for _, rule := range registry.Match(path) {
			if discoversCommands(rule) {
				if rule.Resolver != "project-script-v1" {
					s.projectDirs[filepath.Dir(path)] = true
				}
				phase := commandPhase(rule)
				if !matched[phase] {
					paths[phase] = append(paths[phase], path)
					matched[phase] = true
				}
			}
		}
	}
	// Keep the original candidates and their read/command budgets ahead of new
	// configured targets. Within each phase reserve manifest reads before sources.
	for phase, files := range paths {
		sort.Slice(files, func(i, j int) bool {
			left, right := strings.Count(files[i], string(filepath.Separator)), strings.Count(files[j], string(filepath.Separator))
			if left != right {
				return left < right
			}
			return files[i] < files[j]
		})
		for _, path := range files {
			s.read(path)
		}
		for _, path := range files {
			if err := ctx.Err(); err != nil {
				if phase > 0 && parentContext.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
					s.issue(".", "scanLimit")
					return s.result, nil
				}
				return Result{}, err
			}
			data := s.read(path)
			if data == nil {
				continue
			}
			if err := s.detect(path, data, phase); err != nil {
				s.issue(path, "scanInvalidManifest")
			}
		}
	}
	if err := ctx.Err(); err != nil {
		if parentContext.Err() == nil && errors.Is(err, context.DeadlineExceeded) && len(paths[1])+len(paths[2])+len(paths[3]) > 0 {
			s.issue(".", "scanLimit")
			return s.result, nil
		}
		return Result{}, err
	}
	return s.result, nil
}

func forbidden(path string) bool { return strings.Contains(strings.ToLower(path), "trash") }
func ignored(name string) bool {
	n := strings.ToLower(name)
	if strings.HasPrefix(n, ".") && n != ".vscode" {
		return true
	}
	switch n {
	case "node_modules", "vendor", "target", "dist", "build", "builds", "obj", "deriveddata", "pods", "carthage", "coverage", "test-results":
		return true
	case "__pycache__":
		return true
	}
	return forbidden(name) || strings.HasSuffix(n, ".egg-info")
}
func (s *scan) walk(path string, depth int) {
	if s.ctx.Err() != nil {
		return
	}
	if depth > maxDepth {
		s.issue(path, "scanDepthLimit")
		return
	}
	if s.directories >= maxDirectories || s.entryCount >= maxEntries {
		if s.directories >= maxDirectories {
			s.issue(".", "scanDirectoryLimit")
		}
		if s.entryCount >= maxEntries {
			s.issue(".", "scanEntryLimit")
		}
		s.pending = nil
		return
	}
	s.directories++
	f, err := s.root.Open(path)
	if err != nil {
		s.issue(path, "scanReadFailed")
		return
	}
	entries, err := f.ReadDir(maxEntries - s.entryCount + 1)
	f.Close()
	if err != nil && err != io.EOF {
		s.issue(path, "scanReadFailed")
		return
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if entry.Name() == ".gitignore" && entry.Type().IsRegular() {
			file := filepath.Join(path, entry.Name())
			s.entries[file] = true
			s.scope.load(path, s.read(file), s.issue)
			break
		}
	}
	children := []string{}
	for _, e := range entries {
		if s.ctx.Err() != nil {
			return
		}
		if s.entryCount >= maxEntries {
			s.issue(".", "scanEntryLimit")
			s.pending = nil
			return
		}
		s.entryCount++
		if forbidden(e.Name()) || (ignored(e.Name()) && !(e.Type().IsRegular() && strings.EqualFold(e.Name(), ".flaskenv"))) {
			continue
		}
		next := filepath.Join(path, e.Name())
		if e.Type()&os.ModeSymlink != 0 {
			s.issue(next, "scanLinkSkipped")
			continue
		}
		if e.IsDir() {
			if s.scope.excluded(next) {
				continue
			}
			if strings.EqualFold(e.Name(), "bin") && filepath.Base(path) != "src" {
				continue
			}
			children = append(children, next)
			continue
		}
		if e.Type().IsRegular() {
			s.entries[next] = true
		}
	}
	for _, child := range children {
		s.pending = append(s.pending, scanDirectory{child, depth + 1})
	}
}
func (s *scan) read(path string) []byte {
	path = filepath.Clean(path)
	if data, ok := s.cache[path]; ok {
		return data
	}
	if !filepath.IsLocal(path) || forbidden(path) || !s.entries[path] {
		return nil
	}
	s.cache[path] = nil
	if s.ctx.Err() != nil {
		return nil
	}
	if s.reads >= 512 || s.bytes >= maxTotalBytes {
		s.issue(path, "scanLimit")
		return nil
	}
	s.reads++
	// Root confines even a concurrent symlink replacement to the selected directory.
	info, err := s.root.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		s.issue(path, "scanReadFailed")
		return nil
	}
	if info.Size() > maxFileBytes || info.Size() > int64(maxTotalBytes-s.bytes) {
		s.issue(path, "scanLimit")
		return nil
	}
	f, err := s.root.Open(path)
	if err != nil {
		s.issue(path, "scanReadFailed")
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(f, int64(min(maxFileBytes, maxTotalBytes-s.bytes))+1))
	f.Close()
	s.bytes += len(data)
	if len(data) > maxFileBytes || s.bytes > maxTotalBytes {
		s.issue(path, "scanLimit")
		return nil
	}
	if err != nil {
		s.issue(path, "scanReadFailed")
		return nil
	}
	s.cache[path] = data
	return data
}
func (s *scan) issue(path, code string) {
	path = filepath.ToSlash(path)
	for _, v := range s.result.Issues {
		if v.Path == path && v.Code == code {
			return
		}
	}
	if len(s.result.Issues) < 128 {
		s.result.Issues = append(s.result.Issues, Issue{filepath.ToSlash(path), code})
	}
}
func (s *scan) detect(path string, data []byte, phase int) error {
	var result error
	for _, rule := range s.registry.Match(path) {
		if !discoversCommands(rule) || commandPhase(rule) != phase {
			continue
		}
		var err error
		switch rule.Resolver {
		case "node-v1":
			err = s.node(path, data, rule)
		case "msbuild-v1":
			err = s.msbuild(path, data, rule)
		case "go-v1":
			err = s.golang(path, data)
		case "cargo-v1":
			err = s.cargo(path, data)
		case "swift-v1":
			err = s.swift(path, data)
		case "cmake-v1":
			err = s.cmake(path, data)
		case "xcode-v1":
			err = s.xcode(path, data)
		case "python-commands-v1":
			err = s.pythonCommands(path, data, rule)
		case "flutter-commands-v1":
			err = s.flutterCommands(path, data, rule)
		case "dotnet-config-commands-v1":
			err = s.dotnetConfiguredCommands(path, data, rule)
		case "nest-cli-v1":
			err = s.nestCommands(path, data, rule)
		case "expo-commands-v1":
			err = s.expoCommands(path, data, rule)
		case "react-native-commands-v1":
			err = s.reactNativeCommands(path, data, rule)
		case "eas-commands-v1":
			err = s.easCommands(path, data, rule)
		case "maven-commands-v1":
			err = s.mavenCommands(path, data, rule)
		case "gradle-java-v1", "spring-boot-gradle-v1", "android-gradle-v1":
			err = s.gradleCommands(path, data, rule)
		case "ios-xcode-commands-v1":
			err = s.iosXcodeCommands(path, data, rule)
		case "project-script-v1":
			s.projectScript(path, data, rule)
		}
		if err != nil {
			result = err
		}
	}
	return result
}

// Metadata extensions must not consume the manifest read budget reserved for
// command discovery or make its output depend on unrelated framework packs.
func discoversCommands(rule Rule) bool {
	switch rule.Resolver {
	case "project-script-v1":
		return len(rule.Commands) > 0
	case "node-v1", "msbuild-v1", "python-commands-v1", "flutter-commands-v1", "dotnet-config-commands-v1", "nest-cli-v1", "expo-commands-v1", "react-native-commands-v1", "eas-commands-v1", "maven-commands-v1", "gradle-java-v1", "spring-boot-gradle-v1", "android-gradle-v1", "ios-xcode-commands-v1":
		return len(rule.Commands) > 0
	case "go-v1", "cargo-v1", "swift-v1", "cmake-v1", "xcode-v1":
		return true
	default:
		return false
	}
}

func configuredCommands(rule Rule) bool {
	return commandPhase(rule) > 0
}

// New ecosystems follow earlier discovery groups so adding their manifests
// cannot consume budgets previously available to existing commands.
func commandPhase(rule Rule) int {
	switch rule.Resolver {
	case "project-script-v1":
		return 3
	case "python-commands-v1", "flutter-commands-v1", "dotnet-config-commands-v1", "nest-cli-v1", "expo-commands-v1", "react-native-commands-v1", "eas-commands-v1":
		return 1
	case "maven-commands-v1", "gradle-java-v1", "spring-boot-gradle-v1", "android-gradle-v1", "ios-xcode-commands-v1":
		return 2
	default:
		return 0
	}
}

var safeArg = regexp.MustCompile(`^[A-Za-z0-9_./:@=,+-]+$`)

func quote(arg, platform string) string {
	if safeArg.MatchString(arg) && !strings.HasPrefix(arg, "~") {
		return arg
	}
	if !strings.ContainsAny(arg, "\"$`\\%!") {
		return `"` + arg + `"`
	}
	if platform == "Windows" || (platform != "macOS" && runtime.GOOS == "windows") {
		return "'" + strings.ReplaceAll(arg, "'", "''") + "'"
	}
	return "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
}
func (s *scan) add(path, dir, tool, origin, platform string, review bool, args ...string) {
	if len(s.result.Candidates) >= maxCommands {
		s.issue(path, "scanLimit")
		return
	}
	parts := []string{tool}
	for _, arg := range args {
		if strings.ContainsAny(arg, "\x00\r\n") {
			s.issue(path, "scanInvalidManifest")
			return
		}
		parts = append(parts, quote(arg, platform))
	}
	text := strings.Join(parts, " ")
	if len(text) > 8000 {
		s.issue(path, "scanLimit")
		return
	}
	key := path + "\x00" + dir + "\x00" + text
	if s.seen[key] {
		return
	}
	s.seen[key] = true
	sourceKey, directoryKey := filepath.Join(s.result.Directory, path), filepath.Join(s.result.Directory, dir)
	if runtime.GOOS == "windows" {
		sourceKey = strings.ToLower(sourceKey)
		directoryKey = strings.ToLower(directoryKey)
	}
	identity := sourceKey + "\x00" + directoryKey + "\x00" + text
	digest := sha256.Sum256([]byte(identity))
	title := text
	if len(title) > 200 {
		title = tool + " · " + filepath.Base(path)
	}
	if platform == "" {
		platform = "macOS / Windows"
	}
	shell := "default"
	switch platform {
	case "Windows":
		shell = "PowerShell"
	case "macOS":
		shell = "zsh"
	}
	s.result.Candidates = append(s.result.Candidates, Candidate{ID: hex.EncodeToString(digest[:]), Title: title, Text: text, Source: filepath.Join(s.result.Directory, path), Directory: filepath.Join(s.result.Directory, dir), Tool: tool, Origin: origin, Review: review, Shell: shell, Platform: platform, Icon: "play"})
}
