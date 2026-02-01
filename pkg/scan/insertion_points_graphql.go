package scan

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/pyneda/sukyan/lib"
	"github.com/rs/zerolog/log"
)

func isGraphQLBody(jsonData map[string]any) bool {
	queryVal, ok := jsonData["query"]
	if !ok {
		return false
	}
	queryStr, ok := queryVal.(string)
	if !ok {
		return false
	}
	trimmed := strings.TrimSpace(queryStr)
	return strings.HasPrefix(trimmed, "query ") ||
		strings.HasPrefix(trimmed, "mutation ") ||
		strings.HasPrefix(trimmed, "subscription ") ||
		strings.HasPrefix(trimmed, "{")
}

func extractGraphQLVariablePoints(path string, variables map[string]any, originalBody string) []InsertionPoint {
	var points []InsertionPoint

	for key, value := range variables {
		currentPath := key
		if path != "" {
			currentPath = path + "." + key
		}

		switch v := value.(type) {
		case map[string]any:
			points = append(points, extractGraphQLVariablePoints(currentPath, v, originalBody)...)

		case []any:
			points = append(points, extractGraphQLArrayPoints(currentPath, v, originalBody)...)

		default:
			valueStr := fmt.Sprintf("%v", v)
			points = append(points, InsertionPoint{
				Type:         InsertionPointTypeGraphQLVariable,
				Name:         currentPath,
				Value:        valueStr,
				ValueType:    lib.GuessDataType(valueStr),
				OriginalData: originalBody,
			})
		}
	}

	return points
}

func extractGraphQLArrayPoints(path string, array []any, originalBody string) []InsertionPoint {
	var points []InsertionPoint

	for i, item := range array {
		currentPath := fmt.Sprintf("%s[%d]", path, i)

		switch v := item.(type) {
		case map[string]any:
			points = append(points, extractGraphQLVariablePoints(currentPath, v, originalBody)...)

		case []any:
			points = append(points, extractGraphQLArrayPoints(currentPath, v, originalBody)...)

		default:
			valueStr := fmt.Sprintf("%v", v)
			points = append(points, InsertionPoint{
				Type:         InsertionPointTypeGraphQLVariable,
				Name:         currentPath,
				Value:        valueStr,
				ValueType:    lib.GuessDataType(valueStr),
				OriginalData: originalBody,
			})
		}
	}

	return points
}

func modifyGraphQLVariables(body []byte, builders []InsertionPointBuilder) ([]byte, error) {
	var fullBody map[string]any
	if err := json.Unmarshal(body, &fullBody); err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL body: %w", err)
	}

	if _, hasQuery := fullBody["query"]; !hasQuery {
		return nil, fmt.Errorf("GraphQL body missing required 'query' field")
	}

	variables, ok := fullBody["variables"].(map[string]any)
	if !ok {
		variables = make(map[string]any)
	}

	for _, builder := range builders {
		setNestedValue(variables, builder.Point.Name, builder.Payload)
	}

	fullBody["variables"] = variables
	return json.Marshal(fullBody)
}

func setNestedValue(obj map[string]any, path string, payload string) {
	parts := strings.SplitN(path, ".", 2)
	key := parts[0]

	if idx, name, isArray := parseGraphQLArrayAccess(key); isArray {
		arr, ok := obj[name].([]any)
		if !ok || idx >= len(arr) {
			log.Warn().Str("path", path).Int("index", idx).Msg("GraphQL variable array index out of bounds during injection")
			return
		}

		if len(parts) == 1 {
			arr[idx] = coercePayloadType(arr[idx], payload)
			obj[name] = arr
			return
		}

		if nested, ok := arr[idx].(map[string]any); ok {
			setNestedValue(nested, parts[1], payload)
			arr[idx] = nested
			obj[name] = arr
		}
		return
	}

	if len(parts) == 1 {
		obj[key] = coercePayloadType(obj[key], payload)
		return
	}

	nested, ok := obj[key].(map[string]any)
	if !ok {
		log.Warn().Str("path", path).Str("key", key).Msg("GraphQL variable path not found during injection")
		return
	}
	setNestedValue(nested, parts[1], payload)
}

