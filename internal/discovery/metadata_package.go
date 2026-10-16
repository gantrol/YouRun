package discovery

import (
	"encoding/json"
	"path/filepath"
	"strings"
)

// Frameworks share package parsing and rule predicates. Adding another package
// dependency or conventional asset path does not require a resolver branch.
func (s *inspector) packageMetadata(path string, data []byte, rule Rule) {
	if !strings.EqualFold(filepath.Base(path), "package.json") {
		return
	}
	if s.packageValues == nil {
		s.packageValues = map[string]map[string]any{}
	}
	values, cached := s.packageValues[path]
	if !cached {
		if json.Unmarshal(data, &values) != nil || values == nil {
			s.issue(path, "scanInvalidManifest")
			values = nil
		}
		s.packageValues[path] = values
	}
	if values == nil || !rule.MatchesDependencies(values) {
		return
	}
	unit := s.unit(path, rule)
	s.applyMetadata(rule, path, unit, values, "")
	for _, config := range rule.Options.ConfigFiles {
		resolved := localJoin(filepath.Dir(path), config)
		if resolved == "" || resolved == path {
			continue
		}
		// Also track absent files, so creating a configured manifest invalidates
		// the host cache without requiring the user to re-import the project.
		if s.read(resolved) == nil {
			continue
		}
		key := s.unitKey(path)
		if previous := s.configUnits[resolved]; previous != "" && previous != key {
			s.issue(resolved, "scanPartial")
			continue
		}
		s.configUnits[resolved] = key
		s.references[resolved] = appendUnique(s.references[resolved], filepath.ToSlash(path)+"#dependencies")
		s.process(resolved)
	}
}

// A package rule can explicitly associate a native config with its frontend.
// Otherwise the configuration remains an independent unit in its own directory.
func (s *inspector) tauriUnit(path string, rule Rule) *ProjectUnit {
	return s.unit(path, rule)
}
