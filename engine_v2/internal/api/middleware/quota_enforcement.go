package middleware

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/internal/storage"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// QuotaEnforcementMiddleware middleware para enforcement de cuotas
type QuotaEnforcementMiddleware struct {
	config       *config.Config
	logger       *logger.Logger
	usageTracker *storage.UsageTracker
	planConfig   *config.PlanConfiguration
	auditLogger  *AuditLogger
}

// NewQuotaEnforcementMiddleware crear nuevo middleware de enforcement de cuotas
func NewQuotaEnforcementMiddleware(
	cfg *config.Config,
	log *logger.Logger,
	usageTracker *storage.UsageTracker,
	auditLogger *AuditLogger,
) *QuotaEnforcementMiddleware {
	return &QuotaEnforcementMiddleware{
		config:       cfg,
		logger:       log,
		usageTracker: usageTracker,
		planConfig:   config.GetDefaultPlanConfiguration(),
		auditLogger:  auditLogger,
	}
}

// QuotaError error específico para límites de cuota
type QuotaError struct {
	Code         string      `json:"code"`
	Message      string      `json:"message"`
	RequiredPlan string      `json:"required_plan,omitempty"`
	CurrentUsage interface{} `json:"current_usage,omitempty"`
	Limits       interface{} `json:"limits,omitempty"`
	ResetTime    *time.Time  `json:"reset_time,omitempty"`
}

// EnforceQuotas middleware principal de enforcement
func (q *QuotaEnforcementMiddleware) EnforceQuotas() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Obtener información del usuario del contexto
		userID, ok := c.Locals("userID").(string)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "User ID not found in context",
				"code": "MISSING_USER_ID",
			})
		}
		
		userPlan, ok := c.Locals("userPlan").(string)
		if !ok {
			userPlan = string(config.PlanFree)
		}
		
		// Obtener límites del plan
		plan := config.Plan(userPlan)
		planLimits := q.planConfig.GetPlanLimits(plan)
		
		// Construir operación a validar
		operation, err := q.buildOperationFromRequest(c, userID)
		if err != nil {
			q.logger.Warn("Failed to build operation from request",
				"user_id", userID,
				"path", c.Path(),
				"error", err.Error(),
			)
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Failed to parse operation details",
				"code": "INVALID_OPERATION",
			})
		}
		
		// Verificar límites antes de procesar
		if err := q.usageTracker.CheckLimits(context.Background(), userID, operation, planLimits); err != nil {
			// Log del evento de cuota excedida
			q.auditLogger.LogQuotaEvent(AuditEvent{
				EventType: EventQuotaExceeded,
				UserID:    userID,
				Data: map[string]interface{}{
					"operation":      operation.OperationType,
					"current_plan":   userPlan,
					"limit_exceeded": err.Error(),
					"file_size":      operation.FileSize,
					"pages":          operation.Pages,
				},
				Timestamp: time.Now(),
			})
			
			return q.handleQuotaError(c, userID, userPlan, err, planLimits)
		}
		
		// Guardar operación en contexto para tracking posterior
		c.Locals("pendingOperation", operation)
		c.Locals("planLimits", planLimits)
		
		return c.Next()
	}
}

// PostProcessingTracker middleware para rastrear uso después del procesamiento
func (q *QuotaEnforcementMiddleware) PostProcessingTracker() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Este middleware se ejecuta después del handler principal
		defer func() {
			// Obtener operación pendiente
			if operation, ok := c.Locals("pendingOperation").(*storage.UsageOperation); ok {
				// Determinar si la operación fue exitosa basado en el status code
				operation.Success = c.Response().StatusCode() < 400
				operation.ProcessingTime = time.Since(operation.Timestamp).Milliseconds()
				
				// Rastrear el uso real
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				
				if err := q.usageTracker.TrackUsage(ctx, operation); err != nil {
					q.logger.Error("Failed to track usage",
						"user_id", operation.UserID,
						"operation", operation.OperationType,
						"error", err.Error(),
					)
				} else {
					// Log de auditoría para operación exitosa
					q.auditLogger.LogUsageEvent(AuditEvent{
						EventType: EventUsageTracked,
						UserID:    operation.UserID,
						Data: map[string]interface{}{
							"operation_type":   operation.OperationType,
							"file_size":        operation.FileSize,
							"pages":            operation.Pages,
							"processing_time":  operation.ProcessingTime,
							"success":          operation.Success,
						},
						Timestamp: time.Now(),
					})
				}
			}
		}()
		
		return c.Next()
	}
}

