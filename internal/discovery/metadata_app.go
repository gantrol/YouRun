package discovery

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// Static application configs reuse field/path rules; framework-specific names,
// wrapper fields, config precedence and asset directories live in the pack.
func (s *inspector) jsonAppMetadata(path string, data []byte, rule Rule) {
	if len(rule.Match.DependenciesAny) > 0 {
		packagePath := filepath.Join(filepath.Dir(path), "package.json")
		var pkg map[string]any
		if raw := s.read(packagePath); raw == nil || json.Unmarshal(raw, &pkg) != nil || !rule.MatchesDependencies(pkg) {
			return
		}
		s.process(packagePath)
		s.configUnits[path] = s.unitKey(packagePath)
	}
	for _, name := range rule.Options.DynamicConfigFiles {
		config := localJoin(filepath.Dir(path), name)
		if config != "" && s.read(config) != nil {
			s.issue(config, "scanPartial")
			return
		}
	}
	for _, name := range rule.Options.ConfigFiles {
		config := localJoin(filepath.Dir(path), name)
		if config != "" && s.read(config) != nil {
			if config != path {
				// A preferred filename is relevant only if another application
				// rule can actually handle it for this project's dependencies.
				applicable := false
				for _, candidate := range s.registry.Match(config) {
					if candidate.Resolver != "json-app-v1" {
						continue
					}
					if len(candidate.Match.DependenciesAny) == 0 {
						applicable = true
					} else {
						var pkg map[string]any
						raw := s.read(filepath.Join(filepath.Dir(config), "package.json"))
						applicable = applicable || json.Unmarshal(raw, &pkg) == nil && candidate.MatchesDependencies(pkg)
					}
				}
				if !applicable {
					continue
				}
				s.process(config)
				return
			}
			break
		}
	}
	if !strings.EqualFold(filepath.Ext(path), ".json") {
		s.issue(path, "scanPartial")
		return
	}
	var values map[string]any
	if json.Unmarshal(data, &values) != nil || values == nil {
		s.issue(path, "scanInvalidManifest")
		return
	}
	if len(rule.Options.ValueRoot) > 0 {
		current := values
		for _, field := range rule.Options.ValueRoot {
			value, exists := current[field]
			if !exists {
				current = nil
				break
			}
			nested, ok := value.(map[string]any)
			if !ok {
				s.issue(path, "scanInvalidManifest")
				return
			}
			current = nested
		}
		if current != nil {
			values = current
		}
	}
	unit := s.unit(path, rule)
	resources := fieldString(values, rule.Options.BuildResourcesField)
	if resources == "" {
		resources = rule.Options.DefaultBuildResources
	}
	resourceRoot := localJoin(filepath.Dir(path), resources)
	if resources != "" && resourceRoot == "" {
		s.issue(path, "scanPartial")
	}
	s.applyMetadata(rule, path, unit, values, resourceRoot)
}
