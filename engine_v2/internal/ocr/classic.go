package ocr

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/internal/utils"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// ClassicService interface para OCR clásico
type ClassicService interface {
	ExtractText(imagePath string) (*OCRResult, error)
	ExtractTextWithLang(imagePath, language string) (*OCRResult, error)
	SupportedLanguages() []string
	IsAvailable() bool
}

// OCRResult resultado de OCR
type OCRResult struct {
	Text       string            `json:"text"`
	Confidence float64           `json:"confidence"`
	Language   string            `json:"language"`
	Engine     string            `json:"engine"`
	Duration   time.Duration     `json:"duration_ms"`
	Metadata   map[string]string `json:"metadata"`
}

// TesseractService implementación con Tesseract
type TesseractService struct {
	config *config.Config
	logger *logger.Logger
	path   string
}

// PaddleOCRService implementación con PaddleOCR
type PaddleOCRService struct {
	config *config.Config
	logger *logger.Logger
	enabled bool
}

// NewClassicService crea una instancia del servicio OCR clásico
func NewClassicService(cfg *config.Config, log *logger.Logger) ClassicService {
	switch cfg.OCR.Provider {
	case "paddle":
		if cfg.OCR.PaddleEnabled {
			return &PaddleOCRService{
				config:  cfg,
				logger:  log,
				enabled: true,
			}
		}
		fallthrough
	case "tesseract":
		fallthrough
	default:
		return &TesseractService{
			config: cfg,
			logger: log,
			path:   cfg.OCR.TesseractPath,
		}
	}
}

// Tesseract Implementation
func (s *TesseractService) ExtractText(imagePath string) (*OCRResult, error) {
	// Usar primer idioma disponible como default
	defaultLang := "eng"
	if len(s.config.OCR.Languages) > 0 {
		defaultLang = s.config.OCR.Languages[0]
	}
	
	return s.ExtractTextWithLang(imagePath, defaultLang)
}

func (s *TesseractService) ExtractTextWithLang(imagePath, language string) (*OCRResult, error) {
	startTime := time.Now()
	
	s.logger.Info("👁️ Starting Tesseract OCR",
		"image", filepath.Base(imagePath),
		"language", language,
		"engine", "tesseract",
	)

	// Verificar disponibilidad
	if !s.IsAvailable() {
		return nil, fmt.Errorf("Tesseract not available at path: %s", s.path)
	}

	// Validar archivo de entrada
	if err := validateImageFile(imagePath); err != nil {
		return nil, fmt.Errorf("invalid image file: %w", err)
	}

	// Crear archivo temporal para output
	tempDir := os.TempDir()
	outputBase := filepath.Join(tempDir, fmt.Sprintf("ocr_output_%d", time.Now().Unix()))
	outputFile := outputBase + ".txt"

	// Limpiar archivo temporal al finalizar
	defer func() {
		os.Remove(outputFile)
	}()

	// Validar paths para prevenir command injection
	sanitizer := utils.NewCommandArgSanitizer()
	if !sanitizer.IsValidPath(imagePath) {
		return nil, fmt.Errorf("invalid image path for security reasons")
	}
	if !sanitizer.IsValidPath(outputBase) {
		return nil, fmt.Errorf("invalid output path for security reasons")
	}
	
	// Sanitizar language parameter (solo permitir valores seguros)
	language = sanitizer.SanitizeCommandArg(language)
	if language == "" {
		language = "eng" // Default seguro
	}

	// Preparar comando Tesseract
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	args := []string{
		imagePath,
		outputBase, // Tesseract añade .txt automáticamente
		"-l", language,
		"--psm", "3", // Page Segmentation Mode: Fully automatic page segmentation
		"--oem", "3", // OCR Engine Mode: Default, based on what is available
	}

	cmd := exec.CommandContext(ctx, s.path, args...)
	
	// Ejecutar OCR
	stderr, err := cmd.CombinedOutput()
	duration := time.Since(startTime)

	if err != nil {
		s.logger.Error("❌ Tesseract OCR failed",
			"image", filepath.Base(imagePath),
			"language", language,
			"error", err.Error(),
			"stderr", string(stderr),
			"duration_ms", duration.Milliseconds(),
		)
		return nil, fmt.Errorf("Tesseract OCR failed: %w\nOutput: %s", err, string(stderr))
	}

	// Leer resultado
	textBytes, err := os.ReadFile(outputFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read OCR output: %w", err)
	}

	text := strings.TrimSpace(string(textBytes))
	
	// Calcular confianza básica (aproximada por longitud de texto)
	confidence := calculateConfidence(text, string(stderr))

	result := &OCRResult{
		Text:       text,
		Confidence: confidence,
		Language:   language,
		Engine:     "tesseract",
		Duration:   duration,
		Metadata: map[string]string{
			"psm":        "3",
			"oem":        "3",
			"input_size": fmt.Sprintf("%d", getFileSize(imagePath)),
		},
	}

	s.logger.Info("✅ Tesseract OCR completed",
		"image", filepath.Base(imagePath),
		"language", language,
		"text_length", len(text),
		"confidence", fmt.Sprintf("%.2f", confidence),
		"duration_ms", duration.Milliseconds(),
	)

	return result, nil
}

func (s *TesseractService) SupportedLanguages() []string {
	return s.config.OCR.Languages
}

func (s *TesseractService) IsAvailable() bool {
	if s.path == "" {
		return false
	}

	// Verificar que Tesseract existe
	if _, err := os.Stat(s.path); os.IsNotExist(err) {
		return false
	}

	// Test rápido de version
	cmd := exec.Command(s.path, "--version")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd = exec.CommandContext(ctx, s.path, "--version")

	err := cmd.Run()
	return err == nil
}

