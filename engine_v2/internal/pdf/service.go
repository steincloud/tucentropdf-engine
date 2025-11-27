package pdf

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// Service interface para operaciones PDF
type Service interface {
	Merge(inputPaths []string, outputPath string) (*PDFResult, error)
	Split(inputPath string, mode string, rangeStr string) (*PDFResult, error)
	Optimize(inputPath, outputPath string, level string) (*PDFResult, error)
	Rotate(inputPath, outputPath string, angle int, pages string) (*PDFResult, error)
	Unlock(inputPath, outputPath, password string) (*PDFResult, error)
	Lock(inputPath, outputPath, password string, permissions []string) (*PDFResult, error)
	ExtractText(inputPath string, pages string) (string, error)
	ExtractImages(inputPath, outputDir string, pages string) error
	AddWatermark(inputPath, outputPath, watermarkText string, position string) (*PDFResult, error)
	GetInfo(inputPath string) (*PDFInfo, error)
	Validate(inputPath string) error
	IsAvailable() bool
}

// PDFResult resultado de operaciones PDF
type PDFResult struct {
	Pages       int           `json:"pages"`
	Size        int64         `json:"size"`
	OutputFiles []string      `json:"output_files,omitempty"`
	Duration    time.Duration `json:"-"`
	Operation   string        `json:"operation"`
	Success     bool          `json:"success"`
	Message     string        `json:"message,omitempty"`
}

// PDFInfo se define en utils.go

// PDFCPUService implementación usando pdfcpu nativo
type PDFCPUService struct {
	config *config.Config
	logger *logger.Logger
	conf   *model.Configuration
}

// NewService crea una instancia del servicio PDF
func NewService(cfg *config.Config, log *logger.Logger) Service {
	// Configuración pdfcpu con valores por defecto
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	
	return &PDFCPUService{
		config: cfg,
		logger: log,
		conf:   conf,
	}
}

// IsAvailable verifica si el servicio está disponible
func (s *PDFCPUService) IsAvailable() bool {
	return true // pdfcpu nativo siempre disponible
}

func (s *PDFCPUService) Merge(inputPaths []string, outputPath string) (*PDFResult, error) {
	startTime := time.Now()
	s.logger.Info("🔗 Starting PDF merge",
		"input_count", len(inputPaths),
		"output", filepath.Base(outputPath),
	)

	if len(inputPaths) < 2 {
		return nil, fmt.Errorf("need at least 2 PDF files to merge")
	}

	// Validar todos los archivos de entrada
	for _, path := range inputPaths {
		if err := s.validatePDFFile(path); err != nil {
			return nil, fmt.Errorf("invalid input file %s: %w", path, err)
		}
	}

	// Usar API nativa de pdfcpu
	err := api.MergeCreateFile(inputPaths, outputPath, false, s.conf)
	if err != nil {
		s.logger.Error("❌ PDF merge failed", "error", err, "inputs", len(inputPaths))
		return nil, fmt.Errorf("merge operation failed: %w", err)
	}

	// Obtener información del archivo de salida
	stat, err := os.Stat(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat output file: %w", err)
	}

	// Obtener número de páginas
	pages, err := s.getPageCount(outputPath)
	if err != nil {
		pages = -1 // Si no podemos obtenerlo, usar -1
	}

	duration := time.Since(startTime)
	s.logger.Info("✅ PDF merge completed",
		"input_files", len(inputPaths),
		"output_pages", pages,
		"output_size", stat.Size(),
		"duration_ms", duration.Milliseconds(),
	)

	return &PDFResult{
		Pages:     pages,
		Size:      stat.Size(),
		Duration:  duration,
		Operation: "merge",
		Success:   true,
		Message:   fmt.Sprintf("Successfully merged %d files", len(inputPaths)),
	}, nil
}

