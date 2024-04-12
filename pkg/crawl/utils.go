package crawl

import (
	"net/url"
)

// normalizeURLParams normalizes the URL parameters by appending an "X" to each value.
func normalizeURLParams(rawURL string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", err
	}

	queryParams := u.Query()

	for key, values := range queryParams {
		for i := range values {
			values[i] = "X"
		}
		queryParams[key] = values
	}

	u.RawQuery = queryParams.Encode()

	return u.String(), nil
}
