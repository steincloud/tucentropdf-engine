package legal_audit

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// Handler maneja requests HTTP para sistema de auditoría legal
type Handler struct {
	service       *Service
	exportManager *ExportManager
	logger        *logger.Logger
}

// NewHandler crea nueva instancia del handler
func NewHandler(service *Service, exportManager *ExportManager, log *logger.Logger) *Handler {
	return &Handler{
		service:       service,
		exportManager: exportManager,
		logger:        log,
	}
}

// RegisterRoutes registra rutas del sistema de auditoría legal
func (h *Handler) RegisterRoutes(router *gin.Engine) {
	legal := router.Group("/api/v2/legal-audit")
	{
		// Rutas para administradores
		admin := legal.Group("/admin")
		admin.Use(h.RequireAdminAuth())
		{
			admin.GET("/logs", h.GetAuditLogs)
			admin.GET("/logs/:id", h.GetAuditLogByID)
			admin.POST("/export", h.CreateExport)
			admin.GET("/exports/:id/download", h.DownloadExport)
			admin.POST("/integrity-package", h.CreateIntegrityPackage)
			admin.GET("/integrity/verify", h.VerifyIntegrity)
			admin.GET("/stats", h.GetAuditStats)
		}

		// Rutas públicas (limitadas)
		legal.GET("/health", h.HealthCheck)
		legal.GET("/status", h.GetSystemStatus)
	}
}

// GetAuditLogs obtiene registros de auditoría con filtros
func (h *Handler) GetAuditLogs(c *gin.Context) {
	h.logger.Debug("📋 Getting audit logs", "admin_id", h.getAdminID(c))

	// Parsear filtros
	filter, err := h.parseAuditFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid filter parameters", "details": err.Error()})
		return
	}

	// Obtener registros
	logs, err := h.service.GetAuditLogs(filter)
	if err != nil {
		h.logger.Error("Failed to get audit logs", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve audit logs", "details": err.Error()})
		return
	}

	// Obtener estadísticas adicionales
	stats, err := h.service.GetAuditStats(&AuditFilter{
		FromDate: filter.FromDate,
		ToDate:   filter.ToDate,
	})
	if err != nil {
		h.logger.Warn("Failed to get audit stats", "error", err)
		stats = &AuditStats{} // Usar stats vacías si falla
	}

	c.JSON(200, gin.H{
		"logs":  logs,
		"stats": stats,
		"filter": gin.H{
			"start_date": filter.FromDate,
			"end_date":   filter.ToDate,
			"tool":       filter.Tool,
			"action":     filter.Action,
			"user_id":    filter.UserID,
			"limit":      filter.Limit,
			"offset":     filter.Offset,
		},
	})
}

// GetAuditLogByID obtiene registro específico por ID
func (h *Handler) GetAuditLogByID(c *gin.Context) {
	logID := c.Param("id")
	if logID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Log ID is required"})
		return
	}

	uuidLogID, err := uuid.Parse(logID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid log ID format"})
		return
	}

	log, err := h.service.GetAuditRecord(uuidLogID)
	if err != nil {
		h.logger.Error("Failed to get audit log by ID", "id", logID, "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve audit log", "details": err.Error()})
		return
	}

	// Verificar integridad del registro específico
	verified, _ := h.service.VerifyRecord(uuidLogID)

	c.JSON(200, gin.H{
		"log":      log,
		"verified": verified,
	})
}