func (s *PDFCPUService) Split(inputPath string, mode string, rangeStr string) (*PDFResult, error) {
	startTime := time.Now()
	s.logger.Info("✂️ Starting PDF split",
		"input", filepath.Base(inputPath),
		"mode", mode,
		"range", rangeStr,
	)

	if err := s.validatePDFFile(inputPath); err != nil {
		return nil, err
	}

	// Obtener número de páginas del archivo original
	pages, err := s.getPageCount(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get page count: %w", err)
	}

	// Crear directorio temporal para archivos de salida
	outputDir := filepath.Dir(inputPath) + "/split_" + strconv.FormatInt(time.Now().Unix(), 10)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	var outputFiles []string

	if mode == "pages" || rangeStr == "" {
		// Dividir en páginas individuales usando span
		span := 1 // una página por archivo
		err = api.SplitFile(inputPath, outputDir, span, s.conf)
		if err != nil {
			return nil, fmt.Errorf("failed to split pages: %w", err)
		}
		
		// Recopilar archivos generados
		files, err := filepath.Glob(filepath.Join(outputDir, "*.pdf"))
		if err == nil {
			outputFiles = files
		}
	} else {
		// Dividir por rango especificado - para esto necesitaríamos implementar lógica específica
		// Por ahora, dividir en páginas individuales
		span := 1
		err = api.SplitFile(inputPath, outputDir, span, s.conf)
		if err != nil {
			return nil, fmt.Errorf("failed to split by range: %w", err)
		}
		
		// Recopilar archivos generados
		files, err := filepath.Glob(filepath.Join(outputDir, "*.pdf"))
		if err == nil {
			outputFiles = files
		}
	}

	duration := time.Since(startTime)
	s.logger.Info("✅ PDF split completed",
		"input_pages", pages,
		"output_files", len(outputFiles),
		"duration_ms", duration.Milliseconds(),
	)

	return &PDFResult{
		Pages:       pages,
		OutputFiles: outputFiles,
		Duration:    duration,
		Operation:   "split",
		Success:     true,
		Message:     fmt.Sprintf("Successfully split into %d files", len(outputFiles)),
	}, nil
}

func (s *PDFCPUService) Optimize(inputPath, outputPath string, level string) (*PDFResult, error) {
	startTime := time.Now()
	s.logger.Info("⚡ Starting PDF optimization",
		"input", filepath.Base(inputPath),
		"output", filepath.Base(outputPath),
		"level", level,
	)

	if err := s.validatePDFFile(inputPath); err != nil {
		return nil, err
	}

	// Obtener tamaño original
	originalStat, err := os.Stat(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat input file: %w", err)
	}

	// Configurar optimización según nivel
	optConf := model.NewDefaultConfiguration()
	switch level {
	case "low":
		optConf.Optimize = true
		optConf.OptimizeResourceDicts = false
		optConf.OptimizeDuplicateContentStreams = false
	case "medium":
		optConf.Optimize = true  
		optConf.OptimizeResourceDicts = true
		optConf.OptimizeDuplicateContentStreams = false
	case "high":
		optConf.Optimize = true
		optConf.OptimizeResourceDicts = true
		optConf.OptimizeDuplicateContentStreams = true
	default:
		optConf.Optimize = true // medium por defecto
		optConf.OptimizeResourceDicts = true
		optConf.OptimizeDuplicateContentStreams = false
	}

	// Usar API nativa de pdfcpu
	err = api.OptimizeFile(inputPath, outputPath, optConf)
	if err != nil {
		s.logger.Error("❌ PDF optimization failed", "error", err, "level", level)
		return nil, fmt.Errorf("optimization failed: %w", err)
	}

	// Obtener tamaño optimizado
	optimizedStat, err := os.Stat(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat output file: %w", err)
	}

	duration := time.Since(startTime)
	s.logger.Info("✅ PDF optimization completed",
		"original_size", originalStat.Size(),
		"optimized_size", optimizedStat.Size(),
		"reduction_percent", (1.0-float64(optimizedStat.Size())/float64(originalStat.Size()))*100,
		"duration_ms", duration.Milliseconds(),
	)

	return &PDFResult{
		Size:      optimizedStat.Size(),
		Duration:  duration,
		Operation: "optimize",
		Success:   true,
		Message:   fmt.Sprintf("Optimized from %d to %d bytes", originalStat.Size(), optimizedStat.Size()),
	}, nil
}

