package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Queue metrics - Métricas de la cola de trabajos
var (
	// JobsEnqueuedTotal contador total de jobs encolados
	JobsEnqueuedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tucentropdf_jobs_enqueued_total",
			Help: "Total number of jobs enqueued by type and plan",
		},
		[]string{"type", "plan"},
	)

	// JobsCompletedTotal contador total de jobs completados
	JobsCompletedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tucentropdf_jobs_completed_total",
			Help: "Total number of jobs completed successfully by type",
		},
		[]string{"type", "plan"},
	)

	// JobsFailedTotal contador total de jobs fallidos
	JobsFailedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tucentropdf_jobs_failed_total",
			Help: "Total number of jobs failed by type and reason",
		},
		[]string{"type", "plan", "reason"},
	)

	// JobsCancelledTotal contador total de jobs cancelados
	JobsCancelledTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tucentropdf_jobs_cancelled_total",
			Help: "Total number of jobs cancelled by type",
		},
		[]string{"type", "plan"},
	)

	// JobDurationSeconds histograma de duración de jobs
	JobDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tucentropdf_job_duration_seconds",
			Help:    "Job processing duration in seconds",
			Buckets: []float64{1, 5, 10, 30, 60, 120, 300, 600}, // 1s a 10min
		},
		[]string{"type", "plan"},
	)

	// QueueLength gauge de longitud de cola por tipo
	QueueLength = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tucentropdf_queue_length",
			Help: "Current number of pending jobs in queue by type",
		},
		[]string{"queue"},
	)

	// JobRetryCount contador de reintentos
	JobRetryCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tucentropdf_job_retry_total",
			Help: "Total number of job retries by type",
		},
		[]string{"type", "attempt"},
	)

	// JobPayloadSize histograma de tamaño de payload
	JobPayloadSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tucentropdf_job_payload_bytes",
			Help:    "Job payload size in bytes",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 10), // 1KB a 1MB
		},
		[]string{"type"},
	)

	// JobResultSize histograma de tamaño de resultado
	JobResultSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tucentropdf_job_result_bytes",
			Help:    "Job result size in bytes",
			Buckets: prometheus.ExponentialBuckets(1024, 2, 10), // 1KB a 1MB
		},
		[]string{"type"},
	)
)

// RecordJobEnqueued registra job encolado
func RecordJobEnqueued(jobType, plan string) {
	JobsEnqueuedTotal.WithLabelValues(jobType, plan).Inc()
}

// RecordJobCompleted registra job completado
func RecordJobCompleted(jobType, plan string, durationSeconds float64) {
	JobsCompletedTotal.WithLabelValues(jobType, plan).Inc()
	JobDurationSeconds.WithLabelValues(jobType, plan).Observe(durationSeconds)
}

// RecordJobFailed registra job fallido
func RecordJobFailed(jobType, plan, reason string) {
	JobsFailedTotal.WithLabelValues(jobType, plan, reason).Inc()
}

// RecordJobCancelled registra job cancelado
func RecordJobCancelled(jobType, plan string) {
	JobsCancelledTotal.WithLabelValues(jobType, plan).Inc()
}

// UpdateQueueLength actualiza longitud de cola
func UpdateQueueLength(queue string, length int64) {
	QueueLength.WithLabelValues(queue).Set(float64(length))
}

// RecordJobRetry registra reintento de job
func RecordJobRetry(jobType string, attempt int) {
	JobRetryCount.WithLabelValues(jobType, string(rune(attempt+'0'))).Inc()
}

// RecordJobPayloadSize registra tamaño de payload
func RecordJobPayloadSize(jobType string, sizeBytes int64) {
	JobPayloadSize.WithLabelValues(jobType).Observe(float64(sizeBytes))
}

// RecordJobResultSize registra tamaño de resultado
func RecordJobResultSize(jobType string, sizeBytes int64) {
	JobResultSize.WithLabelValues(jobType).Observe(float64(sizeBytes))
}
