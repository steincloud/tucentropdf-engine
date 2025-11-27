package handlers

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pdfcpu/pdfcpu/pkg/api"

	"github.com/tucentropdf/engine-v2/internal/pdf"
	"github.com/tucentropdf/engine-v2/internal/storage"
)

// MergePDF handler para fusionar PDFs
// @Summary Fusionar PDFs
// @Description Combina múltiples archivos PDF en uno solo
// @Tags pdf
// @Accept multipart/form-data
// @Produce application/pdf
// @Security ApiKeyAuth
// @Param files formData file true "Archivos PDF a fusionar" multiple
// @Success 200 {file} application/pdf "PDF fusionado"
// @Failure 400 {object} APIResponse "Error en los archivos"
// @Router /api/v1/pdf/merge [post]
func (h *Handlers) MergePDF(c *fiber.Ctx) error {
	startTime := time.Now()
	h.logger.Info("🗋 PDF merge requested", "ip", c.IP())

	// Obtener archivos subidos
	form, err := c.MultipartForm()
	if err != nil {
		return h.ErrorResponse(c, "FORM_ERROR", "Failed to parse multipart form", err.Error(), 400)
	}

	files := form.File["files"]
	if len(files) < 2 {
		return h.ErrorResponse(c, "INSUFFICIENT_FILES", "At least 2 PDF files are required", "", 400)
	}

	// Crear servicios
	storageService := storage.NewService(h.config, h.logger)
	pdfService := pdf.NewService(h.config, h.logger)

	// Verificar disponibilidad
	if !pdfService.IsAvailable() {
		return h.ErrorResponse(c, "PDF_UNAVAILABLE", "PDF processing service is not available", "", 503)
	}

	// Guardar archivos y validar formato
	var inputPaths []string
	var fileIDs []string
	
	for _, file := range files {
		// Validar extensión
		if !strings.HasSuffix(strings.ToLower(file.Filename), ".pdf") {
			return h.ErrorResponse(c, "INVALID_FORMAT", "Only PDF files are allowed", 
				"File: "+file.Filename, 400)
		}

		// Guardar archivo
		fileInfo, err := storageService.SaveUpload(file)
		if err != nil {
			return h.ErrorResponse(c, "STORAGE_ERROR", "Failed to save uploaded file", err.Error(), 500)
		}

		inputPaths = append(inputPaths, fileInfo.Path)
		fileIDs = append(fileIDs, fileInfo.ID)
	}

	// Limpiar archivos al finalizar
	defer func() {
		for _, fileID := range fileIDs {
			if err := storageService.Delete(fileID); err != nil {
				h.logger.Warn("Failed to cleanup temp file", "file_id", fileID, "error", err)
			}
		}
	}()

	// Crear archivo de salida
	outputPath := storageService.GenerateOutputPath("merged.pdf")

	// Ejecutar merge
	mergeStart := time.Now()
	result, err := pdfService.Merge(inputPaths, outputPath)
	if err != nil {
		return h.ErrorResponse(c, "MERGE_FAILED", "PDF merge operation failed", err.Error(), 500)
	}
	mergeDuration := time.Since(mergeStart)
	totalDuration := time.Since(startTime)

	// Limpiar archivo de salida al finalizar
	defer func() {
		if err := storageService.DeletePath(outputPath); err != nil {
			h.logger.Warn("Failed to cleanup output file", "path", outputPath, "error", err)
		}
	}()

	h.logger.Info("✅ PDF merge completed",
		"input_files", len(inputPaths),
		"output_pages", result.Pages,
		"output_size", result.Size,
		"merge_duration_ms", mergeDuration.Milliseconds(),
		"total_duration_ms", totalDuration.Milliseconds(),
	)

	// Leer archivo resultado
	fileData, err := storageService.ReadFile(outputPath)
	if err != nil {
		return h.ErrorResponse(c, "READ_ERROR", "Failed to read merged PDF", err.Error(), 500)
	}

	// Configurar headers para descarga
	filename := "merged.pdf"
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Set("Content-Length", strconv.Itoa(len(fileData)))

	return c.Send(fileData)
}

