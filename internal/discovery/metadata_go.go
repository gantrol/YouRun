package discovery

import (
	"path/filepath"

	"golang.org/x/mod/modfile"
)

// Module declarations identify Go frameworks without importing or running Go
// code. Only direct requirements count; transitive tools are not app evidence.
func (s *inspector) goModuleMetadata(path string, data []byte, rule Rule) {
	parsed, err := modfile.Parse(path, data, nil)
	if err != nil || parsed.Module == nil {
		s.issue(path, "scanInvalidManifest")
		return
	}
	dependencies := map[string]any{}
	for _, required := range parsed.Require {
		if !required.Indirect {
			dependencies[required.Mod.Path] = required.Mod.Version
		}
	}
	values := map[string]any{"dependencies": dependencies, "module": parsed.Module.Mod.Path}
	if !rule.MatchesDependencies(values) {
		return
	}
	unit := s.unit(path, rule)
	s.applyMetadata(rule, path, unit, values, "")
	for _, name := range rule.Options.ConfigFiles {
		config := localJoin(filepath.Dir(path), name)
		if config == "" || s.read(config) == nil {
			continue
		}
		s.configUnits[config] = s.unitKey(path)
		s.process(config)
	}
}
