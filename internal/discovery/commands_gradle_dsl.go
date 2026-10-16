package discovery

import (
	"fmt"
	"strings"
)

// This is a bounded structural reader, not a Groovy/Kotlin interpreter. Only
// literal statements at explicitly supported scopes contribute command values.
type gradleToken struct {
	text string
	kind byte // i: identifier, s: literal string, d: interpolated string, p: punctuation
}

type gradleNode struct {
	head  []gradleToken
	body  []gradleNode
	block bool
}

func gradleParse(data []byte) ([]gradleNode, error) {
	tokens := make([]gradleToken, 0)
	for i := 0; i < len(data); {
		if len(tokens) >= 32768 {
			return nil, fmt.Errorf("Gradle token budget exceeded")
		}
		c := data[i]
		if c == ' ' || c == '\t' || c == '\r' {
			i++
			continue
		}
		if c == '/' && i+1 < len(data) && data[i+1] == '/' {
			for i < len(data) && data[i] != '\n' {
				i++
			}
			continue
		}
		if c == '/' && i+1 < len(data) && data[i+1] == '*' {
			i += 2
			depth := 1
			for i+1 < len(data) && depth > 0 {
				if data[i] == '/' && data[i+1] == '*' {
					depth++
					i += 2
				} else if data[i] == '*' && data[i+1] == '/' {
					depth--
					i += 2
				} else {
					i++
				}
			}
			if depth != 0 {
				return nil, fmt.Errorf("unterminated Gradle comment")
			}
			continue
		}
		// Groovy slashy/dollar-slashy strings may contain convincing DSL
		// examples. Reject unsupported slash syntax instead of tokenizing the
		// string contents as executable project configuration.
		if c == '/' {
			return nil, fmt.Errorf("unsupported Gradle slash expression")
		}
		if c == '\'' || c == '"' || c == '`' {
			quote := c
			i++
			var value strings.Builder
			kind := byte('s')
			if quote == '`' {
				kind = 'i'
			}
			closed := false
			for i < len(data) {
				c = data[i]
				i++
				if c == quote {
					closed = true
					break
				}
				if c == '\n' || c == '\r' || c == 0 {
					return nil, fmt.Errorf("unsupported multiline Gradle string")
				}
				if c == '$' && quote == '"' {
					kind = 'd'
				}
				if c == '\\' {
					if i >= len(data) {
						break
					}
					c = data[i]
					i++
					if c != '\\' && c != '\'' && c != '"' {
						kind = 'd'
					}
				}
				value.WriteByte(c)
				if value.Len() > 4096 {
					return nil, fmt.Errorf("Gradle string budget exceeded")
				}
			}
			if !closed {
				return nil, fmt.Errorf("unterminated Gradle string")
			}
			tokens = append(tokens, gradleToken{value.String(), kind})
			continue
		}
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '.' || c == '$' {
			start := i
			for i < len(data) {
				c = data[i]
				if !((c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '.' || c == '$') {
					break
				}
				i++
			}
			tokens = append(tokens, gradleToken{string(data[start:i]), 'i'})
			continue
		}
		tokens = append(tokens, gradleToken{string(c), 'p'})
		i++
	}
	index, count := 0, 0
	var parse func(int, bool) ([]gradleNode, error)
	parse = func(depth int, nested bool) ([]gradleNode, error) {
		if depth > 32 {
			return nil, fmt.Errorf("Gradle block budget exceeded")
		}
		nodes := []gradleNode{}
		head := []gradleToken{}
		parens := 0
		flush := func(body []gradleNode, block bool) {
			if len(head) > 0 {
				count++
				nodes = append(nodes, gradleNode{head, body, block})
				head = nil
			}
		}
		for index < len(tokens) {
			if count >= 4096 {
				return nil, fmt.Errorf("Gradle statement budget exceeded")
			}
			t := tokens[index]
			index++
			if t.kind != 'p' {
				head = append(head, t)
				continue
			}
			switch t.text {
			case "(", "[":
				parens++
			case ")", "]":
				parens--
				if parens < 0 {
					return nil, fmt.Errorf("unbalanced Gradle expression")
				}
			case "\n", ";":
				if parens == 0 {
					flush(nil, false)
				}
				continue
			case "{":
				if parens != 0 || len(head) == 0 {
					return nil, fmt.Errorf("unsupported Gradle closure")
				}
				body, err := parse(depth+1, true)
				if err != nil {
					return nil, err
				}
				flush(body, true)
				continue
			case "}":
				if !nested || parens != 0 {
					return nil, fmt.Errorf("unbalanced Gradle block")
				}
				flush(nil, false)
				return nodes, nil
			}
			head = append(head, t)
		}
		if nested || parens != 0 {
			return nil, fmt.Errorf("unterminated Gradle block")
		}
		flush(nil, false)
		return nodes, nil
	}
	return parse(0, false)
}

func gradleHead(node gradleNode) string {
	if len(node.head) == 0 || node.head[0].kind != 'i' {
		return ""
	}
	return node.head[0].text
}

func gradleLiteral(tokens []gradleToken) (string, bool) {
	if len(tokens) == 1 && tokens[0].kind == 's' {
		return tokens[0].text, true
	}
	return "", false
}

func gradleArguments(tokens []gradleToken) ([]gradleToken, bool) {
	if len(tokens) >= 2 && tokens[0].text == "(" && tokens[len(tokens)-1].text == ")" {
		return tokens[1 : len(tokens)-1], true
	}
	return tokens, false
}

// Accept only direct assignment, Groovy property calls and Property.set(value).
func gradleProperty(node gradleNode, name string) ([]gradleToken, bool) {
	if node.block || len(node.head) < 2 {
		return nil, false
	}
	head := gradleHead(node)
	if head == name+".set" {
		args, parens := gradleArguments(node.head[1:])
		return args, parens
	}
	if head != name {
		return nil, false
	}
	args := node.head[1:]
	if args[0].text == "=" {
		args = args[1:]
	} else {
		args, _ = gradleArguments(args)
	}
	return args, true
}

func gradleBlockName(node gradleNode) (string, bool) {
	if !node.block {
		return "", false
	}
	if len(node.head) == 1 && node.head[0].kind == 'i' {
		return node.head[0].text, true
	}
	if containsString([]string{"create", "register", "getByName", "named", "maybeCreate"}, gradleHead(node)) {
		args, parens := gradleArguments(node.head[1:])
		if parens {
			return gradleLiteral(args)
		}
	}
	return "", false
}

func gradleStringList(tokens []gradleToken) ([]string, bool) {
	if len(tokens) > 0 && (tokens[0].text == "listOf" || tokens[0].text == "arrayOf") {
		tokens = tokens[1:]
	}
	tokens, _ = gradleArguments(tokens)
	if len(tokens) >= 2 && tokens[0].text == "[" && tokens[len(tokens)-1].text == "]" {
		tokens = tokens[1 : len(tokens)-1]
	}
	values := []string{}
	expectValue := true
	for _, token := range tokens {
		if expectValue {
			if token.kind != 's' || len(values) >= 32 {
				return nil, false
			}
			values = append(values, token.text)
		} else if token.text != "," {
			return nil, false
		}
		expectValue = !expectValue
	}
	return values, len(values) > 0
}
