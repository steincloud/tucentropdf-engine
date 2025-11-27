package pdf

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

func TestMain(m *testing.M) {
	// Setup para todos los tests
	setupTestData()
	code := m.Run()
	// Cleanup después de todos los tests
	cleanupTestData()
	os.Exit(code)
}

func setupTestData() {
	// Crear directorio de pruebas temporal
	os.MkdirAll("../../testdata/pdf", 0755)
}

func cleanupTestData() {
	// Limpiar archivos temporales de testing
	os.RemoveAll("../../temp_test")
}

func getTestConfig() *config.Config {
	return &config.Config{
		Environment: "test",
		Storage: config.StorageConfig{
			TempDir: "../../temp_test",
		},
	}
}

func getTestLogger() *logger.Logger {
	zapLogger, _ := logger.New("error", "json", "stdout")
	return logger.NewLogger(zapLogger)
}

func TestPDFService_MergePDFs(t *testing.T) {
	cfg := getTestConfig()
	log := getTestLogger()
	service := NewService(cfg, log)

	tests := []struct {
		name        string
		inputFiles  []string
		expectError bool
		expectPages int
	}{
		{
			name: "merge_two_valid_pdfs",
			inputFiles: []string{
				"../../testdata/pdf/sample1.pdf",
				"../../testdata/pdf/sample2.pdf",
			},
			expectError: false,
			expectPages: 7, // 2 + 5 páginas
		},
		{
			name: "merge_with_nonexistent_file",
			inputFiles: []string{
				"../../testdata/pdf/sample1.pdf",
				"../../testdata/pdf/nonexistent.pdf",
			},
			expectError: true,
			expectPages: 0,
		},
		{
			name: "merge_single_file",
			inputFiles: []string{
				"../../testdata/pdf/sample1.pdf",
			},
			expectError: false,
			expectPages: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Crear directorio temporal para output
			outputDir := "../../temp_test"
			os.MkdirAll(outputDir, 0755)
			defer os.RemoveAll(outputDir)

			outputPath := filepath.Join(outputDir, "merged_"+tt.name+".pdf")

			result, err := service.MergePDFs(tt.inputFiles, outputPath)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, outputPath, result.OutputPath)
				assert.Equal(t, tt.expectPages, result.PageCount)
				assert.Greater(t, result.FileSize, int64(0))

				// Verificar que el archivo fue creado
				_, err := os.Stat(outputPath)
				assert.NoError(t, err)
			}
		})
	}
}

func TestPDFService_SplitPDF(t *testing.T) {
	cfg := getTestConfig()
	log := getTestLogger()
	service := NewService(cfg, log)

	tests := []struct {
		name         string
		inputFile    string
		pagesPerFile int
		expectError  bool
		expectFiles  int
	}{
		{
			name:         "split_5_page_pdf_by_2",
			inputFile:    "../../testdata/pdf/sample2.pdf",
			pagesPerFile: 2,
			expectError:  false,
			expectFiles:  3, // 5 páginas / 2 = 3 archivos (2,2,1)
		},
		{
			name:         "split_nonexistent_file",
			inputFile:    "../../testdata/pdf/nonexistent.pdf",
			pagesPerFile: 1,
			expectError:  true,
			expectFiles:  0,
		},
		{
			name:         "split_with_invalid_pages_per_file",
			inputFile:    "../../testdata/pdf/sample1.pdf",
			pagesPerFile: 0,
			expectError:  true,
			expectFiles:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputDir := "../../temp_test/" + tt.name
			os.MkdirAll(outputDir, 0755)
			defer os.RemoveAll(outputDir)

			if tt.pagesPerFile <= 0 {
				// Test de validación de parámetros
				_, err := service.SplitPDF(tt.inputFile, outputDir, tt.pagesPerFile)
				assert.Error(t, err)
				return
			}

			result, err := service.SplitPDF(tt.inputFile, outputDir, tt.pagesPerFile)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Len(t, result.OutputFiles, tt.expectFiles)

				// Verificar que los archivos fueron creados
				for _, file := range result.OutputFiles {
					_, err := os.Stat(file)
					assert.NoError(t, err, "Archivo split debería existir: %s", file)
				}
			}
		})
	}
}

