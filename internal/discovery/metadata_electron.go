package discovery

import (
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

func (s *inspector) electronMetadata(path string, data []byte, rule Rule) {
	values := map[string]any{}
	if strings.EqualFold(filepath.Base(path), "package.json") && s.configUnits[path] == "" {
		if json.Unmarshal(data, &values) != nil {
			s.issue(path, "scanInvalidManifest")
			return
		}
		_, hasBuild := values["build"]
		electron := hasBuild
		for _, key := range []string{"dependencies", "devDependencies"} {
			deps, _ := values[key].(map[string]any)
			for _, name := range []string{"electron", "electron-builder"} {
				if _, ok := deps[name]; ok {
					electron = true
				}
			}
		}
		if !electron {
			return
		}
		u := s.unit(path, rule)
		s.terminalDirs[filepath.Dir(path)] = true
		resources := fieldString(values, rule.Options.BuildResourcesField)
		if resources == "" {
			resources = rule.Options.DefaultBuildResources
		}
		s.applyMetadata(rule, path, u, values, localJoin(filepath.Dir(path), resources))
		explicit := s.electronScriptConfigs(path, values, u)
		for _, name := range rule.Options.ConfigFiles {
			config := localJoin(filepath.Dir(path), name)
			if !explicit && config != "" && s.read(config) != nil {
				s.defaultConfigs[u.ID] = config
				s.process(config)
				break
			}
		}
		if config, ok := values["build"].(map[string]any); ok {
			if _, ok := config["extends"]; ok {
				s.issue(path, "scanPartial")
			}
		}
		return
	}
	if u := s.units[s.unitKey(path)]; u != nil && s.configUnits[path] == "" {
		if s.explicitConfigs[u.ID] {
			return
		}
		if selected := s.defaultConfigs[u.ID]; selected != "" && selected != path {
			s.issue(path, "scanPartial")
			return
		}
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		if json.Unmarshal(data, &values) != nil {
			s.issue(path, "scanInvalidManifest")
			return
		}
	case ".yml", ".yaml":
		var document yaml.Node
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		if decoder.Decode(&document) != nil {
			s.issue(path, "scanInvalidManifest")
			return
		}
		nodes := 0
		if !staticYAML(&document, 0, &nodes) {
			s.issue(path, "scanPartial")
			return
		}
		if document.Decode(&values) != nil {
			s.issue(path, "scanInvalidManifest")
			return
		}
		var extra yaml.Node
		if decoder.Decode(&extra) != io.EOF {
			s.issue(path, "scanInvalidManifest")
			return
		}
	default:
		s.issue(path, "scanPartial")
		return
	}
	if values == nil {
		s.issue(path, "scanInvalidManifest")
		return
	}
	if _, ok := values["extends"]; ok {
		s.issue(path, "scanPartial")
	}
	u := s.unit(path, rule)
	s.terminalDirs[filepath.FromSlash(u.Directory)] = true
	resources := fieldString(values, rule.Options.BuildResourcesField)
	if resources == "" {
		resources = rule.Options.DefaultBuildResources
	}
	s.applyMetadata(rule, path, u, values, localJoin(filepath.FromSlash(u.Directory), resources))
}

func (s *inspector) electronScriptConfigs(path string, values map[string]any, u *ProjectUnit) bool {
	scripts, _ := values["scripts"].(map[string]any)
	configs := map[string]string{}
	names := make([]string, 0, len(scripts))
	for name := range scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		value := scripts[name]
		text, _ := value.(string)
		if len(text) > 8000 {
			continue
		}
		tokens := referenceTokens(text)
		for i, token := range tokens {
			if token != "electron-builder" {
				continue
			}
			platforms := map[string]bool{}
			config := ""
			for j := i + 1; j < len(tokens) && tokens[j] != "&" && tokens[j] != ";" && tokens[j] != "|"; j++ {
				flag, value, hasValue := strings.Cut(tokens[j], "=")
				switch flag {
				case "--win", "-w":
					platforms["windows"] = true
				case "--mac", "--macos", "-m":
					platforms["darwin"] = true
				case "--linux", "-l":
					platforms["linux"] = true
				case "--config", "-c":
					if !hasValue && j+1 < len(tokens) {
						j++
						value = tokens[j]
					}
					config = value
				}
			}
			if config == "" || (len(platforms) > 0 && !platforms[s.options.Platform]) {
				continue
			}
			resolved := localJoin(filepath.Dir(path), config)
			if resolved == "" {
				s.issue(path, "scanPartial")
				continue
			}
			configs[resolved] = filepath.ToSlash(path) + "#scripts." + name
		}
	}
	if len(configs) == 0 {
		return false
	}
	if len(configs) > 1 {
		s.issue(path, "scanPartial")
	}
	s.explicitConfigs[u.ID] = true
	paths := make([]string, 0, len(configs))
	for config := range configs {
		paths = append(paths, config)
	}
	sort.Strings(paths)
	for _, config := range paths {
		s.configUnits[config] = s.unitKey(path)
		s.references[config] = appendUnique(s.references[config], configs[config])
		raw := s.read(config)
		if raw == nil {
			s.issue(config, "scanReadFailed")
			continue
		}
		for _, rule := range s.registry.Match("electron-builder.json") {
			if rule.Resolver == "electron-builder-static-v1" {
				s.electronMetadata(config, raw, rule)
			}
		}
		s.processed[config] = true
	}
	return true
}

func staticYAML(node *yaml.Node, depth int, count *int) bool {
	*count++
	if depth > 48 || *count > 16384 || node.Kind == yaml.AliasNode || node.Anchor != "" {
		return false
	}
	if node.Tag != "" && !strings.HasPrefix(node.Tag, "!!") {
		return false
	}
	for _, child := range node.Content {
		if !staticYAML(child, depth+1, count) {
			return false
		}
	}
	return true
}
