package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/tucentropdf/engine-v2/internal/metrics"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// TaskType tipos de tareas disponibles
const (
	TypeOCRClassic    = "ocr:classic"
	TypeOCRAI         = "ocr:ai"
	TypeOfficeToPDF   = "office:to-pdf"
	TypePDFOperation  = "pdf:operation"
)

// JobStatus estados de un job
type JobStatus string

const (
	StatusPending    JobStatus = "pending"
	StatusProcessing JobStatus = "processing"
	StatusCompleted  JobStatus = "completed"
	StatusFailed     JobStatus = "failed"
	StatusCancelled  JobStatus = "cancelled"
)

// OCRJobPayload payload para jobs de OCR
type OCRJobPayload struct {
	JobID        string            `json:"job_id"`
	UserID       string            `json:"user_id"`
	Plan         string            `json:"plan"`
	FilePath     string            `json:"file_path"`
	Language     string            `json:"language"`
	UseAI        bool              `json:"use_ai"`
	OutputFormat string            `json:"output_format"`
	Options      map[string]string `json:"options,omitempty"`
	CreatedAt    time.Time         `json:"created_at"`
}

// OfficeJobPayload payload para jobs de conversión Office
type OfficeJobPayload struct {
	JobID      string            `json:"job_id"`
	UserID     string            `json:"user_id"`
	Plan       string            `json:"plan"`
	FilePath   string            `json:"file_path"`
	OutputPath string            `json:"output_path"`
	Format     string            `json:"format"` // docx, xlsx, pptx, etc.
	Options    map[string]string `json:"options,omitempty"`
	CreatedAt  time.Time         `json:"created_at"`
}

