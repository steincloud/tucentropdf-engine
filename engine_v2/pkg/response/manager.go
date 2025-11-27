package response

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/tucentropdf/engine-v2/pkg/logger"
	"go.uber.org/zap"
)

// APIResponse estructura estándar para todas las respuestas
type APIResponse struct {
	Success   bool                   `json:"success"`
	Error     *ErrorDetails          `json:"error,omitempty"`
	Message   string                 `json:"message,omitempty"`
	Data      interface{}            `json:"data,omitempty"`
	Meta      *MetaInfo              `json:"meta,omitempty"`
	Timestamp time.Time              `json:"timestamp"`
}

// ErrorDetails detalles del error
type ErrorDetails struct {
	Code    string      `json:"code"`
	Message string      `json:"message"`
	Details interface{} `json:"details,omitempty"`
	Type    string      `json:"type,omitempty"`
}

// MetaInfo información adicional
type MetaInfo struct {
	RequestID    string        `json:"request_id,omitempty"`
	Duration     time.Duration `json:"duration_ms,omitempty"`
	Version      string        `json:"version,omitempty"`
	Plan         string        `json:"plan,omitempty"`
	UsageLimits  interface{}   `json:"usage_limits,omitempty"`
}

// ErrorType tipos de errores
const (
	ErrorTypeValidation    = "VALIDATION_ERROR"
	ErrorTypeAuth         = "AUTHENTICATION_ERROR"
	ErrorTypePermission   = "PERMISSION_ERROR"
	ErrorTypeNotFound     = "NOT_FOUND_ERROR"
	ErrorTypeRateLimit    = "RATE_LIMIT_ERROR"
	ErrorTypeService      = "SERVICE_ERROR"
	ErrorTypeInternal     = "INTERNAL_ERROR"
	ErrorTypeExternal     = "EXTERNAL_SERVICE_ERROR"
)

// Error codes comunes
const (
	ErrCodeInvalidFile     = "INVALID_FILE"
	ErrCodeFileTooLarge    = "FILE_TOO_LARGE"
	ErrCodeUnsupportedType = "UNSUPPORTED_TYPE"
	ErrCodePlanLimit      = "PLAN_LIMIT_EXCEEDED"
	ErrCodeInvalidSecret  = "INVALID_SECRET"
	ErrCodeServiceDown    = "SERVICE_UNAVAILABLE"
	ErrCodeProcessingFail = "PROCESSING_FAILED"
	ErrCodeMissingFile    = "MISSING_FILE"
	ErrCodeStorageError   = "STORAGE_ERROR"
)

// ResponseManager gestor de respuestas
type ResponseManager struct {
	logger *logger.Logger
}

// NewResponseManager crea nueva instancia
func NewResponseManager(log *logger.Logger) *ResponseManager {
	return &ResponseManager{
		logger: log,
	}
}

// Success respuesta exitosa
func (rm *ResponseManager) Success(c *fiber.Ctx, data interface{}, message ...string) error {
	msg := "Operation completed successfully"
	if len(message) > 0 && message[0] != "" {
		msg = message[0]
	}

	response := &APIResponse{
		Success:   true,
		Message:   msg,
		Data:      data,
		Timestamp: time.Now(),
		Meta: &MetaInfo{
			RequestID: getRequestID(c),
			Duration:  getDuration(c),
			Version:   "v2.0.0",
		},
	}

	return c.Status(200).JSON(response)
}

// Error respuesta de error
func (rm *ResponseManager) Error(c *fiber.Ctx, code string, message string, details interface{}, statusCode int) error {
	// Log del error
	rm.logError(c, code, message, details, statusCode)

	response := &APIResponse{
		Success:   false,
		Message:   message,
		Error: &ErrorDetails{
			Code:    code,
			Message: message,
			Details: details,
			Type:    getErrorType(code),
		},
		Timestamp: time.Now(),
		Meta: &MetaInfo{
			RequestID: getRequestID(c),
			Duration:  getDuration(c),
			Version:   "v2.0.0",
		},
	}

	return c.Status(statusCode).JSON(response)
}

