package discovery

import (
	"encoding/json"
	"net/url"
	"os"
	urlpath "path"
	"path/filepath"
	"strings"
)

// Keep declared web icons discoverable without traversing a package's sources.
// Only immediate manifest files in the rule's serving roots are inspected.
func (s *inspector) packageWebManifests(base string) {
	visited := map[string]bool{}
	for _, rule := range s.registry.Rules() {
		if rule.Resolver != "web-manifest-v1" {
			continue
		}
		for _, name := range rule.Options.WebRoots {
			directory := localJoin(base, name)
			if directory == "" || visited[directory] || ignored(name) || s.scope.excluded(directory) {
				continue
			}
			visited[directory] = true
			for _, entry := range s.readDirectory(directory) {
				path := filepath.Join(directory, entry.Name())
				if forbidden(path) {
					continue
				}
				for _, matched := range s.registry.Match(filepath.ToSlash(path)) {
					if matched.Resolver != "web-manifest-v1" {
						continue
					}
					if entry.Type()&os.ModeSymlink != 0 {
						s.issue(path, "scanLinkSkipped")
					} else if entry.Type().IsRegular() {
						s.process(path)
					}
					break
				}
			}
		}
	}
}

// Web manifest icon src values are URLs, not filesystem paths. Relative URLs
// use the manifest directory; root URLs require a known local serving root.
func (s *inspector) webManifestMetadata(path string, data []byte, rule Rule) {
	var values map[string]any
	if json.Unmarshal(data, &values) != nil || values == nil {
		s.issue(path, "scanInvalidManifest")
		return
	}
	icons, ok := values["icons"].([]any)
	if !ok || len(icons) == 0 {
		// Many unrelated tools use manifest.json. An empty manifest is not
		// sufficient evidence to create another application in the workspace.
		return
	}
	unit := s.webManifestUnit(path, rule)
	namesOnly := rule
	namesOnly.Emit.Icons = nil
	s.applyMetadata(namesOnly, path, unit, values, "")
	iconsOnly := rule
	iconsOnly.Emit.Names = nil
	for i, entry := range icons {
		if i >= 32 {
			s.issue(path, "scanLimit")
			break
		}
		icon, ok := entry.(map[string]any)
		if !ok {
			s.issue(path, "scanPartial")
			continue
		}
		source, _ := icon["src"].(string)
		asset := s.webManifestAsset(path, source, rule.Options.WebRoots)
		if asset == "" {
			s.issue(path, "scanPartial")
			continue
		}
		// applyMetadata retains the actual manifest as evidence while using
		// the normalized URL's path relative to that same manifest.
		relative, err := filepath.Rel(filepath.Dir(path), asset)
		if err != nil {
			s.issue(path, "scanPartial")
			continue
		}
		icon["src"] = filepath.ToSlash(relative)
		s.applyMetadata(iconsOnly, path, unit, icon, "")
	}
}

func (s *inspector) webManifestUnit(path string, rule Rule) *ProjectUnit {
	// A manifest in public/ belongs to its nearest package, rather than
	// appearing as a second application beside that package's fallback icon.
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		key := filepath.ToSlash(dir)
		unit := s.units[key]
		if unit == nil {
			packagePath := filepath.Join(dir, "package.json")
			if info := s.safeInfo(packagePath); info != nil && info.Mode().IsRegular() {
				s.process(packagePath)
				unit = s.units[key]
			}
		}
		if unit != nil {
			unit.Sources = appendUnique(unit.Sources, filepath.ToSlash(path))
			for _, framework := range rule.Emit.Frameworks {
				unit.Technologies = appendUnique(unit.Technologies, framework)
			}
			return unit
		}
		if dir == "." {
			return s.unit(path, rule)
		}
	}
}

func (s *inspector) webManifestAsset(manifest, source string, webRoots []string) string {
	if source == "" || len(source) > 4096 || strings.ContainsAny(source, "\\\x00\r\n") {
		return ""
	}
	parsed, err := url.Parse(source)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.User != nil || parsed.Opaque != "" || parsed.Path == "" {
		return ""
	}
	// url.Parse has already decoded percent escapes. localJoin then enforces
	// the selected directory boundary, including encoded parent segments.
	base := filepath.Dir(manifest)
	for dir := base; ; dir = filepath.Dir(dir) {
		directoryName := filepath.Base(dir)
		if dir == "." {
			directoryName = filepath.Base(s.result.Root)
		}
		for _, known := range webRoots {
			if !strings.EqualFold(directoryName, known) {
				continue
			}
			relative, err := filepath.Rel(dir, base)
			if err != nil {
				return ""
			}
			resolved := parsed.Path
			if !strings.HasPrefix(resolved, "/") {
				resolved = "/" + filepath.ToSlash(relative) + "/" + resolved
			}
			// URL parent segments cannot climb above a confirmed serving root.
			return localJoin(dir, strings.TrimPrefix(urlpath.Clean(resolved), "/"))
		}
		if dir == "." {
			break
		}
	}
	if strings.HasPrefix(parsed.Path, "/") {
		return ""
	}
	return localJoin(base, parsed.Path)
}