// PDFJobPayload payload para operaciones PDF
type PDFJobPayload struct {
	JobID     string            `json:"job_id"`
	UserID    string            `json:"user_id"`
	Plan      string            `json:"plan"`
	Operation string            `json:"operation"` // merge, split, compress, etc.
	Files     []string          `json:"files"`
	Options   map[string]string `json:"options,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

// JobResult resultado de un job procesado
type JobResult struct {
	JobID      string            `json:"job_id"`
	Status     JobStatus         `json:"status"`
	ResultPath string            `json:"result_path,omitempty"`
	ResultData map[string]any    `json:"result_data,omitempty"`
	Error      string            `json:"error,omitempty"`
	Duration   time.Duration     `json:"duration"`
	Metadata   map[string]string `json:"metadata,omitempty"`
	CompletedAt time.Time        `json:"completed_at"`
}

// Client wrapper para Asynq client con métodos helper
type Client struct {
	asynqClient *asynq.Client
	config      *Config
	logger      *logger.Logger
}

// NewQueueClient crea un nuevo cliente de cola
func NewQueueClient(cfg *Config, log *logger.Logger) *Client {
	return &Client{
		asynqClient: NewClient(cfg),
		config:      cfg,
		logger:      log,
	}
}

// EnqueueOCRJob encola un job de OCR
func (c *Client) EnqueueOCRJob(ctx context.Context, payload *OCRJobPayload) (*asynq.TaskInfo, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	taskType := TypeOCRClassic
	if payload.UseAI {
		taskType = TypeOCRAI
	}

	task := asynq.NewTask(taskType, payloadBytes)
	
	priority := c.config.GetPriorityForPlan(payload.Plan)
	queue := c.config.GetQueueForTask("ocr")

	info, err := c.asynqClient.EnqueueContext(ctx, task,
		asynq.Queue(queue),
		asynq.MaxRetry(c.config.MaxRetries),
		asynq.Timeout(10*time.Minute),
		asynq.ProcessIn(0), // Procesar inmediatamente
		asynq.TaskID(payload.JobID),
		asynq.Retention(24*time.Hour), // Retener info por 24h
	)

	if err != nil {
		c.logger.Error("Failed to enqueue OCR job",
			"job_id", payload.JobID,
			"user_id", payload.UserID,
			"error", err,
		)
		return nil, fmt.Errorf("failed to enqueue task: %w", err)
	}

	c.logger.Info("OCR job enqueued",
		"job_id", payload.JobID,
		"user_id", payload.UserID,
		"plan", payload.Plan,
		"priority", priority,
		"queue", queue,
		"task_id", info.ID,
	)

	// Registrar métrica de job encolado
	metrics.RecordJobEnqueued(taskType, payload.Plan)
	metrics.RecordJobPayloadSize(taskType, int64(len(payloadBytes)))

	return info, nil
}

// EnqueueOfficeJob encola un job de conversión Office
func (c *Client) EnqueueOfficeJob(ctx context.Context, payload *OfficeJobPayload) (*asynq.TaskInfo, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	task := asynq.NewTask(TypeOfficeToPDF, payloadBytes)
	
	priority := c.config.GetPriorityForPlan(payload.Plan)
	queue := c.config.GetQueueForTask("office")

	info, err := c.asynqClient.EnqueueContext(ctx, task,
		asynq.Queue(queue),
		asynq.MaxRetry(c.config.MaxRetries),
		asynq.Timeout(15*time.Minute),
		asynq.ProcessIn(0),
		asynq.TaskID(payload.JobID),
		asynq.Retention(24*time.Hour),
	)

	if err != nil {
		c.logger.Error("Failed to enqueue Office job",
			"job_id", payload.JobID,
			"user_id", payload.UserID,
			"error", err,
		)
		return nil, fmt.Errorf("failed to enqueue task: %w", err)
	}

	c.logger.Info("Office job enqueued",
		"job_id", payload.JobID,
		"user_id", payload.UserID,
		"plan", payload.Plan,
		"priority", priority,
		"queue", queue,
		"task_id", info.ID,
	)

	// Registrar métrica de job encolado
	metrics.RecordJobEnqueued(TypeOfficeToPDF, payload.Plan)
	metrics.RecordJobPayloadSize(TypeOfficeToPDF, int64(len(payloadBytes)))

	return info, nil
}

// EnqueuePDFJob encola un job de operación PDF
func (c *Client) EnqueuePDFJob(ctx context.Context, payload *PDFJobPayload) (*asynq.TaskInfo, error) {
	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	task := asynq.NewTask(TypePDFOperation, payloadBytes)
	
	priority := c.config.GetPriorityForPlan(payload.Plan)

	info, err := c.asynqClient.EnqueueContext(ctx, task,
		asynq.Queue(c.config.DefaultQueue),
		asynq.MaxRetry(c.config.MaxRetries),
		asynq.Timeout(20*time.Minute),
		asynq.ProcessIn(0),
		asynq.TaskID(payload.JobID),
		asynq.Retention(24*time.Hour),
	)

	if err != nil {
		c.logger.Error("Failed to enqueue PDF job",
			"job_id", payload.JobID,
			"user_id", payload.UserID,
			"error", err,
		)
		return nil, fmt.Errorf("failed to enqueue task: %w", err)
	}

	c.logger.Info("PDF job enqueued",
		"job_id", payload.JobID,
		"user_id", payload.UserID,
		"plan", payload.Plan,
		"priority", priority,
		"operation", payload.Operation,
		"task_id", info.ID,
	)

	// Registrar métrica de job encolado
	metrics.RecordJobEnqueued(TypePDFOperation, payload.Plan)
	metrics.RecordJobPayloadSize(TypePDFOperation, int64(len(payloadBytes)))

	return info, nil
}

// CancelJob cancela un job por ID
func (c *Client) CancelJob(ctx context.Context, jobID string, jobType string, plan string) error {
	// Asynq usa el TaskID como identificador
	// Cancelar usando Inspector (API pública)
	inspector := asynq.NewInspector(asynq.RedisClientOpt{
		Addr:     c.config.RedisAddr,
		Password: c.config.RedisPassword,
		DB:       c.config.RedisDB,
	})
	defer inspector.Close()
	err := inspector.CancelProcessing(jobID)
	if err != nil {
		c.logger.Error("Failed to cancel job", "job_id", jobID, "error", err)
		return fmt.Errorf("failed to cancel job: %w", err)
	}

	c.logger.Info("Job cancelled", "job_id", jobID)
	// Registrar métrica de cancelación
	metrics.RecordJobCancelled(jobType, plan)
	return nil
}

// Close cierra el cliente
func (c *Client) Close() error {
	return c.asynqClient.Close()
}