func TestPDFService_OptimizePDF(t *testing.T) {
	cfg := getTestConfig()
	log := getTestLogger()
	service := NewService(cfg, log)

	tests := []struct {
		name        string
		inputFile   string
		quality     string
		expectError bool
	}{
		{
			name:        "optimize_low_quality",
			inputFile:   "../../testdata/pdf/sample1.pdf",
			quality:     "low",
			expectError: false,
		},
		{
			name:        "optimize_medium_quality",
			inputFile:   "../../testdata/pdf/sample2.pdf",
			quality:     "medium",
			expectError: false,
		},
		{
			name:        "optimize_high_quality",
			inputFile:   "../../testdata/pdf/sample1.pdf",
			quality:     "high",
			expectError: false,
		},
		{
			name:        "optimize_nonexistent_file",
			inputFile:   "../../testdata/pdf/nonexistent.pdf",
			quality:     "medium",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outputDir := "../../temp_test"
			os.MkdirAll(outputDir, 0755)
			defer os.RemoveAll(outputDir)

			outputPath := filepath.Join(outputDir, "optimized_"+tt.name+".pdf")

			result, err := service.OptimizePDF(tt.inputFile, outputPath, tt.quality)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, outputPath, result.OutputPath)
				assert.Greater(t, result.OriginalSize, int64(0))
				assert.Greater(t, result.OptimizedSize, int64(0))
				assert.Greater(t, result.CompressionRatio, 0.0)

				// Verificar que el archivo optimizado existe
				_, err := os.Stat(outputPath)
				assert.NoError(t, err)
			}
		})
	}
}

func TestPDFUtils_CountPages(t *testing.T) {
	log := getTestLogger()
	utils := NewPDFUtils(log)

	tests := []struct {
		name          string
		inputFile     string
		expectedPages int
		expectError   bool
	}{
		{
			name:          "count_pages_sample1",
			inputFile:     "../../testdata/pdf/sample1.pdf",
			expectedPages: 2,
			expectError:   false,
		},
		{
			name:          "count_pages_sample2",
			inputFile:     "../../testdata/pdf/sample2.pdf",
			expectedPages: 5,
			expectError:   false,
		},
		{
			name:        "count_pages_nonexistent",
			inputFile:   "../../testdata/pdf/nonexistent.pdf",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pages, err := utils.CountPages(tt.inputFile)

			if tt.expectError {
				assert.Error(t, err)
				assert.Equal(t, 0, pages)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedPages, pages)
			}
		})
	}
}

func TestPDFUtils_ValidatePDF(t *testing.T) {
	log := getTestLogger()
	utils := NewPDFUtils(log)

	tests := []struct {
		name        string
		inputFile   string
		expectError bool
	}{
		{
			name:        "validate_valid_pdf",
			inputFile:   "../../testdata/pdf/sample1.pdf",
			expectError: false,
		},
		{
			name:        "validate_nonexistent_pdf",
			inputFile:   "../../testdata/pdf/nonexistent.pdf",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := utils.ValidatePDF(tt.inputFile)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPDFUtils_GetPDFInfo(t *testing.T) {
	log := getTestLogger()
	utils := NewPDFUtils(log)

	t.Run("get_info_valid_pdf", func(t *testing.T) {
		info, err := utils.GetPDFInfo("../../testdata/pdf/sample1.pdf")

		require.NoError(t, err)
		require.NotNil(t, info)
		assert.Equal(t, "../../testdata/pdf/sample1.pdf", info.FilePath)
		assert.Equal(t, 2, info.PageCount)
		assert.Greater(t, info.FileSize, int64(0))
		assert.NotEmpty(t, info.Version)
		assert.False(t, info.Encrypted)
	})

	t.Run("get_info_nonexistent_pdf", func(t *testing.T) {
		info, err := utils.GetPDFInfo("../../testdata/pdf/nonexistent.pdf")

		assert.Error(t, err)
		assert.Nil(t, info)
	})
}

func TestPDFService_EstimateProcessingCost(t *testing.T) {
	cfg := getTestConfig()
	log := getTestLogger()
	service := NewService(cfg, log)

	tests := []struct {
		name      string
		inputFile string
		operation string
		expectMin float64
		expectMax float64
	}{
		{
			name:      "estimate_merge_cost",
			inputFile: "../../testdata/pdf/sample1.pdf",
			operation: "merge",
			expectMin: 0.001,
			expectMax: 0.1,
		},
		{
			name:      "estimate_ocr_cost",
			inputFile: "../../testdata/pdf/sample2.pdf",
			operation: "ocr",
			expectMin: 0.01,
			expectMax: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost, err := service.EstimateProcessingCost(tt.inputFile, tt.operation)

			assert.NoError(t, err)
			assert.GreaterOrEqual(t, cost, tt.expectMin)
			assert.LessOrEqual(t, cost, tt.expectMax)
		})
	}
}

// Benchmarks para operaciones PDF
func BenchmarkPDFMerge(b *testing.B) {
	cfg := getTestConfig()
	log := getTestLogger()
	service := NewService(cfg, log)

	inputFiles := []string{
		"../../testdata/pdf/sample1.pdf",
		"../../testdata/pdf/sample2.pdf",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		outputPath := filepath.Join("../../temp_test", "benchmark_merge.pdf")
		os.MkdirAll("../../temp_test", 0755)
		service.MergePDFs(inputFiles, outputPath)
		os.Remove(outputPath)
	}
}

func BenchmarkPDFPageCount(b *testing.B) {
	log := getTestLogger()
	utils := NewPDFUtils(log)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		utils.CountPages("../../testdata/pdf/sample2.pdf")
	}
}