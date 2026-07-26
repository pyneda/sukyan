package scan

import (
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/pyneda/sukyan/lib"
)

// XMLPointOptions configures XML insertion point extraction. The point types are
// supplied by the caller so the same codec serves HTTP bodies and WebSocket
// messages without the extractor knowing which transport it is running under.
type XMLPointOptions struct {
	ElementType       InsertionPointType
	AttributeType     InsertionPointType
	IncludeAttributes bool
	MaxPoints         int
	// ReservedNames are names the caller uses for its own synthetic points. A leaf
	// that would take one is given its path instead, so the two stay distinguishable
	// in launch conditions and in the issue-dedup key.
	ReservedNames []string
}

var (
	xmlTextEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	xmlAttrEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	xmlUnescaper   = strings.NewReplacer("&lt;", "<", "&gt;", ">", "&quot;", `"`, "&apos;", "'", "&amp;", "&")
)

type xmlFrame struct {
	localName string
	path      string
	hasChild  bool
	textStart int
	textEnd   int
	text      string
}

type xmlCandidate struct {
	base      string
	pathName  string
	value     string
	span      InsertionPointSpan
	attribute bool
}

// ExtractXMLPoints returns one insertion point per leaf element value (and per
// attribute value when enabled), each carrying the byte span of that value inside
// data. Malformed input yields no points, leaving the caller's whole-body point as
// the only surface.
func ExtractXMLPoints(data string, opts XMLPointOptions) []InsertionPoint {
	if strings.TrimSpace(data) == "" {
		return nil
	}

	decoder := xml.NewDecoder(strings.NewReader(data))
	var (
		stack      []xmlFrame
		candidates []xmlCandidate
		prevEnd    int
		truncated  bool
	)

	// Once enough candidates exist to fill the cap with room to spare for name
	// disambiguation, walking the rest of a document an attacker controls is wasted work.
	ceiling := 0
	if opts.MaxPoints > 0 {
		ceiling = opts.MaxPoints * 4
	}

	for {
		if ceiling > 0 && len(candidates) >= ceiling {
			truncated = true
			break
		}
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil
		}
		tokenStart, tokenEnd := prevEnd, int(decoder.InputOffset())
		prevEnd = tokenEnd

		switch t := token.(type) {
		case xml.StartElement:
			parentPath := ""
			if len(stack) > 0 {
				stack[len(stack)-1].hasChild = true
				parentPath = stack[len(stack)-1].path
			}
			stack = append(stack, xmlFrame{
				localName: t.Name.Local,
				path:      parentPath + "/" + t.Name.Local,
				textStart: -1,
			})
			if opts.IncludeAttributes {
				elementPath := stack[len(stack)-1].path
				for _, attr := range parseXMLAttributeSpans(data[tokenStart:tokenEnd], tokenStart) {
					candidates = append(candidates, xmlCandidate{
						base:      attr.name,
						pathName:  elementPath + "/@" + attr.name,
						value:     attr.value,
						span:      InsertionPointSpan{Start: attr.start, End: attr.end, Valid: true},
						attribute: true,
					})
				}
			}

		case xml.CharData:
			if len(stack) == 0 {
				continue
			}
			top := &stack[len(stack)-1]
			if top.textStart < 0 {
				top.textStart = tokenStart
			}
			top.textEnd = tokenEnd
			top.text += string(t)

		case xml.EndElement:
			if len(stack) == 0 {
				continue
			}
			frame := stack[len(stack)-1]
			stack = stack[:len(stack)-1]

			// A self-closing element produces a synthesized EndElement that consumes
			// no input, so it has no place to hold a value.
			if frame.hasChild || tokenStart == tokenEnd {
				continue
			}
			start, end := frame.textStart, frame.textEnd
			if start < 0 {
				start, end = tokenStart, tokenStart
			}
			candidates = append(candidates, xmlCandidate{
				base:     frame.localName,
				pathName: frame.path,
				value:    frame.text,
				span:     InsertionPointSpan{Start: start, End: end, Valid: true},
			})
		}
	}

	if len(stack) != 0 && !truncated {
		return nil
	}
	return nameXMLPoints(data, candidates, opts)
}

