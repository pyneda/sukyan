package db

import (
	"encoding/json"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildSitemap runs the real tree-construction path over a fixed set of history
// records, so the tests cannot drift from production.
func buildSitemap(t *testing.T, histories []History) []*SitemapNode {
	t.Helper()
	rows := make([]sitemapRow, 0, len(histories))
	for _, h := range histories {
		rows = append(rows, sitemapRow{ID: h.ID, URL: h.URL, Method: h.Method, StatusCode: h.StatusCode})
	}
	return buildSitemapTree(rows)
}

func hist(id uint, method, rawURL string, status int) History {
	return History{
		BaseModel:  BaseModel{ID: id},
		Method:     method,
		URL:        rawURL,
		StatusCode: status,
	}
}

func TestBuildSitemapTreeSharedWithProduction(t *testing.T) {
	rows := []sitemapRow{
		{ID: 1, URL: "https://app.test/api/users", Method: "GET", StatusCode: 200},
		{ID: 2, URL: "https://app.test/api/orders", Method: "POST", StatusCode: 201},
	}

	roots := buildSitemapTree(rows)

	require.Len(t, roots, 1)
	assert.Equal(t, "https://app.test", roots[0].URL)
	assert.Equal(t, 2, roots[0].Requests)
	require.Len(t, roots[0].Children, 1)
	api := roots[0].Children[0]
	assert.Equal(t, "api", api.Path)
	assert.Equal(t, 2, api.Requests)
	assert.Len(t, api.Children, 2)
}

// Requests must count each history record once per node on its path. The
// previous implementation shipped ID arrays that the UI unioned across
// ancestors and descendants, inflating every badge by 2x-9x.
func TestSitemapRequestCountsAreDeduplicated(t *testing.T) {
	histories := []History{
		hist(1, "GET", "https://app.example.com/api/v1/users", 200),
		hist(2, "GET", "https://app.example.com/api/v1/users", 200),
		hist(3, "GET", "https://app.example.com/api/v1/accounts", 200),
		hist(4, "GET", "https://app.example.com/assets/app.js", 200),
	}

	roots := buildSitemap(t, histories)
	require.Len(t, roots, 1)
	root := roots[0]

	assert.Equal(t, 4, root.Requests, "root counts every request exactly once")

	api := findChild(t, root, "api")
	assert.Equal(t, 3, api.Requests)

	v1 := findChild(t, api, "v1")
	assert.Equal(t, 3, v1.Requests)

	users := findChild(t, v1, "users")
	assert.Equal(t, 2, users.Requests, "two records hit /api/v1/users")

	assets := findChild(t, root, "assets")
	assert.Equal(t, 1, assets.Requests)

	// A parent's count equals the sum of its children when their paths are disjoint.
	assert.Equal(t, api.Requests+assets.Requests, root.Requests)
}

// Query strings must not become nodes. The old builder appended one child per
// record with no dedup, producing hundreds of visually identical rows.
func TestSitemapCollapsesQueryStringsIntoOneEndpoint(t *testing.T) {
	histories := []History{
		hist(1, "GET", "https://cta.example.com/embed?portalId=1&contentId=9", 200),
		hist(2, "GET", "https://cta.example.com/embed?portalId=2&contentId=8", 200),
		hist(3, "GET", "https://cta.example.com/embed?portalId=3&utk=abc", 200),
	}

	roots := buildSitemap(t, histories)
	require.Len(t, roots, 1)

	embed := findChild(t, roots[0], "embed")
	assert.Empty(t, embed.Children, "query strings never become child nodes")
	assert.Equal(t, 3, embed.Requests)
	assert.Equal(t, []string{"contentId", "portalId", "utk"}, embed.Params,
		"parameter names are unioned across every request to the endpoint")
}

// Ordering must be identical across runs. Go randomises map iteration, and the
// previous implementation ranged over a map, so the tree reshuffled on every
// fetch.
func TestSitemapOrderingIsDeterministic(t *testing.T) {
	histories := []History{}
	var id uint = 1
	for _, host := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		for i := 0; i < len(host); i++ {
			histories = append(histories, hist(id, "GET", "https://"+host+".example.com/x", 200))
			id++
		}
	}

	first, err := json.Marshal(buildSitemap(t, histories))
	require.NoError(t, err)

	for i := 0; i < 25; i++ {
		next, err := json.Marshal(buildSitemap(t, histories))
		require.NoError(t, err)
		require.JSONEq(t, string(first), string(next), "run %d differed", i)
	}
}

