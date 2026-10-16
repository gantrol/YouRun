package discovery

import (
	"encoding/json"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// tauriMetadata resolves one static Tauri v2 application configuration. Platform
// files patch the base configuration; they are never separate applications.
func (s *inspector) tauriMetadata(path string, data []byte, rule Rule) {
	base := ""
	for _, name := range rule.Options.ConfigFiles {
		candidate := localJoin(filepath.Dir(path), name)
		if candidate != "" && s.read(candidate) != nil {
			base = candidate
			break
		}
	}
	if base == "" {
		s.issue(path, "scanPartial")
		return
	}
	if base != path {
		s.process(base)
		return
	}
	s.bindTauriPackage(base)
	unit := s.tauriUnit(base, rule)
	s.bindTauriCargo(base, unit)
	values, ok := s.readTauriConfig(base, data)
	if !ok {
		return
	}
	// v1's package/tauri nesting has different semantics. A v2 adapter must not
	// quietly reinterpret it and present a fallback asset as the declared icon.
	if _, legacy := values["tauri"]; legacy {
		s.issue(base, "scanPartial")
		return
	}
	evidence := []string{filepath.ToSlash(base)}
	for _, name := range rule.Options.PlatformConfigFiles[s.options.Platform] {
		if (strings.EqualFold(filepath.Ext(base), ".toml")) != strings.EqualFold(filepath.Ext(name), ".toml") {
			continue
		}
		overlay := localJoin(filepath.Dir(base), name)
		if overlay == "" {
			continue
		}
		raw := s.read(overlay)
		if raw == nil {
			continue
		}
		s.configUnits[overlay] = s.unitKey(base)
		s.processed[overlay] = true
		patch, valid := s.readTauriConfig(overlay, raw)
		if !valid {
			// The current platform may replace the base icon. An unreadable
			// patch cannot justify presenting the unpatched value as current.
			return
		}
		values = mergeTauriConfig(values, patch)
		unit.Sources = appendUnique(unit.Sources, filepath.ToSlash(overlay))
		evidence = append(evidence, filepath.ToSlash(overlay))
		break
	}
	if fieldString(values, []string{"identifier"}) == "" {
		s.issue(base, "scanInvalidManifest")
		return
	}
	names := rule
	names.Emit.Icons = nil
	s.applyMetadata(names, base, unit, values, "")
	start := len(s.proposals)
	for _, icon := range rule.Emit.Icons {
		if icon.Platform != "" && icon.Platform != s.options.Platform {
			continue
		}
		icons := rule
		icons.Emit.Names = nil
		icons.Emit.Icons = []IconRule{icon}
		if len(icon.Field) == 0 {
			s.applyMetadata(icons, base, unit, nil, "")
			continue
		}
		value := tauriField(values, icon.Field)
		if value == nil {
			continue
		}
		paths, valid := value.([]any)
		if !valid {
			s.issue(base, "scanPartial")
			continue
		}
		for i, candidate := range paths {
			if i >= 32 {
				s.issue(base, "scanLimit")
				break
			}
			path, valid := candidate.(string)
			if !valid || strings.TrimSpace(path) == "" {
				s.issue(base, "scanPartial")
				continue
			}
			// Reuse the common rule interpretation for each declared path. The
			// rule owns extension/platform preference, origin, and precedence.
			var scalar any = path
			for j := len(icon.Field) - 1; j >= 0; j-- {
				scalar = map[string]any{icon.Field[j]: scalar}
			}
			s.applyMetadata(icons, base, unit, scalar.(map[string]any), "")
		}
	}
	for i := start; i < len(s.proposals); i++ {
		s.proposals[i].evidence = append([]string{}, evidence...)
	}
	s.terminalDirs[filepath.Dir(base)] = true
}

func (s *inspector) readTauriConfig(path string, data []byte) (map[string]any, bool) {
	values := map[string]any{}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".json":
		if json.Unmarshal(data, &values) != nil {
			// Tauri can also enable JSON5 syntax in a .json file. This first
			// static adapter does not claim to support that Cargo feature.
			s.issue(path, "scanPartial")
			return nil, false
		}
	case ".toml":
		if toml.Unmarshal(data, &values) != nil {
			s.issue(path, "scanInvalidManifest")
			return nil, false
		}
	default:
		s.issue(path, "scanPartial")
		return nil, false
	}
	if values == nil {
		s.issue(path, "scanInvalidManifest")
		return nil, false
	}
	nodes := 0
	if !boundedTauriValue(values, 0, &nodes) {
		s.issue(path, "scanLimit")
		return nil, false
	}
	return values, true
}

