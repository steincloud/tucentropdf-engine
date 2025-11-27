package legal_audit

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// AuditMiddleware middleware para auditoría automática de todas las operaciones
type AuditMiddleware struct {
	service *Service
}

// NewAuditMiddleware crea nueva instancia del middleware
func NewAuditMiddleware(service *Service) *AuditMiddleware {
	return &AuditMiddleware{
		service: service,
	}
}

// Handler retorna el handler de Gin para auditoría automática
func (am *AuditMiddleware) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// Capturar request body si es necesario
		var requestBody []byte
		if am.shouldCaptureBody(c) {
			requestBody = am.captureRequestBody(c)
		}

		// Procesar request
		c.Next()

		// Crear evento de auditoría después del procesamiento
		am.createAuditEvent(c, start, requestBody)
	}
}

// createAuditEvent crea evento de auditoría basado en la request/response
func (am *AuditMiddleware) createAuditEvent(c *gin.Context, startTime time.Time, requestBody []byte) {
	// Extraer información del contexto
	userID := am.getUserID(c)
	plan := am.getUserPlan(c)
	companyID := am.getCompanyID(c)
	apiKeyID := am.getAPIKeyID(c)
	adminID := am.getAdminID(c)

	// Determinar herramienta basada en la ruta
	tool := am.determineTool(c.Request.URL.Path)

	// Determinar acción basada en método y ruta
	action := am.determineAction(c.Request.Method, c.Request.URL.Path, c.Writer.Status())

	// Determinar estado basado en response status
	status := am.determineStatus(c.Writer.Status())

	// Obtener tamaño del archivo si aplica
	fileSize := am.getFileSize(c, requestBody)

	// Detectar abuso
	abuse := am.detectAbuse(c, userID)

	// Obtener razón de fallo si aplica
	reason := am.getReason(c, status)

	// Crear metadata detallada
	metadata := am.createMetadata(c, startTime, requestBody)

	// Crear evento de auditoría
	event := &AuditEvent{
		UserID:    userID,
		Tool:      tool,
		Action:    action,
		Plan:      plan,
		FileSize:  fileSize,
		IP:        am.getClientIP(c),
		UserAgent: c.GetHeader("User-Agent"),
		Status:    status,
		Reason:    reason,
		Metadata:  metadata,
		Abuse:     abuse,
		CompanyID: companyID,
		APIKeyID:  apiKeyID,
		AdminID:   adminID,
	}

	// Registrar evento (async para no bloquear response)
	go func() {
		if err := am.service.RecordEvent(event); err != nil {
			am.service.logger.Error("Failed to record audit event", "error", err)
		}
	}()
}

// determineTool determina la herramienta basada en la ruta
func (am *AuditMiddleware) determineTool(path string) string {
	switch {
	case strings.Contains(path, "/pdf/compress"):
		return ToolPDFCompress
	case strings.Contains(path, "/pdf/merge"):
		return ToolPDFMerge
	case strings.Contains(path, "/pdf/split"):
		return ToolPDFSplit
	case strings.Contains(path, "/pdf/extract"):
		return ToolPDFExtract
	case strings.Contains(path, "/ocr"):
		return ToolOCR
	case strings.Contains(path, "/office"):
		return ToolOfficeConvert
	case strings.Contains(path, "/image"):
		return ToolImageProcess
	case strings.Contains(path, "/api") || strings.HasPrefix(path, "/api/"):
		return ToolAPI
	case strings.Contains(path, "/admin"):
		return ToolAdmin
	case strings.Contains(path, "/auth") || strings.Contains(path, "/login") || strings.Contains(path, "/logout"):
		return ToolAuth
	case strings.Contains(path, "/subscription") || strings.Contains(path, "/payment"):
		return ToolSubscription
	default:
		return "unknown"
	}
}

// determineAction determina la acción basada en método y ruta
func (am *AuditMiddleware) determineAction(method, path string, status int) string {
	switch method {
	case "POST":
		switch {
		case strings.Contains(path, "/upload"):
			return ActionUpload
		case strings.Contains(path, "/process"):
			return ActionProcess
		case strings.Contains(path, "/login"):
			return ActionLogin
		case strings.Contains(path, "/subscribe"):
			return ActionSubscribe
		case strings.Contains(path, "/upgrade"):
			return ActionUpgrade
		case strings.Contains(path, "/api-key"):
			return ActionAPIKeyCreate
		default:
			if status >= 400 {
				return ActionFail
			}
			return ActionProcess
		}
	case "GET":
		switch {
		case strings.Contains(path, "/download"):
			return ActionDownload
		case strings.Contains(path, "/admin"):
			return ActionAdminAccess
		default:
			return "view"
		}
	case "PUT", "PATCH":
		return ActionSystemChange
	case "DELETE":
		switch {
		case strings.Contains(path, "/api-key"):
			return ActionAPIKeyRevoke
		case strings.Contains(path, "/subscription"):
			return ActionCancel
		default:
			return "delete"
		}
	default:
		return "unknown"
	}
}

