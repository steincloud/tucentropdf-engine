package office

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

func getTestOfficeConfig() *config.Config {
	return &config.Config{
		Environment: "test",
		Storage: config.StorageConfig{
			TempDir: "../../temp_test",
		},
		Office: config.OfficeConfig{
			LibreOfficePath: getLibreOfficePath(),
			Timeout:         30,
		},
	}
}

func getTestOfficeLogger() *logger.Logger {
	zapLogger, _ := logger.New("error", "json", "stdout")
	return logger.NewLogger(zapLogger)
}

// getLibreOfficePath intenta encontrar LibreOffice en el sistema
func getLibreOfficePath() string {
	// Rutas comunes de LibreOffice en Windows
	commonPaths := []string{
		"C:\\Program Files\\LibreOffice\\program\\soffice.exe",
		"C:\\Program Files (x86)\\LibreOffice\\program\\soffice.exe",
		"soffice", // En PATH
	}

	for _, path := range commonPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	return "soffice" // Fallback
}

func setupOfficeTestData() {
	os.MkdirAll("../../temp_test", 0755)
	os.MkdirAll("../../testdata/office", 0755)
}

func cleanupOfficeTestData() {
	os.RemoveAll("../../temp_test")
}

func TestOfficeService_ConvertToPDF(t *testing.T) {
	setupOfficeTestData()
	defer cleanupOfficeTestData()

	cfg := getTestOfficeConfig()
	log := getTestOfficeLogger()
	service := NewService(cfg, log)

	tests := []struct {
		name        string
		inputFile   string
		expectError bool
		skipReason  string
	}{
		{
			name:        "convert_docx_to_pdf",
			inputFile:   "../../testdata/office/sample.docx",
			expectError: false,
		},
		{
			name:        "convert_txt_to_pdf",
			inputFile:   "../../testdata/office/sample.txt",
			expectError: false,
		},
		{
			name:        "convert_xlsx_to_pdf",
			inputFile:   "../../testdata/office/sample.xlsx",
			expectError: false,
		},
		{
			name:        "convert_pptx_to_pdf",
			inputFile:   "../../testdata/office/sample.pptx",
			expectError: false,
		},
		{
			name:        "convert_nonexistent_file",
			inputFile:   "../../testdata/office/nonexistent.docx",
			expectError: true,
		},
		{
			name:        "convert_unsupported_format",
			inputFile:   "../../testdata/office/unsupported.xyz",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip si LibreOffice no está disponible
			if !isLibreOfficeAvailable() {
				t.Skip("LibreOffice no está disponible en el sistema")
			}

			// Skip si el archivo no existe y se espera que exista
			if !tt.expectError {
				if _, err := os.Stat(tt.inputFile); os.IsNotExist(err) {
					t.Skip("Archivo de test no existe:", tt.inputFile)
				}
			}

			outputPath := filepath.Join("../../temp_test", "converted_"+filepath.Base(tt.inputFile)+".pdf")

			result, err := service.ConvertToPDF(tt.inputFile, outputPath)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.inputFile, result.InputPath)
				assert.Equal(t, outputPath, result.OutputPath)
				assert.Greater(t, result.InputSize, int64(0))
				assert.Greater(t, result.OutputSize, int64(0))
				assert.Greater(t, result.ConversionTimeMs, 0)

				// Verificar que el archivo PDF fue creado
				_, err := os.Stat(outputPath)
				assert.NoError(t, err, "El archivo PDF convertido debería existir")
			}
		})
	}
}

func TestOfficeService_GetSupportedFormats(t *testing.T) {
	cfg := getTestOfficeConfig()
	log := getTestOfficeLogger()
	service := NewService(cfg, log)

	formats := service.GetSupportedFormats()

	// Verificar que incluye formatos comunes
	expectedFormats := []string{
		".docx", ".doc", ".odt",
		".xlsx", ".xls", ".ods",
		".pptx", ".ppt", ".odp",
		".txt", ".rtf",
	}

	for _, format := range expectedFormats {
		assert.Contains(t, formats, format, "Formato %s debería estar soportado", format)
	}

	// Verificar que no incluye formatos no soportados
	unsupportedFormats := []string{
		".pdf", ".jpg", ".png", ".mp4", ".zip",
	}

	for _, format := range unsupportedFormats {
		assert.NotContains(t, formats, format, "Formato %s NO debería estar soportado", format)
	}
}