// Busiest first, ties broken by path.
func TestSitemapSortsBusiestFirst(t *testing.T) {
	histories := []History{
		hist(1, "GET", "https://x.example.com/quiet", 200),
		hist(2, "GET", "https://x.example.com/busy", 200),
		hist(3, "GET", "https://x.example.com/busy", 200),
		hist(4, "GET", "https://x.example.com/busy", 200),
		hist(5, "GET", "https://x.example.com/also-quiet", 200),
	}

	roots := buildSitemap(t, histories)
	require.Len(t, roots, 1)

	paths := []string{}
	for _, c := range roots[0].Children {
		paths = append(paths, c.Path)
	}
	assert.Equal(t, []string{"busy", "also-quiet", "quiet"}, paths,
		"requests desc, then path asc for equal counts")
}

func TestSitemapAggregatesMethodsAndStatusClasses(t *testing.T) {
	histories := []History{
		hist(1, "GET", "https://api.example.com/v1/thing", 200),
		hist(2, "POST", "https://api.example.com/v1/thing", 201),
		hist(3, "GET", "https://api.example.com/v1/thing", 404),
		hist(4, "DELETE", "https://api.example.com/v1/thing", 500),
		hist(5, "GET", "https://api.example.com/v1/thing", 302),
		hist(6, "GET", "https://api.example.com/v1/thing", 0), // transport failure
	}

	roots := buildSitemap(t, histories)
	thing := findChild(t, findChild(t, roots[0], "v1"), "thing")

	assert.Equal(t, []string{"DELETE", "GET", "POST"}, thing.Methods, "sorted and deduplicated")
	assert.Equal(t, map[string]int{"2xx": 2, "3xx": 1, "4xx": 1, "5xx": 1, "none": 1}, thing.StatusClasses)

	total := 0
	for _, n := range thing.StatusClasses {
		total += n
	}
	assert.Equal(t, thing.Requests, total, "status classes account for every request")
}

func TestSitemapReservesIssueAndCoverageFields(t *testing.T) {
	roots := buildSitemap(t, []History{hist(1, "GET", "https://x.example.com/a", 200)})
	require.Len(t, roots, 1)

	assert.Nil(t, roots[0].Issues, "reserved until the issue_requests join is scoped")
	assert.Nil(t, roots[0].Scanned)

	// The fields must survive serialisation as explicit nulls so clients can rely
	// on the shape rather than probing for absence.
	raw, err := json.Marshal(roots[0])
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Contains(t, decoded, "issues")
	assert.Contains(t, decoded, "scanned")
	assert.Contains(t, decoded, "has_children")
}

func TestSitemapHasChildrenReflectsTree(t *testing.T) {
	roots := buildSitemap(t, []History{
		hist(1, "GET", "https://x.example.com/dir/leaf.js", 200),
	})
	root := roots[0]
	assert.True(t, root.HasChildren)

	dir := findChild(t, root, "dir")
	assert.True(t, dir.HasChildren)

	leaf := findChild(t, dir, "leaf.js")
	assert.False(t, leaf.HasChildren)
	assert.Empty(t, leaf.Children)
	assert.Equal(t, SitemapNodeTypeJs, leaf.Type)
}

func TestQueryParamNames(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{"https://x.com/a", nil},
		{"https://x.com/a?", nil},
		{"https://x.com/a?b=1&a=2", []string{"a", "b"}},
		{"https://x.com/a?dup=1&dup=2", []string{"dup"}},
		{"https://x.com/a?=empty", nil},
		{"https://x.com/a?flag", []string{"flag"}},
	}
	for _, c := range cases {
		u, err := url.Parse(c.raw)
		require.NoError(t, err)
		assert.Equal(t, c.want, queryParamNames(u), c.raw)
	}
}

func TestStatusClass(t *testing.T) {
	assert.Equal(t, "1xx", statusClass(100))
	assert.Equal(t, "2xx", statusClass(204))
	assert.Equal(t, "3xx", statusClass(301))
	assert.Equal(t, "4xx", statusClass(404))
	assert.Equal(t, "5xx", statusClass(503))
	assert.Equal(t, "none", statusClass(0))
	assert.Equal(t, "none", statusClass(-1))
}

// Params are bounded so a tracking endpoint crawled with thousands of unique
// one-off names cannot bloat the payload.
func TestSitemapBoundsParamsPerNode(t *testing.T) {
	histories := []History{}
	for i := 0; i < maxParamsPerNode+50; i++ {
		histories = append(histories, hist(uint(i+1), "GET", "https://x.example.com/px?p"+itoa(i)+"=1", 200))
	}
	roots := buildSitemap(t, histories)
	px := findChild(t, roots[0], "px")
	assert.LessOrEqual(t, len(px.Params), maxParamsPerNode)
	assert.Equal(t, maxParamsPerNode+50, px.Requests, "capping names never drops requests")
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := ""
	for i > 0 {
		digits = string(rune('0'+i%10)) + digits
		i /= 10
	}
	return digits
}

