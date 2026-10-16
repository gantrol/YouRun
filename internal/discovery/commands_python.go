package discovery

import (
	"bufio"
	"bytes"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// These commands describe declared entry points. Discovery does not import the
// Python module, install the project, select an interpreter, or activate a venv.
func (s *scan) pythonCommands(path string, data []byte, rule Rule) error {
	switch strings.ToLower(filepath.Base(path)) {
	case "pyproject.toml":
		return s.pythonProjectCommands(path, data, rule)
	case "manage.py":
		s.pythonDjangoCommands(path, data, rule)
	case ".flaskenv":
		s.pythonFlaskCommands(path, data, rule)
	}
	return nil
}

var pythonCommandName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.-]{0,127}$`)
var pythonObjectReference = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*(?::[A-Za-z_][A-Za-z0-9_]*(?:\.[A-Za-z_][A-Za-z0-9_]*)*)?$`)
var pythonEntryExtras = regexp.MustCompile(`\s*\[[A-Za-z0-9_.-]+(?:\s*,\s*[A-Za-z0-9_.-]+)*\]\s*$`)

func pythonCallableReference(value string) bool {
	value = strings.TrimSpace(value)
	value = pythonEntryExtras.ReplaceAllString(value, "")
	module, target, found := strings.Cut(value, ":")
	return found && pythonObjectReference.MatchString(strings.TrimSpace(module)) &&
		!strings.Contains(module, ":") && pythonObjectReference.MatchString(strings.TrimSpace(target)) && !strings.Contains(target, ":")
}

func pythonTable(values map[string]any, fields ...string) map[string]any {
	for _, field := range fields {
		values, _ = values[field].(map[string]any)
	}
	return values
}

func (s *scan) pythonProjectCommands(path string, data []byte, rule Rule) error {
	var values map[string]any
	if err := toml.Unmarshal(data, &values); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	project := pythonTable(values, "project")
	poetry := pythonTable(values, "tool", "poetry")
	build := pythonTable(values, "build-system")
	backend, _ := build["build-backend"].(string)
	poetryBackend := backend == "poetry.core.masonry.api" || backend == "poetry.masonry.api"
	poetryProject := poetry != nil || poetryBackend || s.entries[filepath.Join(dir, "poetry.lock")]
	uv := pythonTable(values, "tool", "uv")
	uvProject := uv != nil || s.entries[filepath.Join(dir, "uv.lock")]
	scriptRunner := "script"
	if uvProject {
		scriptRunner = "uv-script"
	} else if poetryProject {
		scriptRunner = "poetry-script"
	}
	packageDisabled := poetry["package-mode"] == false
	uvPackageDisabled := scriptRunner == "uv-script" && (uv["package"] == false || build == nil && uv["package"] != true)
	_, plainScripts := rule.Commands["script"]
	_, poetryScripts := rule.Commands["poetry-script"]
	_, uvScripts := rule.Commands["uv-script"]
	if plainScripts || poetryScripts || uvScripts {
		dynamicScripts := false
		if dynamic, ok := project["dynamic"].([]any); ok {
			for _, field := range dynamic {
				if field == "scripts" {
					dynamicScripts = true
					s.issue(path, "scanPartial")
				}
			}
		}
		declared := pythonTable(project, "scripts")
		if !dynamicScripts && !packageDisabled && !uvPackageDisabled {
			s.pythonScriptCommands(path, dir, rule, declared, scriptRunner, false)
		}
		if uvPackageDisabled && len(declared) > 0 {
			s.issue(path, "scanPartial")
		}
		legacy := pythonTable(poetry, "scripts")
		if packageDisabled && (len(declared) > 0 || len(legacy) > 0) {
			s.issue(path, "scanPartial")
		}
		if !packageDisabled {
			remaining := map[string]any{}
			for name, entry := range legacy {
				if _, exists := declared[name]; !exists {
					remaining[name] = entry
				}
			}
			s.pythonScriptCommands(path, dir, rule, remaining, "poetry-script", true)
		}
	}
	if build != nil && !packageDisabled {
		requires, valid := build["requires"].([]any)
		valid = valid && len(requires) <= 256
		for _, requirement := range requires[:min(len(requires), 256)] {
			text, ok := requirement.(string)
			valid = valid && ok && strings.TrimSpace(text) != "" && len(text) <= 8000
		}
		if declared, exists := build["build-backend"]; exists {
			text, ok := declared.(string)
			valid = valid && ok && pythonObjectReference.MatchString(text)
		}
		if valid {
			key := "build"
			if poetryBackend {
				key = "poetry-build"
			}
			s.addRuleCommand(rule, key, path, dir, nil, "", true)
		} else {
			s.issue(path, "scanPartial")
		}
	}
	if fastapi := pythonTable(values, "tool", "fastapi"); fastapi != nil {
		if raw, declared := fastapi["entrypoint"]; declared {
			entrypoint, ok := raw.(string)
			entrypoint = strings.TrimSpace(entrypoint)
			if !ok || len(entrypoint) > 512 || !pythonObjectReference.MatchString(entrypoint) || !strings.Contains(entrypoint, ":") {
				s.issue(path, "scanPartial")
			} else {
				variables := map[string]string{"entrypoint": entrypoint}
				s.addRuleCommand(rule, "fastapi-dev", path, dir, variables, "", true)
				s.addRuleCommand(rule, "fastapi-run", path, dir, variables, "", true)
			}
		}
	}
	return nil
}

