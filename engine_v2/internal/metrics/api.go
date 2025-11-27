package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// API metrics - Métricas de la API HTTP
var (
	// HTTPRequestsTotal contador total de requests HTTP
	HTTPRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tucentropdf_http_requests_total",
			Help: "Total number of HTTP requests by method, endpoint and status code",
		},
		[]string{"method", "endpoint", "status_code"},
	)

	// HTTPRequestDurationSeconds histograma de duración de requests
	HTTPRequestDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tucentropdf_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds",
			Buckets: []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10}, // 1ms a 10s
		},
		[]string{"method", "endpoint"},
	)

	// HTTPErrorsTotal contador total de errores HTTP
	HTTPErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tucentropdf_http_errors_total",
			Help: "Total number of HTTP errors by endpoint and error type",
		},
		[]string{"method", "endpoint", "error_type"},
	)

	// HTTPRequestSize histograma de tamaño de request
	HTTPRequestSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tucentropdf_http_request_bytes",
			Help:    "HTTP request size in bytes",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 10), // 1KB a 1MB
		},
		[]string{"method", "endpoint"},
	)

	// HTTPResponseSize histograma de tamaño de response
	HTTPResponseSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tucentropdf_http_response_bytes",
			Help:    "HTTP response size in bytes",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 10), // 1KB a 1MB
		},
		[]string{"method", "endpoint"},
	)

	// HTTPActiveRequests gauge de requests activos
	HTTPActiveRequests = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tucentropdf_http_active_requests",
			Help: "Number of currently active HTTP requests",
		},
		[]string{"method", "endpoint"},
	)

	// RateLimitHitsTotal contador de rate limit hits
	RateLimitHitsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tucentropdf_rate_limit_hits_total",
			Help: "Total number of rate limit hits by plan",
		},
		[]string{"plan", "endpoint"},
	)

	// AuthFailuresTotal contador de fallos de autenticación
	AuthFailuresTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tucentropdf_auth_failures_total",
			Help: "Total number of authentication failures by reason",
		},
		[]string{"reason"},
	)

	// APIUptime gauge de uptime del API
	APIUptime = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "tucentropdf_api_uptime_seconds",
			Help: "API uptime in seconds",
		},
	)
)

// RecordHTTPRequest registra request HTTP
func RecordHTTPRequest(method, endpoint, statusCode string, durationSeconds float64) {
	HTTPRequestsTotal.WithLabelValues(method, endpoint, statusCode).Inc()
	HTTPRequestDurationSeconds.WithLabelValues(method, endpoint).Observe(durationSeconds)
}

// RecordHTTPError registra error HTTP
func RecordHTTPError(method, endpoint, errorType string) {
	HTTPErrorsTotal.WithLabelValues(method, endpoint, errorType).Inc()
}

// RecordHTTPRequestSize registra tamaño de request
func RecordHTTPRequestSize(method, endpoint string, sizeBytes int64) {
	HTTPRequestSize.WithLabelValues(method, endpoint).Observe(float64(sizeBytes))
}

// RecordHTTPResponseSize registra tamaño de response
func RecordHTTPResponseSize(method, endpoint string, sizeBytes int64) {
	HTTPResponseSize.WithLabelValues(method, endpoint).Observe(float64(sizeBytes))
}

// IncrementActiveRequests incrementa requests activos
func IncrementActiveRequests(method, endpoint string) {
	HTTPActiveRequests.WithLabelValues(method, endpoint).Inc()
}

// DecrementActiveRequests decrementa requests activos
func DecrementActiveRequests(method, endpoint string) {
	HTTPActiveRequests.WithLabelValues(method, endpoint).Dec()
}

// RecordRateLimitHit registra hit de rate limit
func RecordRateLimitHit(plan, endpoint string) {
	RateLimitHitsTotal.WithLabelValues(plan, endpoint).Inc()
}

// RecordAuthFailure registra fallo de autenticación
func RecordAuthFailure(reason string) {
	AuthFailuresTotal.WithLabelValues(reason).Inc()
}

// SetAPIUptime establece uptime del API
func SetAPIUptime(seconds float64) {
	APIUptime.Set(seconds)
}
