package http_utils

import (
	"fmt"
	"net/http"
	"net/textproto"
	"strings"

	"github.com/pyneda/sukyan/db"
	"github.com/rs/zerolog/log"
)

// type RequestHeaders map[string][]string

func SetRequestHeadersFromHistoryItem(request *http.Request, historyItem *db.History) error {
	headers, err := historyItem.GetRequestHeadersAsMap()
	if err != nil {
		log.Error().Err(err).Msg("Error setting headers for a new request due to an error getting the original request headers")
	} else {
		for key, values := range headers {
			if strings.ToLower(key) == "content-length" {
				continue
			}
			for _, value := range values {
				log.Debug().Str("key", key).Str("value", value).Msg("Setting header")
				request.Header.Set(key, value)
			}
		}
	}

	return nil
}

func HeadersToString(headersMap map[string][]string) string {
	headers := make([]string, 0, len(headersMap))
	for name, values := range headersMap {
		for _, value := range values {
			headers = append(headers, fmt.Sprintf("%s: %s", name, value))
		}
	}
	return strings.Join(headers, "\n")
}

// ClassifyHTTPResponseHeader classifies a given HTTP response header key by its purpose.
func ClassifyHTTPResponseHeader(headerKey string) string {
	headerCategories := map[string]map[string]bool{
		"Caching": {
			"Age":           true,
			"Cache-Control": true,
			"Expires":       true,
			"Pragma":        true,
			"Vary":          true,
			"Warning":       true,
		},
		"Security": {
			"Access-Control-Allow-Origin":      true,
			"Access-Control-Allow-Methods":     true,
			"Access-Control-Allow-Headers":     true,
			"Access-Control-Allow-Credentials": true,
			"Access-Control-Max-Age":           true,
			"Access-Control-Expose-Headers":    true,
			"Access-Control-Request-Method":    true,
			"Access-Control-Request-Headers":   true,
			"Strict-Transport-Security":        true,
			"Content-Security-Policy":          true,
			"X-Content-Type-Options":           true,
			"X-XSS-Protection":                 true,
			"X-Frame-Options":                  true,
		},
		"Transport": {
			"Transfer-Encoding": true,
			"Trailer":           true,
			"Connection":        true,
			"Keep-Alive":        true,
			"Upgrade":           true,
		},
		"Information": {
			"Allow":       true,
			"Date":        true,
			"Location":    true,
			"Retry-After": true,
			"Via":         true,
		},
		"Content": {
			"Accept-Ranges":    true,
			"Content-Encoding": true,
			"Content-Language": true,
			"Content-Length":   true,
			"Content-Location": true,
			"Content-MD5":      true,
			"Content-Range":    true,
			"Content-Type":     true,
			"ETag":             true,
			"Last-Modified":    true,
		},
		"Rate-Limiting": {
			"RateLimit-Limit":     true,
			"RateLimit-Remaining": true,
			"RateLimit-Reset":     true,
		},
		"Authentication": {
			"WWW-Authenticate": true,
			"Set-Cookie":       true,
		},
		"Fingerprint": {
			"Server":           true,
			"X-Powered-By":     true,
			"X-AspNet-Version": true,
			"X-Runtime":        true,
			"X-Version":        true,
			"X-Generator":      true,
			"X-Drupal-Cache":   true,
		},
	}

	// Normalize the header key to capitalize each word, similar to the canonical MIME header key format
	canonicalHeaderKey := textproto.CanonicalMIMEHeaderKey(headerKey)

	for category, headers := range headerCategories {
		if headers[canonicalHeaderKey] {
			return category
		}
	}
	return "Uncommon"
}