func (s *scan) pythonScriptCommands(path, dir string, rule Rule, scripts map[string]any, key string, legacy bool) {
	names := make([]string, 0, len(scripts))
	for name := range scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	for index, name := range names {
		if index >= 128 {
			s.issue(path, "scanLimit")
			break
		}
		if !pythonCommandName.MatchString(name) || key == "script" && pythonShellWord(name) {
			s.issue(path, "scanPartial")
			continue
		}
		valid := false
		switch declared := scripts[name].(type) {
		case string:
			valid = len(declared) <= 1024 && pythonCallableReference(declared)
		case map[string]any:
			if legacy {
				reference, _ := declared["reference"].(string)
				switch declared["type"] {
				case "console":
					valid = len(reference) <= 1024 && pythonCallableReference(reference)
				case "file":
					file := localJoin(dir, reference)
					valid = file != "" && s.entries[file]
				}
			}
		}
		if !valid {
			s.issue(path, "scanPartial")
			continue
		}
		s.addRuleCommand(rule, key, path, dir, map[string]string{"scriptName": name}, "", true)
	}
}

// A bare command is resolved by the selected shell. Built-ins and syntax words
// cannot be treated as an installed console script; environment runners avoid
// this ambiguity and remain usable for these otherwise valid script names.
func pythonShellWord(name string) bool {
	switch strings.ToLower(name) {
	case "if", "then", "else", "elif", "fi", "for", "foreach", "while", "until", "do", "done", "case", "esac", "select", "function", "in", "switch", "try", "catch", "finally", "trap", "return", "break", "continue", "exit", "throw", "param", "class", "enum", "using", "begin", "process", "end", "filter", "data", "configuration", "workflow", "cd", "pwd", "echo", "printf", "read", "set", "unset", "export", "source", "eval", "exec", "alias", "unalias", "type", "command", "builtin", "enable", "local", "declare", "typeset", "readonly", "shift", "wait", "jobs", "fg", "bg", "kill", "true", "false", "pushd", "popd", "dirs", "history", "help", "get-command", "invoke-expression", "iex":
		return true
	}
	return false
}

var djangoFromImport = regexp.MustCompile(`(?m)^[\t ]*from[\t ]+django\.core\.management[\t ]+import[\t ]+execute_from_command_line(?:[\t ]+as[\t ]+([A-Za-z_][A-Za-z0-9_]*))?[\t ]*$`)
var djangoModuleImport = regexp.MustCompile(`(?m)^[\t ]*import[\t ]+django\.core\.management(?:[\t ]+as[\t ]+([A-Za-z_][A-Za-z0-9_]*))?[\t ]*$`)