func TestOfficeService_IsFormatSupported(t *testing.T) {
	cfg := getTestOfficeConfig()
	log := getTestOfficeLogger()
	service := NewService(cfg, log)

	tests := []struct {
		name       string
		filename   string
		isSupported bool
	}{
		{
			name:       "docx_supported",
			filename:   "document.docx",
			isSupported: true,
		},
		{
			name:       "xlsx_supported",
			filename:   "spreadsheet.xlsx",
			isSupported: true,
		},
		{
			name:       "txt_supported",
			filename:   "text.txt",
			isSupported: true,
		},
		{
			name:       "pdf_not_supported",
			filename:   "document.pdf",
			isSupported: false,
		},
		{
			name:       "image_not_supported",
			filename:   "image.jpg",
			isSupported: false,
		},
		{
			name:       "no_extension_not_supported",
			filename:   "document",
			isSupported: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			supported := service.IsFormatSupported(tt.filename)
			assert.Equal(t, tt.isSupported, supported)
		})
	}
}

func TestOfficeService_EstimateConversionTime(t *testing.T) {
	cfg := getTestOfficeConfig()
	log := getTestOfficeLogger()
	service := NewService(cfg, log)

	tests := []struct {
		name         string
		inputFile    string
		minTime      int
		maxTime      int
		expectError  bool
	}{
		{
			name:        "estimate_txt_conversion",
			inputFile:   "../../testdata/office/sample.txt",
			minTime:     500,   // 0.5 segundos
			maxTime:     5000,  // 5 segundos
			expectError: false,
		},
		{
			name:        "estimate_docx_conversion",
			inputFile:   "../../testdata/office/sample.docx",
			minTime:     1000,  // 1 segundo
			maxTime:     10000, // 10 segundos
			expectError: false,
		},
		{
			name:        "estimate_nonexistent_file",
			inputFile:   "../../testdata/office/nonexistent.txt",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip si el archivo no existe y se espera que exista
			if !tt.expectError {
				if _, err := os.Stat(tt.inputFile); os.IsNotExist(err) {
					t.Skip("Archivo de test no existe:", tt.inputFile)
				}
			}

			estimatedMs, err := service.EstimateConversionTime(tt.inputFile)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, 0, estimatedMs)
			} else {
				assert.NoError(t, err)
				assert.GreaterOrEqual(t, estimatedMs, tt.minTime)
				assert.LessOrEqual(t, estimatedMs, tt.maxTime)
			}
		})
	}
}

func TestOfficeService_ValidateInputFile(t *testing.T) {
	cfg := getTestOfficeConfig()
	log := getTestOfficeLogger()
	service := NewService(cfg, log)

	tests := []struct {
		name        string
		inputFile   string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid_txt_file",
			inputFile:   "../../testdata/office/sample.txt",
			expectError: false,
		},
		{
			name:        "nonexistent_file",
			inputFile:   "../../testdata/office/nonexistent.txt",
			expectError: true,
			errorMsg:    "no existe",
		},
		{
			name:        "unsupported_format",
			inputFile:   "../../testdata/office/image.jpg",
			expectError: true,
			errorMsg:    "formato no soportado",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Crear archivo temporal para tests que lo requieren
			if tt.name == "unsupported_format" {
				// Crear archivo temporal con extensión no soportada
				tempFile := "../../testdata/office/image.jpg"
				os.MkdirAll(filepath.Dir(tempFile), 0755)
				os.WriteFile(tempFile, []byte("fake image"), 0644)
				defer os.Remove(tempFile)
			}

			err := service.ValidateInputFile(tt.inputFile)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				// Skip si el archivo no existe
				if _, fileErr := os.Stat(tt.inputFile); os.IsNotExist(fileErr) {
					t.Skip("Archivo de test no existe:", tt.inputFile)
				}
				assert.NoError(t, err)
			}
		})
	}
}

func TestOfficeService_GetFileInfo(t *testing.T) {
	cfg := getTestOfficeConfig()
	log := getTestOfficeLogger()
	service := NewService(cfg, log)

	tests := []struct {
		name        string
		inputFile   string
		expectError bool
	}{
		{
			name:        "get_info_txt_file",
			inputFile:   "../../testdata/office/sample.txt",
			expectError: false,
		},
		{
			name:        "get_info_nonexistent_file",
			inputFile:   "../../testdata/office/nonexistent.txt",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip si el archivo no existe y se espera que exista
			if !tt.expectError {
				if _, err := os.Stat(tt.inputFile); os.IsNotExist(err) {
					t.Skip("Archivo de test no existe:", tt.inputFile)
				}
			}

			info, err := service.GetFileInfo(tt.inputFile)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, info)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, info)
				assert.Equal(t, tt.inputFile, info.FilePath)
				assert.Greater(t, info.Size, int64(0))
				assert.NotEmpty(t, info.Extension)
				assert.NotEmpty(t, info.MimeType)
			}
		})
	}
}

