package handlers

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/internal/queue"
	"github.com/tucentropdf/engine-v2/pkg/logger"
	"github.com/tucentropdf/engine-v2/pkg/response"
)

const (
	// Límites de batch processing
	MaxBatchSize       = 10
	MaxBatchSizeFree   = 3
	MaxBatchSizePremium = 5
	MaxBatchSizePro    = 10
	
	// Timeout para batch
	BatchTimeout = 10 * time.Minute
)

// BatchHandler maneja operaciones en batch
type BatchHandler struct {
	queueClient *queue.Client
	logger      *logger.Logger
	config      *config.Config
}

// NewBatchHandler crea una nueva instancia
func NewBatchHandler(queueClient *queue.Client, cfg *config.Config, log *logger.Logger) *BatchHandler {
	return &BatchHandler{
		queueClient: queueClient,
		config:      cfg,
		logger:      log,
	}
}

// BatchOCRRequest request para OCR en batch
type BatchOCRRequest struct {
	Files        []BatchFile       `json:"files" validate:"required,min=1,max=10"`
	Language     string            `json:"language" validate:"required"`
	UseAI        bool              `json:"use_ai"`
	OutputFormat string            `json:"output_format" validate:"required,oneof=txt json"`
	Options      map[string]string `json:"options,omitempty"`
}

// BatchOfficeRequest request para conversión Office en batch
type BatchOfficeRequest struct {
	Files   []BatchFile       `json:"files" validate:"required,min=1,max=10"`
	Format  string            `json:"format" validate:"required,oneof=pdf"`
	Options map[string]string `json:"options,omitempty"`
}

// BatchPDFRequest request para operaciones PDF en batch
type BatchPDFRequest struct {
	Operation string            `json:"operation" validate:"required"`
	Files     []BatchFile       `json:"files" validate:"required,min=1,max=10"`
	Options   map[string]string `json:"options,omitempty"`
}

// BatchFile representa un archivo en el batch
type BatchFile struct {
	FileID   string            `json:"file_id" validate:"required"`
	FileName string            `json:"file_name,omitempty"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// BatchResponse respuesta de batch processing
type BatchResponse struct {
	BatchID   string             `json:"batch_id"`
	Status    string             `json:"status"`
	Total     int                `json:"total"`
	Completed int                `json:"completed"`
	Failed    int                `json:"failed"`
	Jobs      []BatchJobStatus   `json:"jobs"`
	CreatedAt time.Time          `json:"created_at"`
}

// BatchJobStatus estado de un job individual en el batch
type BatchJobStatus struct {
	JobID    string `json:"job_id"`
	FileID   string `json:"file_id"`
	FileName string `json:"file_name,omitempty"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}

// ProcessBatchOCR procesa OCR en batch
// @Summary Procesar OCR en batch
// @Description Procesa múltiples archivos OCR en paralelo
// @Tags batch
// @Accept json
// @Produce json
// @Param request body BatchOCRRequest true "Batch OCR Request"
// @Success 200 {object} response.Response
// @Router /api/v2/batch/ocr [post]
func (h *BatchHandler) ProcessBatchOCR(c *fiber.Ctx) error {
	var req BatchOCRRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	
	// Validar límites por plan
	userPlan := h.getUserPlan(c)
	maxBatchSize := h.getMaxBatchSize(userPlan)
	
	if len(req.Files) > maxBatchSize {
		return response.BadRequest(c, fmt.Sprintf("Batch size exceeds plan limit. Max: %d", maxBatchSize))
	}
	
	// Obtener user ID
	userID := h.getUserID(c)
	
	// Generar batch ID
	batchID := h.generateBatchID()
	
	h.logger.Info("Processing batch OCR",
		"batch_id", batchID,
		"user_id", userID,
		"plan", userPlan,
		"files", len(req.Files),
	)
	
	// Procesar archivos en paralelo con semáforo
	ctx, cancel := context.WithTimeout(c.Context(), BatchTimeout)
	defer cancel()
	
	results := h.processBatchParallel(ctx, req.Files, userID, userPlan, func(file BatchFile) (string, error) {
		// Encolar job de OCR
		payload := &queue.OCRJobPayload{
			JobID:        h.generateJobID(),
			UserID:       userID,
			Plan:         userPlan,
			FilePath:     file.FileID,
			Language:     req.Language,
			UseAI:        req.UseAI,
			OutputFormat: req.OutputFormat,
			Options:      req.Options,
			CreatedAt:    time.Now(),
		}
		
		taskInfo, err := h.queueClient.EnqueueOCRJob(ctx, payload)
		if err != nil {
			return "", err
		}
		
		return taskInfo.ID, nil
	})
	
	// Construir respuesta
	batchResponse := h.buildBatchResponse(batchID, req.Files, results)
	
	return response.Success(c, "Batch OCR processing started", batchResponse)
}

