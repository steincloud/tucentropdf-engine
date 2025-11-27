package handlers

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/tucentropdf/engine-v2/internal/ocr"
	"github.com/tucentropdf/engine-v2/internal/pdf"
	"github.com/tucentropdf/engine-v2/internal/storage"
)

// ClassicOCR handler para OCR clásico
// @Summary OCR clásico
// @Description Extrae texto de imágenes o PDFs usando Tesseract/PaddleOCR
// @Tags ocr
// @Accept multipart/form-data
// @Produce json
// @Security ApiKeyAuth
// @Param file formData file true "Imagen o PDF"
// @Param language formData string false "Idioma OCR (eng, spa, por, fra)"
// @Param engine formData string false "Motor OCR (tesseract, paddle)"
// @Param preprocess formData bool false "Aplicar preprocesamiento"
// @Success 200 {object} APIResponse
// @Failure 415 {object} APIResponse "Formato no soportado"
// @Router /api/v1/ocr/classic [post]
func (h *Handlers) ClassicOCR(c *fiber.Ctx) error {
	startTime := time.Now()
	language := c.FormValue("language", "eng")
	engine := c.FormValue("engine", h.config.OCR.Provider)
	preprocess, _ := strconv.ParseBool(c.FormValue("preprocess", "true"))

	h.logger.Info("👁️ Classic OCR requested", 
		"ip", c.IP(),
		"language", language,
		"engine", engine,
		"preprocess", preprocess,
	)

	// Obtener archivo subido
	file, err := c.FormFile("file")
	if err != nil {
		return h.ErrorResponse(c, "MISSING_FILE", "No file uploaded", err.Error(), 400)
	}

	// Crear servicios
	storageService := storage.NewService(h.config, h.logger)
	ocrService := ocr.NewClassicService(h.config, h.logger)

	// Verificar disponibilidad
	if !ocrService.IsAvailable() {
		return h.ErrorResponse(c, 
			"OCR_UNAVAILABLE", 
			"OCR service is not available", 
			"Tesseract not found or not accessible", 
			503,
		)
	}

	// Guardar archivo subido
	fileInfo, err := storageService.SaveUpload(file)
	if err != nil {
		return h.ErrorResponse(c, "STORAGE_ERROR", "Failed to save uploaded file", err.Error(), 500)
	}

	// Limpiar archivo al finalizar
	defer func() {
		if err := storageService.Delete(fileInfo.ID); err != nil {
			h.logger.Warn("Failed to cleanup temp file", "file_id", fileInfo.ID, "error", err)
		}
	}()

	// TODO: Si es PDF, convertir a imagen primero
	processPath := fileInfo.Path
	if filepath.Ext(fileInfo.Path) == ".pdf" {
		// Por ahora rechazar PDFs hasta implementar conversión
		return h.ErrorResponse(c, "PDF_NOT_SUPPORTED", "PDF to image conversion not yet implemented", "Use image files for now", 400)
	}

	// Ejecutar OCR
	ocrStart := time.Now()
	result, err := ocrService.ExtractTextWithLang(processPath, language)
	if err != nil {
		return h.ErrorResponse(c, "OCR_FAILED", "Text extraction failed", err.Error(), 500)
	}
	ocrDuration := time.Since(ocrStart)
	totalDuration := time.Since(startTime)

	h.logger.Info("✅ Classic OCR completed",
		"input_file", file.Filename,
		"language", language,
		"engine", result.Engine,
		"text_length", len(result.Text),
		"confidence", result.Confidence,
		"ocr_duration_ms", ocrDuration.Milliseconds(),
		"total_duration_ms", totalDuration.Milliseconds(),
	)

	// Preparar respuesta
	response := map[string]interface{}{
		"text":             result.Text,
		"confidence":       result.Confidence,
		"language":         result.Language,
		"engine":           result.Engine,
		"duration_ms":      result.Duration.Milliseconds(),
		"total_duration_ms": totalDuration.Milliseconds(),
		"metadata":         result.Metadata,
		"file_info": map[string]interface{}{
			"name":      file.Filename,
			"size":      fileInfo.Size,
			"mime_type": fileInfo.MimeType,
		},
	}

	return h.SuccessResponse(c, response)
}