// determineStatus determina el estado basado en HTTP status code
func (am *AuditMiddleware) determineStatus(statusCode int) string {
	switch {
	case statusCode >= 200 && statusCode < 300:
		return StatusSuccess
	case statusCode == 400:
		return StatusRejected
	case statusCode == 401 || statusCode == 403:
		return StatusProtectorBlocked
	case statusCode == 408:
		return StatusTimeout
	case statusCode == 429:
		return StatusAbuse
	case statusCode >= 400:
		return StatusFail
	default:
		return StatusSuccess
	}
}

// createMetadata crea metadata detallada para el evento
func (am *AuditMiddleware) createMetadata(c *gin.Context, startTime time.Time, requestBody []byte) *JSONBMetadata {
	metadata := &JSONBMetadata{
		Duration:      time.Since(startTime).Milliseconds(),
		Endpoint:      c.Request.URL.Path,
		RequestMethod: c.Request.Method,
		ResponseSize:  int64(c.Writer.Size()),
		Extra:         make(map[string]interface{}),
	}

	// Información de geolocalización (básica)
	if ip := am.getClientIP(c); ip != "" {
		metadata.GeoLocation = am.getGeoLocation(ip)
	}

	// Detectar VPN/Proxy
	metadata.VPNDetected = am.detectVPN(c)

	// User Agent sospechoso
	metadata.SuspiciousUA = am.detectSuspiciousUA(c.GetHeader("User-Agent"))

	// Rate limiting
	if rateLimitHit, exists := c.Get("rate_limit_hit"); exists {
		metadata.RateLimitHit = rateLimitHit.(bool)
	}

	// Información del archivo
	if fileHeader, err := c.FormFile("file"); err == nil {
		metadata.OriginalName = fileHeader.Filename
		metadata.FileType = am.getFileType(fileHeader.Filename)
	}

	// Información del worker
	if workerID, exists := c.Get("worker_id"); exists {
		metadata.WorkerID = workerID.(string)
	}
	if processingID, exists := c.Get("processing_id"); exists {
		metadata.ProcessingID = processingID.(string)
	}

	// Información de dominio para API corporativa
	if domain := c.GetHeader("Origin"); domain != "" {
		metadata.Domain = domain
	} else if referer := c.GetHeader("Referer"); referer != "" {
		metadata.Domain = am.extractDomain(referer)
	}

	// Headers relevantes para auditoría
	metadata.Extra["content_type"] = c.GetHeader("Content-Type")
	metadata.Extra["accept"] = c.GetHeader("Accept")
	metadata.Extra["x_forwarded_for"] = c.GetHeader("X-Forwarded-For")

	// Información de límites si aplica
	if limitType, exists := c.Get("limit_exceeded"); exists {
		metadata.LimitType = limitType.(string)
	}
	if limitValue, exists := c.Get("limit_value"); exists {
		metadata.LimitValue = limitValue.(int64)
	}
	if currentUsage, exists := c.Get("current_usage"); exists {
		metadata.CurrentUsage = currentUsage.(int64)
	}

	return metadata
}

// Funciones helper para extraer información del contexto

func (am *AuditMiddleware) getUserID(c *gin.Context) *int64 {
	if userID, exists := c.Get("user_id"); exists {
		if id, ok := userID.(int64); ok {
			return &id
		}
		if id, ok := userID.(int); ok {
			id64 := int64(id)
			return &id64
		}
	}
	return nil
}

func (am *AuditMiddleware) getUserPlan(c *gin.Context) string {
	if plan, exists := c.Get("user_plan"); exists {
		if planStr, ok := plan.(string); ok {
			return planStr
		}
	}
	return ""
}

func (am *AuditMiddleware) getCompanyID(c *gin.Context) *int64 {
	if companyID, exists := c.Get("company_id"); exists {
		if id, ok := companyID.(int64); ok {
			return &id
		}
		if id, ok := companyID.(int); ok {
			id64 := int64(id)
			return &id64
		}
	}
	return nil
}

func (am *AuditMiddleware) getAPIKeyID(c *gin.Context) *string {
	if keyID, exists := c.Get("api_key_id"); exists {
		if keyStr, ok := keyID.(string); ok {
			return &keyStr
		}
	}
	return nil
}

func (am *AuditMiddleware) getAdminID(c *gin.Context) *int64 {
	if adminID, exists := c.Get("admin_id"); exists {
		if id, ok := adminID.(int64); ok {
			return &id
		}
		if id, ok := adminID.(int); ok {
			id64 := int64(id)
			return &id64
		}
	}
	return nil
}