func coercePayloadType(original any, payload string) any {
	if original == nil {
		return payload
	}
	switch original.(type) {
	case float64:
		if v, err := strconv.ParseFloat(payload, 64); err == nil {
			return v
		}
	case bool:
		if payload == "true" {
			return true
		}
		if payload == "false" {
			return false
		}
	}
	return payload
}

func parseGraphQLArrayAccess(s string) (int, string, bool) {
	bracketIdx := strings.Index(s, "[")
	if bracketIdx == -1 || !strings.HasSuffix(s, "]") {
		return 0, "", false
	}

	name := s[:bracketIdx]
	idxStr := s[bracketIdx+1 : len(s)-1]
	idx, err := strconv.Atoi(idxStr)
	if err != nil {
		return 0, "", false
	}

	return idx, name, true
}

func extractGraphQLInlineArgPoints(query string, originalBody string) []InsertionPoint {
	var points []InsertionPoint
	i := 0
	n := len(query)

	for i < n {
		if query[i] == '(' {
			i++
			points = append(points, parseArgList(query, &i, n, originalBody)...)
		} else if query[i] == '"' {
			i++
			for i < n && query[i] != '"' {
				if query[i] == '\\' {
					i++
				}
				i++
			}
			if i < n {
				i++
			}
		} else {
			i++
		}
	}

	return points
}

func isVariableDefinitionList(query string, pos int, n int) bool {
	for pos < n && (query[pos] == ' ' || query[pos] == '\t' || query[pos] == '\n' || query[pos] == '\r') {
		pos++
	}
	return pos < n && query[pos] == '$'
}

func skipParenthesized(query string, pos *int, n int) {
	depth := 1
	for *pos < n && depth > 0 {
		switch query[*pos] {
		case '(':
			depth++
		case ')':
			depth--
		case '"':
			*pos++
			for *pos < n && query[*pos] != '"' {
				if query[*pos] == '\\' {
					*pos++
				}
				*pos++
			}
		}
		if *pos < n {
			*pos++
		}
	}
}

func parseArgList(query string, pos *int, n int, originalBody string) []InsertionPoint {
	if isVariableDefinitionList(query, *pos, n) {
		skipParenthesized(query, pos, n)
		return nil
	}

	var points []InsertionPoint
	depth := 1

	for *pos < n && depth > 0 {
		skipWhitespace(query, pos, n)
		if *pos >= n || depth <= 0 {
			break
		}

		ch := query[*pos]
		if ch == ')' {
			depth--
			*pos++
			continue
		}
		if ch == '(' {
			depth++
			*pos++
			continue
		}

		name := readArgName(query, pos, n)
		if name == "" {
			*pos++
			continue
		}

		skipWhitespace(query, pos, n)
		if *pos >= n || query[*pos] != ':' {
			continue
		}
		*pos++
		skipWhitespace(query, pos, n)

		if *pos >= n {
			break
		}

		if query[*pos] == '$' {
			skipUntilArgBoundary(query, pos, n)
			continue
		}

		value := readArgValue(query, pos, n)
		if value != "" {
			points = append(points, InsertionPoint{
				Type:         InsertionPointTypeGraphQLInlineArg,
				Name:         name,
				Value:        value,
				ValueType:    lib.GuessDataType(value),
				OriginalData: originalBody,
			})
		}

		skipWhitespace(query, pos, n)
		if *pos < n && query[*pos] == ',' {
			*pos++
		}
	}

	return points
}

func skipWhitespace(s string, pos *int, n int) {
	for *pos < n && (s[*pos] == ' ' || s[*pos] == '\t' || s[*pos] == '\n' || s[*pos] == '\r') {
		*pos++
	}
}

