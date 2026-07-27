package graphql

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Introspection reports argument and input-field defaults as GraphQL literals
// rather than JSON: `first: Int = 10` comes back as the text "10", a string
// default arrives already quoted, and an enum default arrives as a bare name.
// Sending those texts straight through as variable values makes every defaulted
// argument a type error at the server, so they are decoded here.

const maxLiteralDepth = 32

// ParseLiteral decodes a GraphQL value literal into the Go value that JSON
// encoding will turn back into the same value. Enum literals decode to their
// name as a string, which is how enums travel in a variables map.
func ParseLiteral(literal string) (interface{}, error) {
	p := &literalParser{input: literal}
	p.skipIgnored()

	value, err := p.parseValue(0)
	if err != nil {
		return nil, err
	}

	p.skipIgnored()
	if p.pos != len(p.input) {
		return nil, fmt.Errorf("unexpected trailing input at offset %d in %q", p.pos, literal)
	}
	return value, nil
}

type literalParser struct {
	input string
	pos   int
}

func (p *literalParser) parseValue(depth int) (interface{}, error) {
	if depth > maxLiteralDepth {
		return nil, fmt.Errorf("literal nested deeper than %d levels", maxLiteralDepth)
	}
	if p.pos >= len(p.input) {
		return nil, fmt.Errorf("unexpected end of literal")
	}

	switch c := p.input[p.pos]; {
	case c == '[':
		return p.parseList(depth)
	case c == '{':
		return p.parseObject(depth)
	case c == '"':
		return p.parseString()
	case c == '$':
		return nil, fmt.Errorf("variable references are not valid default values")
	case c == '-' || (c >= '0' && c <= '9'):
		return p.parseNumber()
	case isNameStart(c):
		return p.parseName()
	default:
		return nil, fmt.Errorf("unexpected character %q at offset %d", c, p.pos)
	}
}

func (p *literalParser) parseList(depth int) (interface{}, error) {
	p.pos++ // consume '['
	list := []interface{}{}

	for {
		p.skipIgnored()
		if p.pos >= len(p.input) {
			return nil, fmt.Errorf("unterminated list literal")
		}
		if p.input[p.pos] == ']' {
			p.pos++
			return list, nil
		}

		item, err := p.parseValue(depth + 1)
		if err != nil {
			return nil, err
		}
		list = append(list, item)

		p.skipIgnored()
		if p.pos < len(p.input) && p.input[p.pos] == ',' {
			p.pos++
		}
	}
}

func (p *literalParser) parseObject(depth int) (interface{}, error) {
	p.pos++ // consume '{'
	obj := map[string]interface{}{}

	for {
		p.skipIgnored()
		if p.pos >= len(p.input) {
			return nil, fmt.Errorf("unterminated object literal")
		}
		if p.input[p.pos] == '}' {
			p.pos++
			return obj, nil
		}

		if !isNameStart(p.input[p.pos]) {
			return nil, fmt.Errorf("expected field name at offset %d", p.pos)
		}
		name, err := p.parseName()
		if err != nil {
			return nil, err
		}

		p.skipIgnored()
		if p.pos >= len(p.input) || p.input[p.pos] != ':' {
			return nil, fmt.Errorf("expected ':' after field %q", name)
		}
		p.pos++

		p.skipIgnored()
		value, err := p.parseValue(depth + 1)
		if err != nil {
			return nil, err
		}
		obj[fmt.Sprint(name)] = value

		p.skipIgnored()
		if p.pos < len(p.input) && p.input[p.pos] == ',' {
			p.pos++
		}
	}
}

// parseName handles the three literals spelled as bare names -- true, false and
// null -- and treats anything else as an enum value.
func (p *literalParser) parseName() (interface{}, error) {
	start := p.pos
	for p.pos < len(p.input) && isNameContinue(p.input[p.pos]) {
		p.pos++
	}

	switch name := p.input[start:p.pos]; name {
	case "true":
		return true, nil
	case "false":
		return false, nil
	case "null":
		return nil, nil
	default:
		return name, nil
	}
}

func (p *literalParser) parseNumber() (interface{}, error) {
	start := p.pos
	if p.pos < len(p.input) && p.input[p.pos] == '-' {
		p.pos++
	}

	float := false
	for p.pos < len(p.input) {
		c := p.input[p.pos]
		switch {
		case c >= '0' && c <= '9':
		case c == '.' || c == 'e' || c == 'E':
			float = true
		case (c == '+' || c == '-') && float:
		default:
			goto done
		}
		p.pos++
	}

done:
	text := p.input[start:p.pos]
	if text == "" || text == "-" {
		return nil, fmt.Errorf("malformed number at offset %d", start)
	}

	if !float {
		if n, err := strconv.ParseInt(text, 10, 64); err == nil {
			return n, nil
		}
	}

	f, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return nil, fmt.Errorf("malformed number %q: %w", text, err)
	}
	return f, nil
}