// Funciones placeholder para Phase 2.5
func (s *PDFCPUService) Rotate(inputPath, outputPath string, angle int, pages string) (*PDFResult, error) {
	startTime := time.Now()
	s.logger.Info("🔄 Starting PDF rotation",
		"input", filepath.Base(inputPath),
		"angle", angle,
		"pages", pages,
	)

	if err := s.validatePDFFile(inputPath); err != nil {
		return nil, err
	}

	// Validar ángulo (solo 90, 180, 270, -90)
	if angle != 90 && angle != 180 && angle != 270 && angle != -90 {
		return nil, fmt.Errorf("invalid rotation angle: %d (must be 90, 180, 270, or -90)", angle)
	}

	// Parse páginas (nil = todas)
	var pageSelection []string
	if pages != "" && pages != "all" {
		pageSelection = []string{pages}
	}

	// Usar API nativa de pdfcpu
	err := api.RotateFile(inputPath, outputPath, angle, pageSelection, s.conf)
	if err != nil {
		s.logger.Error("❌ PDF rotation failed", "error", err, "angle", angle)
		return nil, fmt.Errorf("rotation operation failed: %w", err)
	}

	// Obtener información del archivo de salida
	stat, err := os.Stat(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat output file: %w", err)
	}

	duration := time.Since(startTime)
	s.logger.Info("✅ PDF rotation completed",
		"angle", angle,
		"pages", pages,
		"size", stat.Size(),
		"duration", duration,
	)

	return &PDFResult{
		Size:        stat.Size(),
		OutputFiles: []string{outputPath},
		Duration:    duration,
		Operation:   fmt.Sprintf("rotate_%d", angle),
		Success:     true,
	}, nil
}

func (s *PDFCPUService) Unlock(inputPath, outputPath, password string) (*PDFResult, error) {
	startTime := time.Now()
	s.logger.Info("🔓 Starting PDF unlock",
		"input", filepath.Base(inputPath),
	)

	if err := s.validatePDFFile(inputPath); err != nil {
		return nil, err
	}

	if password == "" {
		return nil, fmt.Errorf("password is required for unlocking")
	}

	// Configurar contraseña
	conf := *s.conf
	conf.UserPW = password
	conf.OwnerPW = password

	// Usar API nativa de pdfcpu para desencriptar
	err := api.DecryptFile(inputPath, outputPath, &conf)
	if err != nil {
		s.logger.Error("❌ PDF unlock failed", "error", err)
		return nil, fmt.Errorf("unlock operation failed (wrong password?): %w", err)
	}

	// Obtener información del archivo de salida
	stat, err := os.Stat(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat output file: %w", err)
	}

	duration := time.Since(startTime)
	s.logger.Info("✅ PDF unlocked successfully",
		"size", stat.Size(),
		"duration", duration,
	)

	return &PDFResult{
		Size:        stat.Size(),
		OutputFiles: []string{outputPath},
		Duration:    duration,
		Operation:   "unlock",
		Success:     true,
	}, nil
}

func (s *PDFCPUService) Lock(inputPath, outputPath, password string, permissions []string) (*PDFResult, error) {
	startTime := time.Now()
	s.logger.Info("🔒 Starting PDF lock",
		"input", filepath.Base(inputPath),
		"permissions", permissions,
	)

	if err := s.validatePDFFile(inputPath); err != nil {
		return nil, err
	}

	if password == "" {
		return nil, fmt.Errorf("password is required for locking")
	}

	// Configurar encriptación
	conf := *s.conf
	conf.UserPW = password
	conf.OwnerPW = password

	// Configurar permisos (por defecto, restringir todo)
	conf.Permissions = 0
	for _, perm := range permissions {
		switch strings.ToLower(perm) {
		case "print":
			conf.Permissions = model.PermissionsPrint
		case "all":
			conf.Permissions = model.PermissionsAll
		}
	}

	// Usar API nativa de pdfcpu para encriptar
	err := api.EncryptFile(inputPath, outputPath, &conf)
	if err != nil {
		s.logger.Error("❌ PDF lock failed", "error", err)
		return nil, fmt.Errorf("lock operation failed: %w", err)
	}

	// Obtener información del archivo de salida
	stat, err := os.Stat(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat output file: %w", err)
	}

	duration := time.Since(startTime)
	s.logger.Info("✅ PDF locked successfully",
		"permissions", permissions,
		"size", stat.Size(),
		"duration", duration,
	)

	return &PDFResult{
		Size:        stat.Size(),
		OutputFiles: []string{outputPath},
		Duration:    duration,
		Operation:   "lock",
		Success:     true,
		Message:     fmt.Sprintf("Locked with permissions: %v", permissions),
	}, nil
}

