package discovery

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

type cmakeCondition struct {
	kind        string
	value       bool
	left, right string
	list        []string
	children    []*cmakeCondition
}

func parseCMakeCondition(raw json.RawMessage, depth int, nodes *int) (*cmakeCondition, error) {
	(*nodes)++
	if depth > 32 || *nodes > 512 {
		return nil, errCMakeLimit
	}
	if cmakeNull(raw) {
		return nil, errCMakeInvalid
	}
	c := &cmakeCondition{kind: "const"}
	if json.Unmarshal(raw, &c.value) == nil {
		return c, nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil || json.Unmarshal(fields["type"], &c.kind) != nil {
		return nil, errCMakeInvalid
	}
	read := func(key string, value any) bool {
		return !cmakeNull(fields[key]) && json.Unmarshal(fields[key], value) == nil
	}
	var children []json.RawMessage
	valid := false
	switch c.kind {
	case "const":
		valid = read("value", &c.value)
	case "equals", "notEquals":
		valid = read("lhs", &c.left) && read("rhs", &c.right)
	case "inList", "notInList":
		var err error
		c.list, err = cmakeStrings(fields["list"], 512)
		if err != nil {
			return nil, err
		}
		valid = read("string", &c.left)
	case "matches", "notMatches":
		valid = read("string", &c.left) && read("regex", &c.right)
	case "anyOf", "allOf":
		valid = read("conditions", &children)
	case "not":
		value, ok := fields["condition"]
		valid = ok
		children = []json.RawMessage{value}
	}
	if !valid {
		return nil, errCMakeInvalid
	}
	if len(c.list) > 512 || len(children) > 512 {
		return nil, errCMakeLimit
	}
	for _, rawChild := range children {
		child, err := parseCMakeCondition(rawChild, depth+1, nodes)
		if err != nil {
			return nil, err
		}
		c.children = append(c.children, child)
	}
	return c, nil
}

type cmakeContext struct {
	graph       *cmakeGraph
	preset      *cmakePreset
	generator   string
	environment map[string]cmakeField
	values      map[string]string
	visiting    map[string]bool
	steps       int
	bytes       int
}

func newCMakeContext(graph *cmakeGraph, preset *cmakePreset) *cmakeContext {
	c := &cmakeContext{graph: graph, preset: preset, values: map[string]string{}, visiting: map[string]bool{}}
	if preset != nil {
		c.generator = preset.text("generator")
		c.environment = maps.Clone(preset.environment)
	}
	return c
}

func (c *cmakeContext) condition(condition *cmakeCondition, file *cmakeFile, depth int) (bool, error) {
	c.steps++
	if c.steps > 4096 || depth > 32 || c.graph.s.ctx.Err() != nil {
		return false, errCMakeLimit
	}
	if condition == nil {
		return true, nil
	}
	switch condition.kind {
	case "const":
		return condition.value, nil
	case "not":
		value, err := c.condition(condition.children[0], file, depth+1)
		return !value, err
	case "anyOf", "allOf":
		for _, child := range condition.children {
			value, err := c.condition(child, file, depth+1)
			if err != nil {
				return false, err
			}
			if (condition.kind == "anyOf" && value) || (condition.kind == "allOf" && !value) {
				return value, nil
			}
		}
		return condition.kind == "allOf", nil
	}
	left, err := c.expand(condition.left, file, false, 0)
	if err != nil {
		return false, err
	}
	if condition.kind == "inList" || condition.kind == "notInList" {
		for _, item := range condition.list {
			right, err := c.expand(item, file, false, 0)
			if err != nil {
				return false, err
			}
			if left == right {
				return condition.kind == "inList", nil
			}
		}
		return condition.kind == "notInList", nil
	}
	right, err := c.expand(condition.right, file, false, 0)
	if err != nil {
		return false, err
	}
	if condition.kind == "equals" || condition.kind == "notEquals" {
		return (left == right) == (condition.kind == "equals"), nil
	}
	matched, err := matchCMakeRegex(right, left)
	return matched == (condition.kind == "matches"), err
}

func (c *cmakeContext) env(name string, depth int) (string, error) {
	if name == "" {
		return "", errCMakeInvalid
	}
	if value, ok := c.values[name]; ok {
		return value, nil
	}
	if c.visiting[name] {
		return "", errCMakeInvalid
	}
	if depth > 32 {
		return "", errCMakeLimit
	}
	field, exists := c.environment[name]
	if !exists || cmakeNull(field.value) {
		return os.Getenv(name), nil
	}
	c.visiting[name] = true
	defer delete(c.visiting, name)
	var raw string
	if json.Unmarshal(field.value, &raw) != nil {
		return "", errCMakeInvalid
	}
	value, err := c.expand(raw, field.file, false, depth+1)
	if err == nil {
		c.values[name] = value
	}
	return value, err
}

// Values are expanded once, without interpreting dollars introduced by a
// replacement. Inherited values use the final preset's context; v12 fileDir
// alone uses the file in which that value was originally declared.
func (c *cmakeContext) expand(value string, file *cmakeFile, include bool, depth int) (string, error) {
	c.steps++
	if c.steps > 4096 || depth > 32 || len(value) > 64<<10 || c.graph.s.ctx.Err() != nil {
		return "", errCMakeLimit
	}
	if include && file.version < 7 {
		return value, nil
	}
	var output strings.Builder
	for offset := 0; offset < len(value); {
		if output.Len() > 64<<10 {
			return "", errCMakeLimit
		}
		if value[offset] != '$' {
			output.WriteByte(value[offset])
			offset++
			continue
		}
		namespace, begin := "", offset+1
		for begin < len(value) && value[begin] != '{' {
			namespace += string(value[begin])
			begin++
			if !strings.HasPrefix("env", namespace) && !strings.HasPrefix("penv", namespace) && !strings.HasPrefix("vendor", namespace) {
				break
			}
		}
		if begin == len(value) || (namespace != "" && namespace != "env" && namespace != "penv" && namespace != "vendor") || value[begin] != '{' {
			// A non-macro prefix is consumed literally, including its final byte.
			// In particular, $${sourceDir} must not expand the second dollar.
			output.WriteString(value[offset:begin])
			offset = begin
			continue
		}
		end := strings.IndexByte(value[begin+1:], '}')
		if end < 0 {
			return "", errCMakeInvalid
		}
		end += begin + 1
		name := value[begin+1 : end]
		if include && (file.version < 7 || (file.version < 9 && namespace != "penv") || namespace == "env" || (namespace == "" && (name == "presetName" || name == "generator"))) {
			return "", errCMakePartial
		}
		var replacement string
		var err error
		switch namespace {
		case "vendor":
			return "", errCMakePartial
		case "env":
			replacement, err = c.env(name, depth+1)
		case "penv":
			if name == "" {
				return "", errCMakeInvalid
			}
			replacement = os.Getenv(name)
		default:
			source := filepath.Join(c.graph.s.result.Directory, c.graph.directory)
			version := file.version
			if !include && c.preset != nil {
				version = c.preset.file.version
			}
			switch name {
			case "sourceDir":
				replacement = filepath.ToSlash(source)
			case "sourceParentDir":
				replacement = filepath.ToSlash(filepath.Dir(source))
			case "sourceDirName":
				replacement = filepath.Base(source)
			case "dollar":
				replacement = "$"
			case "presetName":
				if c.preset == nil {
					return "", errCMakePartial
				}
				replacement = c.preset.name
			case "generator":
				if c.preset == nil || c.generator == "" {
					return "", errCMakePartial
				}
				replacement = c.generator
			case "fileDir":
				if version < 4 && file.version < 12 {
					return "", errCMakeInvalid
				}
				origin := file
				if !include && file.version < 12 && c.preset != nil {
					origin = c.preset.file
				}
				replacement = filepath.ToSlash(filepath.Dir(filepath.Join(c.graph.s.result.Directory, origin.path)))
			case "pathListSep":
				if version < 5 {
					return "", errCMakeInvalid
				}
				replacement = string(os.PathListSeparator)
			case "hostSystemName":
				if version < 3 {
					return "", errCMakeInvalid
				}
				replacement = map[string]string{"windows": "Windows", "darwin": "Darwin", "linux": "Linux", "freebsd": "FreeBSD", "openbsd": "OpenBSD", "netbsd": "NetBSD", "aix": "AIX", "solaris": "SunOS"}[runtime.GOOS]
				if replacement == "" {
					return "", errCMakePartial
				}
			default:
				return "", errCMakePartial
			}
		}
		if err != nil {
			return "", err
		}
		output.WriteString(replacement)
		offset = end + 1
	}
	c.bytes += output.Len()
	if output.Len() > 64<<10 || c.bytes > 1<<20 {
		return "", errCMakeLimit
	}
	return output.String(), nil
}

// CMake's byte-oriented regex dialect is smaller than RE2. Quote constructs
// which RE2 would reinterpret (notably {n}, \d and \b); do not guess on Unicode.
func matchCMakeRegex(pattern, value string) (bool, error) {
	if len(pattern) > 8192 {
		return false, errCMakeLimit
	}
	for _, text := range []string{pattern, value} {
		for _, char := range text {
			if char > 127 {
				return false, errCMakePartial
			}
		}
	}
	var translated strings.Builder
	translated.WriteString("(?s)")
	groups := 0
	for i := 0; i < len(pattern); i++ {
		char := pattern[i]
		if char == '\\' {
			i++
			if i == len(pattern) {
				return false, errCMakeInvalid
			}
			translated.WriteString(regexp.QuoteMeta(string(pattern[i])))
			continue
		}
		switch char {
		case '[':
			end := i + 1
			if end < len(pattern) && pattern[end] == '^' {
				end++
			}
			if end < len(pattern) && pattern[end] == ']' {
				end++
			}
			for end < len(pattern) && pattern[end] != ']' {
				end++
			}
			if end == len(pattern) {
				return false, errCMakeInvalid
			}
			characterClass := pattern[i : end+1]
			if strings.ContainsAny(characterClass[1:], "\\[") {
				return false, errCMakePartial
			}
			translated.WriteString(characterClass)
			i = end
			continue
		case '(':
			groups++
			if groups > 9 || (i+1 < len(pattern) && pattern[i+1] == '?') {
				return false, errCMakeInvalid
			}
		case '{', '}':
			translated.WriteByte('\\')
		case '*', '+', '?':
			if i+1 < len(pattern) && strings.ContainsRune("*+?", rune(pattern[i+1])) {
				return false, errCMakeInvalid
			}
		}
		translated.WriteByte(char)
	}
	compiled, err := regexp.Compile(translated.String())
	if err != nil {
		return false, errCMakeInvalid
	}
	return compiled.MatchString(value), nil
}
