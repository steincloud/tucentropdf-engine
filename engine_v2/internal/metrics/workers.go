package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Worker metrics - Métricas de workers
var (
	// WorkerJobsProcessedTotal contador total de jobs procesados por worker
	WorkerJobsProcessedTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tucentropdf_worker_jobs_processed_total",
			Help: "Total number of jobs processed by worker type",
		},
		[]string{"worker", "status"}, // status: success, failed
	)

	// WorkerErrorsTotal contador total de errores por worker
	WorkerErrorsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tucentropdf_worker_errors_total",
			Help: "Total number of errors by worker and error type",
		},
		[]string{"worker", "error_type"},
	)

	// WorkerProcessingTimeSeconds histograma de tiempo de procesamiento
	WorkerProcessingTimeSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tucentropdf_worker_processing_seconds",
			Help:    "Worker processing time in seconds",
			Buckets: []float64{0.5, 1, 2, 5, 10, 30, 60, 120, 300, 600}, // 0.5s a 10min
		},
		[]string{"worker", "operation"},
	)

	// WorkerHealth gauge de salud del worker (1=healthy, 0=unhealthy)
	WorkerHealth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tucentropdf_worker_health",
			Help: "Worker health status (1=healthy, 0=unhealthy)",
		},
		[]string{"worker", "instance"},
	)

	// WorkerActiveJobs gauge de jobs activos en worker
	WorkerActiveJobs = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tucentropdf_worker_active_jobs",
			Help: "Number of currently active jobs in worker",
		},
		[]string{"worker"},
	)

	// WorkerMemoryUsageBytes gauge de uso de memoria por worker
	WorkerMemoryUsageBytes = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tucentropdf_worker_memory_bytes",
			Help: "Worker memory usage in bytes",
		},
		[]string{"worker", "instance"},
	)

	// WorkerCPUUsagePercent gauge de uso de CPU por worker
	WorkerCPUUsagePercent = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tucentropdf_worker_cpu_percent",
			Help: "Worker CPU usage percentage",
		},
		[]string{"worker", "instance"},
	)

	// WorkerRestartCount contador de reinicios de worker
	WorkerRestartCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tucentropdf_worker_restart_total",
			Help: "Total number of worker restarts",
		},
		[]string{"worker", "reason"},
	)

	// WorkerConcurrency gauge de concurrencia configurada
	WorkerConcurrency = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "tucentropdf_worker_concurrency",
			Help: "Worker concurrency limit",
		},
		[]string{"worker"},
	)
)

// RecordWorkerJobProcessed registra job procesado por worker
func RecordWorkerJobProcessed(worker, status string, durationSeconds float64, operation string) {
	WorkerJobsProcessedTotal.WithLabelValues(worker, status).Inc()
	WorkerProcessingTimeSeconds.WithLabelValues(worker, operation).Observe(durationSeconds)
}

// RecordWorkerError registra error en worker
func RecordWorkerError(worker, errorType string) {
	WorkerErrorsTotal.WithLabelValues(worker, errorType).Inc()
}

// SetWorkerHealth establece salud del worker
func SetWorkerHealth(worker, instance string, healthy bool) {
	healthValue := 0.0
	if healthy {
		healthValue = 1.0
	}
	WorkerHealth.WithLabelValues(worker, instance).Set(healthValue)
}

// SetWorkerActiveJobs establece jobs activos en worker
func SetWorkerActiveJobs(worker string, count int) {
	WorkerActiveJobs.WithLabelValues(worker).Set(float64(count))
}

// SetWorkerMemoryUsage establece uso de memoria
func SetWorkerMemoryUsage(worker, instance string, bytes int64) {
	WorkerMemoryUsageBytes.WithLabelValues(worker, instance).Set(float64(bytes))
}

// SetWorkerCPUUsage establece uso de CPU
func SetWorkerCPUUsage(worker, instance string, percent float64) {
	WorkerCPUUsagePercent.WithLabelValues(worker, instance).Set(percent)
}

// RecordWorkerRestart registra reinicio de worker
func RecordWorkerRestart(worker, reason string) {
	WorkerRestartCount.WithLabelValues(worker, reason).Inc()
}

// SetWorkerConcurrency establece límite de concurrencia
func SetWorkerConcurrency(worker string, limit int) {
	WorkerConcurrency.WithLabelValues(worker).Set(float64(limit))
}
