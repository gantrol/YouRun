package discovery

import (
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

type cargoTarget struct {
	Name             string
	Path             string
	RequiredFeatures []string `toml:"required-features"`
	CrateType        []string `toml:"crate-type"`
}
type cargoManifest struct {
	Package struct {
		Name         string
		Edition      string
		Autobins     *bool
		Autoexamples *bool
		Publish      any
	}
	Workspace map[string]any
	Bin       []cargoTarget
	Example   []cargoTarget
	Profile   map[string]any
}

func (s *scan) cargo(path string, data []byte) error {
	var config cargoManifest
	if err := toml.Unmarshal(data, &config); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if config.Package.Name == "" && config.Workspace == nil {
		s.issue(path, "scanInvalidManifest")
		return nil
	}
	workspace := []string{}
	if config.Package.Name == "" {
		workspace = []string{"--workspace"}
	}
	for _, args := range [][]string{{"build"}, {"build", "--release"}, {"test"}, {"check"}} {
		s.add(path, dir, "cargo", "template", "", false, append(args, workspace...)...)
	}
	profiles := make([]string, 0)
	for profile := range config.Profile {
		if profile != "dev" && profile != "release" && profile != "test" && profile != "bench" {
			profiles = append(profiles, profile)
		}
	}
	sort.Strings(profiles)
	for _, profile := range profiles {
		s.add(path, dir, "cargo", "template", "", false, append([]string{"build", "--profile", profile}, workspace...)...)
	}
	if config.Package.Name == "" {
		return nil
	}
	for _, kind := range []string{"bin", "example"} {
		targets := map[string]cargoTarget{}
		auto := true
		explicit := config.Bin
		if kind == "bin" {
			if config.Package.Autobins != nil {
				auto = *config.Package.Autobins
			} else if (config.Package.Edition == "" || config.Package.Edition == "2015") && len(config.Bin) > 0 {
				auto = false
			}
		} else {
			explicit = config.Example
			if config.Package.Autoexamples != nil {
				auto = *config.Package.Autoexamples
			}
		}
		if auto {
			if kind == "bin" && s.entries[filepath.Join(dir, "src", "main.rs")] {
				targets[config.Package.Name] = cargoTarget{Name: config.Package.Name}
			}
			base := filepath.Join(dir, "src", "bin")
			if kind == "example" {
				base = filepath.Join(dir, "examples")
			}
			for entry := range s.entries {
				rel, err := filepath.Rel(base, entry)
				if err != nil || !filepath.IsLocal(rel) {
					continue
				}
				name := ""
				if filepath.Dir(rel) == "." && strings.HasSuffix(rel, ".rs") {
					name = strings.TrimSuffix(rel, ".rs")
				} else if filepath.Base(rel) == "main.rs" && filepath.Dir(filepath.Dir(rel)) == "." {
					name = filepath.Dir(rel)
				}
				if name != "" {
					targets[name] = cargoTarget{Name: name}
				}
			}
		}
		for _, target := range explicit {
			if target.Name == "" {
				s.issue(path, "scanPartial")
				continue
			}
			if len(target.CrateType) > 0 && !(len(target.CrateType) == 1 && target.CrateType[0] == "bin") {
				delete(targets, target.Name)
				continue
			}
			if target.Path != "" {
				targetPath := filepath.Clean(filepath.Join(dir, filepath.FromSlash(target.Path)))
				if !filepath.IsLocal(targetPath) || !s.entries[targetPath] {
					s.issue(path, "scanPartial")
					delete(targets, target.Name)
					continue
				}
			}
			targets[target.Name] = target
		}
		names := make([]string, 0, len(targets))
		for name := range targets {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			target := targets[name]
			args := []string{"run", "--" + kind, name}
			if len(target.RequiredFeatures) > 0 {
				args = append(args, "--features", strings.Join(target.RequiredFeatures, ","))
			}
			s.add(path, dir, "cargo", "template", "", false, args...)
		}
	}
	s.add(path, dir, "cargo", "template", "", false, "package")
	canPublish := true
	registry := ""
	switch value := config.Package.Publish.(type) {
	case bool:
		canPublish = value
	case []any:
		canPublish = len(value) > 0
		if len(value) == 1 {
			registry, _ = value[0].(string)
		}
	}
	if canPublish {
		args := []string{"publish"}
		if registry != "" {
			args = append(args, "--registry", registry)
		}
		s.add(path, dir, "cargo", "template", "", true, args...)
	}
	return nil
}

// Swift manifests are programs. Only literal executable products are proposed;
// conditional/dynamic declarations still require review, and are never evaluated.
var swiftProduct = regexp.MustCompile(`\.executable\s*\(\s*name\s*:\s*"([^"\\\r\n]+)"\s*,`)
var swiftTarget = regexp.MustCompile(`\.executableTarget\s*\(\s*name\s*:\s*"([^"\\\r\n]+)"\s*[,)]`)

func (s *scan) swift(path string, data []byte) error {
	dir := filepath.Dir(path)
	for _, args := range [][]string{{"build"}, {"build", "-c", "release"}, {"test"}} {
		s.add(path, dir, "swift", "template", "", false, args...)
	}
	code := stripComments(string(data))
	matches := swiftProduct.FindAllStringSubmatch(code, -1)
	if len(matches) == 0 {
		matches = swiftTarget.FindAllStringSubmatch(code, -1)
	}
	for _, match := range matches {
		s.add(path, dir, "swift", "template", "", true, "run", match[1])
	}
	s.issue(path, "scanPartial")
	return nil
}

// Preserve quoted content while masking line and nested block comments.
func stripComments(code string) string {
	b := []byte(code)
	for i := 0; i < len(b); {
		if b[i] == '"' {
			i++
			for i < len(b) {
				if b[i] == '\\' {
					i += 2
					continue
				}
				if b[i] == '"' {
					i++
					break
				}
				i++
			}
			continue
		}
		if i+1 < len(b) && b[i] == '/' && b[i+1] == '/' {
			for i < len(b) && b[i] != '\n' {
				b[i] = ' '
				i++
			}
			continue
		}
		if i+1 < len(b) && b[i] == '/' && b[i+1] == '*' {
			depth := 1
			b[i] = ' '
			b[i+1] = ' '
			i += 2
			for i < len(b) && depth > 0 {
				if i+1 < len(b) && b[i] == '/' && b[i+1] == '*' {
					depth++
					b[i] = ' '
					b[i+1] = ' '
					i += 2
				} else if i+1 < len(b) && b[i] == '*' && b[i+1] == '/' {
					depth--
					b[i] = ' '
					b[i+1] = ' '
					i += 2
				} else {
					if b[i] != '\n' {
						b[i] = ' '
					}
					i++
				}
			}
			continue
		}
		i++
	}
	return string(b)
}
