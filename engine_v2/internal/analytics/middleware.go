package analytics

import (
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/tucentropdf/engine-v2/internal/analytics/models"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// Middleware middleware para capturar analíticas automáticamente
type Middleware struct {
	service *Service
	logger  *logger.Logger
}

// NewMiddleware crea nuevo middleware de analytics
func NewMiddleware(service *Service, log *logger.Logger) *Middleware {
	return &Middleware{
		service: service,
		logger:  log,
	}
}

// Capture middleware que captura datos de analytics para cada operación
func (m *Middleware) Capture() fiber.Handler {
	return func(c *fiber.Ctx) error {
		startTime := time.Now()
		
		// Crear contexto de analytics
		analyticsCtx := &AnalyticsContext{
			StartTime: startTime,
			RequestID: uuid.New().String(),
		}
		
		// Guardar contexto en locals
		c.Locals("analytics", analyticsCtx)
		
		// Procesar request
		err := c.Next()
		
		// Capturar datos después del procesamiento
		go m.captureOperationData(c, analyticsCtx, err)
		
		return err
	}
}

// AnalyticsContext contexto de analytics para una request
type AnalyticsContext struct {
	StartTime   time.Time
	RequestID   string
	Tool        string
	Operation   string
	FileSize    int64
	ResultSize  int64
	Pages       int
	Worker      string
	Status      string
	FailReason  string
	CPUUsed     float64
	RAMUsed     int64
	Retries     int
}

// captureOperationData captura datos de la operación completada
func (m *Middleware) captureOperationData(c *fiber.Ctx, ctx *AnalyticsContext, requestError error) {
	// Solo capturar en endpoints de operaciones (no en health, info, etc.)
	if !m.shouldCaptureEndpoint(c.Path()) {
		return
	}
	
	// Extraer datos del usuario
	userID := m.extractUserID(c)
	plan := m.extractUserPlan(c)
	country := m.extractUserCountry(c)
	isTeamMember := m.extractIsTeamMember(c)
	
	// Extraer datos de la operación
	tool, operation := m.extractToolAndOperation(c.Path(), c.Method())
	fileSize := m.extractFileSize(c)
	worker := m.extractWorker(c)
	
	// Determinar estado
	status := "success"
	failReason := ""
	
	if requestError != nil {
		status = "failed"
		failReason = requestError.Error()
	} else if c.Response().StatusCode() >= 400 {
		status = "failed"
		failReason = "HTTP " + strconv.Itoa(c.Response().StatusCode())
	} else if c.Response().StatusCode() == 408 {
		status = "timeout"
		failReason = "Request timeout"
	}
	
	// Obtener datos adicionales del contexto si existen
	if ctx.Status != "" {
		status = ctx.Status
	}
	if ctx.FailReason != "" {
		failReason = ctx.FailReason
	}
	if ctx.FileSize > 0 {
		fileSize = ctx.FileSize
	}
	if ctx.Worker != "" {
		worker = ctx.Worker
	}
	
	// Calcular duración
	duration := time.Since(ctx.StartTime).Milliseconds()
	
	// Crear registro de analytics
	op := &models.AnalyticsOperation{
		ID:           uuid.New(),
		UserID:       userID,
		Plan:         plan,
		IsTeamMember: isTeamMember,
		Country:      country,
		Tool:         tool,
		Operation:    operation,
		FileSize:     fileSize,
		ResultSize:   ctx.ResultSize,
		Pages:        ctx.Pages,
		Worker:       worker,
		Status:       status,
		FailReason:   failReason,
		Duration:     duration,
		CPUUsed:      ctx.CPUUsed,
		RAMUsed:      ctx.RAMUsed,
		QueueTime:    0, // Queue time requires request queuing system (future enhancement)
		Retries:      ctx.Retries,
		Timestamp:    ctx.StartTime,
	}
	
	// Registrar operación
	if err := m.service.RegisterOperation(op); err != nil {
		m.logger.Error("Error registering analytics operation", 
			"error", err, 
			"user_id", userID, 
			"tool", tool,
			"status", status)
	}
}

// shouldCaptureEndpoint determina si debe capturar analytics para este endpoint
func (m *Middleware) shouldCaptureEndpoint(path string) bool {
	// Lista de paths que NO deben ser capturados
	excludePaths := []string{
		"/health",
		"/info",
		"/status",
		"/docs",
		"/swagger",
		"/metrics",
		"/analytics", // Evitar recursion en endpoints de analytics
		"/limits",
		"/plans",
	}
	
	for _, exclude := range excludePaths {
		if len(path) >= len(exclude) && path[:len(exclude)] == exclude {
			return false
		}
	}
	
	return true
}

// extractUserID extrae el ID del usuario de la request
func (m *Middleware) extractUserID(c *fiber.Ctx) string {
	// Intentar obtener de varios lugares
	if userID := c.Get("X-User-ID"); userID != "" {
		return userID
	}
	
	if userID := c.Locals("userID"); userID != nil {
		if uid, ok := userID.(string); ok {
			return uid
		}
	}
	
	// Generar ID temporal basado en IP si no hay usuario autenticado
	return "anonymous_" + c.IP()
}

// extractUserPlan extrae el plan del usuario
func (m *Middleware) extractUserPlan(c *fiber.Ctx) string {
	if plan := c.Get("X-User-Plan"); plan != "" {
		return plan
	}
	
	if plan := c.Locals("userPlan"); plan != nil {
		if p, ok := plan.(string); ok {
			return p
		}
	}
	
	return "free" // Plan por defecto
}

// extractUserCountry extrae el país del usuario
func (m *Middleware) extractUserCountry(c *fiber.Ctx) string {
	// Intentar obtener de headers
	if country := c.Get("CF-IPCountry"); country != "" { // Cloudflare
		return country
	}
	
	if country := c.Get("X-Country"); country != "" {
		return country
	}
	
	if country := c.Locals("country"); country != nil {
		if c, ok := country.(string); ok {
			return c
		}
	}
	
	return "unknown"
}

// extractIsTeamMember determina si el usuario es miembro de un equipo
func (m *Middleware) extractIsTeamMember(c *fiber.Ctx) bool {
	if teamMember := c.Get("X-Team-Member"); teamMember == "true" {
		return true
	}
	
	if teamMember := c.Locals("isTeamMember"); teamMember != nil {
		if tm, ok := teamMember.(bool); ok {
			return tm
		}
	}
	
	return false
}

// extractToolAndOperation extrae la herramienta y operación del path
func (m *Middleware) extractToolAndOperation(path, method string) (string, string) {
	// Parsear path para extraer herramienta
	// Ejemplos:
	// /api/v1/pdf/merge -> tool: pdf_merge, operation: merge
	// /api/v2/ocr/ai -> tool: ocr_ai, operation: ai
	// /api/v1/office/convert -> tool: office_convert, operation: convert
	
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 3 {
		return "unknown", method
	}
	
	// Extraer categoria y operación
	var category, operation string
	
	for i, part := range parts {
		if part == "pdf" || part == "ocr" || part == "office" || part == "utils" {
			category = part
			if i+1 < len(parts) {
				operation = parts[i+1]
			}
			break
		}
	}
	
	if category == "" {
		category = "unknown"
		operation = method
	}
	
	if operation == "" {
		operation = method
	}
	
	tool := category + "_" + operation
	return tool, operation
}

// extractFileSize extrae el tamaño del archivo de la request
func (m *Middleware) extractFileSize(c *fiber.Ctx) int64 {
	// Intentar obtener de form file
	file, err := c.FormFile("file")
	if err == nil && file != nil {
		return file.Size
	}
	
	// Intentar obtener de Content-Length
	if contentLength := c.Get("Content-Length"); contentLength != "" {
		if size, err := strconv.ParseInt(contentLength, 10, 64); err == nil {
			return size
		}
	}
	
	// Intentar obtener de locals (puede ser seteado por handlers)
	if size := c.Locals("fileSize"); size != nil {
		if s, ok := size.(int64); ok {
			return s
		}
	}
	
	return 0
}

// extractWorker extrae qué worker procesó la request
func (m *Middleware) extractWorker(c *fiber.Ctx) string {
	// Intentar obtener de locals (puede ser seteado por handlers)
	if worker := c.Locals("worker"); worker != nil {
		if w, ok := worker.(string); ok {
			return w
		}
	}
	
	// Intentar obtener de headers
	if worker := c.Get("X-Worker"); worker != "" {
		return worker
	}
	
	return "api" // Worker por defecto
}

// SetAnalyticsData permite a los handlers establecer datos adicionales de analytics
func SetAnalyticsData(c *fiber.Ctx, key string, value interface{}) {
	if analytics := c.Locals("analytics"); analytics != nil {
		if ctx, ok := analytics.(*AnalyticsContext); ok {
			switch key {
			case "tool":
				if v, ok := value.(string); ok {
					ctx.Tool = v
				}
			case "operation":
				if v, ok := value.(string); ok {
					ctx.Operation = v
				}
			case "file_size":
				if v, ok := value.(int64); ok {
					ctx.FileSize = v
				}
			case "result_size":
				if v, ok := value.(int64); ok {
					ctx.ResultSize = v
				}
			case "pages":
				if v, ok := value.(int); ok {
					ctx.Pages = v
				}
			case "worker":
				if v, ok := value.(string); ok {
					ctx.Worker = v
				}
			case "status":
				if v, ok := value.(string); ok {
					ctx.Status = v
				}
			case "fail_reason":
				if v, ok := value.(string); ok {
					ctx.FailReason = v
				}
			case "cpu_used":
				if v, ok := value.(float64); ok {
					ctx.CPUUsed = v
				}
			case "ram_used":
				if v, ok := value.(int64); ok {
					ctx.RAMUsed = v
				}
			case "retries":
				if v, ok := value.(int); ok {
					ctx.Retries = v
				}
			}
		}
	}
}

// GetAnalyticsContext obtiene el contexto de analytics
func GetAnalyticsContext(c *fiber.Ctx) *AnalyticsContext {
	if analytics := c.Locals("analytics"); analytics != nil {
		if ctx, ok := analytics.(*AnalyticsContext); ok {
			return ctx
		}
	}
	return nil
}