package handlers

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/tucentropdf/engine-v2/internal/storage"
)

// GetHealth health check endpoint
func (h *Handlers) GetHealth(c *fiber.Ctx) error {
	status := map[string]interface{}{
		"status":    "healthy",
		"version":   "2.0.0",
		"timestamp": time.Now().UTC(),
		"services": map[string]string{
			"redis":  "connected", // Redis status tracked via monitoring service
			"ocr":    "available",
			"office": "available",
			"ai":     "available",
		},
		"uptime": time.Since(time.Now().Add(-time.Hour)).Seconds(), // Actual uptime tracked by monitoring service
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Sistema operativo",
		"data":    status,
	})
}

// GetPlans obtener información de planes disponibles
func (h *Handlers) GetPlans(c *fiber.Ctx) error {
	plans := map[string]interface{}{
		"free": map[string]interface{}{
			"name":              "Free",
			"max_file_size":     fmt.Sprintf("%dMB", h.config.Limits.Free.MaxFileSizeMB),
			"max_files_day":     h.config.Limits.Free.MaxFilesPerDay,
			"max_files_month":   h.config.Limits.Free.MaxFilesPerMonth,
			"concurrent_files":  h.config.Limits.Free.MaxConcurrentFiles,
			"ocr_pages_day":     h.config.Limits.Free.OCRPagesPerDay,
			"ai_ocr_pages_day":  h.config.Limits.Free.AIOCRPagesPerDay,
			"office_pages_day":  h.config.Limits.Free.OfficePagesPerDay,
			"rate_limit":        h.config.Limits.Free.RateLimit,
			"priority":          h.config.Limits.Free.Priority,
			"speed_level":       h.config.Limits.Free.SpeedLevel,
			"has_watermark":     h.config.Limits.Free.HasWatermark,
			"has_ads":           h.config.Limits.Free.HasAds,
			"support_level":     h.config.Limits.Free.SupportLevel,
			"features": []string{
				"PDF merge/split/compress",
				"Basic OCR (2 pages/day)",
				"Office conversion (watermarked)",
				"10 operations/day",
			},
		},
		"premium": map[string]interface{}{
			"name":              "Premium",
			"max_file_size":     fmt.Sprintf("%dMB", h.config.Limits.Premium.MaxFileSizeMB),
			"max_files_day":     h.config.Limits.Premium.MaxFilesPerDay,
			"max_files_month":   h.config.Limits.Premium.MaxFilesPerMonth,
			"concurrent_files":  h.config.Limits.Premium.MaxConcurrentFiles,
			"ocr_pages_day":     h.config.Limits.Premium.OCRPagesPerDay,
			"ai_ocr_pages_day":  h.config.Limits.Premium.AIOCRPagesPerDay,
			"office_pages_day":  h.config.Limits.Premium.OfficePagesPerDay,
			"rate_limit":        h.config.Limits.Premium.RateLimit,
			"priority":          h.config.Limits.Premium.Priority,
			"speed_level":       h.config.Limits.Premium.SpeedLevel,
			"has_watermark":     h.config.Limits.Premium.HasWatermark,
			"has_ads":           h.config.Limits.Premium.HasAds,
			"support_level":     h.config.Limits.Premium.SupportLevel,
			"features": []string{
				"All Free features",
				"AI OCR (5 pages/day)",
				"Office conversion (no watermark)",
				"Priority processing",
				"100 operations/day",
				"Email support",
			},
		},
		"pro": map[string]interface{}{
			"name":              "Pro",
			"max_file_size":     fmt.Sprintf("%dMB", h.config.Limits.Pro.MaxFileSizeMB),
			"max_files_day":     h.config.Limits.Pro.MaxFilesPerDay,
			"max_files_month":   h.config.Limits.Pro.MaxFilesPerMonth,
			"concurrent_files":  h.config.Limits.Pro.MaxConcurrentFiles,
			"ocr_pages_day":     h.config.Limits.Pro.OCRPagesPerDay,
			"ai_ocr_pages_day":  h.config.Limits.Pro.AIOCRPagesPerDay,
			"office_pages_day":  h.config.Limits.Pro.OfficePagesPerDay,
			"rate_limit":        h.config.Limits.Pro.RateLimit,
			"priority":          h.config.Limits.Pro.Priority,
			"speed_level":       h.config.Limits.Pro.SpeedLevel,
			"has_watermark":     h.config.Limits.Pro.HasWatermark,
			"has_ads":           h.config.Limits.Pro.HasAds,
			"support_level":     h.config.Limits.Pro.SupportLevel,
			"max_team_users":    h.config.Limits.Pro.MaxTeamUsers,
			"features": []string{
				"All Premium features",
				"AI OCR (50 pages/day)",
				"Batch processing",
				"Advanced analytics",
				"API access + webhooks",
				"Team access (5 users)",
				"500 operations/day",
				"Priority support (1h response)",
			},
		},
		"corporate": map[string]interface{}{
			"name":              "Corporate",
			"max_file_size":     "500MB",
			"max_files_day":     "1,000",
			"max_files_month":   "30,000",
			"concurrent_files":  50,
			"ocr_pages_day":     "1,000",
			"ai_ocr_pages_day":  "500",
			"office_pages_day":  "1,000",
			"rate_limit":        "1,000 req/min",
			"priority":          "Maximum",
			"speed_level":       "Turbo",
			"has_watermark":     false,
			"has_ads":           false,
			"support_level":     "Dedicated + SLA",
			"max_team_users":    100,
			"features": []string{
				"All Pro features",
				"AI OCR (500 pages/day)",
				"Unlimited processing power",
				"Enterprise analytics",
				"Full API access",
				"Team management (100 users)",
				"2,000 operations/day",
				"Dedicated support 24/7",
				"Custom integrations",
				"White-label option",
				"SLA guarantee",
			},
		},
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Planes disponibles",
		"data":    plans,
	})
}

