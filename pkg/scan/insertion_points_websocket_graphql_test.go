package scan

import (
	"encoding/json"
	"testing"

	"github.com/pyneda/sukyan/db"
	scan_options "github.com/pyneda/sukyan/pkg/scan/options"
	"gorm.io/datatypes"
)

// The exact frame the ws-sqli graphql console sends (ui.go): a compact document
// with no space after "query" and the injectable id inside the query string.
const gqlWSSubscribeFrame = `{"id":"1","type":"subscribe","payload":{"query":"query{ user(id:\"1\"){ name } }"}}`

func queryFromFrame(t *testing.T, frame string) string {
	t.Helper()
	var obj map[string]any
	if err := json.Unmarshal([]byte(frame), &obj); err != nil {
		t.Fatalf("frame is not valid JSON: %v\n%s", err, frame)
	}
	payload, ok := obj["payload"].(map[string]any)
	if !ok {
		t.Fatalf("frame has no payload object: %s", frame)
	}
	q, ok := payload["query"].(string)
	if !ok {
		t.Fatalf("frame payload has no query string: %s", frame)
	}
	return q
}

func TestLooksLikeGraphQLQuery(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{`query{ user(id:"1"){ name } }`, true}, // compact, no space
		{`query { user }`, true},
		{`query Foo($id: ID!) { user(id: $id) { name } }`, true},
		{`mutation{ x }`, true},
		{`subscription Sub { onX }`, true},
		{`{ user(id: "1") { name } }`, true}, // anonymous shorthand
		{`  {x}`, true},                      // leading whitespace
		{`querystring`, false},               // not a keyword boundary
		{`mutationsPerSecond`, false},
		{`hello world`, false},
		{``, false},
	}
	for _, c := range cases {
		if got := looksLikeGraphQLQuery(c.in); got != c.want {
			t.Errorf("looksLikeGraphQLQuery(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestExtractGraphQLTransportWSInsertionPoints_InlineArg(t *testing.T) {
	points := extractGraphQLTransportWSInsertionPoints(gqlWSSubscribeFrame)

	var inline *InsertionPoint
	for i := range points {
		if points[i].Type == InsertionPointTypeWSGraphQLInlineArg && points[i].Name == "id" {
			inline = &points[i]
		}
	}
	if inline == nil {
		t.Fatalf("expected a ws_graphql_inline_arg point named 'id', got %+v", points)
	}
	if inline.Value != "1" {
		t.Errorf("inline arg value = %q, want %q", inline.Value, "1")
	}
}

func TestExtractGraphQLTransportWSInsertionPoints_Variables(t *testing.T) {
	frame := `{"id":"1","type":"subscribe","payload":{"query":"query Q($id: ID!){ user(id:$id){ name } }","variables":{"id":"7"}}}`
	points := extractGraphQLTransportWSInsertionPoints(frame)

	var found bool
	for _, p := range points {
		if p.Type == InsertionPointTypeWSGraphQLVariable && p.Name == "id" && p.Value == "7" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a ws_graphql_variable point id=7, got %+v", points)
	}
}

func TestExtractGraphQLTransportWSInsertionPoints_NotGraphQL(t *testing.T) {
	// A plain JSON frame and a control frame must yield no graphql points.
	for _, frame := range []string{
		`{"id":"1","action":"login","user":"bob"}`,
		`{"type":"connection_init"}`,
		`{"type":"ping"}`,
		`not json`,
	} {
		if pts := extractGraphQLTransportWSInsertionPoints(frame); len(pts) != 0 {
			t.Errorf("expected no graphql points for %q, got %+v", frame, pts)
		}
	}
}

func TestModifyGraphQLTransportWSFrame_InlineArg(t *testing.T) {
	point := InsertionPoint{Type: InsertionPointTypeWSGraphQLInlineArg, Name: "id", Value: "1"}

	got, err := modifyGraphQLTransportWSFrame(gqlWSSubscribeFrame, point, "1'")
	if err != nil {
		t.Fatalf("modify: %v", err)
	}

	// The whole envelope must remain intact; only the id argument inside the
	// query string changes. The regex sink in the testbed extracts id:"1'".
	wantQuery := `query{ user(id:"1'"){ name } }`
	if q := queryFromFrame(t, got); q != wantQuery {
		t.Errorf("modified query = %q, want %q", q, wantQuery)
	}

	// Envelope fields preserved.
	var obj map[string]any
	if err := json.Unmarshal([]byte(got), &obj); err != nil {
		t.Fatalf("modified frame not valid JSON: %v", err)
	}
	if obj["type"] != "subscribe" || obj["id"] != "1" {
		t.Errorf("envelope fields altered: %s", got)
	}
}

func TestModifyGraphQLTransportWSFrame_InlineArgSQLPayload(t *testing.T) {
	point := InsertionPoint{Type: InsertionPointTypeWSGraphQLInlineArg, Name: "id", Value: "1"}
	got, err := modifyGraphQLTransportWSFrame(gqlWSSubscribeFrame, point, "1' OR '1'='1")
	if err != nil {
		t.Fatalf("modify: %v", err)
	}
	wantQuery := `query{ user(id:"1' OR '1'='1"){ name } }`
	if q := queryFromFrame(t, got); q != wantQuery {
		t.Errorf("modified query = %q, want %q", q, wantQuery)
	}
}

func TestModifyGraphQLTransportWSFrame_Variable(t *testing.T) {
	frame := `{"id":"1","type":"subscribe","payload":{"query":"query Q($id: ID!){ user(id:$id){ name } }","variables":{"id":"7"}}}`
	point := InsertionPoint{Type: InsertionPointTypeWSGraphQLVariable, Name: "id", Value: "7"}

	got, err := modifyGraphQLTransportWSFrame(frame, point, "7' OR 1=1--")
	if err != nil {
		t.Fatalf("modify: %v", err)
	}

	var obj map[string]any
	if err := json.Unmarshal([]byte(got), &obj); err != nil {
		t.Fatalf("modified frame not valid JSON: %v", err)
	}
	payload := obj["payload"].(map[string]any)
	vars := payload["variables"].(map[string]any)
	if vars["id"] != "7' OR 1=1--" {
		t.Errorf("variable id = %v, want %q", vars["id"], "7' OR 1=1--")
	}
}

func TestCreateModifiedWebSocketMessage_GraphQLInlineArg(t *testing.T) {
	original := &db.WebSocketMessage{Opcode: 1, PayloadData: gqlWSSubscribeFrame}
	point := InsertionPoint{Type: InsertionPointTypeWSGraphQLInlineArg, Name: "id", Value: "1"}

	modified, err := CreateModifiedWebSocketMessage(original, point, "1'")
	if err != nil {
		t.Fatalf("CreateModifiedWebSocketMessage: %v", err)
	}
	wantQuery := `query{ user(id:"1'"){ name } }`
	if q := queryFromFrame(t, modified.PayloadData); q != wantQuery {
		t.Errorf("modified query = %q, want %q", q, wantQuery)
	}
}

func TestGetWebSocketMessageInsertionPoints_GraphQLWS(t *testing.T) {
	msg := &db.WebSocketMessage{PayloadData: gqlWSSubscribeFrame}

	// Full production WS scope: every WS insertion-point type.
	scoped := []string{}
	for _, tpe := range WebSocketInsertionPointTypes() {
		scoped = append(scoped, tpe.String())
	}

	points, err := GetWebSocketMessageInsertionPoints(msg, scoped)
	if err != nil {
		t.Fatalf("GetWebSocketMessageInsertionPoints: %v", err)
	}

	var hasInlineArg, hasJSON bool
	for _, p := range points {
		if p.Type == InsertionPointTypeWSGraphQLInlineArg && p.Name == "id" {
			hasInlineArg = true
		}
		if p.Type == InsertionPointTypeWSJSONValue {
			hasJSON = true
		}
	}
	if !hasInlineArg {
		t.Errorf("expected a graphql inline-arg point in production scope, got %d points", len(points))
	}
	if !hasJSON {
		t.Errorf("expected the plain JSON points to still be produced alongside graphql points")
	}
}

func TestIsGraphQLWSControlFrame(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{`{"type":"connection_init"}`, true},
		{`{"type":"connection_init","payload":{}}`, true},   // empty payload is still pure control
		{`{"type":"connection_init","payload":null}`, true}, // null payload is pure control
		{`{"type":"ping"}`, true},
		{`{"id":"1","type":"complete"}`, true},
		{`{"type":"connection_terminate"}`, true},
		// connection_init carrying auth credentials has injectable fields -> NOT skipped.
		{`{"type":"connection_init","payload":{"authToken":"abc123"}}`, false},
		{gqlWSSubscribeFrame, false},                 // carries a query -> data frame
		{`{"type":"subscribe","payload":{}}`, false}, // subscribe type but no query -> not our control set
		{`{"action":"login"}`, false},
		{`not json`, false},
	}
	for _, c := range cases {
		if got := isGraphQLWSControlFrame(c.in); got != c.want {
			t.Errorf("isGraphQLWSControlFrame(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestIsGraphQLTransportWSSubscribeFrame(t *testing.T) {
	if !isGraphQLTransportWSSubscribeFrame(gqlWSSubscribeFrame) {
		t.Error("expected subscribe frame to be recognised")
	}
	if isGraphQLTransportWSSubscribeFrame(`{"type":"connection_init"}`) {
		t.Error("connection_init must not be recognised as a subscribe frame")
	}
	if isGraphQLTransportWSSubscribeFrame(`{"id":"1"}`) {
		t.Error("plain JSON must not be recognised as a subscribe frame")
	}
}

func TestShouldReplayPriorFrames(t *testing.T) {
	subscribe := db.WebSocketMessage{PayloadData: gqlWSSubscribeFrame}
	plain := db.WebSocketMessage{PayloadData: `{"id":"1"}`}

	withProto, _ := json.Marshal(map[string][]string{"Sec-WebSocket-Protocol": {"graphql-transport-ws"}})
	gqlConn := &db.WebSocketConnection{RequestHeaders: datatypes.JSON(withProto)}
	plainConn := &db.WebSocketConnection{}

	off := scan_options.WebSocketScanOptions{ReplayMessages: false}
	on := scan_options.WebSocketScanOptions{ReplayMessages: true}

	// graphql-ws subscribe on a graphql-ws connection forces replay even with
	// global replay OFF (the connection_init prologue must precede it).
	if !shouldReplayPriorFrames(off, gqlConn, subscribe) {
		t.Error("graphql-ws subscribe on a graphql-ws conn should force replay when ReplayMessages is off")
	}
	// The SAME subscribe frame on a connection that did NOT negotiate the
	// subprotocol must NOT force replay - the graphql-ws behaviour is gated so
	// non-GraphQL apps are unaffected.
	if shouldReplayPriorFrames(off, plainConn, subscribe) {
		t.Error("subscribe frame on a non-graphql-ws conn must not force replay when ReplayMessages is off")
	}
	// A plain frame does not force replay when global replay is off.
	if shouldReplayPriorFrames(off, gqlConn, plain) {
		t.Error("plain frame should not force replay when ReplayMessages is off")
	}
	// Global replay ON always replays, regardless of subprotocol.
	if !shouldReplayPriorFrames(on, plainConn, plain) {
		t.Error("ReplayMessages=true should always replay")
	}
}

// TestGetWebSocketMessageInsertionPoints_GraphQLGatedOut verifies that a scope
// without the ws_graphql kinds (a non-graphql-ws connection) yields NO graphql
// points for a frame that would otherwise be graphql-ws, keeping the feature from
// altering non-GraphQL scans.
func TestGetWebSocketMessageInsertionPoints_GraphQLGatedOut(t *testing.T) {
	msg := &db.WebSocketMessage{PayloadData: gqlWSSubscribeFrame}
	scoped := []string{"ws_raw", "ws_json"} // no ws_graphql

	points, err := GetWebSocketMessageInsertionPoints(msg, scoped)
	if err != nil {
		t.Fatalf("GetWebSocketMessageInsertionPoints: %v", err)
	}
	for _, p := range points {
		if p.Type == InsertionPointTypeWSGraphQLInlineArg || p.Type == InsertionPointTypeWSGraphQLVariable {
			t.Errorf("expected no graphql points without ws_graphql scope, got %s", p.String())
		}
	}
}

func TestConnectionUsesGraphQLWSSubprotocol(t *testing.T) {
	withProto, _ := json.Marshal(map[string][]string{"Sec-WebSocket-Protocol": {"graphql-transport-ws"}})
	withoutProto, _ := json.Marshal(map[string][]string{"Origin": {"http://x"}})

	yes := &db.WebSocketConnection{RequestHeaders: datatypes.JSON(withProto)}
	no := &db.WebSocketConnection{RequestHeaders: datatypes.JSON(withoutProto)}

	if !connectionUsesGraphQLWSSubprotocol(yes) {
		t.Error("expected graphql-transport-ws subprotocol to be detected")
	}
	if connectionUsesGraphQLWSSubprotocol(no) {
		t.Error("did not expect a graphql-ws subprotocol")
	}
}