func (am *AuditMiddleware) getClientIP(c *gin.Context) string {
	// Priorizar headers de proxy
	if xff := c.GetHeader("X-Forwarded-For"); xff != "" {
		if ips := strings.Split(xff, ","); len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}
	if xri := c.GetHeader("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	
	// Obtener IP directa
	ip, _, err := net.SplitHostPort(c.Request.RemoteAddr)
	if err != nil {
		return c.Request.RemoteAddr
	}
	return ip
}

func (am *AuditMiddleware) getFileSize(c *gin.Context, requestBody []byte) *int64 {
	// Intentar obtener de multipart form
	if fileHeader, err := c.FormFile("file"); err == nil {
		return &fileHeader.Size
	}
	
	// Usar tamaño del body si está disponible
	if len(requestBody) > 0 {
		size := int64(len(requestBody))
		return &size
	}
	
	// Usar Content-Length header
	if contentLength := c.GetHeader("Content-Length"); contentLength != "" {
		if size, err := strconv.ParseInt(contentLength, 10, 64); err == nil {
			return &size
		}
	}
	
	return nil
}

func (am *AuditMiddleware) detectAbuse(c *gin.Context, userID *int64) bool {
	// Verificar rate limiting
	if rateLimitHit, exists := c.Get("rate_limit_hit"); exists && rateLimitHit.(bool) {
		return true
	}
	
	// Verificar códigos de estado de abuso
	if c.Writer.Status() == 429 {
		return true
	}
	
	// Verificar patrones sospechosos en user agent
	userAgent := c.GetHeader("User-Agent")
	if am.detectSuspiciousUA(userAgent) {
		return true
	}
	
	// Verificar actividad anómala (implementar lógica específica)
	if userID != nil {
		// Aquí se podría implementar detección de abuso específica
		// basada en histórico del usuario
	}
	
	return false
}

func (am *AuditMiddleware) getReason(c *gin.Context, status string) *string {
	if status == StatusSuccess {
		return nil
	}
	
	// Intentar obtener razón específica del contexto
	if reason, exists := c.Get("error_reason"); exists {
		if reasonStr, ok := reason.(string); ok {
			return &reasonStr
		}
	}
	
	// Razones basadas en códigos de estado
	statusCode := c.Writer.Status()
	switch statusCode {
	case 400:
		reason := "Bad request or invalid parameters"
		return &reason
	case 401:
		reason := "Unauthorized access"
		return &reason
	case 403:
		reason := "Forbidden or access denied"
		return &reason
	case 413:
		reason := "File size too large"
		return &reason
	case 415:
		reason := "Unsupported file type"
		return &reason
	case 429:
		reason := "Rate limit exceeded"
		return &reason
	case 500:
		reason := "Internal server error"
		return &reason
	case 503:
		reason := "Service temporarily unavailable"
		return &reason
	default:
		if statusCode >= 400 {
			reason := fmt.Sprintf("HTTP error %d", statusCode)
			return &reason
		}
	}
	
	return nil
}

// Funciones helper adicionales

func (am *AuditMiddleware) shouldCaptureBody(c *gin.Context) bool {
	// Solo capturar body para ciertas rutas críticas
	path := c.Request.URL.Path
	method := c.Request.Method
	
	// Capturar para uploads y procesamiento
	if method == "POST" && (strings.Contains(path, "/upload") || strings.Contains(path, "/process")) {
		return true
	}
	
	// Capturar para APIs administrativas
	if strings.Contains(path, "/admin") {
		return true
	}
	
	return false
}

func (am *AuditMiddleware) captureRequestBody(c *gin.Context) []byte {
	if c.Request.Body == nil {
		return nil
	}
	
	// Leer body
	bodyBytes, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return nil
	}
	
	// Restaurar body para que pueda ser leído nuevamente
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
	
	// Limitar tamaño del body capturado (max 1MB)
	if len(bodyBytes) > 1024*1024 {
		return bodyBytes[:1024*1024]
	}
	
	return bodyBytes
}

func (am *AuditMiddleware) getGeoLocation(ip string) string {
	// Implementación básica - en producción usar servicio de geolocalización
	if ip == "127.0.0.1" || ip == "::1" || strings.HasPrefix(ip, "192.168.") {
		return "local"
	}
	return "unknown"
}

func (am *AuditMiddleware) detectVPN(c *gin.Context) bool {
	// Implementación básica - verificar headers comunes de VPN/Proxy
	headers := []string{
		"X-Forwarded-For",
		"X-Proxy-Connection",
		"X-Real-IP",
		"Via",
	}
	
	for _, header := range headers {
		if c.GetHeader(header) != "" {
			return true
		}
	}
	
	return false
}

func (am *AuditMiddleware) detectSuspiciousUA(userAgent string) bool {
	// User agents sospechosos
	suspiciousPatterns := []string{
		"bot", "crawler", "spider", "scraper",
		"curl", "wget", "python", "postman",
		"test", "automation", "headless",
	}
	
	uaLower := strings.ToLower(userAgent)
	for _, pattern := range suspiciousPatterns {
		if strings.Contains(uaLower, pattern) {
			return true
		}
	}
	
	return false
}

func (am *AuditMiddleware) getFileType(filename string) string {
	if strings.Contains(filename, ".") {
		parts := strings.Split(filename, ".")
		return strings.ToLower(parts[len(parts)-1])
	}
	return "unknown"
}

func (am *AuditMiddleware) extractDomain(url string) string {
	if strings.Contains(url, "://") {
		parts := strings.Split(url, "://")
		if len(parts) > 1 {
			domain := strings.Split(parts[1], "/")[0]
			return domain
		}
	}
	return ""
}