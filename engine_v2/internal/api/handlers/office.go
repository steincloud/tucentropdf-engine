package handlers

import (
	"os"
	"path/filepath"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/tucentropdf/engine-v2/internal/office"
	"github.com/tucentropdf/engine-v2/internal/storage"
)

// ConvertOffice handler para convertir archivos Office a PDF
// @Summary Convertir Office a PDF
// @Description Convierte archivos DOC/DOCX/XLS/XLSX/PPT/PPTX a PDF
// @Tags office
// @Accept multipart/form-data
// @Produce json
// @Security ApiKeyAuth
// @Param file formData file true "Archivo Office (DOC, DOCX, XLS, XLSX, PPT, PPTX)"
// @Param quality formData string false "Calidad de conversión (draft, normal, high)"
// @Success 200 {object} APIResponse
// @Failure 400 {object} APIResponse "Formato no soportado"
// @Failure 415 {object} APIResponse "Tipo de archivo inválido"
// @Router /api/v1/office/convert [post]
func (h *Handlers) ConvertOffice(c *fiber.Ctx) error {
	startTime := time.Now()
	h.logger.Info("📄 Office conversion requested", 
		"ip", c.IP(),
		"user_agent", c.Get("User-Agent"),
	)

	// Verificar si Office está habilitado
	if !h.config.Office.Enabled {
		return h.ErrorResponse(c, 
			"OFFICE_DISABLED", 
			"Office conversion is currently disabled", 
			"Enable OFFICE_ENABLED in configuration", 
			503,
		)
	}

	// Obtener archivo subido
	file, err := c.FormFile("file")
	if err != nil {
		return h.ErrorResponse(c, "MISSING_FILE", "No file uploaded", err.Error(), 400)
	}

	// Crear servicios
	storageService := storage.NewService(h.config, h.logger)
	officeService := office.NewService(h.config, h.logger)

	// Verificar disponibilidad
	if !officeService.IsAvailable() {
		return h.ErrorResponse(c, 
			"OFFICE_UNAVAILABLE", 
			"Office conversion service is not available", 
			"LibreOffice not found or not accessible", 
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

	// Validar archivo Office
	if err := office.ValidateOfficeFile(fileInfo.Path); err != nil {
		return h.ErrorResponse(c, "INVALID_FILE", "Invalid Office file", err.Error(), 400)
	}

	// Crear archivo de salida
	outputPath := filepath.Join(filepath.Dir(fileInfo.Path), fileInfo.ID+"_converted.pdf")
	defer os.Remove(outputPath) // Limpiar salida

	// Convertir a PDF
	conversionStart := time.Now()
	if err := officeService.ConvertToPDF(fileInfo.Path, outputPath); err != nil {
		return h.ErrorResponse(c, "CONVERSION_FAILED", "Office to PDF conversion failed", err.Error(), 500)
	}
	conversionDuration := time.Since(conversionStart)

	// Leer PDF convertido
	pdfData, err := os.ReadFile(outputPath)
	if err != nil {
		return h.ErrorResponse(c, "READ_ERROR", "Failed to read converted PDF", err.Error(), 500)
	}

	// Obtener info del archivo convertido
	stat, _ := os.Stat(outputPath)
	outputSize := int64(0)
	if stat != nil {
		outputSize = stat.Size()
	}

	totalDuration := time.Since(startTime)

	h.logger.Info("✅ Office conversion completed",
		"input_file", file.Filename,
		"input_size", fileInfo.Size,
		"output_size", outputSize,
		"conversion_duration_ms", conversionDuration.Milliseconds(),
		"total_duration_ms", totalDuration.Milliseconds(),
		"provider", h.config.Office.Provider,
	)

	// Configurar headers para descarga
	outputFilename := getBaseFilename(file.Filename) + ".pdf"
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", `attachment; filename="`+outputFilename+`"`)
	c.Set("Content-Length", string(rune(len(pdfData))))

	// Enviar archivo PDF
	return c.Send(pdfData)
}

// Helper function para obtener nombre base sin extensión
func getBaseFilename(filename string) string {
	ext := filepath.Ext(filename)
	return filename[:len(filename)-len(ext)]
}