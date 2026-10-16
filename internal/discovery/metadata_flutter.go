package discovery

import (
	"bytes"
	"io"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

func (s *inspector) flutterMetadata(path string, data []byte, rule Rule) {
	pubspec := filepath.Join(filepath.Dir(path), "pubspec.yaml")
	if path != pubspec {
		// A launcher-icons config alone is not evidence of a Flutter app.
		s.process(pubspec)
		return
	}
	values := s.staticYAMLValues(path, data)
	if values == nil || fieldString(values, []string{"dependencies", "flutter", "sdk"}) != "flutter" {
		return
	}
	unit := s.unit(path, rule)
	sources := []string{filepath.ToSlash(path)}
	for _, name := range rule.Options.ConfigFiles {
		config := localJoin(filepath.Dir(path), name)
		if config == "" || config == path {
			continue
		}
		if raw := s.read(config); raw != nil {
			configured := s.staticYAMLValues(config, raw)
			if configured == nil {
				return
			}
			// flutter_launcher_icons' separate default config replaces its
			// pubspec section; it does not merge alternate flavors or run tools.
			values["flutter_launcher_icons"] = configured["flutter_launcher_icons"]
			unit.Sources = appendUnique(unit.Sources, filepath.ToSlash(config))
			sources = append(sources, filepath.ToSlash(config))
			s.processed[config] = true
			break
		}
	}
	if launcher, ok := values["flutter_launcher_icons"].(map[string]any); ok {
		for _, platform := range []string{"windows", "macos", "web"} {
			if config, ok := launcher[platform].(map[string]any); ok && config["generate"] != true {
				delete(launcher, platform)
			}
		}
		if launcher["android"] == false {
			delete(launcher, "image_path_android")
		}
		if launcher["ios"] == false {
			delete(launcher, "image_path_ios")
		}
	}
	start := len(s.proposals)
	s.applyMetadata(rule, path, unit, values, "")
	for i := start; i < len(s.proposals); i++ {
		s.proposals[i].evidence = append([]string{}, sources...)
	}
}

func (s *inspector) staticYAMLValues(path string, data []byte) map[string]any {
	var document yaml.Node
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	if decoder.Decode(&document) != nil {
		s.issue(path, "scanInvalidManifest")
		return nil
	}
	nodes := 0
	if !staticYAML(&document, 0, &nodes) {
		s.issue(path, "scanPartial")
		return nil
	}
	var values map[string]any
	var extra yaml.Node
	if document.Decode(&values) != nil || values == nil || decoder.Decode(&extra) != io.EOF {
		s.issue(path, "scanInvalidManifest")
		return nil
	}
	return values
}