// nameXMLPoints keeps the bare local name when it is unambiguous, so payload
// templates that gate on insertion_point_name still match, and falls back to the
// element path (then a positional index) only when it has to.
func nameXMLPoints(data string, candidates []xmlCandidate, opts XMLPointOptions) []InsertionPoint {
	baseCount := make(map[string]int, len(candidates))
	pathCount := make(map[string]int, len(candidates))
	for _, c := range candidates {
		baseCount[c.base]++
		pathCount[c.pathName]++
	}

	reserved := make(map[string]bool, len(opts.ReservedNames))
	for _, name := range opts.ReservedNames {
		reserved[name] = true
	}

	occurrence := make(map[string]int, len(candidates))
	points := make([]InsertionPoint, 0, len(candidates))
	for _, c := range candidates {
		name := c.base
		if baseCount[c.base] > 1 || reserved[c.base] {
			name = c.pathName
			if pathCount[c.pathName] > 1 {
				occurrence[c.pathName]++
				name = fmt.Sprintf("%s[%d]", c.pathName, occurrence[c.pathName])
			}
		}

		pointType := opts.ElementType
		if c.attribute {
			pointType = opts.AttributeType
		}
		points = append(points, InsertionPoint{
			Type:         pointType,
			Name:         name,
			Value:        c.value,
			ValueType:    lib.GuessDataType(c.value),
			OriginalData: data,
			Span:         c.span,
		})

		if opts.MaxPoints > 0 && len(points) >= opts.MaxPoints {
			break
		}
	}
	return points
}

// ApplyXMLPointPayload splices payload into the point's byte span. Addressing by
// span rather than by name is what keeps repeated siblings independently
// targetable and leaves the rest of the document byte-identical.
func ApplyXMLPointPayload(data string, point InsertionPoint, payload string) (string, error) {
	span := point.Span
	if !span.Valid {
		return "", fmt.Errorf("insertion point %q carries no span", point.Name)
	}
	if span.Start < 0 || span.End < span.Start || span.End > len(data) {
		return "", fmt.Errorf("insertion point %q has span [%d,%d] outside a %d byte document", point.Name, span.Start, span.End, len(data))
	}

	representable := stripNonXMLChars(payload)
	encoded := xmlTextEscaper.Replace(representable)
	if point.Type == InsertionPointTypeXMLAttribute || point.Type == InsertionPointTypeWSXMLAttribute {
		encoded = xmlAttrEscaper.Replace(representable)
	}
	return data[:span.Start] + encoded + data[span.End:], nil
}

// stripNonXMLChars drops the code points XML has no representation for. Leaving them
// in makes the whole document unparseable, so the probe would test nothing at all --
// several shipped payloads (ldap-injection) carry literal NUL bytes.
func stripNonXMLChars(payload string) string {
	if strings.IndexFunc(payload, isNonXMLChar) < 0 {
		return payload
	}
	return strings.Map(func(r rune) rune {
		if isNonXMLChar(r) {
			return -1
		}
		return r
	}, payload)
}

func isNonXMLChar(r rune) bool {
	switch {
	case r == 0x09 || r == 0x0A || r == 0x0D:
		return false
	case r >= 0x20 && r <= 0xD7FF:
		return false
	case r >= 0xE000 && r <= 0xFFFD:
		return false
	case r >= 0x10000 && r <= 0x10FFFF:
		return false
	default:
		return true
	}
}

type xmlAttrSpan struct {
	name  string
	value string
	start int
	end   int
}

// parseXMLAttributeSpans locates attribute value spans inside a raw start tag.
// encoding/xml decodes attributes but discards their offsets, and offsets are what
// the writer needs.
func parseXMLAttributeSpans(tag string, offset int) []xmlAttrSpan {
	var spans []xmlAttrSpan

	i := 0
	if i < len(tag) && tag[i] == '<' {
		i++
	}
	for i < len(tag) && !isXMLSpace(tag[i]) && tag[i] != '>' && tag[i] != '/' {
		i++
	}

	for i < len(tag) {
		for i < len(tag) && isXMLSpace(tag[i]) {
			i++
		}
		if i >= len(tag) || tag[i] == '>' || tag[i] == '/' {
			break
		}

		nameStart := i
		for i < len(tag) && tag[i] != '=' && !isXMLSpace(tag[i]) && tag[i] != '>' && tag[i] != '/' {
			i++
		}
		name := tag[nameStart:i]

		for i < len(tag) && isXMLSpace(tag[i]) {
			i++
		}
		if i >= len(tag) || tag[i] != '=' {
			break
		}
		i++
		for i < len(tag) && isXMLSpace(tag[i]) {
			i++
		}
		if i >= len(tag) || (tag[i] != '"' && tag[i] != '\'') {
			break
		}

		quote := tag[i]
		i++
		valueStart := i
		for i < len(tag) && tag[i] != quote {
			i++
		}
		if i >= len(tag) {
			break
		}
		valueEnd := i
		i++

		if name != "" && !strings.HasPrefix(name, "xmlns") {
			spans = append(spans, xmlAttrSpan{
				name:  name,
				value: xmlUnescaper.Replace(tag[valueStart:valueEnd]),
				start: offset + valueStart,
				end:   offset + valueEnd,
			})
		}
	}
	return spans
}

func isXMLSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
