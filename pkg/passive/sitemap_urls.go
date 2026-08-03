package passive

import (
	"bytes"
	"compress/gzip"
	"encoding/xml"
	"io"
	"net/url"
	"strings"
)

const (
	maxSitemapDecompressedBytes = 32 << 20
	maxSitemapLocations         = 50000
	maxSitemapLocationLength    = 4096
)

// Sitemap holds the locations a sitemaps.org document declares, split by the
// role the document gives them: Sitemaps are further documents to read, URLs are
// crawlable content.
type Sitemap struct {
	URLs     []string
	Sitemaps []string
}

// ParseSitemap reports whether body is a sitemaps.org document and, if so,
// returns the absolute locations it declares.
//
// The generic extractor reads XML as text, which cannot work here: <loc> values
// escape & as &amp; per the spec, so every parameterised URL comes back carrying
// a bogus "amp;" parameter and the real page is never crawled. Gzipped documents
// are accepted because sitemap indexes routinely point at .xml.gz children.
func ParseSitemap(body []byte, base *url.URL) (Sitemap, bool) {
	data, ok := decompressSitemap(body)
	if !ok {
		return Sitemap{}, false
	}

	decoder := xml.NewDecoder(bytes.NewReader(data))
	decoder.Strict = false

	var (
		sitemap   Sitemap
		root      string
		container string
		location  strings.Builder
		inLoc     bool
		found     int
	)

	add := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		absoluteURL, urlType, err := analyzeURL(raw, base)
		if err != nil || urlType != "web" {
			return
		}
		if container == "sitemap" || (container == "" && root == "sitemapindex") {
			sitemap.Sitemaps = append(sitemap.Sitemaps, absoluteURL)
			return
		}
		sitemap.URLs = append(sitemap.URLs, absoluteURL)
	}

	for found < maxSitemapLocations {
		token, err := decoder.Token()
		if err != nil {
			// A truncated or malformed tail still leaves the locations already read
			// usable, which matters on the multi-megabyte documents large sites serve.
			if err == io.EOF {
				break
			}
			break
		}

		switch element := token.(type) {
		case xml.StartElement:
			name := strings.ToLower(element.Name.Local)
			if root == "" {
				root = name
				if root != "urlset" && root != "sitemapindex" {
					return Sitemap{}, false
				}
			}
			switch name {
			case "url", "sitemap":
				container = name
			case "loc":
				inLoc = true
				location.Reset()
			}
		case xml.CharData:
			if inLoc && location.Len() < maxSitemapLocationLength {
				location.Write(element)
			}
		case xml.EndElement:
			switch strings.ToLower(element.Name.Local) {
			case "loc":
				if inLoc {
					inLoc = false
					add(location.String())
					found++
				}
			case "url", "sitemap":
				container = ""
			}
		}
	}

	if root == "" {
		return Sitemap{}, false
	}
	return sitemap, true
}

// Locations returns every declared location, child sitemaps included, as the
// crawler frontier consumes both.
func (s Sitemap) Locations() []string {
	return append(append([]string{}, s.Sitemaps...), s.URLs...)
}

func decompressSitemap(body []byte) ([]byte, bool) {
	if len(body) < 2 || body[0] != 0x1f || body[1] != 0x8b {
		return body, true
	}
	reader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, false
	}
	defer reader.Close()

	data, err := io.ReadAll(io.LimitReader(reader, maxSitemapDecompressedBytes))
	if err != nil && len(data) == 0 {
		return nil, false
	}
	return data, true
}