func (s *PDFCPUService) ExtractText(inputPath string, pages string) (string, error) {
	s.logger.Info("📝 Starting text extraction",
		"input", filepath.Base(inputPath),
		"pages", pages,
	)

	if err := s.validatePDFFile(inputPath); err != nil {
		return "", err
	}

	// Parse páginas (nil = todas)
	var pageSelection []string
	if pages != "" && pages != "all" {
		pageSelection = []string{pages}
	}

	// Usar API nativa de pdfcpu para extraer texto
	// ExtractContent signature: (inFile string, outDir string, pageSelection []string, conf *Configuration)
	// Para extraer a string, usamos un directorio temporal
	tempDir := filepath.Join(os.TempDir(), "pdfcpu_extract")
	os.MkdirAll(tempDir, 0755)
	defer os.RemoveAll(tempDir)

	err := api.ExtractContentFile(inputPath, tempDir, pageSelection, s.conf)
	if err != nil {
		s.logger.Error("❌ Text extraction failed", "error", err)
		return "", fmt.Errorf("text extraction failed: %w", err)
	}

	// Leer archivos de texto generados
	files, _ := os.ReadDir(tempDir)
	var text strings.Builder
	for _, file := range files {
		if !file.IsDir() {
			content, _ := os.ReadFile(filepath.Join(tempDir, file.Name()))
			text.Write(content)
			text.WriteString("\n")
		}
	}

	extractedText := text.String()
	s.logger.Info("✅ Text extracted successfully",
		"length", len(extractedText),
	)

	return extractedText, nil
}

func (s *PDFCPUService) ExtractImages(inputPath, outputDir string, pages string) error {
	s.logger.Info("🖼️ Starting image extraction",
		"input", filepath.Base(inputPath),
		"output_dir", outputDir,
		"pages", pages,
	)

	if err := s.validatePDFFile(inputPath); err != nil {
		return err
	}

	// Crear directorio de salida si no existe
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Parse páginas (nil = todas)
	var pageSelection []string
	if pages != "" && pages != "all" {
		pageSelection = []string{pages}
	}

	// Usar API nativa de pdfcpu para extraer imágenes
	err := api.ExtractImagesFile(inputPath, outputDir, pageSelection, s.conf)
	if err != nil {
		s.logger.Error("❌ Image extraction failed", "error", err)
		return fmt.Errorf("image extraction failed: %w", err)
	}

	s.logger.Info("✅ Images extracted successfully",
		"output_dir", outputDir,
	)

	return nil
}

func (s *PDFCPUService) AddWatermark(inputPath, outputPath, watermarkText string, position string) (*PDFResult, error) {
	startTime := time.Now()
	s.logger.Info("🏷️ Starting watermark addition",
		"input", filepath.Base(inputPath),
		"text", watermarkText,
		"position", position,
	)

	if err := s.validatePDFFile(inputPath); err != nil {
		return nil, err
	}

	if watermarkText == "" {
		return nil, fmt.Errorf("watermark text is required")
	}

	// Crear watermark con configuración por defecto
	// TextWatermark signature: (text, desc string, onTop, update bool, unit types.DisplayUnit)
	watermarkDesc := "font:Helvetica, points:48, rot:45, op:0.5"
	wm, err := api.TextWatermark(watermarkText, watermarkDesc, false, false, types.POINTS)
	if err != nil {
		return nil, fmt.Errorf("failed to create watermark: %w", err)
	}

	// Aplicar watermark usando API nativa
	err = api.AddWatermarksFile(inputPath, outputPath, nil, wm, s.conf)
	if err != nil {
		s.logger.Error("❌ Watermark addition failed", "error", err)
		return nil, fmt.Errorf("watermark operation failed: %w", err)
	}

	// Obtener información del archivo de salida
	stat, err := os.Stat(outputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat output file: %w", err)
	}

	duration := time.Since(startTime)
	s.logger.Info("✅ Watermark added successfully",
		"text", watermarkText,
		"position", position,
		"size", stat.Size(),
		"duration", duration,
	)

	return &PDFResult{
		Size:        stat.Size(),
		OutputFiles: []string{outputPath},
		Duration:    duration,
		Operation:   "watermark",
		Success:     true,
	}, nil
}

