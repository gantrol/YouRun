package discovery

import (
	"bytes"
	"errors"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

var errIOSLimit = errors.New("Xcode configuration limit")

// Xcode's OpenStep property lists contain dictionaries, arrays, and strings.
// Parse the object graph rather than searching unrelated targets' settings.
type iosPBXParser struct {
	data         []byte
	index, nodes int
}

func parseIOSPBX(data []byte) (map[string]any, error) {
	parser := iosPBXParser{data: bytes.TrimPrefix(data, []byte{0xef, 0xbb, 0xbf})}
	if !utf8.Valid(parser.data) {
		return nil, errors.New("unsupported project encoding")
	}
	value, err := parser.value(0)
	if err != nil {
		return nil, err
	}
	if err := parser.space(); err != nil {
		return nil, err
	}
	root, ok := value.(map[string]any)
	if !ok || parser.index != len(parser.data) {
		return nil, errors.New("invalid project root")
	}
	return root, nil
}

func (p *iosPBXParser) space() error {
	for p.index < len(p.data) {
		switch p.data[p.index] {
		case ' ', '\t', '\r', '\n':
			p.index++
			continue
		case '/':
			if p.index+1 < len(p.data) && p.data[p.index+1] == '/' {
				p.index += 2
				for p.index < len(p.data) && p.data[p.index] != '\n' && p.data[p.index] != '\r' {
					p.index++
				}
				continue
			}
			if p.index+1 < len(p.data) && p.data[p.index+1] == '*' {
				end := bytes.Index(p.data[p.index+2:], []byte("*/"))
				if end < 0 {
					return errors.New("unterminated project comment")
				}
				p.index += end + 4
				continue
			}
		}
		break
	}
	return nil
}

func (p *iosPBXParser) take(char byte) bool {
	if p.space() != nil || p.index >= len(p.data) || p.data[p.index] != char {
		return false
	}
	p.index++
	return true
}

func (p *iosPBXParser) value(depth int) (any, error) {
	p.nodes++
	if p.nodes > 65536 || depth > 48 {
		return nil, errIOSLimit
	}
	if err := p.space(); err != nil {
		return nil, err
	}
	if p.index >= len(p.data) {
		return nil, errors.New("missing project value")
	}
	switch p.data[p.index] {
	case '{':
		p.index++
		values := map[string]any{}
		for !p.take('}') {
			key, err := p.string()
			if err != nil || key == "" || !p.take('=') {
				return nil, errors.New("invalid project dictionary")
			}
			if _, duplicate := values[key]; duplicate {
				return nil, errors.New("ambiguous project dictionary")
			}
			value, err := p.value(depth + 1)
			if err != nil {
				return nil, err
			}
			if !p.take(';') {
				return nil, errors.New("invalid project dictionary separator")
			}
			values[key] = value
		}
		return values, nil
	case '(':
		p.index++
		values := []any{}
		for !p.take(')') {
			value, err := p.value(depth + 1)
			if err != nil {
				return nil, err
			}
			values = append(values, value)
			if p.take(')') {
				break
			}
			if !p.take(',') {
				return nil, errors.New("invalid project array separator")
			}
		}
		return values, nil
	default:
		return p.string()
	}
}

func (p *iosPBXParser) string() (string, error) {
	if err := p.space(); err != nil {
		return "", err
	}
	if p.index >= len(p.data) {
		return "", errors.New("missing project string")
	}
	if p.data[p.index] != '"' {
		start := p.index
		for p.index < len(p.data) && !strings.ContainsRune(" \t\r\n{}()=;,\"", rune(p.data[p.index])) {
			if p.data[p.index] == '/' && p.index+1 < len(p.data) && (p.data[p.index+1] == '*' || p.data[p.index+1] == '/') {
				break
			}
			p.index++
		}
		if start == p.index || p.index-start > 65536 {
			return "", errors.New("invalid project string")
		}
		return string(p.data[start:p.index]), nil
	}
	p.index++
	var value strings.Builder
	for p.index < len(p.data) {
		char := p.data[p.index]
		p.index++
		if char == '"' {
			return value.String(), nil
		}
		if char != '\\' {
			value.WriteByte(char)
			continue
		}
		if p.index >= len(p.data) {
			break
		}
		char = p.data[p.index]
		p.index++
		switch char {
		case '\\', '"':
			value.WriteByte(char)
		case 'n':
			value.WriteByte('\n')
		case 'r':
			value.WriteByte('\r')
		case 't':
			value.WriteByte('\t')
		case 'b':
			value.WriteByte('\b')
		case 'f':
			value.WriteByte('\f')
		case 'U', 'u':
			if p.index+4 > len(p.data) {
				return "", errors.New("invalid Unicode escape")
			}
			code, err := strconv.ParseUint(string(p.data[p.index:p.index+4]), 16, 16)
			if err != nil || utf16.IsSurrogate(rune(code)) {
				return "", errors.New("unsupported Unicode escape")
			}
			p.index += 4
			value.WriteRune(rune(code))
		default:
			if char < '0' || char > '7' {
				return "", errors.New("unsupported project escape")
			}
			start := p.index - 1
			for p.index < len(p.data) && p.index-start < 3 && p.data[p.index] >= '0' && p.data[p.index] <= '7' {
				p.index++
			}
			code, err := strconv.ParseUint(string(p.data[start:p.index]), 8, 8)
			if err != nil {
				return "", errors.New("invalid octal escape")
			}
			value.WriteByte(byte(code))
		}
	}
	return "", errors.New("unterminated project string")
}
