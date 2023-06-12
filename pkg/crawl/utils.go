package crawl

import (
	"net/url"
)

func stripHash(rawURL string) (string, error) {
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	parsedURL.Fragment = ""

	return parsedURL.String(), nil
}