func (p *literalParser) parseString() (interface{}, error) {
	if strings.HasPrefix(p.input[p.pos:], `"""`) {
		return p.parseBlockString()
	}

	p.pos++ // consume '"'
	var sb strings.Builder

	for p.pos < len(p.input) {
		switch c := p.input[p.pos]; c {
		case '"':
			p.pos++
			return sb.String(), nil
		case '\\':
			if err := p.parseEscape(&sb); err != nil {
				return nil, err
			}
		case '\n', '\r':
			return nil, fmt.Errorf("unterminated string literal")
		default:
			sb.WriteByte(c)
			p.pos++
		}
	}

	return nil, fmt.Errorf("unterminated string literal")
}

func (p *literalParser) parseEscape(sb *strings.Builder) error {
	p.pos++ // consume '\'
	if p.pos >= len(p.input) {
		return fmt.Errorf("unterminated escape sequence")
	}

	switch c := p.input[p.pos]; c {
	case '"', '\\', '/':
		sb.WriteByte(c)
		p.pos++
	case 'b':
		sb.WriteByte('\b')
		p.pos++
	case 'f':
		sb.WriteByte('\f')
		p.pos++
	case 'n':
		sb.WriteByte('\n')
		p.pos++
	case 'r':
		sb.WriteByte('\r')
		p.pos++
	case 't':
		sb.WriteByte('\t')
		p.pos++
	case 'u':
		return p.parseUnicodeEscape(sb)
	default:
		return fmt.Errorf("unknown escape sequence \\%c", c)
	}
	return nil
}

func (p *literalParser) parseUnicodeEscape(sb *strings.Builder) error {
	p.pos++ // consume 'u'
	if p.pos+4 > len(p.input) {
		return fmt.Errorf("truncated unicode escape")
	}

	code, err := strconv.ParseUint(p.input[p.pos:p.pos+4], 16, 32)
	if err != nil {
		return fmt.Errorf("malformed unicode escape: %w", err)
	}
	p.pos += 4

	r := rune(code)
	if utf16.IsSurrogate(r) && strings.HasPrefix(p.input[p.pos:], `\u`) && p.pos+6 <= len(p.input) {
		if low, err := strconv.ParseUint(p.input[p.pos+2:p.pos+6], 16, 32); err == nil {
			if combined := utf16.DecodeRune(r, rune(low)); combined != utf8.RuneError {
				p.pos += 6
				sb.WriteRune(combined)
				return nil
			}
		}
	}
	if utf16.IsSurrogate(r) {
		r = utf8.RuneError
	}

	sb.WriteRune(r)
	return nil
}

func (p *literalParser) parseBlockString() (interface{}, error) {
	p.pos += 3 // consume '"""'
	start := p.pos

	for p.pos < len(p.input) {
		if strings.HasPrefix(p.input[p.pos:], `\"""`) {
			p.pos += 4
			continue
		}
		if strings.HasPrefix(p.input[p.pos:], `"""`) {
			raw := strings.ReplaceAll(p.input[start:p.pos], `\"""`, `"""`)
			p.pos += 3
			return blockStringValue(raw), nil
		}
		p.pos++
	}

	return nil, fmt.Errorf("unterminated block string literal")
}

// blockStringValue applies the block string semantics from the specification:
// strip blank leading and trailing lines and remove the common indentation.
func blockStringValue(raw string) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")

	common := -1
	for i, line := range lines {
		if i == 0 {
			continue
		}
		trimmed := strings.TrimLeft(line, " \t")
		if trimmed == "" {
			continue
		}
		if indent := len(line) - len(trimmed); common < 0 || indent < common {
			common = indent
		}
	}
	if common > 0 {
		for i := 1; i < len(lines); i++ {
			if len(lines[i]) >= common {
				lines[i] = lines[i][common:]
			} else {
				lines[i] = strings.TrimLeft(lines[i], " \t")
			}
		}
	}

	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	return strings.Join(lines, "\n")
}

// skipIgnored consumes whitespace, commas and comments, all of which GraphQL
// treats as insignificant between tokens.
func (p *literalParser) skipIgnored() {
	for p.pos < len(p.input) {
		switch c := p.input[p.pos]; c {
		case ' ', '\t', '\n', '\r', ',':
			p.pos++
		case '#':
			for p.pos < len(p.input) && p.input[p.pos] != '\n' {
				p.pos++
			}
		default:
			return
		}
	}
}

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isNameContinue(c byte) bool {
	return isNameStart(c) || (c >= '0' && c <= '9')
}

// IsValidName reports whether a string is a legal GraphQL name. Names arriving
// from introspection are attacker-controlled for a scanner, and writing one into
// a document unchecked would let a hostile schema reshape the query being sent.
func IsValidName(name string) bool {
	if name == "" || !isNameStart(name[0]) {
		return false
	}
	for i := 1; i < len(name); i++ {
		if !isNameContinue(name[i]) {
			return false
		}
	}
	return true
}
