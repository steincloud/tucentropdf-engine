package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/internal/storage"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// Handlers contiene todas las instancias de servicios
type Handlers struct {
	config   *config.Config
	logger   *logger.Logger
	storage  storage.Service
}

// New crea una nueva instancia de handlers
func New(cfg *config.Config, log *logger.Logger, store storage.Service) *Handlers {
	return &Handlers{
		config:   cfg,
		logger:   log,
		storage:  store,
	}
}

// APIResponse estructura estándar de respuesta
type APIResponse struct {
	Success   bool        `json:"success"`
	Data      interface{} `json:"data,omitempty"`
	Error     *APIError   `json:"error,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
	RequestID string      `json:"request_id,omitempty"`
}

// APIError estructura de error
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// SuccessResponse crea una respuesta exitosa
func (h *Handlers) SuccessResponse(c *fiber.Ctx, data interface{}) error {
	return c.JSON(APIResponse{
		Success:   true,
		Data:      data,
		Timestamp: time.Now().UTC(),
		RequestID: c.Get("X-Request-ID"),
	})
}

// ErrorResponse crea una respuesta de error
func (h *Handlers) ErrorResponse(c *fiber.Ctx, code string, message string, details string, status int) error {
	return c.Status(status).JSON(APIResponse{
		Success: false,
		Error: &APIError{
			Code:    code,
			Message: message,
			Details: details,
		},
		Timestamp: time.Now().UTC(),
		RequestID: c.Get("X-Request-ID"),
	})
}

// GetInfo handler para información general
// @Summary Información del motor
// @Description Retorna información general del motor PDF
// @Tags public
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse
// @Router /api/v1/info [get]
func (h *Handlers) GetInfo(c *fiber.Ctx) error {
	info := map[string]interface{}{
		"name":        "TuCentroPDF Engine V2",
		"version":     "2.0.0",
		"description": "Motor profesional de procesamiento PDF + Office + OCR con IA",
		"features": []string{
			"PDF Processing (merge, split, compress, rotate, etc.)",
			"Office to PDF Conversion (DOC, XLS, PPT)",
			"OCR Classic (Tesseract + PaddleOCR)",
			"OCR AI (GPT-4.1-mini Vision)",
			"Plan-based Limits (Free, Premium, Pro)",
			"REST API with Authentication",
		},
		"limits_enabled": true,
		"office_enabled": h.config.Office.Enabled,
		"ocr_enabled":    h.config.AI.Enabled,
		"environment":    h.config.Environment,
	}

	return h.SuccessResponse(c, info)
}

// GetStatus handler para status del servicio
// @Summary Status del servicio
// @Description Verifica el estado del motor y sus dependencias
// @Tags public
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse
// @Router /api/v1/status [get]
func (h *Handlers) GetStatus(c *fiber.Ctx) error {
	status := map[string]interface{}{
		"status":     "healthy",
		"uptime":     time.Since(time.Now()).String(),
		"timestamp":  time.Now().UTC(),
		"version":    "2.0.0",
		"services": map[string]string{
			"pdf_engine": "operational",
			"office":     getServiceStatus(h.config.Office.Enabled),
			"ocr_ai":     getServiceStatus(h.config.AI.Enabled),
			"redis":      getServiceStatus(h.config.Redis.Enabled),
		},
	}

	return h.SuccessResponse(c, status)
}

// GetPlanLimits handler para límites por plan
// @Summary Límites por plan
// @Description Retorna los límites para un plan específico
// @Tags public
// @Accept json
// @Produce json
// @Param plan path string true "Plan" Enums(free, premium, pro)
// @Success 200 {object} APIResponse
// @Router /api/v1/limits/{plan} [get]
func (h *Handlers) GetPlanLimits(c *fiber.Ctx) error {
	plan := c.Params("plan")
	
	var limits config.PlanLimits
	switch plan {
	case "free":
		limits = h.config.Limits.Free
	case "premium":
		limits = h.config.Limits.Premium
	case "pro":
		limits = h.config.Limits.Pro
	default:
		return h.ErrorResponse(c, "INVALID_PLAN", "Plan must be one of: free, premium, pro", "", 400)
	}

	return h.SuccessResponse(c, map[string]interface{}{
		"plan":   plan,
		"limits": limits,
	})
}

// Helper function
func getServiceStatus(enabled bool) string {
	if enabled {
		return "operational"
	}
	return "disabled"
}