// CreateExport crea exportación de evidencia legal
func (h *Handler) CreateExport(c *gin.Context) {
	adminID := h.getAdminID(c)
	h.logger.Info("📤 Creating legal evidence export", "admin_id", adminID)

	var req struct {
		Format    string        `json:"format" binding:"required"`
		Encrypted bool          `json:"encrypted"`
		Filter    *AuditFilter  `json:"filter"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	// Validar formato
	if req.Format != FormatJSON && req.Format != FormatCSV && req.Format != FormatXML {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid export format", "details": "Supported formats: json, csv, xml"})
		return
	}

	// Aplicar filtros por defecto si no se especifican
	if req.Filter == nil {
		defaultFrom := time.Now().AddDate(0, -1, 0)
		req.Filter = &AuditFilter{
			FromDate: &defaultFrom,
			ToDate:   &time.Time{},
		}
	}

	// Crear request de exportación
	exportRequest := &ExportRequest{
		Format:    req.Format,
		Encrypted: req.Encrypted,
		AdminID:   adminID,
	}

	// Ejecutar exportación
	result, err := h.exportManager.ExportToFile(req.Filter, exportRequest)
	if err != nil {
		h.logger.Error("Failed to create export", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create export", "details": err.Error()})
		return
	}

	h.logger.Info("Legal evidence export created",
		"export_id", result.ExportID,
		"records", result.RecordCount,
		"file_size", result.FileSize)

	c.JSON(200, gin.H{
		"export_id":       result.ExportID,
		"record_count":    result.RecordCount,
		"file_size":       result.FileSize,
		"encrypted":       result.Encrypted,
		"integrity_score": result.IntegrityReport.IntegrityScore,
		"verified":        result.IntegrityReport.Verified,
		"download_token":  result.DownloadToken,
		"expires_at":      result.ExpiresAt,
	})
}

// DownloadExport descarga archivo de exportación
func (h *Handler) DownloadExport(c *gin.Context) {
	exportID := c.Param("id")
	token := c.Query("token")
	adminID := h.getAdminID(c)

	if exportID == "" || token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Export ID and token are required", "details": "Both exportId and token parameters must be provided"})
		return
	}

	h.logger.Info("📥 Processing export download",
		"export_id", exportID,
		"admin_id", adminID)

	// TODO: Validar token y buscar archivo de exportación
	// En una implementación real:
	// 1. Validar token contra base de datos
	// 2. Verificar permisos del admin
	// 3. Comprobar si el archivo no ha expirado
	// 4. Servir el archivo

	c.JSON(http.StatusNotImplemented, gin.H{"error": "Download functionality not implemented", "details": "This feature is under development"})
}

// CreateIntegrityPackage crea paquete completo de evidencia con integridad
func (h *Handler) CreateIntegrityPackage(c *gin.Context) {
	adminID := h.getAdminID(c)
	h.logger.Info("📦 Creating integrity evidence package", "admin_id", adminID)

	var req struct {
		Filter *AuditFilter `json:"filter"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format", "details": err.Error()})
		return
	}

	// Aplicar filtros por defecto
	if req.Filter == nil {
		defaultFrom := time.Now().AddDate(0, -1, 0)
		defaultTo := time.Now()
		req.Filter = &AuditFilter{
			FromDate: &defaultFrom,
			ToDate:   &defaultTo,
		}
	}

	// Crear paquete de integridad
	pkg, err := h.exportManager.CreateIntegrityPackage(req.Filter, adminID)
	if err != nil {
		h.logger.Error("Failed to create integrity package", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create integrity package", "details": err.Error()})
		return
	}

	h.logger.Info("Integrity evidence package created",
		"package_id", pkg.ID,
		"records", pkg.RecordCount,
		"integrity_score", pkg.IntegrityScore)

	c.JSON(200, gin.H{
		"package_id":      pkg.ID,
		"record_count":    pkg.RecordCount,
		"integrity_score": pkg.IntegrityScore,
		"verified":        pkg.Verified,
		"created_at":      pkg.CreatedAt,
	})
}

// VerifyIntegrity verifica integridad de registros en rango
func (h *Handler) VerifyIntegrity(c *gin.Context) {
	h.logger.Debug("🔍 Verifying record integrity")

	// Parsear filtros
	filter, err := h.parseAuditFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid filter parameters", "details": err.Error()})
		return
	}

	// Ejecutar verificación de integridad
	report, err := h.service.VerifyBatchIntegrity(filter)

	h.logger.Info("Integrity verification completed",
		"total_records", report.RecordCount,
		"integrity_score", report.IntegrityScore,
		"verified", report.Verified)

	c.JSON(200, gin.H{
		"report": report,
	})
}