// ProcessBatchOffice procesa conversión Office en batch
// @Summary Procesar conversión Office en batch
// @Description Convierte múltiples archivos Office a PDF en paralelo
// @Tags batch
// @Accept json
// @Produce json
// @Param request body BatchOfficeRequest true "Batch Office Request"
// @Success 200 {object} response.Response
// @Router /api/v2/batch/office [post]
func (h *BatchHandler) ProcessBatchOffice(c *fiber.Ctx) error {
	var req BatchOfficeRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	
	userPlan := h.getUserPlan(c)
	maxBatchSize := h.getMaxBatchSize(userPlan)
	
	if len(req.Files) > maxBatchSize {
		return response.BadRequest(c, fmt.Sprintf("Batch size exceeds plan limit. Max: %d", maxBatchSize))
	}
	
	userID := h.getUserID(c)
	batchID := h.generateBatchID()
	
	h.logger.Info("Processing batch Office conversion",
		"batch_id", batchID,
		"user_id", userID,
		"files", len(req.Files),
	)
	
	ctx, cancel := context.WithTimeout(c.Context(), BatchTimeout)
	defer cancel()
	
	results := h.processBatchParallel(ctx, req.Files, userID, userPlan, func(file BatchFile) (string, error) {
		payload := &queue.OfficeJobPayload{
			JobID:      h.generateJobID(),
			UserID:     userID,
			Plan:       userPlan,
			FilePath:   file.FileID,
			OutputPath: file.FileID + ".pdf",
			Format:     req.Format,
			Options:    req.Options,
			CreatedAt:  time.Now(),
		}
		
		taskInfo, err := h.queueClient.EnqueueOfficeJob(ctx, payload)
		if err != nil {
			return "", err
		}
		
		return taskInfo.ID, nil
	})
	
	batchResponse := h.buildBatchResponse(batchID, req.Files, results)
	
	return response.Success(c, "Batch Office conversion started", batchResponse)
}

// ProcessBatchPDF procesa operaciones PDF en batch
// @Summary Procesar operaciones PDF en batch
// @Description Procesa múltiples operaciones PDF en paralelo
// @Tags batch
// @Accept json
// @Produce json
// @Param request body BatchPDFRequest true "Batch PDF Request"
// @Success 200 {object} response.Response
// @Router /api/v2/batch/pdf [post]
func (h *BatchHandler) ProcessBatchPDF(c *fiber.Ctx) error {
	var req BatchPDFRequest
	if err := c.BodyParser(&req); err != nil {
		return response.BadRequest(c, "Invalid request body")
	}
	
	userPlan := h.getUserPlan(c)
	maxBatchSize := h.getMaxBatchSize(userPlan)
	
	if len(req.Files) > maxBatchSize {
		return response.BadRequest(c, fmt.Sprintf("Batch size exceeds plan limit. Max: %d", maxBatchSize))
	}
	
	userID := h.getUserID(c)
	batchID := h.generateBatchID()
	
	h.logger.Info("Processing batch PDF operation",
		"batch_id", batchID,
		"user_id", userID,
		"operation", req.Operation,
		"files", len(req.Files),
	)
	
	ctx, cancel := context.WithTimeout(c.Context(), BatchTimeout)
	defer cancel()
	
	results := h.processBatchParallel(ctx, req.Files, userID, userPlan, func(file BatchFile) (string, error) {
		payload := &queue.PDFJobPayload{
			JobID:     h.generateJobID(),
			UserID:    userID,
			Plan:      userPlan,
			Operation: req.Operation,
			Files:     []string{file.FileID},
			Options:   req.Options,
			CreatedAt: time.Now(),
		}
		
		taskInfo, err := h.queueClient.EnqueuePDFJob(ctx, payload)
		if err != nil {
			return "", err
		}
		
		return taskInfo.ID, nil
	})
	
	batchResponse := h.buildBatchResponse(batchID, req.Files, results)
	
	return response.Success(c, "Batch PDF processing started", batchResponse)
}

