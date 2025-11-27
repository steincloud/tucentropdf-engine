package ocr

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// MockAIProvider para testing del servicio AI
type MockAIProvider struct {
	mock.Mock
}

func (m *MockAIProvider) ExtractTextFromImage(ctx context.Context, imagePath string) (*AITextResult, error) {
	args := m.Called(ctx, imagePath)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*AITextResult), args.Error(1)
}

func (m *MockAIProvider) ExtractStructuredData(ctx context.Context, imagePath string, prompt string) (*AIStructuredResult, error) {
	args := m.Called(ctx, imagePath, prompt)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*AIStructuredResult), args.Error(1)
}

func (m *MockAIProvider) CalculateTokens(text string) int {
	args := m.Called(text)
	return args.Int(0)
}

func getTestOCRConfig() *config.Config {
	return &config.Config{
		Environment: "test",
		AI: config.AIConfig{
			OpenAI: config.OpenAIConfig{
				APIKey: "test-key",
				Model:  "gpt-4o-mini",
			},
		},
		PlanLimits: config.PlanLimitsConfig{
			Free: config.PlanConfig{
				MaxFileSizeMB:  5,
				MaxFilesPerDay: 10,
				MaxAIOCRPages:  0,
			},
			Premium: config.PlanConfig{
				MaxFileSizeMB:  25,
				MaxFilesPerDay: 100,
				MaxAIOCRPages:  3,
			},
			Pro: config.PlanConfig{
				MaxFileSizeMB:  100,
				MaxFilesPerDay: 1000,
				MaxAIOCRPages:  20,
			},
		},
	}
}

func getTestOCRLogger() *logger.Logger {
	zapLogger, _ := logger.New("error", "json", "stdout")
	return logger.NewLogger(zapLogger)
}

func TestClassicOCRService_ExtractText(t *testing.T) {
	cfg := getTestOCRConfig()
	log := getTestOCRLogger()
	service := NewClassicService(cfg, log)

	tests := []struct {
		name        string
		imagePath   string
		language    string
		expectError bool
		expectText  bool
	}{
		{
			name:        "extract_text_clean_document",
			imagePath:   "../../testdata/ocr/classic/clean_document.png",
			language:    "spa",
			expectError: false,
			expectText:  true,
		},
		{
			name:        "extract_text_multilingual",
			imagePath:   "../../testdata/ocr/classic/multilingual.png",
			language:    "spa+eng",
			expectError: false,
			expectText:  true,
		},
		{
			name:        "extract_text_poor_quality",
			imagePath:   "../../testdata/ocr/classic/poor_quality.png",
			language:    "spa",
			expectError: false,
			expectText:  false, // Puede que no extraiga texto legible
		},
		{
			name:        "extract_text_nonexistent_file",
			imagePath:   "../../testdata/ocr/nonexistent.png",
			language:    "spa",
			expectError: true,
			expectText:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Skip si el archivo no existe y se espera que exista
			if !tt.expectError {
				if _, err := os.Stat(tt.imagePath); os.IsNotExist(err) {
					t.Skip("Archivo de test no existe:", tt.imagePath)
				}
			}

			ctx := context.Background()
			result, err := service.ExtractText(ctx, tt.imagePath, tt.language)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.imagePath, result.ImagePath)
				assert.Equal(t, tt.language, result.Language)
				assert.Greater(t, result.ProcessingTimeMs, 0)

				if tt.expectText {
					assert.NotEmpty(t, result.Text)
					assert.Greater(t, result.Confidence, 0.0)
				}
			}
		})
	}
}

