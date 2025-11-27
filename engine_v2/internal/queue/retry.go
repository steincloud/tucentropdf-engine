package queue

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/hibiken/asynq"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

const (
	// Configuración de reintentos
	MaxRetries         = 5
	InitialRetryDelay  = 30 * time.Second
	MaxRetryDelay      = 1 * time.Hour
	RetryDelayMultiplier = 2.0
	
	// Dead Letter Queue
	DLQPrefix = "dlq:"
)

var (
	ErrMaxRetriesExceeded = errors.New("max retries exceeded")
	ErrPermanentFailure   = errors.New("permanent failure")
)

// RetryPolicy define la política de reintentos
type RetryPolicy struct {
	MaxRetries    int
	InitialDelay  time.Duration
	MaxDelay      time.Duration
	Multiplier    float64
	logger        *logger.Logger
}

// NewRetryPolicy crea una nueva política de reintentos
func NewRetryPolicy(log *logger.Logger) *RetryPolicy {
	return &RetryPolicy{
		MaxRetries:   MaxRetries,
		InitialDelay: InitialRetryDelay,
		MaxDelay:     MaxRetryDelay,
		Multiplier:   RetryDelayMultiplier,
		logger:       log,
	}
}

// ComputeRetryDelay calcula el delay para el siguiente reintento usando exponential backoff
func (rp *RetryPolicy) ComputeRetryDelay(attempt int) time.Duration {
	if attempt <= 0 {
		return rp.InitialDelay
	}
	
	// Exponential backoff: InitialDelay * (Multiplier ^ attempt)
	delay := time.Duration(float64(rp.InitialDelay) * math.Pow(rp.Multiplier, float64(attempt)))
	
	// Aplicar jitter (±20%) para evitar thundering herd
	jitter := time.Duration(float64(delay) * 0.2 * (2*getRandom() - 1))
	delay += jitter
	
	// Clamp al máximo
	if delay > rp.MaxDelay {
		delay = rp.MaxDelay
	}
	
	rp.logger.Debug("Retry delay computed",
		"attempt", attempt,
		"delay", delay,
		"max_delay", rp.MaxDelay,
	)
	
	return delay
}

// ShouldRetry determina si un job debe ser reintentado
func (rp *RetryPolicy) ShouldRetry(err error, attempt int) bool {
	// No reintentar si se excedió el máximo
	if attempt >= rp.MaxRetries {
		rp.logger.Warn("Max retries exceeded",
			"attempt", attempt,
			"max_retries", rp.MaxRetries,
		)
		return false
	}
	
	// No reintentar errores permanentes
	if rp.isPermanentError(err) {
		rp.logger.Warn("Permanent error detected, not retrying",
			"error", err.Error(),
		)
		return false
	}
	
	return true
}

// isPermanentError determina si un error es permanente
func (rp *RetryPolicy) isPermanentError(err error) bool {
	if err == nil {
		return false
	}
	
	// Errores permanentes que no deben reintentarse
	permanentErrors := []string{
		"invalid input",
		"file not found",
		"unsupported format",
		"validation failed",
		"authentication failed",
		"permission denied",
	}
	
	errMsg := err.Error()
	for _, permErr := range permanentErrors {
		if contains(errMsg, permErr) {
			return true
		}
	}
	
	return false
}

// GetAsynqRetryOption retorna la opción de Asynq para reintentos
func (rp *RetryPolicy) GetAsynqRetryOption() asynq.Option {
	return asynq.MaxRetry(rp.MaxRetries)
}

// GetAsynqRetryDelayFunc retorna la función de delay para Asynq
func (rp *RetryPolicy) GetAsynqRetryDelayFunc() asynq.RetryDelayFunc {
	return func(n int, err error, task *asynq.Task) time.Duration {
		// Verificar si es error permanente
		if rp.isPermanentError(err) {
			rp.logger.Error("Permanent error, moving to DLQ",
				"task_type", task.Type(),
				"error", err.Error(),
			)
			// Retornar 0 para mover inmediatamente a DLQ
			return 0
		}
		
		delay := rp.ComputeRetryDelay(n)
		
		rp.logger.Info("Scheduling retry",
			"task_type", task.Type(),
			"attempt", n+1,
			"delay", delay,
			"error", err.Error(),
		)
		
		return delay
	}
}

// DeadLetterQueue maneja jobs que fallaron permanentemente
type DeadLetterQueue struct {
	logger *logger.Logger
}

// NewDeadLetterQueue crea una nueva instancia de DLQ
func NewDeadLetterQueue(log *logger.Logger) *DeadLetterQueue {
	return &DeadLetterQueue{
		logger: log,
	}
}

