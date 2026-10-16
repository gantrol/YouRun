package discovery

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
)

func (s *inspector) nodeMetadata(path string, data []byte, rule Rule) {
	var values map[string]any
	if json.Unmarshal(data, &values) != nil || values == nil {
		s.issue(path, "scanInvalidManifest")
		return
	}
	// A recognized package bounds metadata discovery. Child applications still
	// enter through workspace/script references and saved-command hints; icons
	// use declared paths and the bounded asset rules, not a source-tree crawl.
	s.terminalDirs[filepath.Dir(path)] = true
	u := s.unit(path, rule)
	s.applyMetadata(rule, path, u, values, "")
	if scripts, ok := values["scripts"].(map[string]any); ok {
		names := make([]string, 0, len(scripts))
		for name := range scripts {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			text, _ := scripts[name].(string)
			if len(text) > 8000 {
				s.issue(path, "scanLimit")
				continue
			}
			s.scriptReferences(filepath.Dir(path), text, filepath.ToSlash(path)+"#scripts."+name)
		}
	}
	workspaces := values["workspaces"]
	if object, ok := workspaces.(map[string]any); ok {
		workspaces = object["packages"]
	}
	if patterns, ok := workspaces.([]any); ok {
		for i, pattern := range patterns {
			if i >= 128 {
				s.issue(path, "scanLimit")
				break
			}
			if value, ok := pattern.(string); ok {
				s.workspaceReference(filepath.Dir(path), value, filepath.ToSlash(path)+"#workspaces")
			}
		}
	}
	s.packageWebManifests(filepath.Dir(path))
}

// These tokens are for literal path hints only, not shell execution. Reject
// variable expansion and substitutions instead of attempting to evaluate them.
func referenceTokens(text string) []string {
	if strings.ContainsAny(text, "`\x00\r\n") {
		return nil
	}
	tokens := []string{}
	var token strings.Builder
	quote := rune(0)
	flush := func() {
		if token.Len() > 0 {
			tokens = append(tokens, token.String())
			token.Reset()
		}
	}
	for _, r := range text {
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				token.WriteRune(r)
			}
			continue
		}
		switch r {
		case '\'', '"':
			quote = r
		case ' ', '\t':
			flush()
		case '&', ';', '|':
			flush()
			tokens = append(tokens, string(r))
		default:
			token.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil
	}
	flush()
	return tokens
}

func (s *inspector) scriptReferences(base, text, evidence string) {
	tokens := referenceTokens(text)
	dir := base
	for i := 0; i < len(tokens); i++ {
		token := tokens[i]
		if token == "cd" && (i == 0 || tokens[i-1] == "&" || tokens[i-1] == ";") {
			j := i + 1
			if j < len(tokens) && strings.EqualFold(tokens[j], "/d") {
				j++
			}
			if j < len(tokens) {
				if next := localJoin(dir, tokens[j]); next != "" {
					dir = next
					s.reference(next, evidence, false)
				}
			}
		}
		if token == "npm" || token == "pnpm" || token == "yarn" || token == "bun" {
			for j := i + 1; j < len(tokens) && tokens[j] != "&" && tokens[j] != ";" && tokens[j] != "|"; j++ {
				flag, value, hasValue := strings.Cut(tokens[j], "=")
				if flag != "--prefix" && flag != "--dir" && flag != "--cwd" && flag != "-C" {
					continue
				}
				if !hasValue && j+1 < len(tokens) {
					j++
					value = tokens[j]
				}
				if next := localJoin(dir, value); next != "" {
					s.reference(next, evidence, false)
				}
			}
		}
		if token == "dotnet" {
			for j := i + 1; j < len(tokens) && tokens[j] != "&" && tokens[j] != ";" && tokens[j] != "|"; j++ {
				value := tokens[j]
				if strings.HasPrefix(value, "--project=") {
					value = strings.TrimPrefix(value, "--project=")
				}
				ext := strings.ToLower(filepath.Ext(value))
				if ext != ".csproj" && ext != ".fsproj" && ext != ".vbproj" && ext != ".sln" && ext != ".slnx" {
					continue
				}
				if next := localJoin(dir, value); next != "" {
					s.reference(next, evidence, true)
				}
			}
		}
	}
}

func (s *inspector) reference(path, evidence string, file bool) {
	if len(s.references) >= 256 {
		s.issue(path, "scanLimit")
		return
	}
	// A missing or linked hint must not hide unrelated children. Only actual
	// child applications scope the scan to a workspace's declared members.
	info := s.safeInfo(path)
	if info == nil {
		return
	}
	if path != "." && (!file || filepath.Dir(path) != ".") {
		s.scoped = true
	}
	s.references[path] = appendUnique(s.references[path], evidence)
	if file {
		s.process(path)
		s.enqueue(filepath.Dir(path), 0, true)
	} else {
		s.enqueue(path, 0, true)
	}
}

func (s *inspector) workspaceReference(base, pattern, evidence string) {
	if strings.ContainsAny(pattern, "!?[{") || strings.Count(pattern, "*") > 1 {
		s.issue(evidence, "scanPartial")
		return
	}
	if !strings.Contains(pattern, "*") {
		if dir := localJoin(base, pattern); dir != "" {
			s.reference(dir, evidence, false)
		}
		return
	}
	if !strings.HasSuffix(pattern, "/*") {
		s.issue(evidence, "scanPartial")
		return
	}
	dir := localJoin(base, strings.TrimSuffix(pattern, "/*"))
	if dir == "" {
		return
	}
	info := s.safeInfo(dir)
	if info == nil || !info.IsDir() {
		return
	}
	s.dependencies[dir] = true
	f, err := s.root.Open(dir)
	if err != nil {
		return
	}
	entries, _ := f.ReadDir(129)
	f.Close()
	if len(entries) > 128 {
		s.issue(dir, "scanLimit")
		entries = entries[:128]
	}
	for _, entry := range entries {
		if entry.IsDir() && !ignored(entry.Name()) {
			s.reference(filepath.Join(dir, entry.Name()), evidence, false)
		}
	}
}
