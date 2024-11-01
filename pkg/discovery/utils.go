package discovery

import (
	"net/http"
	"net/url"
	"path"
	"strings"
)

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func joinURLPath(baseURL, urlPath string) string {
	u, err := url.Parse(baseURL)
	if err != nil {
		return baseURL + "/" + strings.TrimPrefix(urlPath, "/")
	}
	u.Path = path.Join(u.Path, urlPath)
	return u.String()
}

func setDefaultHeaders(req *http.Request, hasBody bool) {
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", DefaultUserAgent)
	}
	if req.Header.Get("Connection") == "" {
		req.Header.Set("Connection", "keep-alive")
	}
	if hasBody && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", "application/json")
	}
}
