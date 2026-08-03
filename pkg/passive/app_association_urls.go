package passive

import (
	"encoding/json"
	"net/url"
	"strings"
)

// appleAppSiteAssociationPaths are the two locations iOS reads the universal
// link declaration from; the root one is legacy but still served.
var appleAppSiteAssociationPaths = []string{
	"/.well-known/apple-app-site-association",
	"/apple-app-site-association",
}

// isAppSiteAssociationURL reports whether the URL points at an
// apple-app-site-association document.
func isAppSiteAssociationURL(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	path := strings.TrimSuffix(parsed.Path, "/")
	for _, known := range appleAppSiteAssociationPaths {
		if strings.EqualFold(path, known) {
			return true
		}
	}
	return false
}

// ParseAppSiteAssociation resolves the universal link path patterns an
// apple-app-site-association document declares. Those patterns are the routes the
// vendor's mobile app deep links into, which are regularly absent from the web
// UI's own navigation. Exclusions ("NOT /admin/*", components marked exclude) are
// skipped: they describe what the app does not handle, not what does not exist,
// but they are equally not a location to fetch.
func ParseAppSiteAssociation(body []byte, base *url.URL) []string {
	locations := make([]string, 0)

	var document struct {
		AppLinks struct {
			Details json.RawMessage `json:"details"`
		} `json:"applinks"`
	}
	if err := json.Unmarshal(body, &document); err != nil || len(document.AppLinks.Details) == 0 {
		return locations
	}

	add := func(pattern string) {
		if pattern == "" || strings.HasPrefix(strings.ToUpper(pattern), "NOT ") {
			return
		}
		trimmed := trimPathPattern(pattern, appLinkWildcards)
		if trimmed == "" {
			return
		}
		absoluteURL, urlType, err := analyzeURL(trimmed, base)
		if err != nil || urlType != "web" {
			return
		}
		locations = append(locations, absoluteURL)
	}

	for _, detail := range appLinkDetails(document.AppLinks.Details) {
		for _, pattern := range detail.Paths {
			add(pattern)
		}
		for _, component := range detail.Components {
			if component.Exclude {
				continue
			}
			add(component.Path)
		}
	}

	return locations
}

func extractURLsFromAppSiteAssociation(body []byte, base *url.URL) ExtractedURLS {
	return ExtractedURLS{Web: ParseAppSiteAssociation(body, base), NonWeb: []string{}}
}

type appLinkComponent struct {
	Path    string
	Exclude bool
}

type appLinkDetail struct {
	Paths      []string
	Components []appLinkComponent
}

func (d *appLinkDetail) UnmarshalJSON(data []byte) error {
	var raw struct {
		Paths      []string         `json:"paths"`
		Components []map[string]any `json:"components"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	d.Paths = raw.Paths
	for _, component := range raw.Components {
		path, _ := component["/"].(string)
		if path == "" {
			continue
		}
		exclude, _ := component["exclude"].(bool)
		d.Components = append(d.Components, appLinkComponent{Path: path, Exclude: exclude})
	}
	return nil
}

// appLinkDetails accepts both shapes Apple has shipped: an array of entries, and
// the older object keyed by app identifier.
func appLinkDetails(raw json.RawMessage) []appLinkDetail {
	var asArray []appLinkDetail
	if err := json.Unmarshal(raw, &asArray); err == nil {
		return asArray
	}
	var asObject map[string]appLinkDetail
	if err := json.Unmarshal(raw, &asObject); err != nil {
		return nil
	}
	details := make([]appLinkDetail, 0, len(asObject))
	for _, detail := range asObject {
		details = append(details, detail)
	}
	return details
}