// Helper function para verificar si LibreOffice está disponible
func isLibreOfficeAvailable() bool {
	cfg := getTestOfficeConfig()
	
	// Intentar ejecutar LibreOffice con --version
	_, err := executeLibreOfficeCommand(cfg.Office.LibreOfficePath, []string{"--version"}, 5000)
	return err == nil
}

// Tests de integración con LibreOffice real
func TestIntegration_OfficeConversion_RealFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	if !isLibreOfficeAvailable() {
		t.Skip("LibreOffice no está disponible en el sistema")
	}

	setupOfficeTestData()
	defer cleanupOfficeTestData()

	cfg := getTestOfficeConfig()
	log := getTestOfficeLogger()
	service := NewService(cfg, log)

	testFiles := []string{
		"../../testdata/office/sample.txt",
	}

	for _, file := range testFiles {
		t.Run("real_conversion_"+filepath.Base(file), func(t *testing.T) {
			// Skip si el archivo no existe
			if _, err := os.Stat(file); os.IsNotExist(err) {
				t.Skip("Archivo de test no existe:", file)
			}

			outputPath := filepath.Join("../../temp_test", "integration_"+filepath.Base(file)+".pdf")
			
			result, err := service.ConvertToPDF(file, outputPath)

			require.NoError(t, err)
			require.NotNil(t, result)
			
			// Verificar que el archivo fue creado y es válido
			stat, err := os.Stat(outputPath)
			require.NoError(t, err)
			assert.Greater(t, stat.Size(), int64(0))

			// Verificar que es un PDF válido (comienza con %PDF)
			content, err := os.ReadFile(outputPath)
			require.NoError(t, err)
			assert.True(t, len(content) > 4)
			assert.Equal(t, "%PDF", string(content[:4]))
		})
	}
}

// Tests de rendimiento
func TestOfficeService_Performance(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance tests in short mode")
	}

	setupOfficeTestData()
	defer cleanupOfficeTestData()

	cfg := getTestOfficeConfig()
	log := getTestOfficeLogger()
	service := NewService(cfg, log)

	// Test de memoria y tiempo para archivos de diferentes tamaños
	testCases := []struct {
		name     string
		fileSize int // en KB
	}{
		{"small_file", 1},
		{"medium_file", 50},
		{"large_file", 500},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Crear archivo de prueba del tamaño especificado
			testFile := filepath.Join("../../temp_test", tc.name+".txt")
			content := make([]byte, tc.fileSize*1024)
			for i := range content {
				content[i] = byte('A' + (i % 26))
			}
			
			err := os.WriteFile(testFile, content, 0644)
			require.NoError(t, err)

			outputPath := filepath.Join("../../temp_test", tc.name+".pdf")

			// Medir tiempo de conversión
			result, err := service.ConvertToPDF(testFile, outputPath)

			if isLibreOfficeAvailable() {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				
				// Verificar que la conversión no toma demasiado tiempo
				// Para archivos pequeños/medianos debería ser < 30s
				if tc.fileSize <= 50 {
					assert.Less(t, result.ConversionTimeMs, 30000)
				}
			} else {
				t.Skip("LibreOffice no disponible para test de performance")
			}
		})
	}
}

// Benchmarks para Office
func BenchmarkOfficeConversion_SmallFile(b *testing.B) {
	if !isLibreOfficeAvailable() {
		b.Skip("LibreOffice no está disponible")
	}

	setupOfficeTestData()
	defer cleanupOfficeTestData()

	cfg := getTestOfficeConfig()
	log := getTestOfficeLogger()
	service := NewService(cfg, log)

	// Crear archivo pequeño para benchmark
	testFile := "../../temp_test/benchmark.txt"
	content := []byte("Este es un archivo de prueba para benchmarking.\nContiene múltiples líneas.\nPara simular un documento real.")
	os.WriteFile(testFile, content, 0644)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		outputPath := filepath.Join("../../temp_test", "benchmark_output.pdf")
		service.ConvertToPDF(testFile, outputPath)
		os.Remove(outputPath)
	}
}

func BenchmarkOfficeService_IsFormatSupported(b *testing.B) {
	cfg := getTestOfficeConfig()
	log := getTestOfficeLogger()
	service := NewService(cfg, log)

	filenames := []string{
		"document.docx",
		"spreadsheet.xlsx",
		"presentation.pptx",
		"text.txt",
		"image.jpg",
		"video.mp4",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, filename := range filenames {
			service.IsFormatSupported(filename)
		}
	}
}