// GetBatchStatus obtiene el estado de un batch
// @Summary Obtener estado de batch
// @Description Obtiene el estado actual de un batch de procesamiento
// @Tags batch
// @Accept json
// @Produce json
// @Param batch_id path string true "Batch ID"
// @Success 200 {object} response.Response
// @Router /api/v2/batch/{batch_id}/status [get]
func (h *BatchHandler) GetBatchStatus(c *fiber.Ctx) error {
	batchID := c.Params("batch_id")
	
	// TODO: Implementar almacenamiento de estado de batch en Redis
	// Por ahora retornar placeholder
	
	return response.Success(c, "Batch status retrieved", fiber.Map{
		"batch_id": batchID,
		"status":   "processing",
		"message":  "Batch status tracking not yet implemented",
	})
}

// processBatchParallel procesa archivos en paralelo con semáforo
func (h *BatchHandler) processBatchParallel(
	ctx context.Context,
	files []BatchFile,
	userID, userPlan string,
	processFunc func(BatchFile) (string, error),
) []batchResult {
	// Semáforo para limitar concurrencia
	semaphore := make(chan struct{}, MaxBatchSize)
	results := make([]batchResult, len(files))
	var wg sync.WaitGroup
	
	for i, file := range files {
		wg.Add(1)
		
		go func(idx int, f BatchFile) {
			defer wg.Done()
			
			// Adquirir semáforo
			select {
			case semaphore <- struct{}{}:
				defer func() { <-semaphore }()
			case <-ctx.Done():
				results[idx] = batchResult{
					FileID: f.FileID,
					Error:  ctx.Err(),
				}
				return
			}
			
			// Procesar archivo
			jobID, err := processFunc(f)
			results[idx] = batchResult{
				FileID: f.FileID,
				JobID:  jobID,
				Error:  err,
			}
			
		}(i, file)
	}
	
	wg.Wait()
	return results
}

// batchResult resultado interno de procesamiento
type batchResult struct {
	FileID string
	JobID  string
	Error  error
}

// buildBatchResponse construye la respuesta del batch
func (h *BatchHandler) buildBatchResponse(batchID string, files []BatchFile, results []batchResult) *BatchResponse {
	batchResp := &BatchResponse{
		BatchID:   batchID,
		Status:    "processing",
		Total:     len(files),
		Jobs:      make([]BatchJobStatus, len(files)),
		CreatedAt: time.Now(),
	}
	
	for i, result := range results {
		jobStatus := BatchJobStatus{
			JobID:    result.JobID,
			FileID:   result.FileID,
			FileName: files[i].FileName,
		}
		
		if result.Error != nil {
			jobStatus.Status = "failed"
			jobStatus.Error = result.Error.Error()
			batchResp.Failed++
		} else {
			jobStatus.Status = "enqueued"
			batchResp.Completed++
		}
		
		batchResp.Jobs[i] = jobStatus
	}
	
	// Ajustar estado general
	if batchResp.Failed == batchResp.Total {
		batchResp.Status = "failed"
	} else if batchResp.Completed == batchResp.Total {
		batchResp.Status = "enqueued"
	}
	
	return batchResp
}

// getMaxBatchSize obtiene el tamaño máximo de batch según plan
func (h *BatchHandler) getMaxBatchSize(plan string) int {
	switch plan {
	case "pro":
		return MaxBatchSizePro
	case "premium":
		return MaxBatchSizePremium
	default:
		return MaxBatchSizeFree
	}
}

// getUserID obtiene el ID del usuario del contexto
func (h *BatchHandler) getUserID(c *fiber.Ctx) string {
	if userID, ok := c.Locals("userID").(string); ok {
		return userID
	}
	return "anonymous"
}

// getUserPlan obtiene el plan del usuario del contexto
func (h *BatchHandler) getUserPlan(c *fiber.Ctx) string {
	if plan, ok := c.Locals("userPlan").(string); ok {
		return plan
	}
	return "free"
}

// generateBatchID genera un ID único para el batch
func (h *BatchHandler) generateBatchID() string {
	return fmt.Sprintf("batch_%d", time.Now().UnixNano())
}

// generateJobID genera un ID único para un job
func (h *BatchHandler) generateJobID() string {
	return fmt.Sprintf("job_%d", time.Now().UnixNano())
}
