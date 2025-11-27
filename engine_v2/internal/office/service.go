package office

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/internal/utils"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// Service interface para conversión Office
type Service interface {
	ConvertToPDF(inputPath, outputPath string) error
	SupportedFormats() []string
	IsAvailable() bool
}

// LibreOfficeService implementación con LibreOffice
type LibreOfficeService struct {
	config *config.Config
	logger *logger.Logger
	path   string
}

// GotenbergService implementación con Gotenberg
type GotenbergService struct {
	config *config.Config
	logger *logger.Logger
	url    string
}

// NewService crea una instancia del servicio Office
func NewService(cfg *config.Config, log *logger.Logger) Service {
	if !cfg.Office.Enabled {
		return &DisabledService{logger: log}
	}

	switch cfg.Office.Provider {
	case "gotenberg":
		return &GotenbergService{
			config: cfg,
			logger: log,
			url:    cfg.Office.GotenbergURL,
		}
	case "libreoffice":
		fallthrough
	default:
		return &LibreOfficeService{
			config: cfg,
			logger: log,
			path:   cfg.Office.LibreOfficePath,
		}
	}
}

// LibreOffice Implementation
func (s *LibreOfficeService) ConvertToPDF(inputPath, outputPath string) error {
	startTime := time.Now()
	s.logger.Info("🔄 Starting LibreOffice conversion",
		"input", filepath.Base(inputPath),
		"provider", "libreoffice",
	)

	// Verificar que LibreOffice esté disponible
	if !s.IsAvailable() {
		return fmt.Errorf("LibreOffice not available at path: %s", s.path)
	}

	// Crear directorio de salida si no existe
	outputDir := filepath.Dir(outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Validar paths para prevenir command injection
	sanitizer := utils.NewCommandArgSanitizer()
	if !sanitizer.IsValidPath(inputPath) {
		return fmt.Errorf("invalid input path for security reasons")
	}
	if !sanitizer.IsValidPath(outputDir) {
		return fmt.Errorf("invalid output directory for security reasons")
	}

	// Preparar comando LibreOffice
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Argumentos para conversión headless con seguridad adicional
	args := []string{
		"--headless",
		"--invisible",
		"--nocrashreport",
		"--nodefault",
		"--nofirststartwizard",
		"--nolockcheck",
		"--nologo",
		"--norestore",
		"--convert-to", "pdf",
		"--outdir", outputDir,
		inputPath,
	}

	cmd := exec.CommandContext(ctx, s.path, args...)
	
	// Configurar variables de entorno para headless
	cmd.Env = append(os.Environ(),
		"DISPLAY=",
		"HOME=/tmp",
	)

	// Ejecutar conversión
	output, err := cmd.CombinedOutput()
	duration := time.Since(startTime)

	if err != nil {
		s.logger.Error("❌ LibreOffice conversion failed",
			"input", filepath.Base(inputPath),
			"error", err.Error(),
			"output", string(output),
			"duration_ms", duration.Milliseconds(),
		)
		return fmt.Errorf("LibreOffice conversion failed: %w\nOutput: %s", err, string(output))
	}

	// Verificar que el archivo PDF fue creado
	baseName := strings.TrimSuffix(filepath.Base(inputPath), filepath.Ext(inputPath))
	expectedPDF := filepath.Join(outputDir, baseName+".pdf")
	
	if _, err := os.Stat(expectedPDF); os.IsNotExist(err) {
		return fmt.Errorf("PDF not generated at expected path: %s", expectedPDF)
	}

	// Mover el archivo al path deseado si es diferente
	if expectedPDF != outputPath {
		if err := os.Rename(expectedPDF, outputPath); err != nil {
			return fmt.Errorf("failed to move PDF to target location: %w", err)
		}
	}

	s.logger.Info("✅ LibreOffice conversion completed",
		"input", filepath.Base(inputPath),
		"output", filepath.Base(outputPath),
		"duration_ms", duration.Milliseconds(),
		"output_size", getFileSize(outputPath),
	)

	return nil
}

func (s *LibreOfficeService) SupportedFormats() []string {
	return []string{
		".doc", ".docx",           // Microsoft Word
		".xls", ".xlsx",           // Microsoft Excel  
		".ppt", ".pptx",           // Microsoft PowerPoint
		".odt", ".ods", ".odp",    // OpenDocument
		".rtf",                    // Rich Text Format
		".txt",                    // Plain Text
	}
}

func (s *LibreOfficeService) IsAvailable() bool {
	if s.path == "" {
		return false
	}

	// Verificar que el ejecutable existe
	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		return false
	}

	// Verificar que es ejecutable (test rápido)
	cmd := exec.Command(s.path, "--version")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd = exec.CommandContext(ctx, s.path, "--version")

	err := cmd.Run()
	return err == nil
}

