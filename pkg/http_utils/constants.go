package http_utils

import "strings"

const DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3"

// Error category constants for request error categorization
const (
	// Network connection errors
	ErrorCategoryConnectionClosedEOF  = "connection_closed_eof"
	ErrorCategoryConnectionRefused    = "connection_refused"
	ErrorCategoryConnectionReset      = "connection_reset"
	ErrorCategoryConnectionBrokenPipe = "connection_broken_pipe"
	ErrorCategoryDNSResolution        = "dns_resolution"
	ErrorCategoryNetworkUnreachable   = "network_unreachable"
	ErrorCategoryHostUnreachable      = "host_unreachable"

	// Timeout errors
	ErrorCategoryTimeoutDeadlineExceeded = "timeout_deadline_exceeded"
	ErrorCategoryTimeoutGeneric          = "timeout_generic"

	// TLS/SSL errors
	ErrorCategoryTLSError         = "tls_error"
	ErrorCategoryCertificateError = "certificate_error"

	// Protocol errors
	ErrorCategoryProtocolError     = "protocol_error"
	ErrorCategoryMalformedResponse = "malformed_response"

	// URL/parsing errors
	ErrorCategoryURLControlCharacter = "url_control_character"
	ErrorCategoryURLInvalid          = "url_invalid"

	// Generic server errors
	ErrorCategoryServerError = "server_error"

	// Unknown/uncategorized errors
	ErrorCategoryUnknown = "unknown"
	ErrorCategoryNone    = "none"
)

// CategorizeRequestError categorizes different types of request errors
func CategorizeRequestError(err error) string {
	if err == nil {
		return ErrorCategoryNone
	}

	errorMsg := strings.ToLower(err.Error())

	if strings.Contains(errorMsg, "eof") {
		return ErrorCategoryConnectionClosedEOF
	}
	if strings.Contains(errorMsg, "connection refused") {
		return ErrorCategoryConnectionRefused
	}
	if strings.Contains(errorMsg, "connection reset") {
		return ErrorCategoryConnectionReset
	}
	if strings.Contains(errorMsg, "broken pipe") {
		return ErrorCategoryConnectionBrokenPipe
	}
	if strings.Contains(errorMsg, "no such host") {
		return ErrorCategoryDNSResolution
	}
	if strings.Contains(errorMsg, "network unreachable") {
		return ErrorCategoryNetworkUnreachable
	}
	if strings.Contains(errorMsg, "host unreachable") {
		return ErrorCategoryHostUnreachable
	}

	if strings.Contains(errorMsg, "context deadline exceeded") {
		return ErrorCategoryTimeoutDeadlineExceeded
	}
	if strings.Contains(errorMsg, "timeout") {
		return ErrorCategoryTimeoutGeneric
	}

	if strings.Contains(errorMsg, "tls") || strings.Contains(errorMsg, "ssl") {
		return ErrorCategoryTLSError
	}
	if strings.Contains(errorMsg, "certificate") {
		return ErrorCategoryCertificateError
	}

	if strings.Contains(errorMsg, "protocol") {
		return ErrorCategoryProtocolError
	}
	if strings.Contains(errorMsg, "malformed") {
		return ErrorCategoryMalformedResponse
	}

	if strings.Contains(errorMsg, "invalid control character in url") {
		return ErrorCategoryURLControlCharacter
	}
	if strings.Contains(errorMsg, "invalid url") {
		return ErrorCategoryURLInvalid
	}

	if strings.Contains(errorMsg, "server") {
		return ErrorCategoryServerError
	}

	return ErrorCategoryUnknown
}
