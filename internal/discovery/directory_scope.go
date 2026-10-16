package discovery

import (
	"path"
	"path/filepath"
	"strings"
)

// Ignore files only control directory enumeration. Explicit configuration and
// icon references still use the confined reader, including ignored local files.
// Rules are relative to each visited .gitignore; nothing above the chosen root
// or from the machine's Git configuration is loaded.
type directoryScope struct {
	rules map[string][]directoryPattern
	count int
}

type directoryPattern struct {
	parts   []string
	include bool
}

func (s *directoryScope) load(directory string, data []byte, issue func(string, string)) {
	if data == nil {
		return
	}
	file := filepath.Join(directory, ".gitignore")
	if len(data) > 64<<10 || s.count >= 2048 {
		issue(file, "scanLimit")
		return
	}
	if s.rules == nil {
		s.rules = map[string][]directoryPattern{}
	}
	lines := strings.Split(strings.TrimPrefix(string(data), "\ufeff"), "\n")
	for _, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		// Git keeps an escaped trailing space.
		for strings.HasSuffix(line, " ") {
			slashes := 0
			for i := len(line) - 2; i >= 0 && line[i] == '\\'; i-- {
				slashes++
			}
			if slashes%2 != 0 {
				break
			}
			line = strings.TrimSuffix(line, " ")
		}
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(line) > 1024 {
			issue(file, "scanLimit")
			continue
		}
		if strings.Contains(line, "[[:") || strings.Contains(line, "[[.") || strings.Contains(line, "[[=") {
			issue(file, "scanPartial")
			continue
		}
		include := strings.HasPrefix(line, "!")
		if include {
			line = line[1:]
		}
		line = strings.TrimSuffix(line, "/")
		if line == "" {
			continue
		}
		anchored := strings.Contains(line, "/")
		line = strings.TrimPrefix(line, "/")
		parts := strings.Split(line, "/")
		valid := len(parts) <= 64
		for i, part := range parts {
			// path.Match uses ^ for a negated class; Git also accepts !.
			var normalized strings.Builder
			for j := 0; j < len(part); j++ {
				normalized.WriteByte(part[j])
				if part[j] == '\\' && j+1 < len(part) {
					j++
					normalized.WriteByte(part[j])
				} else if part[j] == '[' && j+1 < len(part) && part[j+1] == '!' {
					j++
					normalized.WriteByte('^')
				}
			}
			part = normalized.String()
			parts[i] = part
			if _, err := path.Match(part, ""); err != nil {
				valid = false
			}
		}
		if !valid {
			issue(file, "scanPartial")
			continue
		}
		if len(s.rules[directory]) >= 256 || s.count >= 2048 {
			issue(file, "scanLimit")
			break
		}
		if !anchored {
			parts = append([]string{"**"}, parts...)
		}
		s.rules[directory] = append(s.rules[directory], directoryPattern{parts, include})
		s.count++
	}
}

func (s *directoryScope) excluded(directory string) bool {
	parts := strings.Split(filepath.ToSlash(directory), "/")
	excluded := false
	for depth := 0; depth < len(parts); depth++ {
		base := "."
		if depth > 0 {
			base = filepath.FromSlash(strings.Join(parts[:depth], "/"))
		}
		for _, rule := range s.rules[base] {
			if matchDirectory(rule.parts, parts[depth:]) {
				excluded = !rule.include
			}
		}
	}
	return excluded
}

// Dynamic programming bounds ** matching even for adversarial ignore patterns.
func matchDirectory(pattern, parts []string) bool {
	matched := make([]bool, len(parts)+1)
	matched[0] = true
	for index, token := range pattern {
		next := make([]bool, len(parts)+1)
		for i := 0; i <= len(parts); i++ {
			if token == "**" {
				if index > 0 && index == len(pattern)-1 {
					// a/** excludes a's contents, not a itself. This leaves a
					// traversable for a later !a/active exception.
					next[i] = i > 0 && (matched[i-1] || next[i-1])
				} else {
					next[i] = matched[i] || (i > 0 && next[i-1])
				}
			} else if i > 0 && matched[i-1] {
				next[i], _ = path.Match(token, parts[i-1])
			}
		}
		matched = next
	}
	return matched[len(parts)]
}