func (s *scan) pythonDjangoCommands(path string, data []byte, rule Rule) {
	code, valid := pythonCodeWithoutLiterals(data)
	if !valid {
		s.issue(path, "scanPartial")
		return
	}
	callables := []string{}
	for _, match := range djangoFromImport.FindAllStringSubmatch(code, 33) {
		name := match[1]
		if name == "" {
			name = "execute_from_command_line"
		}
		callables = append(callables, name)
	}
	for _, match := range djangoModuleImport.FindAllStringSubmatch(code, 33) {
		name := match[1]
		if name == "" {
			name = "django.core.management"
		}
		callables = append(callables, name+".execute_from_command_line")
	}
	if len(callables) > 32 {
		s.issue(path, "scanLimit")
		return
	}
	for _, name := range callables {
		call := regexp.MustCompile(`(?:^|[^A-Za-z0-9_.])` + regexp.QuoteMeta(name) + `[\t ]*\(`)
		if call.MatchString(code) {
			variables := map[string]string{"manageFile": filepath.Base(path)}
			s.addRuleCommand(rule, "django-runserver", path, filepath.Dir(path), variables, "", true)
			s.addRuleCommand(rule, "django-check", path, filepath.Dir(path), variables, "", true)
			return
		}
	}
}

// Mask comments and all quoted strings, including docstrings. Text describing a
// manage.py example is not evidence that the file imports or invokes Django.
func pythonCodeWithoutLiterals(data []byte) (string, bool) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	code := append([]byte{}, data...)
	for i := 0; i < len(code); {
		if code[i] == '\r' {
			code[i] = ' '
		}
		if code[i] == '#' {
			for i < len(code) && code[i] != '\n' {
				code[i] = ' '
				i++
			}
			continue
		}
		if code[i] != '\'' && code[i] != '"' {
			i++
			continue
		}
		quote := code[i]
		width := 1
		if i+2 < len(code) && code[i+1] == quote && code[i+2] == quote {
			width = 3
		}
		start := i
		i += width
		closed := false
		for i < len(code) {
			if code[i] == '\\' {
				i += 2
				continue
			}
			if code[i] == quote && (width == 1 || i+2 < len(code) && code[i+1] == quote && code[i+2] == quote) {
				i += width
				closed = true
				break
			}
			if width == 1 && code[i] == '\n' {
				return "", false
			}
			i++
		}
		if !closed {
			return "", false
		}
		for j := start; j < i; j++ {
			if code[j] != '\n' {
				code[j] = ' '
			}
		}
	}
	return string(code), true
}

var flaskAppReference = regexp.MustCompile(`^(?:\./)?[A-Za-z_][A-Za-z0-9_./-]*(?::[A-Za-z_][A-Za-z0-9_.]*(?:\(\))?)?$`)

func (s *scan) pythonFlaskCommands(path string, data []byte, rule Rule) {
	scanner := bufio.NewScanner(bytes.NewReader(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))))
	scanner.Buffer(make([]byte, 4096), 16<<10)
	entrypoint := ""
	valid, found := true, false
	for line := 0; scanner.Scan(); line++ {
		if line >= 1024 {
			s.issue(path, "scanLimit")
			return
		}
		text := strings.TrimSpace(scanner.Text())
		text = strings.TrimSpace(strings.TrimPrefix(text, "export "))
		name, value, assignment := strings.Cut(text, "=")
		if !assignment || strings.TrimSpace(name) != "FLASK_APP" {
			continue
		}
		found = true
		value = strings.TrimSpace(value)
		if len(value) > 1 && (value[0] == '\'' || value[0] == '"') {
			end := strings.IndexByte(value[1:], value[0])
			if end < 0 {
				valid = false
				continue
			}
			end++
			rest := strings.TrimSpace(value[end+1:])
			if rest != "" && !strings.HasPrefix(rest, "#") {
				valid = false
				continue
			}
			value = value[1:end]
		} else if comment := strings.Index(value, " #"); comment >= 0 {
			value = strings.TrimSpace(value[:comment])
		}
		if len(value) > 512 || !flaskAppReference.MatchString(value) || strings.Contains(value, "..") {
			valid = false
			continue
		}
		if entrypoint != "" && entrypoint != value {
			valid = false
		}
		entrypoint = value
	}
	if scanner.Err() != nil {
		s.issue(path, "scanLimit")
		return
	}
	if found && (!valid || entrypoint == "") {
		s.issue(path, "scanPartial")
		return
	}
	if entrypoint != "" {
		s.addRuleCommand(rule, "flask-run", path, filepath.Dir(path), map[string]string{"entrypoint": entrypoint}, "", true)
	}
}