// SplitPDF handler para dividir PDF
// @Summary Dividir PDF
// @Description Divide un PDF en páginas individuales o rangos
// @Tags pdf
// @Accept multipart/form-data
// @Produce application/json
// @Security ApiKeyAuth
// @Param file formData file true "Archivo PDF"
// @Param mode formData string false "Modo: pages (individual) o range" 
// @Param range formData string false "Rango de páginas (ej: 1-3,5,7-9)"
// @Success 200 {object} APIResponse
// @Router /api/v1/pdf/split [post]
func (h *Handlers) SplitPDF(c *fiber.Ctx) error {
	startTime := time.Now()
	mode := c.FormValue("mode", "pages")
	rangeStr := c.FormValue("range", "")
	
	h.logger.Info("✂️ PDF split requested", 
		"ip", c.IP(),
		"mode", mode,
		"range", rangeStr,
	)

	// Obtener archivo subido
	file, err := c.FormFile("file")
	if err != nil {
		return h.ErrorResponse(c, "MISSING_FILE", "No file uploaded", err.Error(), 400)
	}

	// Validar formato
	if !strings.HasSuffix(strings.ToLower(file.Filename), ".pdf") {
		return h.ErrorResponse(c, "INVALID_FORMAT", "Only PDF files are allowed", "", 400)
	}

	// Crear servicios
	storageService := storage.NewService(h.config, h.logger)
	pdfService := pdf.NewService(h.config, h.logger)

	// Verificar disponibilidad
	if !pdfService.IsAvailable() {
		return h.ErrorResponse(c, "PDF_UNAVAILABLE", "PDF processing service is not available", "", 503)
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

	// Ejecutar split
	splitStart := time.Now()
	result, err := pdfService.Split(fileInfo.Path, mode, rangeStr)
	if err != nil {
		return h.ErrorResponse(c, "SPLIT_FAILED", "PDF split operation failed", err.Error(), 500)
	}
	splitDuration := time.Since(splitStart)
	totalDuration := time.Since(startTime)

	// TODO: Por ahora devolver info, en el futuro crear ZIP con archivos
	// Para simplificar, limpiar archivos de salida inmediatamente
	defer func() {
		for _, outputPath := range result.OutputFiles {
			if err := storageService.DeletePath(outputPath); err != nil {
				h.logger.Warn("Failed to cleanup split file", "path", outputPath, "error", err)
			}
		}
	}()

	h.logger.Info("✅ PDF split completed",
		"input_file", file.Filename,
		"input_pages", result.Pages,
		"output_files", len(result.OutputFiles),
		"mode", mode,
		"split_duration_ms", splitDuration.Milliseconds(),
		"total_duration_ms", totalDuration.Milliseconds(),
	)

	// Preparar respuesta
	response := map[string]interface{}{
		"mode":               mode,
		"input_pages":        result.Pages,
		"output_files":       len(result.OutputFiles),
		"duration_ms":        result.Duration.Milliseconds(),
		"total_duration_ms":  totalDuration.Milliseconds(),
		"file_info": map[string]interface{}{
			"name":      file.Filename,
			"size":      fileInfo.Size,
			"mime_type": fileInfo.MimeType,
		},
		"message": "PDF split completed - files available temporarily",
		"note":    "File download/ZIP creation will be implemented in future update",
	}

	return h.SuccessResponse(c, response)
}

// CompressPDF handler para comprimir PDF
// @Summary Comprimir archivo PDF
// @Description Reduce el tamaño de un archivo PDF
// @Tags pdf
// @Accept multipart/form-data
// @Produce application/pdf
// @Security ApiKeyAuth
// @Param file formData file true "Archivo PDF a comprimir"
// @Param quality formData string false "Calidad de compresión (low, medium, high)"
// @Success 200 {file} application/pdf "PDF comprimido"
// @Router /api/v1/pdf/compress [post]
func (h *Handlers) CompressPDF(c *fiber.Ctx) error {
	startTime := time.Now()
	level := c.FormValue("quality", "medium")
	
	h.logger.Info("⚡ PDF compress requested", 
		"ip", c.IP(),
		"level", level,
	)

	// Obtener archivo subido
	file, err := c.FormFile("file")
	if err != nil {
		return h.ErrorResponse(c, "MISSING_FILE", "No file uploaded", err.Error(), 400)
	}

	// Validar formato
	if !strings.HasSuffix(strings.ToLower(file.Filename), ".pdf") {
		return h.ErrorResponse(c, "INVALID_FORMAT", "Only PDF files are allowed", "", 400)
	}

	// Crear servicios
	storageService := storage.NewService(h.config, h.logger)
	pdfService := pdf.NewService(h.config, h.logger)

	// Verificar disponibilidad
	if !pdfService.IsAvailable() {
		return h.ErrorResponse(c, "PDF_UNAVAILABLE", "PDF processing service is not available", "", 503)
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

	// Crear archivo de salida
	outputPath := storageService.GenerateOutputPath("compressed.pdf")

	// Ejecutar compresión (usar optimización)
	compressStart := time.Now()
	result, err := pdfService.Optimize(fileInfo.Path, outputPath, level)
	if err != nil {
		return h.ErrorResponse(c, "COMPRESS_FAILED", "PDF compression failed", err.Error(), 500)
	}
	compressDuration := time.Since(compressStart)
	totalDuration := time.Since(startTime)

	// Limpiar archivo de salida al finalizar
	defer func() {
		if err := storageService.DeletePath(outputPath); err != nil {
			h.logger.Warn("Failed to cleanup output file", "path", outputPath, "error", err)
		}
	}()

	// Calcular porcentaje de reducción
	reduction := 0.0
	if fileInfo.Size > 0 && result.Size > 0 {
		reduction = (1.0 - float64(result.Size)/float64(fileInfo.Size)) * 100
	}

	h.logger.Info("✅ PDF compress completed",
		"input_file", file.Filename,
		"level", level,
		"original_size", fileInfo.Size,
		"compressed_size", result.Size,
		"reduction_percent", reduction,
		"compress_duration_ms", compressDuration.Milliseconds(),
		"total_duration_ms", totalDuration.Milliseconds(),
	)

	// Leer archivo resultado
	fileData, err := storageService.ReadFile(outputPath)
	if err != nil {
		return h.ErrorResponse(c, "READ_ERROR", "Failed to read compressed PDF", err.Error(), 500)
	}

	// Configurar headers para descarga
	filename := "compressed_" + filepath.Base(file.Filename)
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", `attachment; filename="`+filename+`"`)
	c.Set("Content-Length", strconv.Itoa(len(fileData)))
	c.Set("X-Original-Size", strconv.FormatInt(fileInfo.Size, 10))
	c.Set("X-Compressed-Size", strconv.FormatInt(result.Size, 10))
	c.Set("X-Size-Reduction", strconv.FormatFloat(reduction, 'f', 2, 64)+"%")

	return c.Send(fileData)
}

// RotatePDF handler para rotar PDFs
// @Summary Rotar páginas PDF
// @Description Rota páginas de un archivo PDF
// @Tags pdf
// @Accept multipart/form-data
// @Produce json
// @Security ApiKeyAuth
// @Param file formData file true "Archivo PDF a rotar"
// @Param angle formData int true "Angulo de rotación (90, 180, 270)"
// @Param pages formData string false "Páginas a rotar (ej: 1,3,5-7)"
// @Success 200 {object} APIResponse
// @Router /api/v1/pdf/rotate [post]
func (h *Handlers) RotatePDF(c *fiber.Ctx) error {
	startTime := time.Now()
	h.logger.Info("🔄 Rotate PDF requested", "ip", c.IP())

	// Obtener archivo
	file, err := c.FormFile("file")
	if err != nil {
		return h.ErrorResponse(c, "MISSING_FILE", "No file uploaded", err.Error(), 400)
	}

	// Obtener ángulo de rotación
	angleStr := c.FormValue("angle")
	if angleStr == "" {
		angleStr = "90"
	}
	angle, err := strconv.Atoi(angleStr)
	if err != nil {
		return h.ErrorResponse(c, "INVALID_ANGLE", "Invalid angle parameter", err.Error(), 400)
	}

	// Obtener páginas (opcional)
	pages := c.FormValue("pages")
	if pages == "" {
		pages = "all"
	}

	// Guardar archivo temporal
	tempStorage := storage.NewService(h.config, h.logger)
	fileInfo, err := tempStorage.SaveUpload(file)
	if err != nil {
		return h.ErrorResponse(c, "UPLOAD_ERROR", "Failed to save file", err.Error(), 500)
	}
	defer tempStorage.Delete(fileInfo.ID)

	// Preparar salida
	outputPath := tempStorage.GenerateOutputPath("rotated_" + file.Filename)

	// Ejecutar rotación
	pdfService := pdf.NewService(h.config, h.logger)
	result, err := pdfService.Rotate(fileInfo.Path, outputPath, angle, pages)
	if err != nil {
		return h.ErrorResponse(c, "ROTATION_FAILED", "PDF rotation failed", err.Error(), 500)
	}

	h.logger.Info("✅ PDF rotated successfully",
		"angle", angle,
		"pages", pages,
		"size", result.Size,
		"duration", time.Since(startTime),
	)

	// Leer archivo antes de eliminar
	h.logger.Info("📝 Reading file", "path", outputPath)
	fileData, err := os.ReadFile(outputPath)
	if err != nil {
		return h.ErrorResponse(c, "READ_ERROR", "Failed to read rotated PDF", err.Error(), 500)
	}

	// Cleanup inmediato
	h.logger.Info("🗑️ Deleting file by path", "path", outputPath)
	if err := tempStorage.DeletePath(outputPath); err != nil {
		h.logger.Warn("Failed to cleanup rotated PDF", "path", outputPath, "error", err)
	}

	// Enviar archivo desde memoria
	c.Set("Content-Type", "application/pdf")
	c.Set("Content-Disposition", "attachment; filename=\""+filepath.Base(outputPath)+"\"")
	return c.Send(fileData)
}

// UnlockPDF handler para desbloquear PDFs
// @Summary Desbloquear archivo PDF
// @Description Remueve la protección de un archivo PDF
// @Tags pdf
// @Accept multipart/form-data
// @Produce json
// @Security ApiKeyAuth
// @Param file formData file true "Archivo PDF protegido"
// @Param password formData string true "Contraseña del PDF"
// @Success 200 {object} APIResponse
// @Router /api/v1/pdf/unlock [post]
func (h *Handlers) UnlockPDF(c *fiber.Ctx) error {
	startTime := time.Now()
	h.logger.Info("🔓 Unlock PDF requested", "ip", c.IP())

	// Obtener archivo
	file, err := c.FormFile("file")
	if err != nil {
		return h.ErrorResponse(c, "MISSING_FILE", "No file uploaded", err.Error(), 400)
	}

	// Obtener contraseña
	password := c.FormValue("password")
	if password == "" {
		return h.ErrorResponse(c, "MISSING_PASSWORD", "Password is required", "", 400)
	}

	// Guardar archivo temporal
	tempStorage := storage.NewService(h.config, h.logger)
	fileInfo, err := tempStorage.SaveUpload(file)
	if err != nil {
		return h.ErrorResponse(c, "UPLOAD_ERROR", "Failed to save file", err.Error(), 500)
	}
	defer tempStorage.Delete(fileInfo.ID)

	// Preparar salida
	outputPath := tempStorage.GenerateOutputPath("unlocked_" + file.Filename)

	// Ejecutar desbloqueo
	pdfService := pdf.NewService(h.config, h.logger)
	result, err := pdfService.Unlock(fileInfo.Path, outputPath, password)
	if err != nil {
		return h.ErrorResponse(c, "UNLOCK_FAILED", "PDF unlock failed (wrong password?)", err.Error(), 400)
	}

	h.logger.Info("✅ PDF unlocked successfully",
		"size", result.Size,
		"duration", time.Since(startTime),
	)

	// Enviar archivo
	return c.Download(outputPath, filepath.Base(outputPath))
}

// LockPDF handler para proteger PDFs
// @Summary Proteger archivo PDF
// @Description Añade protección con contraseña a un PDF
// @Tags pdf
// @Accept multipart/form-data
// @Produce json
// @Security ApiKeyAuth
// @Param file formData file true "Archivo PDF"
// @Param password formData string true "Contraseña para proteger"
// @Param permissions formData string false "Permisos (print, copy, modify)"
// @Success 200 {object} APIResponse
// @Router /api/v1/pdf/lock [post]
func (h *Handlers) LockPDF(c *fiber.Ctx) error {
	startTime := time.Now()
	h.logger.Info("🔒 Lock PDF requested", "ip", c.IP())

	// Obtener archivo
	file, err := c.FormFile("file")
	if err != nil {
		return h.ErrorResponse(c, "MISSING_FILE", "No file uploaded", err.Error(), 400)
	}

	// Obtener contraseña
	password := c.FormValue("password")
	if password == "" {
		return h.ErrorResponse(c, "MISSING_PASSWORD", "Password is required", "", 400)
	}

	// Obtener permisos (opcional)
	permissionsStr := c.FormValue("permissions")
	var permissions []string
	if permissionsStr != "" {
		permissions = strings.Split(permissionsStr, ",")
	}

	// Guardar archivo temporal
	tempStorage := storage.NewService(h.config, h.logger)
	fileInfo, err := tempStorage.SaveUpload(file)
	if err != nil {
		return h.ErrorResponse(c, "UPLOAD_ERROR", "Failed to save file", err.Error(), 500)
	}
	defer tempStorage.Delete(fileInfo.ID)

	// Preparar salida
	outputPath := tempStorage.GenerateOutputPath("locked_" + file.Filename)

	// Ejecutar protección
	pdfService := pdf.NewService(h.config, h.logger)
	result, err := pdfService.Lock(fileInfo.Path, outputPath, password, permissions)
	if err != nil {
		return h.ErrorResponse(c, "LOCK_FAILED", "PDF lock failed", err.Error(), 500)
	}

	h.logger.Info("✅ PDF locked successfully",
		"permissions", permissions,
		"size", result.Size,
		"duration", time.Since(startTime),
	)

	// Enviar archivo
	return c.Download(outputPath, filepath.Base(outputPath))
}

// PDFToJPG handler para convertir PDF a imágenes
// @Summary Convertir PDF a JPG
// @Description Convierte páginas PDF a imágenes JPG
// @Tags pdf
// @Accept multipart/form-data
// @Produce json
// @Security ApiKeyAuth
// @Param file formData file true "Archivo PDF"
// @Param dpi formData int false "DPI de la imagen (default: 150)"
// @Param pages formData string false "Páginas a convertir"
// @Success 200 {object} APIResponse
// @Router /api/v1/pdf/pdf-to-jpg [post]
func (h *Handlers) PDFToJPG(c *fiber.Ctx) error {
	startTime := time.Now()
	h.logger.Info("🖼️ PDF to JPG requested", "ip", c.IP())

	// Obtener archivo
	file, err := c.FormFile("file")
	if err != nil {
		return h.ErrorResponse(c, "MISSING_FILE", "No file uploaded", err.Error(), 400)
	}

	// Obtener páginas (opcional)
	pages := c.FormValue("pages")
	if pages == "" {
		pages = "all"
	}

	// Guardar archivo temporal
	tempStorage := storage.NewService(h.config, h.logger)
	fileInfo, err := tempStorage.SaveUpload(file)
	if err != nil {
		return h.ErrorResponse(c, "UPLOAD_ERROR", "Failed to save file", err.Error(), 500)
	}
	defer tempStorage.Delete(fileInfo.ID)

	// Crear directorio temporal para imágenes
	outputDir := tempStorage.GenerateOutputPath("images_" + fileInfo.ID)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return h.ErrorResponse(c, "OUTPUT_DIR_ERROR", "Failed to create output directory", err.Error(), 500)
	}

	// Ejecutar extracción de imágenes
	pdfService := pdf.NewService(h.config, h.logger)
	err = pdfService.ExtractImages(fileInfo.Path, outputDir, pages)
	if err != nil {
		return h.ErrorResponse(c, "CONVERSION_FAILED", "PDF to JPG conversion failed", err.Error(), 500)
	}

	h.logger.Info("✅ PDF to JPG completed",
		"pages", pages,
		"output_dir", outputDir,
		"duration", time.Since(startTime),
	)

	// Retornar información (en producción, comprimir a ZIP)
	return h.SuccessResponse(c, map[string]interface{}{
		"message":    "Images extracted successfully",
		"output_dir": outputDir,
		"pages":      pages,
	})
}

// JPGToPDF handler para convertir imágenes a PDF
// @Summary Convertir JPG a PDF
// @Description Convierte imágenes JPG a un archivo PDF
// @Tags pdf
// @Accept multipart/form-data
// @Produce json
// @Security ApiKeyAuth
// @Param files formData file true "Imágenes JPG"
// @Param page_size formData string false "Tamaño de página (A4, Letter, etc.)"
// @Success 200 {object} APIResponse
// @Router /api/v1/pdf/jpg-to-pdf [post]
func (h *Handlers) JPGToPDF(c *fiber.Ctx) error {
	startTime := time.Now()
	h.logger.Info("🖼️ JPG to PDF requested", "ip", c.IP())

	// Obtener archivos subidos
	form, err := c.MultipartForm()
	if err != nil {
		return h.ErrorResponse(c, "FORM_ERROR", "Failed to parse multipart form", err.Error(), 400)
	}

	files := form.File["files"]
	if len(files) == 0 {
		return h.ErrorResponse(c, "NO_FILES", "No image files uploaded", "", 400)
	}

	// Guardar imágenes temporales
	tempStorage := storage.NewService(h.config, h.logger)
	var imagePaths []string
	
	for _, file := range files {
		fileInfo, err := tempStorage.SaveUpload(file)
		if err != nil {
			return h.ErrorResponse(c, "UPLOAD_ERROR", "Failed to save image", err.Error(), 500)
		}
		imagePaths = append(imagePaths, fileInfo.Path)
		defer tempStorage.Delete(fileInfo.ID)
	}

	// Preparar salida
	outputPath := tempStorage.GenerateOutputPath("images_to_pdf.pdf")

	// Usar pdfcpu para crear PDF desde imágenes
	pdfService := pdf.NewService(h.config, h.logger)
	
	// pdfcpu puede importar imágenes directamente
	err = api.ImportImagesFile(imagePaths, outputPath, nil, pdfService.(*pdf.PDFCPUService).GetConfig())
	if err != nil {
		return h.ErrorResponse(c, "CONVERSION_FAILED", "JPG to PDF conversion failed", err.Error(), 500)
	}

	h.logger.Info("✅ JPG to PDF completed",
		"image_count", len(imagePaths),
		"duration", time.Since(startTime),
	)

	// Enviar archivo
	return c.Download(outputPath, "images_to_pdf.pdf")
}

// ExtractPDF handler para extraer contenido de PDFs
// @Summary Extraer contenido de PDF
// @Description Extrae texto, imágenes o metadatos de un PDF
// @Tags pdf
// @Accept multipart/form-data
// @Produce json
// @Security ApiKeyAuth
// @Param file formData file true "Archivo PDF"
// @Param type formData string true "Tipo de extracción (text, images, metadata)"
// @Success 200 {object} APIResponse
// @Router /api/v1/pdf/extract [post]
func (h *Handlers) ExtractPDF(c *fiber.Ctx) error {
	startTime := time.Now()
	h.logger.Info("📋 Extract PDF requested", "ip", c.IP())

	// Obtener archivo
	file, err := c.FormFile("file")
	if err != nil {
		return h.ErrorResponse(c, "MISSING_FILE", "No file uploaded", err.Error(), 400)
	}

	// Obtener tipo de extracción
	extractType := c.FormValue("type")
	if extractType == "" {
		extractType = "text"
	}

	// Obtener páginas (opcional)
	pages := c.FormValue("pages")
	if pages == "" {
		pages = "all"
	}

	// Guardar archivo temporal
	tempStorage := storage.NewService(h.config, h.logger)
	fileInfo, err := tempStorage.SaveUpload(file)
	if err != nil {
		return h.ErrorResponse(c, "UPLOAD_ERROR", "Failed to save file", err.Error(), 500)
	}
	defer tempStorage.Delete(fileInfo.ID)

	// Ejecutar extracción según tipo
	pdfService := pdf.NewService(h.config, h.logger)

	switch strings.ToLower(extractType) {
	case "text":
		text, err := pdfService.ExtractText(fileInfo.Path, pages)
		if err != nil {
			return h.ErrorResponse(c, "EXTRACTION_FAILED", "Text extraction failed", err.Error(), 500)
		}

		h.logger.Info("✅ Text extracted successfully",
			"length", len(text),
			"duration", time.Since(startTime),
		)

		return h.SuccessResponse(c, map[string]interface{}{
			"type":   "text",
			"text":   text,
			"length": len(text),
		})

	case "images":
		outputDir := tempStorage.GenerateOutputPath("extracted_" + fileInfo.ID)
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return h.ErrorResponse(c, "OUTPUT_DIR_ERROR", "Failed to create output directory", err.Error(), 500)
		}

		err = pdfService.ExtractImages(fileInfo.Path, outputDir, pages)
		if err != nil {
			return h.ErrorResponse(c, "EXTRACTION_FAILED", "Image extraction failed", err.Error(), 500)
		}

		h.logger.Info("✅ Images extracted successfully",
			"output_dir", outputDir,
			"duration", time.Since(startTime),
		)

		return h.SuccessResponse(c, map[string]interface{}{
			"type":       "images",
			"output_dir": outputDir,
		})

	case "metadata":
		info, err := pdfService.GetInfo(fileInfo.Path)
		if err != nil {
			return h.ErrorResponse(c, "EXTRACTION_FAILED", "Metadata extraction failed", err.Error(), 500)
		}

		h.logger.Info("✅ Metadata extracted successfully",
			"duration", time.Since(startTime),
		)

		return h.SuccessResponse(c, map[string]interface{}{
			"type":     "metadata",
			"metadata": info,
		})

	default:
		return h.ErrorResponse(c, "INVALID_TYPE", "Invalid extraction type (use: text, images, metadata)", "", 400)
	}
}