func (s *PDFCPUService) GetInfo(inputPath string) (*PDFInfo, error) {
	s.logger.Info("📊 Getting PDF info",
		"input", filepath.Base(inputPath),
	)

	if err := s.validatePDFFile(inputPath); err != nil {
		return nil, err
	}

	// Usar API nativa para obtener info
	file, err := os.Open(inputPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()
	
	info, err := api.PDFInfo(file, inputPath, nil, s.conf)
	if err != nil {
		s.logger.Warn("Failed to get detailed PDF info", "error", err)
		// Fallback a info básica
		stat, statErr := os.Stat(inputPath)
		if statErr != nil {
			return nil, fmt.Errorf("failed to get file info: %w", statErr)
		}

		return &PDFInfo{
			Size:      stat.Size(),
			CreatedAt: stat.ModTime(),
			Metadata:  make(map[string]string),
		}, nil
	}

	// Parsear resultado de pdfcpu
	pdfInfo := &PDFInfo{
		Pages:    info.PageCount,
		Title:    info.Title,
		Author:   info.Author,
		Subject:  info.Subject,
		Creator:  info.Creator,
		Producer: info.Producer,
		Version:  info.Version,
		Encrypted: info.Encrypted,
		Metadata: make(map[string]string),
	}

	// Procesar fechas si están disponibles
	if info.CreationDate != "" {
		if createdAt, err := time.Parse(time.RFC3339, info.CreationDate); err == nil {
			pdfInfo.CreatedAt = createdAt
		}
	}
	if info.ModificationDate != "" {
		if modifiedAt, err := time.Parse(time.RFC3339, info.ModificationDate); err == nil {
			pdfInfo.ModifiedAt = modifiedAt
		}
	}

	// Obtener tamaño del archivo
	if stat, err := os.Stat(inputPath); err == nil {
		pdfInfo.Size = stat.Size()
		pdfInfo.CreatedAt = stat.ModTime()
		pdfInfo.ModifiedAt = stat.ModTime()
	}

	return pdfInfo, nil
}

func (s *PDFCPUService) Validate(inputPath string) error {
	s.logger.Info("🔍 Validating PDF",
		"input", filepath.Base(inputPath),
	)

	err := api.ValidateFile(inputPath, s.conf)
	if err != nil {
		s.logger.Error("❌ PDF validation failed", "error", err)
		return fmt.Errorf("PDF validation failed: %w", err)
	}

	s.logger.Info("✅ PDF validation completed")
	return nil
}

// Helper methods
func (s *PDFCPUService) validatePDFFile(path string) error {
	// Verificar que el archivo existe
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("file does not exist: %w", err)
	}

	// Verificar extensión
	if !strings.HasSuffix(strings.ToLower(path), ".pdf") {
		return fmt.Errorf("file is not a PDF: %s", path)
	}

	return nil
}

func (s *PDFCPUService) getPageCount(inputPath string) (int, error) {
	file, err := os.Open(inputPath)
	if err != nil {
		return 0, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()
	
	info, err := api.PDFInfo(file, inputPath, nil, s.conf)
	if err != nil {
		return 0, err
	}
	return info.PageCount, nil
}

// GetConfig returns the pdfcpu configuration for advanced operations
func (s *PDFCPUService) GetConfig() *model.Configuration {
	return s.conf
}