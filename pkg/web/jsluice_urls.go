package web

import "github.com/BishopFox/jsluice"

type ExtractedJSURL struct {
	URL         string            `json:"url"`
	Method      string            `json:"method"`
	Type        string            `json:"type"`
	QueryParams []string          `json:"queryParams"`
	BodyParams  []string          `json:"bodyParams"`
	Headers     map[string]string `json:"headers,omitempty"`
	ContentType string            `json:"contentType,omitempty"`
	Source      string            `json:"source,omitempty"`
}

func ExtractURLsFromJS(code []byte) []ExtractedJSURL {
	analyzer := jsluice.NewAnalyzer(code)
	matches := analyzer.GetURLs()
	urls := make([]ExtractedJSURL, 0, len(matches))
	for _, m := range matches {
		urls = append(urls, ExtractedJSURL{
			URL:         m.URL,
			Method:      m.Method,
			Type:        m.Type,
			QueryParams: m.QueryParams,
			BodyParams:  m.BodyParams,
			Headers:     m.Headers,
			ContentType: m.ContentType,
			Source:      m.Source,
		})
	}
	return urls
}

func ExtractURLsFromJSON(jsonData []byte) []ExtractedJSURL {
	wrapped := make([]byte, 0, len(jsonData)+len("var _=;")+1)
	wrapped = append(wrapped, "var _="...)
	wrapped = append(wrapped, jsonData...)
	wrapped = append(wrapped, ';')
	return ExtractURLsFromJS(wrapped)
}