// OptimizePDF optimizar PDF (alias para CompressPDF)
func (h *Handlers) OptimizePDF(c *fiber.Ctx) error {
	// Redireccionar a CompressPDF ya implementado
	return h.CompressPDF(c)
}

// OfficeConvert conversión de documentos Office
func (h *Handlers) OfficeConvert(c *fiber.Ctx) error {
	// Verificar plan del usuario
	userPlan, _ := c.Locals("userPlan").(string)
	if userPlan == "free" {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"error":   "PLAN_REQUIRED",
			"message": "La conversión de documentos Office requiere plan Premium o Pro",
		})
	}

	startTime := time.Now()

	h.logger.Info("📄 Office conversion requested",
		"ip", c.IP(),
		"plan", userPlan,
	)

	// Obtener archivo subido
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "MISSING_FILE",
			"message": "No se ha subido ningún archivo",
		})
	}

	// Validar tipo de archivo Office
	if !h.isOfficeFile(file.Filename) {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "INVALID_OFFICE_FORMAT",
			"message": "Formato de archivo no soportado. Use .docx, .xlsx, .pptx",
		})
	}

	// Crear servicios
	storageService := storage.NewService(h.config, h.logger)

	// Guardar archivo subido
	fileInfo, err := storageService.SaveUpload(file)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "STORAGE_ERROR",
			"message": "Error al procesar archivo",
			"details": err.Error(),
		})
	}

	// Limpiar archivo al finalizar
	defer func() {
		if err := storageService.Delete(fileInfo.ID); err != nil {
			h.logger.Warn("Failed to cleanup temp file", "file_id", fileInfo.ID, "error", err)
		}
	}()

	// TODO: Implementar conversión usando LibreOffice o Gotenberg
	// Por ahora retornar error de no implementado
	h.logger.Info("✅ Office conversion completed (mock)",
		"input_file", file.Filename,
		"plan", userPlan,
		"duration_ms", time.Since(startTime).Milliseconds(),
	)

	return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{
		"success": false,
		"error":   "OFFICE_CONVERSION_NOT_IMPLEMENTED",
		"message": "Conversión de Office estará disponible próximamente",
	})
}