func TestAIService_ValidatePlanAILimits(t *testing.T) {
	cfg := getTestOCRConfig()
	log := getTestOCRLogger()
	
	mockProvider := &MockAIProvider{}
	service := &AIService{
		config:   cfg,
		logger:   log,
		provider: mockProvider,
	}

	tests := []struct {
		name        string
		plan        string
		pagesUsed   int
		newPages    int
		expectError bool
		errorMsg    string
	}{
		{
			name:        "free_plan_no_ai_allowed",
			plan:        "free",
			pagesUsed:   0,
			newPages:    1,
			expectError: true,
			errorMsg:    "Plan Free no incluye OCR con IA",
		},
		{
			name:        "premium_within_limits",
			plan:        "premium",
			pagesUsed:   1,
			newPages:    2,
			expectError: false,
		},
		{
			name:        "premium_exceeds_limits",
			plan:        "premium",
			pagesUsed:   2,
			newPages:    2,
			expectError: true,
			errorMsg:    "Límite de páginas AI excedido",
		},
		{
			name:        "pro_within_limits",
			plan:        "pro",
			pagesUsed:   10,
			newPages:    5,
			expectError: false,
		},
		{
			name:        "pro_exceeds_limits",
			plan:        "pro",
			pagesUsed:   18,
			newPages:    5,
			expectError: true,
			errorMsg:    "Límite de páginas AI excedido",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := service.ValidatePlanAILimits(tt.plan, tt.pagesUsed, tt.newPages)

			if tt.expectError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestAIService_ExtractText_WithMocks(t *testing.T) {
	cfg := getTestOCRConfig()
	log := getTestOCRLogger()
	
	mockProvider := &MockAIProvider{}
	service := &AIService{
		config:   cfg,
		logger:   log,
		provider: mockProvider,
	}

	tests := []struct {
		name          string
		imagePath     string
		mockResult    *AITextResult
		mockError     error
		expectError   bool
		expectedText  string
		expectedCost  float64
	}{
		{
			name:      "successful_text_extraction",
			imagePath: "test.png",
			mockResult: &AITextResult{
				Text:             "Extracted text from image",
				Confidence:       0.95,
				Language:         "spa",
				TokensUsed:       50,
				ProcessingTimeMs: 1500,
				Cost:             0.02,
			},
			mockError:    nil,
			expectError:  false,
			expectedText: "Extracted text from image",
			expectedCost: 0.02,
		},
		{
			name:        "ai_provider_error",
			imagePath:   "test.png",
			mockResult:  nil,
			mockError:   assert.AnError,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			
			// Setup mock expectations
			mockProvider.On("ExtractTextFromImage", ctx, tt.imagePath).Return(tt.mockResult, tt.mockError)

			result, err := service.ExtractText(ctx, tt.imagePath)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedText, result.Text)
				assert.Equal(t, tt.expectedCost, result.Cost)
				assert.Equal(t, tt.imagePath, result.ImagePath)
			}

			mockProvider.AssertExpectations(t)
		})
	}
}

func TestAIService_ExtractStructuredData_WithMocks(t *testing.T) {
	cfg := getTestOCRConfig()
	log := getTestOCRLogger()
	
	mockProvider := &MockAIProvider{}
	service := &AIService{
		config:   cfg,
		logger:   log,
		provider: mockProvider,
	}

	tests := []struct {
		name         string
		imagePath    string
		prompt       string
		mockResult   *AIStructuredResult
		mockError    error
		expectError  bool
		expectedData map[string]interface{}
	}{
		{
			name:      "successful_invoice_extraction",
			imagePath: "invoice.png",
			prompt:    "Extract invoice data",
			mockResult: &AIStructuredResult{
				Data: map[string]interface{}{
					"total":        "1250.50",
					"currency":     "USD",
					"invoice_number": "INV-2024-001",
				},
				Confidence:       0.92,
				TokensUsed:       75,
				ProcessingTimeMs: 2000,
				Cost:             0.035,
			},
			mockError:   nil,
			expectError: false,
			expectedData: map[string]interface{}{
				"total":          "1250.50",
				"currency":       "USD",
				"invoice_number": "INV-2024-001",
			},
		},
		{
			name:        "extraction_error",
			imagePath:   "invoice.png",
			prompt:      "Extract invoice data",
			mockResult:  nil,
			mockError:   assert.AnError,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			
			mockProvider.On("ExtractStructuredData", ctx, tt.imagePath, tt.prompt).Return(tt.mockResult, tt.mockError)

			result, err := service.ExtractStructuredData(ctx, tt.imagePath, tt.prompt)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.Equal(t, tt.expectedData, result.Data)
				assert.Equal(t, tt.imagePath, result.ImagePath)
			}

			mockProvider.AssertExpectations(t)
		})
	}
}