// WatermarkPDF handler para añadir marcas de agua
// @Summary Añadir marca de agua
// @Description Añade marca de agua a un archivo PDF
// @Tags pdf
// @Accept multipart/form-data
// @Produce json
// @Security ApiKeyAuth
// @Param file formData file true "Archivo PDF"
// @Param watermark formData string true "Texto de la marca de agua"
// @Param position formData string false "Posición (center, top-left, etc.)"
// @Success 200 {object} APIResponse
// @Router /api/v1/pdf/watermark [post]
func (h *Handlers) WatermarkPDF(c *fiber.Ctx) error {
	startTime := time.Now()
	h.logger.Info("🏷️ Watermark PDF requested", "ip", c.IP())

	// Obtener archivo
	file, err := c.FormFile("file")
	if err != nil {
		return h.ErrorResponse(c, "MISSING_FILE", "No file uploaded", err.Error(), 400)
	}

	// Obtener texto de marca de agua
	watermarkText := c.FormValue("watermark")
	if watermarkText == "" {
		return h.ErrorResponse(c, "MISSING_WATERMARK", "Watermark text is required", "", 400)
	}

	// Obtener posición (opcional)
	position := c.FormValue("position")
	if position == "" {
		position = "center"
	}

	// Guardar archivo temporal
	tempStorage := storage.NewService(h.config, h.logger)
	fileInfo, err := tempStorage.SaveUpload(file)
	if err != nil {
		return h.ErrorResponse(c, "UPLOAD_ERROR", "Failed to save file", err.Error(), 500)
	}
	defer tempStorage.Delete(fileInfo.ID)

	// Preparar salida
	outputPath := tempStorage.GenerateOutputPath("watermarked_" + file.Filename)

	// Ejecutar watermark
	pdfService := pdf.NewService(h.config, h.logger)
	result, err := pdfService.AddWatermark(fileInfo.Path, outputPath, watermarkText, position)
	if err != nil {
		return h.ErrorResponse(c, "WATERMARK_FAILED", "Watermark operation failed", err.Error(), 500)
	}

	h.logger.Info("✅ Watermark added successfully",
		"text", watermarkText,
		"position", position,
		"size", result.Size,
		"duration", time.Since(startTime),
	)

	// Enviar archivo
	return c.Download(outputPath, filepath.Base(outputPath))
}

// PDFInfo handler para información de PDF
// @Summary Información de archivo PDF
// @Description Obtiene metadatos e información de un PDF
// @Tags pdf
// @Accept multipart/form-data
// @Produce json
// @Security ApiKeyAuth
// @Param file formData file true "Archivo PDF"
// @Success 200 {object} APIResponse
// @Router /api/v1/pdf/info [post]
func (h *Handlers) PDFInfo(c *fiber.Ctx) error {
	h.logger.Info("📊 PDF Info requested", "ip", c.IP())
	
	return h.SuccessResponse(c, map[string]interface{}{
		"message": "PDF info functionality will be implemented in Phase 2",
		"status":  "placeholder",
	})
}