// buildOperationFromRequest construye una UsageOperation desde el request
func (q *QuotaEnforcementMiddleware) buildOperationFromRequest(c *fiber.Ctx, userID string) (*storage.UsageOperation, error) {
	operation := &storage.UsageOperation{
		UserID:    userID,
		Timestamp: time.Now(),
	}
	
	// Determinar tipo de operación basado en la ruta
	path := c.Path()
	switch {
	case contains(path, "/ocr") && contains(path, "/ai"):
		operation.OperationType = storage.OpTypeAIOCR
	case contains(path, "/ocr"):
		operation.OperationType = storage.OpTypeOCR
	case contains(path, "/office"):
		operation.OperationType = storage.OpTypeOffice
	case contains(path, "/upload"):
		operation.OperationType = storage.OpTypeUpload
	default:
		operation.OperationType = storage.OpTypePDF
	}
	
	// Obtener tamaño de archivo desde headers o form
	if contentLength := c.Get("Content-Length"); contentLength != "" {
		if size, err := strconv.ParseInt(contentLength, 10, 64); err == nil {
			operation.FileSize = size
		}
	}
	
	// Para requests multipart, obtener tamaño del archivo
	if form, err := c.MultipartForm(); err == nil {
		if files, ok := form.File["file"]; ok && len(files) > 0 {
			operation.FileSize = files[0].Size
		}
	}
	
	// Obtener número de páginas desde parámetros (si aplica)
	if pagesStr := c.Query("pages", c.FormValue("pages")); pagesStr != "" {
		if pages, err := strconv.Atoi(pagesStr); err == nil && pages > 0 {
			operation.Pages = pages
		} else {
			// Estimar páginas basado en tamaño de archivo (estimación)
			operation.Pages = q.estimatePages(operation.FileSize, operation.OperationType)
		}
	} else {
		// Estimar páginas basado en tamaño de archivo
		operation.Pages = q.estimatePages(operation.FileSize, operation.OperationType)
	}
	
	return operation, nil
}

// estimatePages estima el número de páginas basado en el tamaño de archivo
func (q *QuotaEnforcementMiddleware) estimatePages(fileSize int64, opType storage.OperationType) int {
	if fileSize <= 0 {
		return 1 // Mínimo 1 página
	}
	
	// Estimaciones basadas en tipo de operación y tamaño promedio
	var avgPageSize int64
	
	switch opType {
	case storage.OpTypePDF:
		avgPageSize = 100 * 1024 // ~100KB por página PDF promedio
	case storage.OpTypeOffice:
		avgPageSize = 50 * 1024  // ~50KB por página Word/PowerPoint promedio
	case storage.OpTypeOCR, storage.OpTypeAIOCR:
		avgPageSize = 500 * 1024 // ~500KB por página de imagen promedio
	default:
		avgPageSize = 100 * 1024
	}
	
	pages := int(fileSize / avgPageSize)
	if pages == 0 {
		pages = 1 // Mínimo 1 página
	}
	
	// Máximo razonable para estimación
	if pages > 1000 {
		pages = 1000
	}
	
	return pages
}

// handleQuotaError maneja errores de cuota y devuelve respuesta estructurada
func (q *QuotaEnforcementMiddleware) handleQuotaError(
	c *fiber.Ctx,
	userID string,
	userPlan string,
	limitErr error,
	planLimits config.PlanLimits,
) error {
	// Obtener estadísticas actuales de uso
	usage, err := q.usageTracker.GetUserUsage(context.Background(), userID)
	if err != nil {
		q.logger.Error("Failed to get usage for quota error", "user_id", userID, "error", err)
	}
	
	// Determinar plan requerido basado en el tipo de límite
	requiredPlan := q.getRequiredPlanForOperation(limitErr.Error(), userPlan)
	
	// Determinar cuándo se resetean los contadores
	resetTime := q.getNextResetTime(limitErr.Error())
	
	quotaError := &QuotaError{
		Code:         q.getQuotaErrorCode(limitErr.Error()),
		Message:      q.getQuotaErrorMessage(limitErr.Error(), userPlan),
		RequiredPlan: requiredPlan,
		CurrentUsage: usage,
		Limits:       planLimits,
		ResetTime:    resetTime,
	}
	
	q.logger.Warn("Quota limit exceeded",
		"user_id", userID,
		"plan", userPlan,
		"error", limitErr.Error(),
		"required_plan", requiredPlan,
	)
	
	return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
		"error": quotaError.Message,
		"code": quotaError.Code,
		"details": quotaError,
	})
}

