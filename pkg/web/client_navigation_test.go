package web

import (
	"strings"
	"testing"
)

func TestResolveClientRoute(t *testing.T) {
	tests := []struct {
		name    string
		rawURL  string
		baseURI string
		want    string
		wantOK  bool
	}{
		{
			name:    "angularjs hashbang route resolves against base href",
			rawURL:  "http://localhost:8092/javascript/frameworks/angularjs/index.html#!ng-href.found",
			baseURI: "http://localhost:8092/javascript/frameworks/angularjs/",
			want:    "http://localhost:8092/javascript/frameworks/angularjs/ng-href.found",
			wantOK:  true,
		},
		{
			name:    "angularjs hashbang with slash route",
			rawURL:  "http://localhost:8092/javascript/frameworks/angularjs/index.html#!/ng-href.found",
			baseURI: "http://localhost:8092/javascript/frameworks/angularjs/",
			want:    "http://localhost:8092/javascript/frameworks/angularjs/ng-href.found",
			wantOK:  true,
		},
		{
			name:    "fragment router hash route",
			rawURL:  "http://site.test/app/index.html#/users/5",
			baseURI: "http://site.test/app/",
			want:    "http://site.test/app/users/5",
			wantOK:  true,
		},
		{
			name:    "plain pushState path unchanged",
			rawURL:  "http://site.test/app/users/5",
			baseURI: "http://site.test/app/",
			want:    "http://site.test/app/users/5",
			wantOK:  true,
		},
		{
			name:    "bare in-page anchor stripped to document",
			rawURL:  "http://site.test/page#section",
			baseURI: "http://site.test/",
			want:    "http://site.test/page",
			wantOK:  true,
		},
		{
			name:    "malformed input rejected",
			rawURL:  "://nonsense",
			baseURI: "http://site.test/",
			want:    "",
			wantOK:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ResolveClientRoute(tt.rawURL, tt.baseURI)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClientNavigationHookScript(t *testing.T) {
	s := ClientNavigationHookScript
	if len(s) == 0 {
		t.Fatal("ClientNavigationHookScript should not be empty")
	}
	checks := []string{
		"(function()",
		"__sukyanClientNav",
		"history.pushState",
		"history.replaceState",
		"popstate",
		"hashchange",
		"__sukyanClientNavReady",
	}
	for _, c := range checks {
		if !strings.Contains(s, c) {
			t.Errorf("hook script missing %q", c)
		}
	}
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "(function()") {
		t.Error("script should be an IIFE")
	}
}

func TestClientNavigationHookPreservesOriginals(t *testing.T) {
	s := ClientNavigationHookScript
	if !strings.Contains(s, "origPushState") {
		t.Error("hook script should store original pushState")
	}
	if !strings.Contains(s, "origReplaceState") {
		t.Error("hook script should store original replaceState")
	}
	if !strings.Contains(s, ".apply(history") && !strings.Contains(s, ".call(history") {
		t.Error("hook script should call through to original history methods")
	}
}