// AIOCR handler para OCR con IA
// @Summary OCR con IA
// @Description Extrae texto de imágenes o PDFs usando OpenAI Vision (Premium/Pro)
// @Tags ocr
// @Accept multipart/form-data
// @Produce json
// @Security ApiKeyAuth
// @Param file formData file true "Imagen o PDF"
// @Param pages formData string false "Páginas específicas (ej: 1-3,5)"
// @Param extract_tables formData bool false "Extraer tablas estructuradas"
// @Param extract_forms formData bool false "Extraer formularios"
// @Param output_format formData string false "Formato de salida (text, json, markdown)"
// @Success 200 {object} APIResponse
// @Failure 403 {object} APIResponse "Plan no tiene acceso a IA"
// @Failure 415 {object} APIResponse "Formato no soportado"
// @Router /api/v2/ocr/ai [post]
func (h *Handlers) AIOCR(c *fiber.Ctx) error {
	startTime := time.Now()
	
	// Obtener información del plan del usuario
	userPlan, ok := c.Locals("userPlan").(string)
	if !ok {
		userPlan = "free"
	}
	
	// Parámetros de la petición
	pagesParam := c.FormValue("pages", "")
	extractTables, _ := strconv.ParseBool(c.FormValue("extract_tables", "false"))
	extractForms, _ := strconv.ParseBool(c.FormValue("extract_forms", "false"))
	outputFormat := c.FormValue("output_format", "text")
	
	h.logger.Info("🤖 AI OCR requested",
		"ip", c.IP(),
		"plan", userPlan,
		"pages", pagesParam,
		"extract_tables", extractTables,
		"extract_forms", extractForms,
		"output_format", outputFormat,
	)
	
	// Obtener archivo subido
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "MISSING_FILE", "message": "No se ha subido ningún archivo"})
	}
	
	// Crear servicios
	storageService := storage.NewService(h.config, h.logger)
	aiService := ocr.NewAIService(h.config, h.logger)
	pdfUtils := pdf.NewPDFUtils(h.logger)
	
	// Verificar disponibilidad del servicio AI
	if !aiService.IsAvailable() {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{"success": false, "error": "AI_OCR_UNAVAILABLE", "message": "Servicio de OCR con IA no disponible. Verifica la configuración de OpenAI"})
	}
	
	// Guardar archivo subido
	fileInfo, err := storageService.SaveUpload(file)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "STORAGE_ERROR", "message": "Error guardando archivo", "details": err.Error()})
	}
	
	// Limpiar archivo al finalizar
	defer func() {
		if err := storageService.Delete(fileInfo.ID); err != nil {
			h.logger.Warn("Failed to cleanup temp file", "file_id", fileInfo.ID, "error", err)
		}
	}()
	
	// Validar tipo de archivo y contar páginas
	var pageCount int
	if strings.ToLower(filepath.Ext(fileInfo.Path)) == ".pdf" {
		// Contar páginas del PDF
		pageCount, err = pdfUtils.CountPages(fileInfo.Path)
		if err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "PDF_READ_ERROR", "message": "Error leyendo PDF", "details": err.Error()})
		}
	} else {
		// Para imágenes, considerar 1 página
		pageCount = 1
	}
	
	// Validar límites del plan para AI OCR
	openaiService, ok := aiService.(*ocr.OpenAIService)
	if ok {
		if err := openaiService.ValidatePlanAILimits(userPlan, pageCount); err != nil {
			h.logger.Warn("AI OCR plan limit exceeded",
				"plan", userPlan,
				"pages", pageCount,
				"error", err.Error(),
			)
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{"success": false, "error": "PLAN_LIMIT_EXCEEDED", "message": err.Error()})
		}
	}
	
	// Estimar costo
	estimatedCost := float64(pageCount) * 0.01 // $0.01 por página base
	if extractTables || extractForms {
		estimatedCost *= 1.5 // Aumentar costo para extracción estructurada
	}
	
	// Preparar opciones de extracción
	options := &ocr.ExtractOptions{
		ExtractTables: extractTables,
		ExtractForms:  extractForms,
		Format:        outputFormat,
		Language:      "auto",
	}
	
	// Ejecutar AI OCR
	ocrStart := time.Now()
	var result *ocr.AIResult
	
	if strings.ToLower(filepath.Ext(fileInfo.Path)) == ".pdf" {
		// TODO: Para PDFs, convertir páginas a imágenes y procesar
		// Por ahora, usar la primera página como ejemplo
		return c.Status(fiber.StatusNotImplemented).JSON(fiber.Map{"success": false, "error": "PDF_AI_OCR_NOT_IMPLEMENTED", "message": "OCR con IA para PDFs estará disponible próximamente. Use imágenes por ahora"})
	} else {
		// Procesar imagen directamente
		result, err = aiService.ExtractStructuredData(fileInfo.Path, options)
		if err != nil {
			// Intentar fallback a extracción básica
			h.logger.Warn("Structured AI OCR failed, trying basic extraction", "error", err.Error())
			result, err = aiService.ExtractText(fileInfo.Path)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "AI_OCR_FAILED", "message": "Error en OCR con IA", "details": err.Error()})
			}
		}
	}
	
	ocrDuration := time.Since(ocrStart)
	totalDuration := time.Since(startTime)
	
	// Log del costo estimado para auditoría
	h.logger.Info("✅ AI OCR completed",
		"input_file", file.Filename,
		"plan", userPlan,
		"pages", pageCount,
		"engine", result.Engine,
		"model", result.Model,
		"text_length", len(result.Text),
		"confidence", result.Confidence,
		"tokens_used", result.TokensUsed,
		"cost_usd", fmt.Sprintf("%.4f", result.Cost),
		"estimated_cost_usd", fmt.Sprintf("%.4f", estimatedCost),
		"ocr_duration_ms", ocrDuration.Milliseconds(),
		"total_duration_ms", totalDuration.Milliseconds(),
	)
	
	// Preparar respuesta
	responseData := map[string]interface{}{
		"text":            result.Text,
		"structured_data": result.StructuredData,
		"language":        result.Language,
		"confidence":      result.Confidence,
		"engine":          result.Engine,
		"model":           result.Model,
		"pages_processed": pageCount,
		"tokens_used":     result.TokensUsed,
		"processing_time": map[string]interface{}{
			"ocr_ms":   ocrDuration.Milliseconds(),
			"total_ms": totalDuration.Milliseconds(),
		},
		"cost_info": map[string]interface{}{
			"estimated_usd": estimatedCost,
			"actual_usd":    result.Cost,
		},
		"metadata": result.Metadata,
	}

	return c.JSON(fiber.Map{"success": true, "message": "OCR con IA completado exitosamente", "data": responseData})
}