// PaddleOCR Implementation (placeholder)
func (s *PaddleOCRService) ExtractText(imagePath string) (*OCRResult, error) {
	return s.ExtractTextWithLang(imagePath, "auto")
}

func (s *PaddleOCRService) ExtractTextWithLang(imagePath, language string) (*OCRResult, error) {
	startTime := time.Now()
	s.logger.Info("🏓 PaddleOCR extraction requested",
		"image", filepath.Base(imagePath),
		"language", language,
	)

	// Validar archivo de imagen
	if err := validateImageFile(imagePath); err != nil {
		return nil, err
	}

	// Crear script Python temporal para PaddleOCR
	pythonScript := `
import sys
import json
try:
    from paddleocr import PaddleOCR
    ocr = PaddleOCR(use_angle_cls=True, lang='%s', show_log=False)
    result = ocr.ocr(sys.argv[1], cls=True)
    
    # Extraer texto de todos los resultados
    text_lines = []
    avg_confidence = 0.0
    count = 0
    
    if result and result[0]:
        for line in result[0]:
            if len(line) >= 2:
                text_lines.append(line[1][0])
                avg_confidence += line[1][1]
                count += 1
    
    output = {
        "text": "\n".join(text_lines),
        "confidence": avg_confidence / count if count > 0 else 0.0,
        "lines": len(text_lines)
    }
    print(json.dumps(output))
except ImportError:
    print(json.dumps({"error": "PaddleOCR not installed", "text": "", "confidence": 0.0}))
except Exception as e:
    print(json.dumps({"error": str(e), "text": "", "confidence": 0.0}))
`
	pythonScript = fmt.Sprintf(pythonScript, language)

	// Crear contexto con timeout
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	// Ejecutar Python con PaddleOCR
	cmd := exec.CommandContext(ctx, "python", "-c", pythonScript, imagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		s.logger.Error("PaddleOCR execution failed",
			"error", err,
			"output", string(output),
		)
		return nil, fmt.Errorf("PaddleOCR execution failed: %w", err)
	}

	// Parse JSON output
	var result struct {
		Text       string  `json:"text"`
		Confidence float64 `json:"confidence"`
		Lines      int     `json:"lines"`
		Error      string  `json:"error"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		s.logger.Error("Failed to parse PaddleOCR output",
			"error", err,
			"output", string(output),
		)
		return nil, fmt.Errorf("failed to parse PaddleOCR output: %w", err)
	}

	// Verificar si hubo error de PaddleOCR
	if result.Error != "" {
		return nil, fmt.Errorf("PaddleOCR error: %s", result.Error)
	}

	duration := time.Since(startTime)
	s.logger.Info("✅ PaddleOCR extraction completed",
		"lines", result.Lines,
		"confidence", result.Confidence,
		"duration", duration,
	)

	return &OCRResult{
		Text:       result.Text,
		Confidence: result.Confidence,
		Language:   language,
		Engine:     "paddleocr",
		Duration:   duration,
		Metadata: map[string]string{
			"lines":      fmt.Sprintf("%d", result.Lines),
			"image_path": imagePath,
		},
	}, nil
}

func (s *PaddleOCRService) SupportedLanguages() []string {
	return []string{"auto", "eng", "spa", "por", "fra", "deu", "ita", "rus", "jpn", "kor", "chi"}
}

func (s *PaddleOCRService) IsAvailable() bool {
	// Verificar PaddleOCR Python environment
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test simple: intentar importar PaddleOCR
	testScript := `
import sys
try:
    from paddleocr import PaddleOCR
    print("OK")
    sys.exit(0)
except ImportError:
    print("NOT_INSTALLED")
    sys.exit(1)
`
	cmd := exec.CommandContext(ctx, "python", "-c", testScript)
	output, err := cmd.CombinedOutput()

	available := err == nil && strings.Contains(string(output), "OK")

	if available {
		s.logger.Debug("PaddleOCR is available")
	} else {
		s.logger.Warn("PaddleOCR not available",
			"error", err,
			"output", string(output),
			"hint", "Install with: pip install paddleocr paddlepaddle",
		)
	}

	return available
}

// Helper functions
func validateImageFile(imagePath string) error {
	// Verificar extensión
	ext := strings.ToLower(filepath.Ext(imagePath))
	supportedExts := map[string]bool{
		".jpg": true, ".jpeg": true, ".png": true,
		".bmp": true, ".tiff": true, ".tif": true,
		".gif": true, ".webp": true,
	}

	if !supportedExts[ext] {
		return fmt.Errorf("unsupported image format: %s", ext)
	}

	// Verificar que existe y no está vacío
	stat, err := os.Stat(imagePath)
	if err != nil {
		return fmt.Errorf("image file not accessible: %w", err)
	}

	if stat.Size() == 0 {
		return fmt.Errorf("image file is empty")
	}

	if stat.Size() > 50*1024*1024 { // 50MB limit para imágenes
		return fmt.Errorf("image file too large: %d bytes (max 50MB)", stat.Size())
	}

	return nil
}

func calculateConfidence(text, stderr string) float64 {
	// Confianza básica basada en longitud de texto y ausencia de errores
	if len(text) == 0 {
		return 0.0
	}

	confidence := 0.5 // Base confidence
	
	// Incrementar por longitud de texto
	if len(text) > 10 {
		confidence += 0.2
	}
	if len(text) > 100 {
		confidence += 0.1
	}
	
	// Decrementar si hay warnings en stderr
	if strings.Contains(strings.ToLower(stderr), "warning") {
		confidence -= 0.1
	}
	
	// Asegurar rango [0, 1]
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0.0 {
		confidence = 0.0
	}

	return confidence
}

func getFileSize(path string) int64 {
	if stat, err := os.Stat(path); err == nil {
		return stat.Size()
	}
	return 0
}