// reverseProxyHeaders contains headers that indicate the presence of a reverse proxy, CDN, or load balancer.
// These are used to determine when to run expensive audits like HTTP request smuggling.
var reverseProxyHeaders = []string{
	"Via",                           // RFC 7230 - proxy chain
	"X-Cache",                       // CDN/proxy caching
	"X-Cache-Hits",                  // Varnish/CDN
	"X-Served-By",                   // CDN node identifier
	"X-Backend-Server",              // Backend identification
	"Cf-Ray",                        // Cloudflare
	"Cf-Cache-Status",               // Cloudflare caching
	"X-Amz-Cf-Id",                   // CloudFront
	"X-Amz-Cf-Pop",                  // CloudFront POP
	"X-Azure-Ref",                   // Azure CDN
	"X-Msedge-Ref",                  // Azure Edge
	"X-Varnish",                     // Varnish cache
	"X-Proxy-Cache",                 // Generic proxy cache
	"X-Akamai-Transformed",          // Akamai
	"Akamai-Cache-Status",           // Akamai caching
	"X-Akamai-Request-Id",           // Akamai request ID
	"Fastly-Debug-Digest",           // Fastly
	"X-Fastly-Request-Id",           // Fastly request ID
	"X-Timer",                       // Fastly timing
	"X-Kong-Upstream-Latency",       // Kong API Gateway
	"X-Kong-Proxy-Latency",          // Kong API Gateway
	"X-Envoy-Upstream-Service-Time", // Envoy proxy
	"X-Envoy-Attempt-Count",         // Envoy proxy
	"X-Cdn-Provider",                // Generic CDN indicator
	"X-Edge-Location",               // Generic edge/CDN
	"X-Sucuri-Id",                   // Sucuri WAF/CDN
	"X-Iinfo",                       // Incapsula/Imperva
	"X-Cdn",                         // Generic CDN
	"X-Proxy",                       // Generic proxy
	"X-Forwarded-Server",            // Forwarding proxy
	"X-Cache-Status",                // Generic cache status
	"X-Vercel-Cache",                // Vercel CDN
	"X-Vercel-Id",                   // Vercel
	"X-Nf-Request-Id",               // Netlify
	"X-Served-By",                   // Generic CDN node
	"Fly-Request-Id",                // Fly.io
	"X-Request-Id",                  // Common in proxied setups (less specific)
	"Cf-Connecting-Ip",              // Cloudflare (indicates CF is in path)
	"True-Client-Ip",                // Akamai/CDN
	"X-Real-Ip",                     // nginx proxy
	"X-Original-Forwarded-For",      // Proxy chain
	"X-Forwarded-Host",              // Proxy forwarding
	"X-Forwarded-Proto",             // Proxy forwarding
	"X-Litespeed-Cache",             // LiteSpeed cache
	"X-Litespeed-Cache-Control",     // LiteSpeed cache
	"X-Qc-Pop",                      // QUIC.cloud
	"Quic-Status",                   // QUIC.cloud
	"X-Middleton-Response",          // Fastly
	"Surrogate-Key",                 // Fastly/Varnish surrogate keys
	"Surrogate-Control",             // CDN surrogate control
	"X-Iplb-Instance",               // OVH Load Balancer
	"X-Iplb-Request-Id",             // OVH Load Balancer
	"X-Hw",                          // Huawei CDN
	"X-Swift-Savetime",              // OpenStack Swift (object storage behind proxy)
	"X-Trans-Id",                    // OpenStack Swift
	"X-Openstack-Request-Id",        // OpenStack
	"Alt-Svc",                       // Often indicates CDN/proxy with HTTP/3 support
}

// reverseProxyServerPatterns contains patterns to match in the Server header
var reverseProxyServerPatterns = []string{
	"nginx",
	"cloudflare",
	"varnish",
	"squid",
	"traefik",
	"envoy",
	"caddy",
	"haproxy",
	"openresty",
	"tengine",
	"litespeed",
	"akamai",
	"fastly",
	"imperva",
	"incapsula",
	"sucuri",
	"apache traffic server",
	"ats/",
	"kong/",
	"apigee",
}

// HasReverseProxyIndicators checks response headers for signs of reverse proxy/CDN/load balancer.
// This is useful for determining whether to run expensive audits like HTTP request smuggling.
func HasReverseProxyIndicators(headers map[string][]string) bool {
	for _, header := range reverseProxyHeaders {
		canonicalHeader := textproto.CanonicalMIMEHeaderKey(header)
		if _, exists := headers[canonicalHeader]; exists {
			return true
		}
	}

	// Check Server header for known proxies/CDNs
	if serverValues, ok := headers["Server"]; ok && len(serverValues) > 0 {
		serverLower := strings.ToLower(serverValues[0])
		for _, pattern := range reverseProxyServerPatterns {
			if strings.Contains(serverLower, pattern) {
				return true
			}
		}
	}

	return false
}

// HasReverseProxyIndicatorsFromHistory is a convenience wrapper that extracts headers from a History object.
func HasReverseProxyIndicatorsFromHistory(history *db.History) bool {
	headers, err := history.ResponseHeaders()
	if err != nil {
		return false
	}
	return HasReverseProxyIndicators(headers)
}

var webSocketProtocolHeaders = []string{
	"Connection",
	"Upgrade",
	"Sec-WebSocket-Key",
	"Sec-WebSocket-Version",
	"Sec-WebSocket-Protocol",
	"Sec-WebSocket-Extensions",
	"Sec-WebSocket-Accept",
}

func IsWebSocketProtocolHeader(key string) bool {
	for _, h := range webSocketProtocolHeaders {
		if strings.EqualFold(key, h) {
			return true
		}
	}
	return false
}
