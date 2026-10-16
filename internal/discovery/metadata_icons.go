package discovery

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

// Directory rules are a lazy fallback for dedicated branding/icon folders.
// They inspect immediate files only, using the metadata walk's cached entries,
// ignore rules, root boundary and shared directory/read budgets.
func (s *inspector) iconDirectoryPaths(proposal iconProposal) []string {
	entries := s.iconDirectoryEntries(proposal)
	paths := []string{}
	for _, entry := range entries {
		if forbidden(entry.Name()) || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(proposal.path, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 {
			s.issue(path, "scanLinkSkipped")
			continue
		}
		if !entry.Type().IsRegular() || s.scope.excluded(path) {
			continue
		}
		extension := strings.ToLower(filepath.Ext(path))
		if !containsString([]string{".svg", ".png", ".ico", ".icns", ".jpg", ".jpeg", ".gif"}, extension) ||
			(len(proposal.extensions) > 0 && !containsString(proposal.extensions, extension)) {
			continue
		}
		paths = append(paths, path)
	}
	sort.Slice(paths, func(i, j int) bool {
		first, second := iconFilenamePriority(paths[i]), iconFilenamePriority(paths[j])
		if first != second {
			return first > second
		}
		firstName, secondName := strings.ToLower(paths[i]), strings.ToLower(paths[j])
		if firstName != secondName {
			return firstName < secondName
		}
		return paths[i] < paths[j]
	})
	return paths
}

func (s *inspector) iconDirectoryEntries(proposal iconProposal) []os.DirEntry {
	parts := strings.Split(proposal.path, string(filepath.Separator))
	if len(parts) > maxDepth {
		s.issue(proposal.path, "scanDepthLimit")
		return nil
	}
	unitDirectory := "."
	for _, unit := range s.result.Units {
		if unit.ID == proposal.unit {
			unitDirectory = filepath.FromSlash(unit.Directory)
			break
		}
	}
	var entries []os.DirEntry
	for i, part := range parts {
		directory := filepath.Join(parts[:i+1]...)
		if s.ctx.Err() != nil || (part != "." && (ignored(part) || strings.EqualFold(part, "bin"))) || s.scope.excluded(directory) {
			return nil
		}
		// Do not borrow the assets of a nested, separately recognized project.
		relative, err := filepath.Rel(unitDirectory, directory)
		if err == nil && relative != "." && filepath.IsLocal(relative) {
			for _, unit := range s.result.Units {
				if unit.ID != proposal.unit && filepath.FromSlash(unit.Directory) == directory {
					return nil
				}
			}
		}
		entries = s.readDirectory(directory)
		if entries == nil {
			return nil
		}
	}
	return entries
}

// Search is the last resort and uses the same cached, root-confined reads as
// metadata discovery. Candidate names are filtered before any image is read.
func (s *inspector) iconSearchPaths(proposal iconProposal) []string {
	for _, unit := range s.result.Units {
		if unit.ID != proposal.unit && filepath.FromSlash(unit.Directory) == proposal.path {
			return nil // Shared source directories cannot identify an application's icon.
		}
	}
	queue := []string{proposal.path}
	paths := []string{}
	for index := 0; index < len(queue) && s.ctx.Err() == nil; index++ {
		directory := queue[index]
		current := proposal
		current.path = directory
		entries := s.iconDirectoryEntries(current)
		if directory != proposal.path && iconSearchProject(entries) {
			continue
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, path := range s.iconDirectoryPaths(current) {
			ext := filepath.Ext(path)
			if !strings.EqualFold(ext, ".ico") && !strings.EqualFold(ext, ".icns") && iconFilenamePriority(path) == 0 && !iconAssetDirectory(filepath.Dir(path)) {
				continue
			}
			if len(paths) >= 128 {
				s.issue(proposal.path, "scanLimit")
				break
			}
			paths = append(paths, path)
		}
		for _, entry := range entries {
			if !entry.IsDir() || entry.Type()&os.ModeSymlink != 0 || ignored(entry.Name()) || iconSearchIgnored(entry.Name()) {
				continue
			}
			next := filepath.Join(directory, entry.Name())
			if s.scope.excluded(next) {
				continue
			}
			if len(queue) >= 64 {
				s.issue(proposal.path, "scanDirectoryLimit")
				break
			}
			queue = append(queue, next)
		}
	}
	sort.SliceStable(paths, func(i, j int) bool {
		if s.options.Platform == "windows" {
			firstICO, secondICO := strings.EqualFold(filepath.Ext(paths[i]), ".ico"), strings.EqualFold(filepath.Ext(paths[j]), ".ico")
			if firstICO != secondICO {
				return firstICO
			}
		} else if s.options.Platform == "darwin" {
			firstICNS, secondICNS := strings.EqualFold(filepath.Ext(paths[i]), ".icns"), strings.EqualFold(filepath.Ext(paths[j]), ".icns")
			if firstICNS != secondICNS {
				return firstICNS
			}
		}
		first, second := iconFilenamePriority(paths[i]), iconFilenamePriority(paths[j])
		if first != second {
			return first > second
		}
		firstDepth, secondDepth := strings.Count(paths[i], string(filepath.Separator)), strings.Count(paths[j], string(filepath.Separator))
		if firstDepth != secondDepth {
			return firstDepth < secondDepth
		}
		return paths[i] < paths[j]
	})
	return paths
}

func iconSearchIgnored(name string) bool {
	switch strings.ToLower(name) {
	case "bin", "out", "output", "outputs", "release", "releases", "venv", "env", "site-packages",
		"docs", "doc", "documentation", "test", "tests", "fixtures", "examples", "samples", "third_party", "third-party", "external":
		return true
	}
	return false
}

func iconAssetDirectory(path string) bool {
	switch strings.ToLower(filepath.Base(path)) {
	case "icon", "icons", "logo", "logos", "brand", "branding":
		return true
	}
	return false
}

// A package not followed by the metadata walk still forms a search boundary.
func iconSearchProject(entries []os.DirEntry) bool {
	for _, entry := range entries {
		if !entry.Type().IsRegular() || forbidden(entry.Name()) {
			continue
		}
		name := strings.ToLower(entry.Name())
		switch name {
		case "package.json", "pyproject.toml", "go.mod", "cargo.toml", "package.swift", "pubspec.yaml", "cmakelists.txt":
			return true
		}
		switch filepath.Ext(name) {
		case ".csproj", ".fsproj", ".vbproj", ".vcxproj":
			return true
		}
	}
	return false
}

// Semantic filename tokens rank variants without requiring a particular brand
// name. Unlabelled files remain candidates in these dedicated asset folders.
func iconFilenamePriority(path string) int {
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	words := strings.FieldsFunc(strings.ToLower(name), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	priority := 0
	for _, word := range words {
		switch word {
		case "icon", "appicon", "app", "application":
			priority = max(priority, 3)
		case "logo":
			priority = max(priority, 2)
		case "favicon":
			priority = max(priority, 1)
		}
	}
	return priority
}