// getQuotaErrorCode obtiene el código de error específico
func (q *QuotaEnforcementMiddleware) getQuotaErrorCode(errorMsg string) string {
	switch {
	case contains(errorMsg, "daily bytes"):
		return "DAILY_BYTES_LIMIT_EXCEEDED"
	case contains(errorMsg, "monthly bytes"):
		return "MONTHLY_BYTES_LIMIT_EXCEEDED"
	case contains(errorMsg, "daily operations"):
		return "DAILY_OPERATIONS_LIMIT_EXCEEDED"
	case contains(errorMsg, "monthly operations"):
		return "MONTHLY_OPERATIONS_LIMIT_EXCEEDED"
	case contains(errorMsg, "daily files"):
		return "DAILY_FILES_LIMIT_EXCEEDED"
	case contains(errorMsg, "monthly files"):
		return "MONTHLY_FILES_LIMIT_EXCEEDED"
	case contains(errorMsg, "AI OCR"):
		return "AI_OCR_LIMIT_EXCEEDED"
	case contains(errorMsg, "OCR"):
		return "OCR_LIMIT_EXCEEDED"
	case contains(errorMsg, "Office"):
		return "OFFICE_LIMIT_EXCEEDED"
	case contains(errorMsg, "pages"):
		return "PAGES_LIMIT_EXCEEDED"
	default:
		return "QUOTA_LIMIT_EXCEEDED"
	}
}

// getQuotaErrorMessage obtiene el mensaje de error amigable
func (q *QuotaEnforcementMiddleware) getQuotaErrorMessage(errorMsg, userPlan string) string {
	planName := q.getPlanDisplayName(userPlan)
	
	switch {
	case contains(errorMsg, "daily bytes"):
		return fmt.Sprintf("Has alcanzado el límite diario de datos procesados de tu plan %s", planName)
	case contains(errorMsg, "monthly bytes"):
		return fmt.Sprintf("Has alcanzado el límite mensual de datos procesados de tu plan %s", planName)
	case contains(errorMsg, "daily operations"):
		return fmt.Sprintf("Has alcanzado el límite diario de operaciones de tu plan %s", planName)
	case contains(errorMsg, "monthly operations"):
		return fmt.Sprintf("Has alcanzado el límite mensual de operaciones de tu plan %s", planName)
	case contains(errorMsg, "AI OCR"):
		return fmt.Sprintf("Has alcanzado el límite de páginas con OCR IA de tu plan %s", planName)
	case contains(errorMsg, "OCR"):
		return fmt.Sprintf("Has alcanzado el límite de páginas OCR de tu plan %s", planName)
	case contains(errorMsg, "Office"):
		return fmt.Sprintf("Has alcanzado el límite de conversión de documentos Office de tu plan %s", planName)
	default:
		return fmt.Sprintf("Has alcanzado un límite de tu plan %s", planName)
	}
}

// getRequiredPlanForOperation determina qué plan se requiere
func (q *QuotaEnforcementMiddleware) getRequiredPlanForOperation(errorMsg, currentPlan string) string {
	switch currentPlan {
	case string(config.PlanFree):
		return string(config.PlanPremium)
	case string(config.PlanPremium):
		if contains(errorMsg, "AI OCR") || contains(errorMsg, "monthly") {
			return string(config.PlanPro)
		}
		return string(config.PlanPro)
	case string(config.PlanPro):
		return "" // Pro no tiene límites que requieran upgrade
	default:
		return string(config.PlanPremium)
	}
}

// getNextResetTime calcula cuándo se resetean los contadores
func (q *QuotaEnforcementMiddleware) getNextResetTime(errorMsg string) *time.Time {
	now := time.Now()
	var resetTime time.Time
	
	if contains(errorMsg, "daily") {
		// Próximo reset diario a medianoche
		resetTime = time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())
	} else if contains(errorMsg, "monthly") {
		// Próximo reset mensual al primer día del siguiente mes
		if now.Month() == time.December {
			resetTime = time.Date(now.Year()+1, time.January, 1, 0, 0, 0, 0, now.Location())
		} else {
			resetTime = time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
		}
	}
	
	if !resetTime.IsZero() {
		return &resetTime
	}
	return nil
}

// getPlanDisplayName obtiene el nombre amigable del plan
func (q *QuotaEnforcementMiddleware) getPlanDisplayName(plan string) string {
	switch plan {
	case string(config.PlanFree):
		return "Gratuito"
	case string(config.PlanPremium):
		return "Premium"
	case string(config.PlanPro):
		return "Pro"
	default:
		return "Gratuito"
	}
}

// contains helper function para verificar si string contiene substring
func contains(str, substr string) bool {
	return len(str) >= len(substr) && (str == substr || 
		len(str) > len(substr) && (str[:len(substr)] == substr || 
		str[len(str)-len(substr):] == substr || 
		findInString(str, substr)))
}

func findInString(str, substr string) bool {
	for i := 0; i <= len(str)-len(substr); i++ {
		if str[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}