func readArgName(s string, pos *int, n int) string {
	start := *pos
	for *pos < n && isNameChar(s[*pos]) {
		*pos++
	}
	if *pos == start {
		return ""
	}
	return s[start:*pos]
}

func isNameChar(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_'
}

func skipUntilArgBoundary(s string, pos *int, n int) {
	for *pos < n && s[*pos] != ')' && s[*pos] != ',' && s[*pos] != ' ' && s[*pos] != '\t' {
		*pos++
	}
}

func readArgValue(s string, pos *int, n int) string {
	if *pos >= n {
		return ""
	}

	if s[*pos] == '"' {
		return readQuotedString(s, pos, n)
	}

	if s[*pos] == '[' || s[*pos] == '{' {
		skipBracketedValue(s, pos, n)
		return ""
	}

	start := *pos
	for *pos < n && s[*pos] != ')' && s[*pos] != ',' && s[*pos] != ' ' && s[*pos] != '\t' && s[*pos] != '\n' {
		*pos++
	}
	return s[start:*pos]
}

func readQuotedString(s string, pos *int, n int) string {
	*pos++
	var buf strings.Builder
	for *pos < n && s[*pos] != '"' {
		if s[*pos] == '\\' && *pos+1 < n {
			*pos++
			buf.WriteByte(s[*pos])
		} else {
			buf.WriteByte(s[*pos])
		}
		*pos++
	}
	if *pos < n {
		*pos++
	}
	return buf.String()
}

func skipBracketedValue(s string, pos *int, n int) {
	open := s[*pos]
	var close byte
	if open == '[' {
		close = ']'
	} else {
		close = '}'
	}
	depth := 1
	*pos++
	for *pos < n && depth > 0 {
		if s[*pos] == open {
			depth++
		} else if s[*pos] == close {
			depth--
		} else if s[*pos] == '"' {
			*pos++
			for *pos < n && s[*pos] != '"' {
				if s[*pos] == '\\' {
					*pos++
				}
				*pos++
			}
		}
		if *pos < n {
			*pos++
		}
	}
}

func modifyGraphQLInlineArg(body []byte, builders []InsertionPointBuilder) ([]byte, error) {
	var fullBody map[string]any
	if err := json.Unmarshal(body, &fullBody); err != nil {
		return nil, fmt.Errorf("failed to parse GraphQL body: %w", err)
	}

	queryStr, ok := fullBody["query"].(string)
	if !ok {
		return nil, fmt.Errorf("GraphQL body missing required 'query' field")
	}

	for _, builder := range builders {
		queryStr = replaceInlineArgValue(queryStr, builder.Point.Name, builder.Payload)
	}

	fullBody["query"] = queryStr
	return json.Marshal(fullBody)
}

func replaceInlineArgValue(query string, argName string, payload string) string {
	var result strings.Builder
	i := 0
	n := len(query)
	replaced := false

	for i < n {
		if !replaced && query[i] == '(' {
			result.WriteByte(query[i])
			i++
			writeArgListWithReplacement(&result, query, &i, n, argName, payload, &replaced)
		} else if query[i] == '"' {
			result.WriteByte(query[i])
			i++
			for i < n && query[i] != '"' {
				if query[i] == '\\' {
					result.WriteByte(query[i])
					i++
					if i < n {
						result.WriteByte(query[i])
						i++
					}
				} else {
					result.WriteByte(query[i])
					i++
				}
			}
			if i < n {
				result.WriteByte(query[i])
				i++
			}
		} else {
			result.WriteByte(query[i])
			i++
		}
	}

	return result.String()
}