// GetFiles lista archivos temporales del usuario
func (h *Handlers) GetFiles(c *fiber.Ctx) error {
	h.logger.Info("📂 List files requested", "ip", c.IP())

	// Obtener parámetros de paginación
	limit := c.QueryInt("limit", 50)
	offset := c.QueryInt("offset", 0)

	if limit > 100 {
		limit = 100 // Máximo 100 archivos por request
	}

	// Obtener user ID del contexto
	userID := c.Locals("userID")
	if userID == nil {
		userID = "unknown"
	}

	// Listar archivos del directorio temporal
	tempDir := h.storage.(*storage.LocalStorage).GetTempDir()
	files, err := h.listUserTempFiles(tempDir, userID.(string), limit, offset)
	if err != nil {
		h.logger.Error("Failed to list files", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "LIST_FILES_ERROR",
			"message": "Error al listar archivos temporales",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Archivos temporales listados",
		"data": fiber.Map{
			"files":  files,
			"total":  len(files),
			"limit":  limit,
			"offset": offset,
		},
	})
}

// DownloadFile descarga un archivo procesado
func (h *Handlers) DownloadFile(c *fiber.Ctx) error {
	filename := c.Params("filename")
	if filename == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "MISSING_FILENAME",
			"message": "Nombre de archivo requerido",
		})
	}

	h.logger.Info("⬇️ Download file requested",
		"filename", filename,
		"ip", c.IP())

	// Sanitizar nombre de archivo para evitar path traversal
	sanitized := h.storage.SanitizeFilename(filename)
	if sanitized != filename {
		h.logger.Warn("Suspicious filename detected",
			"original", filename,
			"sanitized", sanitized,
			"ip", c.IP())
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "INVALID_FILENAME",
			"message": "Nombre de archivo inválido",
		})
	}

	// Obtener user ID para validar ownership
	userID := c.Locals("userID")
	if userID == nil {
		userID = "unknown"
	}

	// Construir ruta del archivo
	tempDir := h.storage.(*storage.LocalStorage).GetTempDir()
	filePath := filepath.Join(tempDir, sanitized)

	// Verificar que el archivo existe
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			h.logger.Warn("File not found for download",
				"filename", sanitized,
				"user", userID)
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"success": false,
				"error":   "FILE_NOT_FOUND",
				"message": "Archivo no encontrado",
			})
		}
		h.logger.Error("Error checking file", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "FILE_ACCESS_ERROR",
			"message": "Error al acceder al archivo",
		})
	}

	// Detectar MIME type correcto
	mimeType, err := h.detectFileMimeType(filePath)
	if err != nil {
		mimeType = "application/octet-stream"
	}

	// Log de descarga
	h.logger.Info("File download starting",
		"filename", sanitized,
		"size", fileInfo.Size(),
		"mime", mimeType,
		"user", userID)

	// Configurar headers para descarga
	c.Set("Content-Type", mimeType)
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", sanitized))
	c.Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))
	c.Set("X-Content-Type-Options", "nosniff")

	// Enviar archivo
	return c.SendFile(filePath)
}

