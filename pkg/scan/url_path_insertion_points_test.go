package scan

import (
	"net/url"
	"testing"
)

func pathPointNames(t *testing.T, rawURL string) []string {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parsing %q: %v", rawURL, err)
	}
	points, err := handleURLPaths(u)
	if err != nil {
		t.Fatalf("handleURLPaths(%q): %v", rawURL, err)
	}
	names := make([]string, 0, len(points))
	for _, p := range points {
		names = append(names, p.Name)
	}
	return names
}

func TestHandleURLPathsSkipsFixedRouteSegments(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want []string
	}{
		{
			// Fuzzing "engine" produced ~2k wasted 404s on the ssti-matrix
			// testbed; only the trailing segment is a plausible sink.
			name: "fixed route names are skipped except the last segment",
			url:  "http://host/engine/jinja/render",
			want: []string{"render"},
		},
		{
			// The one path-parameter vulnerability in the eval ground truth is
			// a numeric :id, which must still be tested.
			name: "numeric ids are kept",
			url:  "http://host/messages/42/content",
			want: []string{"42", "content"},
		},
		{
			name: "uuids are kept",
			url:  "http://host/api/550e8400-e29b-41d4-a716-446655440000/edit",
			want: []string{"550e8400-e29b-41d4-a716-446655440000", "edit"},
		},
		{
			name: "single segment is always tested",
			url:  "http://host/download",
			want: []string{"download"},
		},
		{
			name: "trailing slash still yields the last real segment",
			url:  "http://host/files/report.pdf/",
			want: []string{"report.pdf"},
		},
		{
			name: "root path yields nothing",
			url:  "http://host/",
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pathPointNames(t, tt.url)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("got %v, want %v", got, tt.want)
				}
			}
		})
	}
}