func writeArgListWithReplacement(result *strings.Builder, query string, pos *int, n int, targetName string, payload string, replaced *bool) {
	depth := 1
	for *pos < n && depth > 0 {
		if query[*pos] == ')' {
			depth--
			result.WriteByte(query[*pos])
			*pos++
			continue
		}
		if query[*pos] == '(' {
			depth++
			result.WriteByte(query[*pos])
			*pos++
			continue
		}

		nameStart := *pos
		name := peekArgName(query, pos, n)
		if name == "" {
			result.WriteByte(query[*pos])
			*pos++
			continue
		}

		savedPos := *pos
		skipWhitespace(query, pos, n)
		if *pos >= n || query[*pos] != ':' {
			result.WriteString(query[nameStart:savedPos])
			*pos = savedPos
			continue
		}

		result.WriteString(name)
		for k := savedPos; k < *pos; k++ {
			result.WriteByte(query[k])
		}
		result.WriteByte(':')
		*pos++

		wsStart := *pos
		skipWhitespace(query, pos, n)
		for k := wsStart; k < *pos; k++ {
			result.WriteByte(query[k])
		}

		if *pos >= n {
			break
		}

		if !*replaced && name == targetName && query[*pos] != '$' {
			skipOriginalValue(query, pos, n)
			needsQuotes := payload != "true" && payload != "false" && payload != "null"
			if needsQuotes {
				if _, err := strconv.ParseFloat(payload, 64); err != nil {
					needsQuotes = true
				} else {
					needsQuotes = false
				}
			}
			if needsQuotes {
				result.WriteByte('"')
				for _, c := range payload {
					if c == '"' {
						result.WriteString(`\"`)
					} else if c == '\\' {
						result.WriteString(`\\`)
					} else {
						result.WriteRune(c)
					}
				}
				result.WriteByte('"')
			} else {
				result.WriteString(payload)
			}
			*replaced = true
		} else {
			copyOriginalValue(result, query, pos, n)
		}
	}
}

func peekArgName(s string, pos *int, n int) string {
	start := *pos
	for *pos < n && isNameChar(s[*pos]) {
		*pos++
	}
	if *pos == start {
		return ""
	}
	return s[start:*pos]
}

func skipOriginalValue(s string, pos *int, n int) {
	if *pos >= n {
		return
	}
	if s[*pos] == '"' {
		*pos++
		for *pos < n && s[*pos] != '"' {
			if s[*pos] == '\\' {
				*pos++
			}
			*pos++
		}
		if *pos < n {
			*pos++
		}
	} else if s[*pos] == '[' || s[*pos] == '{' {
		skipBracketedValue(s, pos, n)
	} else {
		for *pos < n && s[*pos] != ')' && s[*pos] != ',' && s[*pos] != ' ' && s[*pos] != '\t' && s[*pos] != '\n' {
			*pos++
		}
	}
}

func copyOriginalValue(result *strings.Builder, s string, pos *int, n int) {
	if *pos >= n {
		return
	}
	if s[*pos] == '"' {
		result.WriteByte('"')
		*pos++
		for *pos < n && s[*pos] != '"' {
			if s[*pos] == '\\' {
				result.WriteByte(s[*pos])
				*pos++
				if *pos < n {
					result.WriteByte(s[*pos])
					*pos++
				}
				continue
			}
			result.WriteByte(s[*pos])
			*pos++
		}
		if *pos < n {
			result.WriteByte('"')
			*pos++
		}
	} else if s[*pos] == '$' {
		for *pos < n && s[*pos] != ')' && s[*pos] != ',' && s[*pos] != ' ' && s[*pos] != '\t' {
			result.WriteByte(s[*pos])
			*pos++
		}
	} else if s[*pos] == '[' || s[*pos] == '{' {
		start := *pos
		skipBracketedValue(s, pos, n)
		result.WriteString(s[start:*pos])
	} else {
		for *pos < n && s[*pos] != ')' && s[*pos] != ',' && s[*pos] != ' ' && s[*pos] != '\t' && s[*pos] != '\n' {
			result.WriteByte(s[*pos])
			*pos++
		}
	}
}
