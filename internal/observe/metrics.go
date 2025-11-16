package observe

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Define Prometheus metrics for monitoring the gateway.
var (
	// Record the total number of HTTP requests received.
	HttpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "lens_gateway",
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests.",
		},
		// exammple query: sum(http_requests_total{method="GET", path="/api/v1", status="500"})
		[]string{"method", "path", "status"},
	)

	// Record the latency of HTTP requests in seconds.
	// A histogram automatically buckets observed values (e.g., request durations), which helps in calculating quantiles like p95, p99.
	HttpRequestDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "lens_gateway",
			Name:      "http_request_duration_seconds",
			Help:      "Latency of HTTP requests in seconds.",
			// Buckets defines the bucket distribution for latency, in seconds.
			// a request taking 0.07s would fall into the "0.1" bucket
			// example quert: sum(rate(http_request_duration_seconds_bucket{le="0.05", method="GET", path="/api/v1"}[5m])) by (le)
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"method", "path"},
	)
)