// GotenbergService Implementation
func (s *GotenbergService) ConvertToPDF(inputPath, outputPath string) error {
	startTime := time.Now()
	s.logger.Info("📡 Gotenberg conversion requested",
		"input", filepath.Base(inputPath),
		"url", s.url,
	)

	// Verificar que el archivo existe
	if _, err := os.Stat(inputPath); err != nil {
		return fmt.Errorf("input file not found: %w", err)
	}

	// Abrir archivo para leer
	file, err := os.Open(inputPath)
	if err != nil {
		return fmt.Errorf("failed to open input file: %w", err)
	}
	defer file.Close()

	// Crear multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Agregar archivo al form
	part, err := writer.CreateFormFile("file", filepath.Base(inputPath))
	if err != nil {
		return fmt.Errorf("failed to create form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	// Cerrar writer para finalizar multipart
	if err := writer.Close(); err != nil {
		return fmt.Errorf("failed to close writer: %w", err)
	}

	// Crear request HTTP
	endpoint := fmt.Sprintf("%s/forms/libreoffice/convert", s.url)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Enviar request
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request to Gotenberg: %w", err)
	}
	defer resp.Body.Close()

	// Verificar respuesta
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Gotenberg returned error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Guardar PDF de salida
	outFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer outFile.Close()

	written, err := io.Copy(outFile, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write output file: %w", err)
	}

	duration := time.Since(startTime)
	s.logger.Info("✅ Gotenberg conversion completed",
		"input", filepath.Base(inputPath),
		"output_size", written,
		"duration", duration,
	)

	return nil
}

func (s *GotenbergService) SupportedFormats() []string {
	return []string{
		".doc", ".docx", ".xls", ".xlsx", ".ppt", ".pptx",
		".odt", ".ods", ".odp", ".html", ".htm",
	}
}

func (s *GotenbergService) IsAvailable() bool {
	// Verificar conectividad con Gotenberg
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	healthEndpoint := fmt.Sprintf("%s/health", s.url)
	req, err := http.NewRequestWithContext(ctx, "GET", healthEndpoint, nil)
	if err != nil {
		s.logger.Warn("Failed to create Gotenberg health check request", "error", err)
		return false
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.logger.Warn("Gotenberg health check failed", "error", err, "url", healthEndpoint)
		return false
	}
	defer resp.Body.Close()

	available := resp.StatusCode == http.StatusOK
	if available {
		s.logger.Debug("Gotenberg is available", "url", s.url)
	} else {
		s.logger.Warn("Gotenberg health check returned non-200", "status", resp.StatusCode)
	}

	return available
}

// DisabledService cuando Office está deshabilitado
type DisabledService struct {
	logger *logger.Logger
}

func (s *DisabledService) ConvertToPDF(inputPath, outputPath string) error {
	return fmt.Errorf("Office conversion is disabled")
}

func (s *DisabledService) SupportedFormats() []string {
	return []string{}
}

func (s *DisabledService) IsAvailable() bool {
	return false
}

// Helper functions
func getFileSize(path string) int64 {
	if stat, err := os.Stat(path); err == nil {
		return stat.Size()
	}
	return 0
}

// ValidateOfficeFile valida que el archivo sea un formato Office soportado
func ValidateOfficeFile(filePath string) error {
	ext := strings.ToLower(filepath.Ext(filePath))
	
	supportedExts := map[string]bool{
		".doc":  true, ".docx": true,
		".xls":  true, ".xlsx": true,
		".ppt":  true, ".pptx": true,
		".odt":  true, ".ods":  true, ".odp": true,
		".rtf":  true, ".txt":  true,
	}

	if !supportedExts[ext] {
		return fmt.Errorf("unsupported office format: %s", ext)
	}

	// Verificar que el archivo existe y no está vacío
	stat, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("file not accessible: %w", err)
	}

	if stat.Size() == 0 {
		return fmt.Errorf("file is empty")
	}

	if stat.Size() > 200*1024*1024 { // 200MB limit
		return fmt.Errorf("file too large: %d bytes (max 200MB)", stat.Size())
	}

	return nil
}