func boundedTauriValue(value any, depth int, nodes *int) bool {
	*nodes++
	if depth > 64 || *nodes > 16384 {
		return false
	}
	switch value := value.(type) {
	case map[string]any:
		for _, child := range value {
			if !boundedTauriValue(child, depth+1, nodes) {
				return false
			}
		}
	case []any:
		for _, child := range value {
			if !boundedTauriValue(child, depth+1, nodes) {
				return false
			}
		}
	}
	return true
}

// RFC 7396: merge objects, remove null members, replace arrays and scalars. Both
// inputs have already passed the depth/node budget before recursive merging.
func mergeTauriConfig(base, patch map[string]any) map[string]any {
	if base == nil {
		base = map[string]any{}
	}
	for key, value := range patch {
		if value == nil {
			delete(base, key)
			continue
		}
		if object, ok := value.(map[string]any); ok {
			existing, _ := base[key].(map[string]any)
			base[key] = mergeTauriConfig(existing, object)
		} else {
			base[key] = value
		}
	}
	return base
}

func tauriField(values map[string]any, fields []string) any {
	var current any = values
	for _, field := range fields {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[field]
	}
	return current
}

// Saved source hints can reach a configuration before its package.json. Only
// an explicit rule-owned configuration reference can give it a package owner.
func (s *inspector) bindTauriPackage(path string) {
	if key := s.configUnits[path]; key != "" {
		packagePath := filepath.Join(filepath.FromSlash(key), "package.json")
		var values map[string]any
		if raw := s.read(packagePath); raw != nil && json.Unmarshal(raw, &values) == nil {
			s.tauriScriptDiagnostics(packagePath, values)
		}
		return
	}
	for directory, depth := filepath.Dir(path), 0; depth <= maxDepth; directory, depth = filepath.Dir(directory), depth+1 {
		packagePath := filepath.Join(directory, "package.json")
		var values map[string]any
		if raw := s.read(packagePath); raw != nil && json.Unmarshal(raw, &values) == nil {
			for _, rule := range s.registry.Match(filepath.ToSlash(packagePath)) {
				if rule.Resolver != "package-metadata-v1" || !rule.MatchesDependencies(values) {
					continue
				}
				for _, name := range rule.Options.ConfigFiles {
					if localJoin(directory, name) != path {
						continue
					}
					s.process(packagePath)
					if unit := s.units[s.unitKey(packagePath)]; unit != nil {
						s.configUnits[path] = s.unitKey(packagePath)
					}
					s.tauriScriptDiagnostics(packagePath, values)
					return
				}
			}
		}
		if directory == "." {
			return
		}
	}
}

func (s *inspector) bindTauriCargo(path string, unit *ProjectUnit) {
	nativeDirectory := filepath.Dir(path)
	key := s.unitKey(path)
	previousKey := filepath.ToSlash(nativeDirectory)
	if key == previousKey {
		return
	}
	cargo := filepath.Join(nativeDirectory, "Cargo.toml")
	if previous := s.units[previousKey]; previous != nil && previous.ID != unit.ID {
		for _, source := range previous.Sources {
			if source != filepath.ToSlash(cargo) {
				// A separately declared application shares this folder. Keep it
				// distinct rather than moving its identity under a parent package.
				s.issue(path, "scanPartial")
				return
			}
		}
		for _, source := range previous.Sources {
			unit.Sources = appendUnique(unit.Sources, source)
		}
		for _, technology := range previous.Technologies {
			unit.Technologies = appendUnique(unit.Technologies, technology)
		}
		for i := range s.proposals {
			if s.proposals[i].unit == previous.ID {
				s.proposals[i].unit = unit.ID
			}
		}
		delete(s.units, previousKey)
	}
	s.configUnits[cargo] = key
}

func (s *inspector) tauriScriptDiagnostics(path string, values map[string]any) {
	scripts, _ := values["scripts"].(map[string]any)
	for _, value := range scripts {
		text, _ := value.(string)
		if len(text) > 8000 {
			continue
		}
		tauri := false
		for _, token := range referenceTokens(text) {
			if token == "&" || token == ";" || token == "|" {
				tauri = false
				continue
			}
			tauri = tauri || token == "tauri"
			flag, _, _ := strings.Cut(token, "=")
			if tauri && (flag == "--config" || flag == "-c") {
				s.issue(path, "scanPartial")
			}
		}
	}
}