func wsRow(id uint, rawURL string, live bool, messages int) sitemapRow {
	return sitemapRow{ID: id, URL: rawURL, WebSocket: true, Live: live, Messages: messages}
}

func TestWebSocketRowMarksOnlyTheEndpointNode(t *testing.T) {
	roots := buildSitemapTree([]sitemapRow{wsRow(1, "https://app.test/ws/feed", true, 12)})

	require.Len(t, roots, 1)
	assert.False(t, roots[0].WebSocket, "host row is not itself an endpoint")
	ws := roots[0].Children[0]
	assert.False(t, ws.WebSocket, "intermediate directory is not an endpoint")
	feed := ws.Children[0]
	assert.True(t, feed.WebSocket)
	assert.Equal(t, "feed", feed.Path)
}

func TestWebSocketAggregatesAreSubtreeInclusive(t *testing.T) {
	roots := buildSitemapTree([]sitemapRow{
		wsRow(1, "https://app.test/ws/feed", true, 12),
		wsRow(2, "https://app.test/ws/feed", false, 8),
		wsRow(3, "https://app.test/ws/alerts", false, 3),
	})

	root := roots[0]
	assert.Equal(t, 3, root.Connections)
	assert.Equal(t, 1, root.LiveConnections)
	assert.Equal(t, 23, root.Messages)

	ws := root.Children[0]
	assert.Equal(t, 3, ws.Connections)

	feed := findChild(t, ws, "feed")
	assert.Equal(t, 2, feed.Connections)
	assert.Equal(t, 1, feed.LiveConnections)
	assert.Equal(t, 20, feed.Messages)
}

func TestWebSocketRowsDoNotTouchRequestsOrStatusClasses(t *testing.T) {
	roots := buildSitemapTree([]sitemapRow{
		{ID: 1, URL: "https://app.test/ws/feed", Method: "GET", StatusCode: 200},
		wsRow(2, "https://app.test/ws/feed", true, 5),
	})

	feed := findChild(t, roots[0].Children[0], "feed")
	assert.Equal(t, 1, feed.Requests, "a connection is not a request")
	assert.Equal(t, map[string]int{"2xx": 1}, feed.StatusClasses, "a connection contributes no status class, not even 'none'")
	assert.Equal(t, []string{"GET"}, feed.Methods, "a connection contributes no method")
	assert.True(t, feed.WebSocket)
}

func TestSortRanksWebSocketOnlyNodesByConnections(t *testing.T) {
	roots := buildSitemapTree([]sitemapRow{
		{ID: 1, URL: "https://app.test/missing", Method: "GET", StatusCode: 404},
		wsRow(2, "https://app.test/live", false, 0),
		wsRow(3, "https://app.test/live", false, 0),
	})

	children := roots[0].Children
	require.Len(t, children, 2)
	assert.Equal(t, "live", children[0].Path, "2 connections outrank 1 request")
}

func TestWebSocketOriginJoinsTheHttpsHostTree(t *testing.T) {
	roots := buildSitemapTree([]sitemapRow{
		{ID: 1, URL: "https://app.test/api", Method: "GET", StatusCode: 200},
		wsRow(2, "wss://app.test/ws/feed", true, 4),
	})

	require.Len(t, roots, 1, "wss:// must not spawn a second root")
	assert.Equal(t, "https://app.test", roots[0].URL)

	ws := findChild(t, roots[0], "ws")
	feed := findChild(t, ws, "feed")
	assert.Equal(t, "https://app.test/ws/feed", feed.URL)
	assert.True(t, feed.WebSocket)
}

func TestInsecureWebSocketOriginJoinsTheHttpHostTree(t *testing.T) {
	roots := buildSitemapTree([]sitemapRow{wsRow(1, "ws://app.test/socket", false, 0)})

	require.Len(t, roots, 1)
	assert.Equal(t, "http://app.test", roots[0].URL)
}

func findChild(t *testing.T, parent *SitemapNode, path string) *SitemapNode {
	t.Helper()
	for _, c := range parent.Children {
		if c.Path == path {
			return c
		}
	}
	t.Fatalf("no child %q under %s", path, parent.URL)
	return nil
}
