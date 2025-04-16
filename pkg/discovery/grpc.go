package discovery

import (
	"encoding/base64"
	"strings"

	"github.com/pyneda/sukyan/db"
)

var gRPCPaths = []string{
	"grpc",
	"rpc",
	"api/grpc",

	// Reflection and health check endpoints
	"grpc.reflection.v1alpha.ServerReflection",
	"grpc.health.v1.Health",

	// Framework specific
	"twirp",
	"connect",
	"grpc-web",

	// Common service paths
	"grpc/status",

	// Documentation/UI paths
	"grpcui",
	"grpcweb",
	"grpcwebtext",
}

func IsGRPCValidationFunc(history *db.History) (bool, string, int) {

	confidence := 50
	details := make([]string, 0)

	contentType := strings.ToLower(history.ResponseContentType)

	if strings.Contains(contentType, "application/grpc") ||
		strings.Contains(contentType, "application/grpc-web") ||
		strings.Contains(contentType, "application/grpc-web-text") {
		confidence += 30
		details = append(details, "gRPC content type detected")
	}

	headersStr, err := history.GetResponseHeadersAsString()
	if err == nil {
		headersLower := strings.ToLower(headersStr)
		headerMarkers := map[string]string{
			"grpc-status":      "gRPC status header",
			"grpc-message":     "gRPC message header",
			"grpc-encoding":    "gRPC encoding header",
			"grpc-accept":      "gRPC accept header",
			"x-grpc-web":       "gRPC web header",
			"x-envoy-upstream": "Potential gRPC proxy header",
		}

		for marker, desc := range headerMarkers {
			if strings.Contains(headersLower, marker) {
				confidence += 15
				details = append(details, desc+" detected")
			}
		}
	}

	body, _ := history.ResponseBody()
	bodyStr := string(body)
	bodyLower := strings.ToLower(bodyStr)

	commonMarkers := []string{
		"\"service\":",
		"\"method\":",
		"protobufs",
		"grpc.reflection",
		"grpc.health",
		"rpc error:",
		"stream error:",
		"unimplemented method",
		"protocol buffer",
	}

	markerCount := 0
	matchedMarkers := []string{}

	for _, marker := range commonMarkers {
		if strings.Contains(bodyLower, strings.ToLower(marker)) && !strings.Contains(history.URL, marker) {
			markerCount++
			matchedMarkers = append(matchedMarkers, marker)
		}
	}

	if markerCount > 0 {
		confidence += markerCount * 10
		details = append(details, "gRPC markers detected:"+strings.Join(matchedMarkers, "\n - "))
	}

	// TODO: When history model is updated to include protocol, we can check if http2 has been enforced

	switch history.StatusCode {
	case 502, 503, 504:
		// Common when hitting gRPC endpoints with regular HTTP
		confidence += 5
	case 426: // Upgrade Required
		confidence += 10
		details = append(details, "HTTP upgrade required (common for gRPC endpoints)")
	}

	if confidence > 50 {
		if confidence > 100 {
			confidence = 100
		}
		return true, strings.Join(details, "\n"), confidence
	}

	return false, "", 0
}

func DiscoverGRPCEndpoints(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	defaultBody := []byte{0x00, 0x00, 0x00, 0x00, 0x00} // Flag byte (0x00) + 4 bytes length (0x00000000)
	encodedBody := base64.StdEncoding.EncodeToString(defaultBody)

	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "POST",
			Body:        encodedBody,
			Paths:       gRPCPaths,
			Concurrency: DefaultConcurrency,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept":       "application/grpc, application/grpc-web, application/grpc-web-text, */*",
				"X-Grpc-Web":   "1",
				"Content-Type": "application/grpc-web+proto",
				"X-User-Agent": "grpc-web-javascript/0.1",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsGRPCValidationFunc,
		IssueCode:      db.GrpcEndpointDetectedCode,
	})
}