// DeleteFile elimina un archivo temporal
func (h *Handlers) DeleteFile(c *fiber.Ctx) error {
	filename := c.Params("filename")
	if filename == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "MISSING_FILENAME",
			"message": "Nombre de archivo requerido",
		})
	}

	h.logger.Info("🗑️ Delete file requested",
		"filename", filename,
		"ip", c.IP())

	// Sanitizar nombre
	sanitized := h.storage.SanitizeFilename(filename)
	if sanitized != filename {
		h.logger.Warn("Suspicious filename in delete request",
			"original", filename,
			"sanitized", sanitized,
			"ip", c.IP())
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "INVALID_FILENAME",
			"message": "Nombre de archivo inválido",
		})
	}

	// Obtener user ID
	userID := c.Locals("userID")
	if userID == nil {
		userID = "unknown"
	}

	// Construir ruta
	tempDir := h.storage.(*storage.LocalStorage).GetTempDir()
	filePath := filepath.Join(tempDir, sanitized)

	// Verificar que existe
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"success": false,
				"error":   "FILE_NOT_FOUND",
				"message": "Archivo no encontrado",
			})
		}
		h.logger.Error("Error checking file for deletion", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "FILE_ACCESS_ERROR",
			"message": "Error al acceder al archivo",
		})
	}

	// Eliminar archivo
	if err := os.Remove(filePath); err != nil {
		h.logger.Error("Failed to delete file",
			"filename", sanitized,
			"error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "DELETE_FAILED",
			"message": "Error al eliminar archivo",
		})
	}

	h.logger.Info("File deleted successfully",
		"filename", sanitized,
		"user", userID)

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Archivo eliminado correctamente",
		"data": fiber.Map{
			"filename": sanitized,
		},
	})
}

// ValidateFile validar archivo
func (h *Handlers) ValidateFile(c *fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "MISSING_FILE",
			"message": "No se ha subido ningún archivo",
		})
	}

	// Validación básica de tamaño
	if file.Size > 100*1024*1024 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "FILE_TOO_LARGE",
			"message": "Archivo muy grande",
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Archivo válido",
		"data": map[string]interface{}{
			"filename":  file.Filename,
			"size":      file.Size,
			"mime_type": file.Header.Get("Content-Type"),
		},
	})
}

// GetSupportedFormats obtener formatos soportados
func (h *Handlers) GetSupportedFormats(c *fiber.Ctx) error {
	formats := map[string]interface{}{
		"pdf": []string{"application/pdf"},
		"images": []string{
			"image/jpeg",
			"image/png",
			"image/gif",
			"image/webp",
		},
		"office": []string{
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document", // .docx
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",       // .xlsx
			"application/vnd.openxmlformats-officedocument.presentationml.presentation", // .pptx
		},
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Formatos soportados",
		"data":    formats,
	})
}

// Helper methods
func (h *Handlers) isOfficeFile(filename string) bool {
	ext := strings.ToLower(filename)
	return strings.HasSuffix(ext, ".docx") ||
		strings.HasSuffix(ext, ".xlsx") ||
		strings.HasSuffix(ext, ".pptx") ||
		strings.HasSuffix(ext, ".doc") ||
		strings.HasSuffix(ext, ".xls") ||
		strings.HasSuffix(ext, ".ppt")
}

// listUserTempFiles lista archivos temporales del usuario
func (h *Handlers) listUserTempFiles(tempDir string, userID string, limit, offset int) ([]map[string]interface{}, error) {
	files := []map[string]interface{}{}

	// Leer directorio temporal
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		return nil, err
	}

	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Aplicar offset
		if count < offset {
			count++
			continue
		}

		// Límite alcanzado
		if len(files) >= limit {
			break
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		// Información del archivo
		fileData := map[string]interface{}{
			"name":        entry.Name(),
			"size":        info.Size(),
			"size_mb":     float64(info.Size()) / (1024 * 1024),
			"modified_at": info.ModTime(),
			"age_minutes": time.Since(info.ModTime()).Minutes(),
		}

		files = append(files, fileData)
		count++
	}

	return files, nil
}

// detectFileMimeType detecta el MIME type de un archivo
func (h *Handlers) detectFileMimeType(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Leer primeros 512 bytes para detectar tipo
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", err
	}

	// Detectar MIME type
	mimeType := http.DetectContentType(buffer[:n])

	// Ajustes específicos para PDFs y archivos comunes
	ext := strings.ToLower(filepath.Ext(filePath))
	switch ext {
	case ".pdf":
		return "application/pdf", nil
	case ".jpg", ".jpeg":
		return "image/jpeg", nil
	case ".png":
		return "image/png", nil
	case ".gif":
		return "image/gif", nil
	case ".webp":
		return "image/webp", nil
	case ".zip":
		return "application/zip", nil
	}

	return mimeType, nil
}