func TestCalculateAICost(t *testing.T) {
	tests := []struct {
		name         string
		tokensUsed   int
		expectedCost float64
	}{
		{
			name:         "minimal_tokens",
			tokensUsed:   100,
			expectedCost: 0.00015, // 100 * 0.0000015
		},
		{
			name:         "moderate_tokens",
			tokensUsed:   1000,
			expectedCost: 0.0015, // 1000 * 0.0000015
		},
		{
			name:         "high_tokens",
			tokensUsed:   10000,
			expectedCost: 0.015, // 10000 * 0.0000015
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cost := CalculateAICost(tt.tokensUsed)
			assert.InDelta(t, tt.expectedCost, cost, 0.000001)
		})
	}
}

func TestGetAIPromptForDocumentType(t *testing.T) {
	tests := []struct {
		name         string
		docType      string
		expectedKeys []string
	}{
		{
			name:    "invoice_prompt",
			docType: "invoice",
			expectedKeys: []string{
				"número de factura", "fecha", "total", "moneda",
				"proveedor", "cliente", "productos", "impuestos",
			},
		},
		{
			name:    "id_document_prompt",
			docType: "id_document",
			expectedKeys: []string{
				"nombre", "apellidos", "documento", "fecha_nacimiento",
				"fecha_expedicion", "fecha_vencimiento",
			},
		},
		{
			name:    "receipt_prompt",
			docType: "receipt",
			expectedKeys: []string{
				"establecimiento", "fecha", "total", "productos",
				"método de pago", "número de transacción",
			},
		},
		{
			name:    "general_prompt",
			docType: "unknown",
			expectedKeys: []string{
				"texto completo", "información estructurada",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prompt := GetAIPromptForDocumentType(tt.docType)
			
			assert.NotEmpty(t, prompt)
			for _, key := range tt.expectedKeys {
				assert.Contains(t, prompt, key)
			}
		})
	}
}

// Tests de integración con archivos reales
func TestIntegration_ClassicOCR_RealFiles(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	cfg := getTestOCRConfig()
	log := getTestOCRLogger()
	service := NewClassicService(cfg, log)

	testFiles := []string{
		"../../testdata/ocr/classic/clean_document.png",
		"../../testdata/ocr/classic/multilingual.png",
	}

	for _, file := range testFiles {
		t.Run("real_file_"+file, func(t *testing.T) {
			// Skip si el archivo no existe
			if _, err := os.Stat(file); os.IsNotExist(err) {
				t.Skip("Archivo de test no existe:", file)
			}

			ctx := context.Background()
			result, err := service.ExtractText(ctx, file, "spa")

			assert.NoError(t, err)
			assert.NotNil(t, result)
			assert.Greater(t, result.ProcessingTimeMs, 0)
			// No validamos el texto específico ya que depende de Tesseract
		})
	}
}

// Benchmarks para OCR
func BenchmarkClassicOCR_ExtractText(b *testing.B) {
	cfg := getTestOCRConfig()
	log := getTestOCRLogger()
	service := NewClassicService(cfg, log)

	imagePath := "../../testdata/ocr/classic/clean_document.png"
	
	// Skip si el archivo no existe
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		b.Skip("Archivo de test no existe:", imagePath)
	}

	ctx := context.Background()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		service.ExtractText(ctx, imagePath, "spa")
	}
}

func BenchmarkCalculateAICost(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalculateAICost(1000)
	}
}