// ValidationError error de validación
func (rm *ResponseManager) ValidationError(c *fiber.Ctx, field string, message string) error {
	details := map[string]string{
		"field": field,
		"issue": message,
	}
	
	return rm.Error(c, ErrCodeInvalidFile, 
		"Validation failed: "+message, details, 400)
}

// AuthError error de autenticación
func (rm *ResponseManager) AuthError(c *fiber.Ctx, message string) error {
	return rm.Error(c, ErrCodeInvalidSecret, 
		"Authentication failed: "+message, nil, 401)
}

// PlanLimitError error de límite de plan
func (rm *ResponseManager) PlanLimitError(c *fiber.Ctx, plan string, limit string, current string) error {
	details := map[string]string{
		"plan":    plan,
		"limit":   limit,
		"current": current,
	}
	
	return rm.Error(c, ErrCodePlanLimit,
		"Plan limit exceeded", details, 429)
}

// ServiceError error de servicio
func (rm *ResponseManager) ServiceError(c *fiber.Ctx, service string, err error) error {
	details := map[string]string{
		"service": service,
		"error":   err.Error(),
	}
	
	return rm.Error(c, ErrCodeServiceDown,
		"Service temporarily unavailable", details, 503)
}

// ProcessingError error de procesamiento
func (rm *ResponseManager) ProcessingError(c *fiber.Ctx, operation string, err error) error {
	details := map[string]string{
		"operation": operation,
		"error":     err.Error(),
	}
	
	return rm.Error(c, ErrCodeProcessingFail,
		"Processing failed", details, 500)
}

// Métodos de logging
func (rm *ResponseManager) logError(c *fiber.Ctx, code string, message string, details interface{}, statusCode int) {
	// Determinar nivel de log según código de estado
	logLevel := zap.ErrorLevel
	if statusCode < 500 {
		logLevel = zap.WarnLevel
	}

	// Campos comunes de log
	fields := []zap.Field{
		zap.String("request_id", getRequestID(c)),
		zap.String("method", c.Method()),
		zap.String("path", c.Path()),
		zap.String("ip", c.IP()),
		zap.String("user_agent", c.Get("User-Agent")),
		zap.String("error_code", code),
		zap.String("error_message", message),
		zap.Int("status_code", statusCode),
		zap.Duration("duration", getDuration(c)),
	}

	// Agregar detalles si existen
	if details != nil {
		fields = append(fields, zap.Any("error_details", details))
	}

	// Log según nivel
	switch logLevel {
	case zap.WarnLevel:
		rm.logger.Warn("API Warning", "error_code", code, "message", message)
	case zap.ErrorLevel:
		rm.logger.Error("API Error", "error_code", code, "message", message)
	default:
		rm.logger.Info("API Info", "error_code", code, "message", message)
	}
}

// Helper functions
func getRequestID(c *fiber.Ctx) string {
	id := c.Get("X-Request-ID")
	if id == "" {
		id = c.Locals("requestid").(string)
	}
	if id == "" {
		id = "unknown"
	}
	return id
}

func getDuration(c *fiber.Ctx) time.Duration {
	start := c.Locals("start_time")
	if start == nil {
		return 0
	}
	
	if startTime, ok := start.(time.Time); ok {
		return time.Since(startTime)
	}
	
	return 0
}

func getErrorType(code string) string {
	switch code {
	case ErrCodeInvalidFile, ErrCodeFileTooLarge, ErrCodeUnsupportedType:
		return ErrorTypeValidation
	case ErrCodeInvalidSecret:
		return ErrorTypeAuth
	case ErrCodePlanLimit:
		return ErrorTypeRateLimit
	case ErrCodeServiceDown:
		return ErrorTypeExternal
	case ErrCodeProcessingFail, ErrCodeStorageError:
		return ErrorTypeService
	default:
		return ErrorTypeInternal
	}
}