// GetAuditStats obtiene estadísticas de auditoría
func (h *Handler) GetAuditStats(c *gin.Context) {
	h.logger.Debug("📊 Getting audit statistics")

	// Parsear rango de fechas
	filter, err := h.parseAuditFilter(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid filter parameters", "details": err.Error()})
		return
	}

	// Obtener estadísticas
	stats, err := h.service.GetAuditStats(filter)
	if err != nil {
		h.logger.Error("Failed to get audit stats", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve audit statistics", "details": err.Error()})
		return
	}

	c.JSON(200, gin.H{
		"stats": stats,
		"period": gin.H{
			"start": filter.FromDate,
			"end":   filter.ToDate,
		},
	})
}

// HealthCheck verifica salud del sistema de auditoría
func (h *Handler) HealthCheck(c *gin.Context) {
	status := gin.H{
		"status":    "healthy",
		"timestamp": time.Now(),
		"version":   "v2.0.0",
	}

	// Verificar conexiones críticas
	if !h.service.IsHealthy() {
		status["status"] = "unhealthy"
		status["issues"] = []string{"database connection failed"}
		c.JSON(http.StatusServiceUnavailable, status)
		return
	}

	c.JSON(http.StatusOK, status)
}

// GetSystemStatus obtiene estado detallado del sistema
func (h *Handler) GetSystemStatus(c *gin.Context) {
	status := gin.H{
		"legal_audit": gin.H{
			"active":          true,
			"retention_days":  1095, // 3 años
			"encryption":      "AES256-GCM",
			"signing":         "HMAC-SHA256",
		},
		"storage": gin.H{
			"type": "PostgreSQL",
			"healthy": h.service.IsHealthy(),
		},
		"compliance": gin.H{
			"immutable_logs":    true,
			"digital_signatures": true,
			"chain_verification": true,
			"legal_evidence":     true,
		},
	}

	c.JSON(http.StatusOK, status)
}

// Middleware y helpers

// RequireAdminAuth middleware para autenticación de administradores
func (h *Handler) RequireAdminAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: Implementar validación de token de administrador
		// Por ahora, simulamos autenticación básica
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required", "details": "Please provide a valid Bearer token"})
			c.Abort()
			return
		}

		// Validación básica - en producción usar JWT
		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid authorization format", "details": "Authorization header must use Bearer token format"})
			c.Abort()
			return
		}

		// Extraer y validar token
		token := strings.TrimPrefix(authHeader, "Bearer ")
		adminID, err := h.validateAdminToken(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid admin token", "details": err.Error()})
			c.Abort()
			return
		}

		// Guardar ID del admin en contexto
		c.Set("admin_id", adminID)
		c.Next()
	}
}

// parseAuditFilter parsea parámetros de filtro desde query parameters
func (h *Handler) parseAuditFilter(c *gin.Context) (*AuditFilter, error) {
	filter := &AuditFilter{
		Limit:  100, // Límite por defecto
		Offset: 0,
	}

	// Fechas
	if startStr := c.Query("start_date"); startStr != "" {
		if startDate, err := time.Parse("2006-01-02", startStr); err == nil {
			filter.FromDate = &startDate
		}
	}

	if endStr := c.Query("end_date"); endStr != "" {
		if endDate, err := time.Parse("2006-01-02", endStr); err == nil {
			filter.ToDate = &endDate
		}
	}

	// Filtros de texto
	if tool := c.Query("tool"); tool != "" {
		filter.Tool = tool
	}
	if action := c.Query("action"); action != "" {
		filter.Action = action
	}

	// User ID
	if userIDStr := c.Query("user_id"); userIDStr != "" {
		if userID, err := strconv.ParseInt(userIDStr, 10, 64); err == nil {
			filter.UserID = &userID
		}
	}

	// Paginación
	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 && limit <= 1000 {
			filter.Limit = limit
		}
	}

	if offsetStr := c.Query("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil && offset >= 0 {
			filter.Offset = offset
		}
	}

	return filter, nil
}

// getAdminID obtiene ID del administrador desde contexto
func (h *Handler) getAdminID(c *gin.Context) int64 {
	if adminID, exists := c.Get("admin_id"); exists {
		if id, ok := adminID.(int64); ok {
			return id
		}
	}
	return 0 // ID por defecto para casos sin autenticación
}

// validateAdminToken valida token de administrador
func (h *Handler) validateAdminToken(token string) (int64, error) {
	// TODO: Implementar validación real de token
	// Por ahora, simulamos validación básica
	
	// Tokens de ejemplo para testing
	switch token {
	case "admin-token-123":
		return 1, nil
	case "admin-token-456":
		return 2, nil
	default:
		return 0, fmt.Errorf("invalid token")
	}
}
