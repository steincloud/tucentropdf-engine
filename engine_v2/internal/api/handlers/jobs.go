package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/tucentropdf/engine-v2/internal/queue"
	"github.com/tucentropdf/engine-v2/pkg/logger"
	"github.com/tucentropdf/engine-v2/pkg/response"
)

// JobsHandler maneja endpoints de jobs asíncronos
type JobsHandler struct {
	logger     *logger.Logger
	statusStore *queue.JobStatusStore
}

// NewJobsHandler crea un nuevo handler de jobs
func NewJobsHandler(log *logger.Logger, store *queue.JobStatusStore) *JobsHandler {
	return &JobsHandler{
		logger:     log,
		statusStore: store,
	}
}

// GetJobStatus godoc
// @Summary Obtener estado de un job
// @Description Obtiene el estado actual de un job asíncrono (pending, processing, completed, failed)
// @Tags Jobs
// @Accept json
// @Produce json
// @Param id path string true "Job ID"
// @Success 200 {object} response.Response{data=queue.JobResult}
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /jobs/{id} [get]
// @Security ApiKeyAuth
func (h *JobsHandler) GetJobStatus(c *fiber.Ctx) error {
	jobID := c.Params("id")
	
	if jobID == "" {
		return response.BadRequest(c, "Job ID is required")
	}

	// Obtener estado del job
	result, err := h.statusStore.GetJobStatus(c.Context(), jobID)
	if err != nil {
		h.logger.Warn("Job not found", "job_id", jobID, "error", err)
		return response.NotFound(c, "Job not found")
	}

	h.logger.Debug("Job status retrieved", "job_id", jobID, "status", result.Status)

	return response.Success(c, fiber.Map{
		"job_id":       result.JobID,
		"status":       result.Status,
		"result_path":  result.ResultPath,
		"error":        result.Error,
		"duration_ms":  result.Duration.Milliseconds(),
		"completed_at": result.CompletedAt,
		"metadata":     result.Metadata,
	}, "Job status retrieved successfully")
}

// GetJobResult godoc
// @Summary Descargar resultado de un job
// @Description Descarga el archivo resultado de un job completado
// @Tags Jobs
// @Accept json
// @Produce application/octet-stream
// @Param id path string true "Job ID"
// @Success 200 {file} binary
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /jobs/{id}/result [get]
// @Security ApiKeyAuth
func (h *JobsHandler) GetJobResult(c *fiber.Ctx) error {
	jobID := c.Params("id")
	
	if jobID == "" {
		return response.BadRequest(c, "Job ID is required")
	}

	// Obtener estado del job
	result, err := h.statusStore.GetJobStatus(c.Context(), jobID)
	if err != nil {
		h.logger.Warn("Job not found", "job_id", jobID, "error", err)
		return response.NotFound(c, "Job not found")
	}

	// Verificar que el job esté completado
	if result.Status != queue.StatusCompleted {
		return response.BadRequest(c, fiber.Map{
			"message": "Job not completed yet",
			"status":  result.Status,
		})
	}

	// Verificar que haya resultado
	if result.ResultPath == "" {
		return response.NotFound(c, "Job result not available")
	}

	h.logger.Info("Downloading job result",
		"job_id", jobID,
		"result_path", result.ResultPath,
	)

	// Enviar archivo
	return c.SendFile(result.ResultPath, true)
}

// GetUserJobs godoc
// @Summary Obtener jobs de un usuario
// @Description Obtiene lista de jobs del usuario autenticado
// @Tags Jobs
// @Accept json
// @Produce json
// @Param limit query int false "Límite de resultados" default(50)
// @Success 200 {object} response.Response{data=[]queue.JobResult}
// @Failure 401 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /jobs [get]
// @Security ApiKeyAuth
func (h *JobsHandler) GetUserJobs(c *fiber.Ctx) error {
	// Obtener user_id del contexto (debería ser seteado por middleware de auth)
	userID := c.Locals("user_id")
	if userID == nil {
		return response.Unauthorized(c, "User not authenticated")
	}

	limit := c.QueryInt("limit", 50)
	if limit > 100 {
		limit = 100
	}

	// Obtener jobs del usuario
	jobs, err := h.statusStore.GetUserJobs(c.Context(), userID.(string), limit)
	if err != nil {
		h.logger.Error("Failed to get user jobs",
			"user_id", userID,
			"error", err,
		)
		return response.InternalError(c, "Failed to retrieve jobs")
	}

	h.logger.Debug("User jobs retrieved",
		"user_id", userID,
		"count", len(jobs),
	)

	return response.Success(c, fiber.Map{
		"jobs":  jobs,
		"count": len(jobs),
	}, "Jobs retrieved successfully")
}

// CancelJob godoc
// @Summary Cancelar un job
// @Description Cancela un job pendiente o en procesamiento
// @Tags Jobs
// @Accept json
// @Produce json
// @Param id path string true "Job ID"
// @Success 200 {object} response.Response
// @Failure 400 {object} response.Response
// @Failure 404 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /jobs/{id}/cancel [post]
// @Security ApiKeyAuth
func (h *JobsHandler) CancelJob(c *fiber.Ctx) error {
	jobID := c.Params("id")
	
	if jobID == "" {
		return response.BadRequest(c, "Job ID is required")
	}

	// Obtener estado actual
	result, err := h.statusStore.GetJobStatus(c.Context(), jobID)
	if err != nil {
		return response.NotFound(c, "Job not found")
	}

	// Verificar que el job pueda ser cancelado
	if result.Status == queue.StatusCompleted || result.Status == queue.StatusFailed {
		return response.BadRequest(c, fiber.Map{
			"message": "Job cannot be cancelled",
			"status":  result.Status,
		})
	}

	// Actualizar estado a cancelled
	result.Status = queue.StatusCancelled
	result.Error = "Job cancelled by user"
	
	if err := h.statusStore.SaveJobStatus(c.Context(), result); err != nil {
		h.logger.Error("Failed to cancel job", "job_id", jobID, "error", err)
		return response.InternalError(c, "Failed to cancel job")
	}

	h.logger.Info("Job cancelled", "job_id", jobID)

	return response.Success(c, fiber.Map{
		"job_id": jobID,
		"status": queue.StatusCancelled,
	}, "Job cancelled successfully")
}

// GetQueueStats godoc
// @Summary Obtener estadísticas de la cola
// @Description Obtiene estadísticas generales de la cola de trabajos
// @Tags Jobs
// @Accept json
// @Produce json
// @Success 200 {object} response.Response
// @Failure 500 {object} response.Response
// @Router /jobs/stats [get]
// @Security ApiKeyAuth
func (h *JobsHandler) GetQueueStats(c *fiber.Ctx) error {
	// Obtener conteo de jobs pendientes
	pendingCount, err := h.statusStore.GetPendingJobsCount(c.Context())
	if err != nil {
		h.logger.Error("Failed to get pending jobs count", "error", err)
		pendingCount = 0
	}

	return response.Success(c, fiber.Map{
		"pending_jobs": pendingCount,
		"timestamp":    fiber.Map{"iso": fiber.Map{}},
	}, "Queue stats retrieved successfully")
}
