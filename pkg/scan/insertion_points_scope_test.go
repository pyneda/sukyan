package scan

import (
	"testing"

	"github.com/pyneda/sukyan/db"
)

func pointNames(points []InsertionPoint, pointType InsertionPointType) []string {
	var names []string
	for _, p := range points {
		if p.Type == pointType {
			names = append(names, p.Name)
		}
	}
	return names
}

func hasPoint(points []InsertionPoint, pointType InsertionPointType, name string) bool {
	for _, p := range points {
		if p.Type == pointType && p.Name == name {
			return true
		}
	}
	return false
}

// An empty scope means "no restriction configured", the same meaning
// HistoryItemScanOptions.IsScopedInsertionPoint gives it. Callers that do not
// configure insertion points — the API scan executor is one — would otherwise
// silently lose every query, path, header and cookie insertion point and only
// ever fuzz request bodies.
func TestGetInsertionPoints_EmptyScopeMeansAll(t *testing.T) {
	history := &db.History{
		URL:        "http://example.com/api/users/42?username=alice&sort=name",
		Method:     "GET",
		RawRequest: []byte("GET /api/users/42?username=alice&sort=name HTTP/1.1\r\nHost: example.com\r\nX-Api-Version: 2\r\nCookie: session=abc\r\n\r\n"),
	}

	points, err := GetInsertionPoints(history, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !hasPoint(points, InsertionPointTypeParameter, "username") {
		t.Errorf("query parameter 'username' missing; parameters found: %v", pointNames(points, InsertionPointTypeParameter))
	}
	if !hasPoint(points, InsertionPointTypeParameter, "sort") {
		t.Errorf("query parameter 'sort' missing; parameters found: %v", pointNames(points, InsertionPointTypeParameter))
	}
	if len(pointNames(points, InsertionPointTypeURLPath)) == 0 {
		t.Error("expected url path insertion points with an empty scope")
	}
	if !hasPoint(points, InsertionPointTypeHeader, "X-Api-Version") {
		t.Errorf("header insertion point missing; headers found: %v", pointNames(points, InsertionPointTypeHeader))
	}
	if !hasPoint(points, InsertionPointTypeCookie, "session") {
		t.Errorf("cookie insertion point missing; cookies found: %v", pointNames(points, InsertionPointTypeCookie))
	}
}

// An explicit scope still restricts, so callers that deliberately narrow the
// attack surface keep working.
func TestGetInsertionPoints_ExplicitScopeStillRestricts(t *testing.T) {
	history := &db.History{
		URL:        "http://example.com/api/users/42?username=alice",
		Method:     "GET",
		RawRequest: []byte("GET /api/users/42?username=alice HTTP/1.1\r\nHost: example.com\r\nX-Api-Version: 2\r\nCookie: session=abc\r\n\r\n"),
	}

	points, err := GetInsertionPoints(history, []string{"parameters"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !hasPoint(points, InsertionPointTypeParameter, "username") {
		t.Error("expected the scoped query parameter to be present")
	}
	if len(pointNames(points, InsertionPointTypeHeader)) != 0 {
		t.Errorf("headers must stay out of scope, got %v", pointNames(points, InsertionPointTypeHeader))
	}
	if len(pointNames(points, InsertionPointTypeCookie)) != 0 {
		t.Errorf("cookies must stay out of scope, got %v", pointNames(points, InsertionPointTypeCookie))
	}
	if len(pointNames(points, InsertionPointTypeURLPath)) != 0 {
		t.Errorf("url path must stay out of scope, got %v", pointNames(points, InsertionPointTypeURLPath))
	}
}
