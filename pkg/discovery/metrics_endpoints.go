package discovery

import (
	"fmt"
	"strings"

	"github.com/pyneda/sukyan/db"
)

var PrometheusMetricsPaths = []string{
	"metrics",
	"api/metrics",
	"prometheus",
	"prometheus/metrics",
	"actuator/prometheus",
	"monitoring/prometheus",
	"monitoring/metrics",
	".well-known/metrics",
	"probe/metrics",
	"metrics/prometheus",
	"status/metrics",
	"_prometheus/metrics",
	"app/metrics",
	"v1/metrics",
	"system/metrics",
	"internal/metrics",
	"admin/metrics",
	"public/metrics",
	"stats/prometheus",
	"federate",
	"metric-proxy",
	"metrics/prometheus/federate",
	"core/metrics",
	"prometheus/federate",
	"application/metrics",
	"service/metrics",
	"node/metrics",
	"api/v1/metrics",
	"api/prometheus",
	"api/prometheus/metrics",
}

func IsPrometheusMetricsValidationFunc(history *db.History) (bool, string, int) {
	if history.StatusCode != 200 {
		return false, "", 0
	}

	details := fmt.Sprintf("Prometheus metrics endpoint found: %s\n", history.URL)
	confidence := 10

	body, _ := history.ResponseBody()
	bodyStr := string(body)

	if strings.Contains(history.ResponseContentType, "text/plain") {
		confidence += 10
		details += "- Correct content type for Prometheus metrics\n"
	}

	if strings.Contains(history.ResponseContentType, "text/html") {
		confidence -= 20
		details += "- Incorrect content type for Prometheus metrics\n"
	}

	metricPatterns := []string{
		"process_cpu",
		"process_memory",
		"process_open_fds",
		"process_resident_memory",
		"process_virtual_memory",
		"go_gc_duration_seconds",
		"go_goroutines",
		"go_threads",
		"go_memstats",
		"http_requests_total",
		"http_request_duration",
		"http_response_size",
		"system_cpu_usage",
		"system_load_average",
		"system_memory_usage",
		"jvm_memory_used",
		"jvm_threads",
		"jvm_gc_collection",
		"node_cpu_seconds",
		"node_memory_bytes",
		"node_filesystem",
		"node_network",
		"container_cpu",
		"container_memory",
		"container_network",
		"kubernetes_pod",
		"kubernetes_container",
		"database_connections",
		"database_queries",
		"database_errors",
		"mq_messages",
		"cache_hits",
		"cache_misses",
		"requests_in_flight",
		"response_time",
		"error_rate",
		"uptime_seconds",
		"heap_usage",
		"non_heap_usage",
		"thread_count",
		"class_count",
		"gc_count",
		"logback_events",
		"tomcat_sessions",
		"hikari_connections",
		"rabbitmq_queued",
		"redis_connected_clients",
		"mongodb_connections",
		"elasticsearch_docs",
	}

	for _, pattern := range metricPatterns {
		if strings.Contains(bodyStr, pattern) {
			details += fmt.Sprintf("- Exposes %s metrics\n", pattern)
			confidence += 40
		}
	}

	sensitivePatterns := []string{
		"password",
		"secret",
		"token",
		"key",
		"database",
		"user",
		"api_key",
		"auth",
		"credentials",
		"private",
		"cert",
		"ssh",
		"aws",
		"azure",
		"gcp",
		"admin",
		"root",
		"jdbc",
		"connection_string",
		"bearer",
		"oauth",
	}

	for _, pattern := range sensitivePatterns {
		if strings.Contains(strings.ToLower(bodyStr), pattern) {
			details += fmt.Sprintf("- Contains potentially sensitive metric: %s\n", pattern)
			confidence += 5
		}
	}

	return confidence >= minConfidence(), details, min(confidence, 100)
}

func DiscoverMetricsEndpoints(options DiscoveryOptions) (DiscoverAndCreateIssueResults, error) {
	return DiscoverAndCreateIssue(DiscoverAndCreateIssueInput{
		DiscoveryInput: DiscoveryInput{
			URL:         options.BaseURL,
			Method:      "GET",
			Paths:       PrometheusMetricsPaths,
			Concurrency: DefaultConcurrency,
			Timeout:     DefaultTimeout,
			Headers: map[string]string{
				"Accept": "text/plain",
			},
			HistoryCreationOptions: options.HistoryCreationOptions,
			HttpClient:             options.HttpClient,
			SiteBehavior:           options.SiteBehavior,
			ScanMode:               options.ScanMode,
		},
		ValidationFunc: IsPrometheusMetricsValidationFunc,
		IssueCode:      db.ExposedPrometheusMetricsCode,
	})
}
