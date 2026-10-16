package discovery

import (
	"bytes"
	"path/filepath"
	"strings"
)

// A conventional filename proposes an invocation, not proof of its effects or
// required arguments. Keep it reviewable and never evaluate its contents.
func (s *scan) projectScript(path string, data []byte, rule Rule) {
	dir := filepath.Dir(path)
	if !s.projectDirs[dir] || len(bytes.TrimSpace(data)) == 0 || bytes.IndexByte(data, 0) >= 0 {
		return
	}
	file := filepath.Base(path)
	if !safeArg.MatchString(file) {
		return
	}
	key := "windows"
	switch strings.ToLower(filepath.Ext(file)) {
	case ".cmd", ".bat", ".ps1":
	case ".sh":
		first, _, _ := strings.Cut(string(data), "\n")
		first = strings.TrimSpace(strings.TrimPrefix(first, "\ufeff"))
		if strings.HasPrefix(first, "#!") {
			interpreter := strings.Fields(strings.TrimSpace(strings.TrimPrefix(first, "#!")))
			if len(interpreter) > 0 && filepath.Base(interpreter[0]) == "env" {
				interpreter = interpreter[1:]
			}
			if len(interpreter) != 1 || (filepath.Base(interpreter[0]) != "sh" && filepath.Base(interpreter[0]) != "bash") {
				s.issue(path, "scanPartial")
				return
			}
		}
		key = "shell"
	default:
		return
	}
	before := len(s.result.Candidates)
	s.addRuleCommand(rule, key, path, dir, map[string]string{"script": "./" + file}, "", true)
	if len(s.result.Candidates) > before {
		item := &s.result.Candidates[len(s.result.Candidates)-1]
		item.Title = file
		if key == "windows" {
			item.Shell = "PowerShell"
		}
	}
}
