package crawl

import "testing"

func TestMergeClientNavURLs(t *testing.T) {
	existing := []string{"http://a/x", "http://a/y"}
	captured := []string{"http://a/y", "http://a/z", "http://a/z"}
	got := mergeClientNavURLs(existing, captured)
	want := map[string]bool{"http://a/x": true, "http://a/y": true, "http://a/z": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %d unique", got, len(want))
	}
	for _, u := range got {
		if !want[u] {
			t.Fatalf("unexpected url %q in %v", u, got)
		}
	}
}
