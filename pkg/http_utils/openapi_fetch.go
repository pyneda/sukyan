package http_utils

import (
	"fmt"
	"io"
	"net/http"
)

// FetchOpenAPISpec fetches an OpenAPI specification from a given URL
func FetchOpenAPISpec(url string) ([]byte, error) {
	client := CreateHttpClient()
	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch from URL: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("received non-200 response: %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	return bodyBytes, nil
}
