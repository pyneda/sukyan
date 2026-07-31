package scan

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

// graphql-ws / graphql-transport-ws frames wrap a GraphQL operation inside a JSON
// envelope ({"type":"subscribe","payload":{"query":"...","variables":{...}}}), so
// the injectable arguments live in payload.query - invisible to the plain JSON
// extractor. These helpers make the scanner graphql-ws-aware.

// graphqlWSClientControlTypes are client message types that carry no injectable
// data; they are skipped as scan targets on graphql-ws connections.
var graphqlWSClientControlTypes = map[string]bool{
	"connection_init":      true,
	"connection_terminate": true,
	"ping":                 true,
	"pong":                 true,
	"complete":             true,
	"stop":                 true,
}

// looksLikeGraphQLQuery is looser than the HTTP isGraphQLBody check (which
// requires a space after the keyword): real clients emit compact documents like
// "query{ user(id:\"1\"){ name } }".
func looksLikeGraphQLQuery(s string) bool {
	t := strings.TrimSpace(s)
	if strings.HasPrefix(t, "{") {
		return true
	}
	for _, kw := range []string{"query", "mutation", "subscription"} {
		if !strings.HasPrefix(t, kw) {
			continue
		}
		rest := t[len(kw):]
		if rest == "" {
			return true
		}
		switch rest[0] {
		case '{', '(', ' ', '\t', '\n', '\r':
			return true
		}
	}
	return false
}

// locateGraphQLWSBody returns the map holding the GraphQL query and variables. The
// returned container is a live reference into obj so callers can mutate and
// re-marshal it.
func locateGraphQLWSBody(obj map[string]any) (container map[string]any, query string, variables map[string]any, ok bool) {
	if payload, isMap := obj["payload"].(map[string]any); isMap {
		if q, isStr := payload["query"].(string); isStr && looksLikeGraphQLQuery(q) {
			vars, _ := payload["variables"].(map[string]any)
			return payload, q, vars, true
		}
	}
	if q, isStr := obj["query"].(string); isStr && looksLikeGraphQLQuery(q) {
		vars, _ := obj["variables"].(map[string]any)
		return obj, q, vars, true
	}
	return nil, "", nil, false
}

// extractGraphQLTransportWSInsertionPoints reuses the HTTP GraphQL extractors and
// re-types the points so CreateModifiedWebSocketMessage routes them back here.
func extractGraphQLTransportWSInsertionPoints(frame string) []InsertionPoint {
	var obj map[string]any
	if err := json.Unmarshal([]byte(frame), &obj); err != nil {
		return nil
	}
	_, query, variables, ok := locateGraphQLWSBody(obj)
	if !ok {
		return nil
	}

	var points []InsertionPoint
	for _, p := range extractGraphQLInlineArgPoints(query, frame) {
		p.Type = InsertionPointTypeWSGraphQLInlineArg
		points = append(points, p)
	}
	for _, p := range extractGraphQLVariablePoints("", variables, frame) {
		p.Type = InsertionPointTypeWSGraphQLVariable
		points = append(points, p)
	}
	return points
}

func modifyGraphQLTransportWSFrame(frame string, point InsertionPoint, payload string) (string, error) {
	var obj map[string]any
	if err := json.Unmarshal([]byte(frame), &obj); err != nil {
		return "", fmt.Errorf("failed to parse graphql-ws frame: %w", err)
	}
	container, query, _, ok := locateGraphQLWSBody(obj)
	if !ok {
		return "", fmt.Errorf("frame is not a graphql-ws body")
	}

	switch point.Type {
	case InsertionPointTypeWSGraphQLInlineArg:
		container["query"] = applyGraphQLInlineArgPayloads(query, []InsertionPointBuilder{{Point: point, Payload: payload}})

	case InsertionPointTypeWSGraphQLVariable:
		variables, isMap := container["variables"].(map[string]any)
		if !isMap {
			variables = make(map[string]any)
		}
		setNestedValue(variables, point.Name, payload)
		container["variables"] = variables

	default:
		return "", fmt.Errorf("unsupported graphql-ws insertion point type: %s", point.Type)
	}

	out, err := json.Marshal(obj)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// isGraphQLTransportWSSubscribeFrame reports whether a frame wraps a GraphQL
// operation, so its connection_init prologue is replayed before probing it.
func isGraphQLTransportWSSubscribeFrame(payload string) bool {
	if !isLikelyJSON(payload) {
		return false
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(payload), &obj); err != nil {
		return false
	}
	_, _, _, ok := locateGraphQLWSBody(obj)
	return ok
}

// isGraphQLWSControlFrame reports whether a frame is a control frame with nothing
// injectable. A control frame carrying a non-empty payload (e.g. connection_init
// auth credentials) is not treated as pure control.
func isGraphQLWSControlFrame(payload string) bool {
	if !isLikelyJSON(payload) {
		return false
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(payload), &obj); err != nil {
		return false
	}
	t, isStr := obj["type"].(string)
	if !isStr || !graphqlWSClientControlTypes[t] {
		return false
	}
	if _, _, _, ok := locateGraphQLWSBody(obj); ok {
		return false
	}
	return !payloadHasContent(obj["payload"])
}

func payloadHasContent(payload any) bool {
	switch v := payload.(type) {
	case nil:
		return false
	case map[string]any:
		return len(v) > 0
	case []any:
		return len(v) > 0
	case string:
		return v != ""
	default:
		return true
	}
}

func connectionUsesGraphQLWSSubprotocol(conn *db.WebSocketConnection) bool {
	headers, err := conn.GetRequestHeadersAsMap()
	if err != nil {
		return false
	}
	for key, values := range headers {
		if !strings.EqualFold(key, "Sec-WebSocket-Protocol") {
			continue
		}
		for _, v := range values {
			lv := strings.ToLower(v)
			if strings.Contains(lv, "graphql-transport-ws") || strings.Contains(lv, "graphql-ws") {
				return true
			}
		}
	}
	return false
}
