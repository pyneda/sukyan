package openapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
)

type Format string

const (
	FormatJSON Format = "json"
	FormatYAML Format = "yaml"
	FormatJS   Format = "js"
)

var (
	ErrInvalidFormat   = errors.New("invalid format specified")
	ErrFormatDetection = errors.New("unable to detect format")
	ErrEmptyContent    = errors.New("empty content provided")
	ErrMissingURL      = errors.New("URL is required")
)

func ValidateFormat(format string) (Format, error) {
	if format == "" {
		return "", nil
	}

	normalizedFormat := Format(strings.ToLower(format))
	switch normalizedFormat {
	case FormatJSON, FormatYAML, FormatJS:
		return normalizedFormat, nil
	default:
		return "", ErrInvalidFormat
	}
}

func DetectFormatFromURL(url string) Format {
	if url == "" {
		return ""
	}

	ext := strings.ToLower(filepath.Ext(url))
	switch ext {
	case ".json":
		return FormatJSON
	case ".yaml", ".yml":
		return FormatYAML
	case ".js":
		return FormatJS
	default:
		return ""
	}
}

func DetectFormatFromHeader(headers http.Header) Format {
	if headers == nil {
		return ""
	}

	contentType := headers.Get("Content-Type")
	switch {
	case strings.Contains(contentType, "application/json"):
		return FormatJSON
	case strings.Contains(contentType, "application/yaml"), strings.Contains(contentType, "text/yaml"):
		return FormatYAML
	case strings.Contains(contentType, "application/javascript"), strings.Contains(contentType, "text/javascript"):
		return FormatJS
	default:
		return ""
	}
}

func DetectFormatFromContent(content []byte) Format {
	if len(content) == 0 {
		return ""
	}

	var js json.RawMessage
	if json.Unmarshal(content, &js) == nil {
		return FormatJSON
	}

	contentStr := string(content)
	if strings.Contains(contentStr, "openapi:") || strings.Contains(contentStr, "swagger:") {
		return FormatYAML
	}

	return ""
}

func DetectFormat(url string, headers http.Header, content []byte) (Format, error) {
	if url == "" {
		return "", ErrMissingURL
	}

	if len(content) == 0 {
		return "", ErrEmptyContent
	}

	format := DetectFormatFromURL(url)
	if format != "" {
		return format, nil
	}

	format = DetectFormatFromHeader(headers)
	if format != "" {
		return format, nil
	}

	format = DetectFormatFromContent(content)
	if format != "" {
		return format, nil
	}

	return "", ErrFormatDetection
}