// AddToDLQ agrega un job al Dead Letter Queue
func (dlq *DeadLetterQueue) AddToDLQ(ctx context.Context, jobType string, payload []byte, err error) error {
	dlq.logger.Error("Job moved to Dead Letter Queue",
		"job_type", jobType,
		"payload_size", len(payload),
		"error", err.Error(),
	)
	
	// TODO: Implementar almacenamiento en Redis
	// Key: dlq:{job_type}:{timestamp}
	// Value: {payload, error, metadata}
	// TTL: 7 días
	
	return nil
}

// ListDLQJobs lista jobs en el DLQ
func (dlq *DeadLetterQueue) ListDLQJobs(ctx context.Context, jobType string, limit int) ([]DLQJob, error) {
	// TODO: Implementar lectura desde Redis
	return []DLQJob{}, nil
}

// RetryDLQJob reintenta un job del DLQ
func (dlq *DeadLetterQueue) RetryDLQJob(ctx context.Context, jobID string) error {
	dlq.logger.Info("Retrying job from DLQ", "job_id", jobID)
	
	// TODO: Implementar re-encolado desde DLQ
	return nil
}

// PurgeDLQ limpia el DLQ
func (dlq *DeadLetterQueue) PurgeDLQ(ctx context.Context, olderThan time.Duration) (int, error) {
	dlq.logger.Info("Purging DLQ", "older_than", olderThan)
	
	// TODO: Implementar limpieza de Redis
	return 0, nil
}

// DLQJob representa un job en el Dead Letter Queue
type DLQJob struct {
	JobID     string                 `json:"job_id"`
	JobType   string                 `json:"job_type"`
	Payload   map[string]interface{} `json:"payload"`
	Error     string                 `json:"error"`
	Attempts  int                    `json:"attempts"`
	FailedAt  time.Time              `json:"failed_at"`
}

// RetryConfig configuración avanzada de reintentos por tipo de job
type RetryConfig struct {
	OCRRetries    int
	OfficeRetries int
	PDFRetries    int
}

// GetRetryConfigForJobType obtiene configuración de reintentos por tipo
func GetRetryConfigForJobType(jobType string) RetryConfig {
	switch jobType {
	case TypeOCRClassic, TypeOCRAI:
		return RetryConfig{
			OCRRetries: 3, // OCR puede fallar por calidad de imagen
		}
	case TypeOfficeToPDF:
		return RetryConfig{
			OfficeRetries: 5, // Office conversion más estable
		}
	case TypePDFOperation:
		return RetryConfig{
			PDFRetries: 5, // PDF operations generalmente confiables
		}
	default:
		return RetryConfig{
			OCRRetries:    MaxRetries,
			OfficeRetries: MaxRetries,
			PDFRetries:    MaxRetries,
		}
	}
}

// RetryStats estadísticas de reintentos
type RetryStats struct {
	TotalRetries      int64   `json:"total_retries"`
	SuccessfulRetries int64   `json:"successful_retries"`
	FailedRetries     int64   `json:"failed_retries"`
	AvgRetryAttempts  float64 `json:"avg_retry_attempts"`
	DLQSize           int64   `json:"dlq_size"`
}

// GetRetryStats obtiene estadísticas de reintentos
func GetRetryStats(ctx context.Context) (*RetryStats, error) {
	// TODO: Implementar lectura de estadísticas desde Redis
	return &RetryStats{
		TotalRetries:      0,
		SuccessfulRetries: 0,
		FailedRetries:     0,
		AvgRetryAttempts:  0.0,
		DLQSize:           0,
	}, nil
}

// RetryWithBackoff ejecuta una función con exponential backoff
func RetryWithBackoff(ctx context.Context, maxRetries int, fn func() error) error {
	policy := &RetryPolicy{
		MaxRetries:   maxRetries,
		InitialDelay: InitialRetryDelay,
		MaxDelay:     MaxRetryDelay,
		Multiplier:   RetryDelayMultiplier,
	}
	
	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		// Ejecutar función
		err := fn()
		if err == nil {
			return nil // Éxito
		}
		
		lastErr = err
		
		// Verificar si debe reintentar
		if !policy.ShouldRetry(err, attempt) {
			return fmt.Errorf("retry failed: %w", err)
		}
		
		// Esperar delay
		delay := policy.ComputeRetryDelay(attempt)
		
		select {
		case <-time.After(delay):
			// Continuar con siguiente intento
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	
	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// Helper functions

func contains(str, substr string) bool {
	return len(str) >= len(substr) && (str == substr || len(str) > len(substr) && containsInner(str, substr))
}

func containsInner(str, substr string) bool {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func getRandom() float64 {
	// Implementación simplificada - en producción usar crypto/rand
	return 0.5
}
