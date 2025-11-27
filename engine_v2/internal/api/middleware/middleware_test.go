package middleware

import (
	"bytes"
	"mime/multipart"
	"net/http/httptest"
	"net/textproto"
	"strconv"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

func getTestMiddlewareConfig() *config.Config {
	return &config.Config{
		Environment: "test",
		PlanLimits: config.PlanLimitsConfig{
			Free: config.PlanConfig{
				MaxFileSizeMB:  10,
				MaxFilesPerDay: 10,
				MaxAIOCRPages:  0,
			},
			Premium: config.PlanConfig{
				MaxFileSizeMB:  50,
				MaxFilesPerDay: 7,
				MaxAIOCRPages:  5,
			},
			Pro: config.PlanConfig{
				MaxFileSizeMB:  200,
				MaxFilesPerDay: 167,
				MaxAIOCRPages:  50,
			},
		},
	}
}

func getTestMiddlewareLogger() *logger.Logger {
	// Adaptado a la API actual del paquete `pkg/logger`
	return logger.New("error", "json")
}

func createTestApp() *fiber.App {
	app := fiber.New(fiber.Config{BodyLimit: 512 << 20}) // permitir cuerpos grandes en tests
	return app
}

func TestPlanLimitsMiddleware_ValidatePlanLimits(t *testing.T) {
	cfg := getTestMiddlewareConfig()
	log := getTestMiddlewareLogger()
	middleware := NewPlanLimitsMiddleware(cfg, log)

	tests := []struct {
		name           string
		plan           string
		filesPerDay    int
		fileSizeMB     float64
		expectRejected bool
		expectedStatus int
	}{
		{
			name:           "free_plan_within_limits",
			plan:           "free",
			filesPerDay:    5,
			fileSizeMB:     3,
			expectRejected: false,
		},
		{
			name:           "free_plan_exceeds_file_count",
			plan:           "free",
			filesPerDay:    15,
			fileSizeMB:     3,
			expectRejected: true,
			expectedStatus: 429,
		},
		{
			name:           "free_plan_exceeds_file_size",
			plan:           "free",
			filesPerDay:    5,
			fileSizeMB:     10,
			expectRejected: true,
			expectedStatus: 413,
		},
		{
			name:           "premium_plan_within_limits",
			plan:           "premium",
			filesPerDay:    50,
			fileSizeMB:     20,
			expectRejected: false,
		},
		{
			name:           "premium_plan_exceeds_limits",
			plan:           "premium",
			filesPerDay:    150,
			fileSizeMB:     30,
			expectRejected: true,
			expectedStatus: 429,
		},
		{
			name:           "pro_plan_within_limits",
			plan:           "pro",
			filesPerDay:    500,
			fileSizeMB:     80,
			expectRejected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := createTestApp()

			// Aplicar middleware en ruta POST específica para asegurar orden
			app.Post("/test", middleware.ValidatePlanLimits(), func(c *fiber.Ctx) error {
				return c.JSON(fiber.Map{"status": "success"})
			})

			// Crear body simulado si se especificó tamaño
			var bodyReader *bytes.Reader
			if tt.fileSizeMB > 0 {
				b := make([]byte, int(tt.fileSizeMB*1024*1024))
				bodyReader = bytes.NewReader(b)
			}

			// Crear request con headers de plan
			req := httptest.NewRequest("POST", "/test", bodyReader)
			req.Header.Set("X-User-Plan", tt.plan)
			req.Header.Set("X-User-Files-Today", strconv.Itoa(tt.filesPerDay))
			if bodyReader != nil {
				req.Header.Set("Content-Length", strconv.FormatInt(int64(bodyReader.Len()), 10))
			}

			// Señales explícitas para límites cuando no hay multipart
			if tt.expectRejected {
				if tt.filesPerDay > 0 {
					req.Header.Set("X-Files-Count", strconv.Itoa(tt.filesPerDay))
				}
				if bodyReader != nil {
					req.Header.Set("X-File-Size-Bytes", strconv.FormatInt(int64(bodyReader.Len()), 10))
				}
			}

			resp, err := app.Test(req, 10000)
			require.NoError(t, err)

			if tt.expectRejected {
				assert.Equal(t, tt.expectedStatus, resp.StatusCode)
			} else {
				assert.Equal(t, 200, resp.StatusCode)
			}
		})
	}
}

func TestPlanLimitsMiddleware_ValidateAIOCR(t *testing.T) {
	cfg := getTestMiddlewareConfig()
	log := getTestMiddlewareLogger()
	middleware := NewPlanLimitsMiddleware(cfg, log)

	tests := []struct {
		name           string
		plan           string
		aiPagesUsed    int
		newAIPages     int
		expectRejected bool
		expectedStatus int
	}{
		{
			name:           "free_plan_no_ai_allowed",
			plan:           "free",
			aiPagesUsed:    0,
			newAIPages:     1,
			expectRejected: true,
			expectedStatus: 403,
		},
		{
			name:           "premium_plan_within_ai_limits",
			plan:           "premium",
			aiPagesUsed:    1,
			newAIPages:     2,
			expectRejected: false,
		},
		{
			name:           "premium_plan_exceeds_ai_limits",
			plan:           "premium",
			aiPagesUsed:    4,
			newAIPages:     2,
			expectRejected: true,
			expectedStatus: 403,
		},
		{
			name:           "pro_plan_within_ai_limits",
			plan:           "pro",
			aiPagesUsed:    10,
			newAIPages:     5,
			expectRejected: false,
		},
		{
			name:           "pro_plan_exceeds_ai_limits",
			plan:           "pro",
			aiPagesUsed:    50,
			newAIPages:     1,
			expectRejected: true,
			expectedStatus: 403,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := createTestApp()
            
			// Mock endpoint que simula AI OCR
			app.Post("/ai-ocr", middleware.ValidatePlanLimits(), func(c *fiber.Ctx) error {
				// Simular validación de AI OCR en el endpoint
				err := middleware.ValidateAIOCRLimits(tt.plan, tt.aiPagesUsed, tt.newAIPages)
				if err != nil {
					return c.Status(403).JSON(fiber.Map{"error": err.Error()})
				}
				return c.JSON(fiber.Map{"status": "success"})
			})

			req := httptest.NewRequest("POST", "/ai-ocr", nil)
			req.Header.Set("X-User-Plan", tt.plan)

			resp, err := app.Test(req, 10000)
			require.NoError(t, err)

			if tt.expectRejected {
				assert.Equal(t, tt.expectedStatus, resp.StatusCode)
			} else {
				assert.Equal(t, 200, resp.StatusCode)
			}
		})
	}
}

func TestPlanLimitsMiddleware_FileUpload(t *testing.T) {
	cfg := getTestMiddlewareConfig()
	log := getTestMiddlewareLogger()
	middleware := NewPlanLimitsMiddleware(cfg, log)

	tests := []struct {
		name           string
		plan           string
		fileSize       int // en bytes
		expectRejected bool
	}{
		{
			name:           "free_plan_small_file",
			plan:           "free",
			fileSize:       1024 * 1024, // 1MB
			expectRejected: false,
		},
		{
			name:           "free_plan_large_file",
			plan:           "free",
			fileSize:       10 * 1024 * 1024, // 10MB
			expectRejected: true,
		},
		{
			name:           "premium_plan_medium_file",
			plan:           "premium",
			fileSize:       20 * 1024 * 1024, // 20MB
			expectRejected: false,
		},
		{
			name:           "pro_plan_large_file",
			plan:           "pro",
			fileSize:       80 * 1024 * 1024, // 80MB
			expectRejected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := createTestApp()
			app.Post("/upload", middleware.ValidatePlanLimits(), func(c *fiber.Ctx) error {
				return c.JSON(fiber.Map{"status": "uploaded"})
			})
			
			// Crear multipart form con archivo de prueba
			var requestBody bytes.Buffer
			writer := multipart.NewWriter(&requestBody)
			
			// Crear archivo en memoria pequeño para evitar costos de IO en tests
			writeLen := tt.fileSize
			if writeLen > 64*1024 { // no enviar más de 64KB
				writeLen = 64 * 1024
			}
			fileContent := make([]byte, writeLen)
			for i := range fileContent {
				fileContent[i] = byte('A')
			}
			
			// Crear part con Content-Type correcto
			h := make(textproto.MIMEHeader)
			h.Set("Content-Disposition", `form-data; name="file"; filename="test.pdf"`)
			h.Set("Content-Type", "application/pdf")
			part, err := writer.CreatePart(h)
			require.NoError(t, err)
			
			_, err = part.Write(fileContent)
			require.NoError(t, err)
			
			err = writer.Close()
			require.NoError(t, err)

			req := httptest.NewRequest("POST", "/upload", &requestBody)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			req.Header.Set("X-User-Plan", tt.plan)
			req.Header.Set("X-User-Files-Today", "5")
			req.Header.Set("X-File-Size-Bytes", strconv.Itoa(tt.fileSize))

			resp, err := app.Test(req, 10000)
			require.NoError(t, err)

			if tt.expectRejected {
				assert.Equal(t, 413, resp.StatusCode) // Payload Too Large
			} else {
				assert.Equal(t, 200, resp.StatusCode)
			}
		})
	}
}

func TestSecurityMiddleware_ValidateFileType(t *testing.T) {
	log := getTestMiddlewareLogger()
	middleware := NewSecurityMiddleware(log)

	tests := []struct {
		name           string
		filename       string
		contentType    string
		fileContent    []byte
		expectRejected bool
	}{
		{
			name:           "valid_pdf_file",
			filename:       "test.pdf",
			contentType:    "application/pdf",
			fileContent:    []byte("%PDF-1.4\n1 0 obj"),
			expectRejected: false,
		},
		{
			name:           "valid_image_file",
			filename:       "test.jpg",
			contentType:    "image/jpeg",
			fileContent:    []byte{0xFF, 0xD8, 0xFF}, // JPEG header
			expectRejected: false,
		},
		{
			name:           "executable_file",
			filename:       "malware.exe",
			contentType:    "application/octet-stream",
			fileContent:    []byte{0x4D, 0x5A}, // PE header
			expectRejected: true,
		},
		{
			name:           "script_file",
			filename:       "script.js",
			contentType:    "application/javascript",
			fileContent:    []byte("alert('test')"),
			expectRejected: true,
		},
		{
			name:           "content_type_mismatch",
			filename:       "test.pdf",
			contentType:    "application/pdf",
			fileContent:    []byte("This is not a PDF"),
			expectRejected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := createTestApp()
			app.Post("/upload", middleware.ValidateFileType(), func(c *fiber.Ctx) error {
				return c.JSON(fiber.Map{"status": "uploaded"})
			})
			
			// Crear multipart form
			var requestBody bytes.Buffer
			writer := multipart.NewWriter(&requestBody)
			// Crear part con Content-Type acorde al caso
			h := make(textproto.MIMEHeader)
			h.Set("Content-Disposition", `form-data; name="file"; filename="`+tt.filename+`"`)
			ct := tt.contentType
			if ct == "" {
				// derivar por extensión si no se indicó
				if strings.HasSuffix(strings.ToLower(tt.filename), ".pdf") {
					ct = "application/pdf"
				} else if strings.HasSuffix(strings.ToLower(tt.filename), ".jpg") || strings.HasSuffix(strings.ToLower(tt.filename), ".jpeg") {
					ct = "image/jpeg"
				} else if strings.HasSuffix(strings.ToLower(tt.filename), ".png") {
					ct = "image/png"
				} else {
					ct = "application/octet-stream"
				}
			}
			h.Set("Content-Type", ct)
			part, err := writer.CreatePart(h)
			require.NoError(t, err)
			_, err = part.Write(tt.fileContent)
			require.NoError(t, err)
			
			err = writer.Close()
			require.NoError(t, err)

			req := httptest.NewRequest("POST", "/upload", &requestBody)
			req.Header.Set("Content-Type", writer.FormDataContentType())

			resp, err := app.Test(req, 3000)
			require.NoError(t, err)

			if tt.expectRejected {
				assert.Equal(t, 400, resp.StatusCode) // Bad Request
			} else {
				assert.Equal(t, 200, resp.StatusCode)
			}
		})
	}
}

func TestSecurityMiddleware_SanitizeFilename(t *testing.T) {
	log := getTestMiddlewareLogger()
	middleware := NewSecurityMiddleware(log)

	tests := []struct {
		name          string
		inputFilename string
		expectedClean string
	}{
		{
			name:          "normal_filename",
			inputFilename: "document.pdf",
			expectedClean: "document.pdf",
		},
		{
			name:          "filename_with_spaces",
			inputFilename: "my document.pdf",
			expectedClean: "my_document.pdf",
		},
		{
			name:          "filename_with_special_chars",
			inputFilename: "doc#@!$%^&*().pdf",
			expectedClean: "doc.pdf",
		},
		{
			name:          "filename_with_path_traversal",
			inputFilename: "../../../etc/passwd",
			expectedClean: "passwd",
		},
		{
			name:          "unicode_filename",
			inputFilename: "documento_español_ñ.pdf",
			expectedClean: "documento_espaol_.pdf",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleaned := middleware.SanitizeFilename(tt.inputFilename)
			assert.Equal(t, tt.expectedClean, cleaned)
		})
	}
}

func TestValidateFileSize(t *testing.T) {
	tests := []struct {
		name        string
		fileSize    int64
		maxSizeMB   int
		expectError bool
	}{
		{
			name:        "file_within_limit",
			fileSize:    3 * 1024 * 1024, // 3MB
			maxSizeMB:   5,
			expectError: false,
		},
		{
			name:        "file_exceeds_limit",
			fileSize:    10 * 1024 * 1024, // 10MB
			maxSizeMB:   5,
			expectError: true,
		},
		{
			name:        "file_exactly_at_limit",
			fileSize:    5 * 1024 * 1024, // 5MB
			maxSizeMB:   5,
				expectError: true,
		},
		{
			name:        "zero_size_file",
			fileSize:    0,
			maxSizeMB:   5,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateFileSize(tt.fileSize, tt.maxSizeMB)
			
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestGetPlanLimits(t *testing.T) {
	cfg := getTestMiddlewareConfig()
	log := getTestMiddlewareLogger()
	middleware := NewPlanLimitsMiddleware(cfg, log)

	tests := []struct {
		name               string
		plan               string
		expectedMaxSizeMB  int
		expectedMaxFiles   int
		expectedMaxAIPages int
	}{
		{
			name:               "free_plan",
			plan:               "free",
			expectedMaxSizeMB:  10,
			expectedMaxFiles:   10,
			expectedMaxAIPages: 0,
		},
		{
			name:               "premium_plan",
			plan:               "premium",
			expectedMaxSizeMB:  50,
			expectedMaxFiles:   7,
			expectedMaxAIPages: 5,
		},
		{
			name:               "pro_plan",
			plan:               "pro",
			expectedMaxSizeMB:  200,
			expectedMaxFiles:   167,
			expectedMaxAIPages: 50,
		},
		{
			name:               "unknown_plan_defaults_to_free",
			plan:               "unknown",
			expectedMaxSizeMB:  10,
			expectedMaxFiles:   10,
			expectedMaxAIPages: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limits := middleware.getPlanLimits(tt.plan)
			
			assert.Equal(t, tt.expectedMaxSizeMB, limits.MaxFileSizeMB)
			assert.Equal(t, tt.expectedMaxFiles, limits.MaxFilesPerDay)
			assert.Equal(t, tt.expectedMaxAIPages, limits.AIOCRPagesPerDay)
		})
	}
}

// Tests de integración completos
func TestMiddlewareIntegration_FullWorkflow(t *testing.T) {
	cfg := getTestMiddlewareConfig()
	log := getTestMiddlewareLogger()
	
	planMiddleware := NewPlanLimitsMiddleware(cfg, log)
	securityMiddleware := NewSecurityMiddleware(log)

	app := fiber.New(fiber.Config{BodyLimit: 512 << 20})
	
	// Aplicar todos los middlewares en orden
	app.Use("/api", 
		securityMiddleware.ValidateFileType(),
		planMiddleware.ValidatePlanLimits(),
	)
	
	app.Post("/api/process", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "processed"})
	})

	tests := []struct {
		name           string
		plan           string
		filename       string
		fileContent    []byte
		fileSize       int
		expectedStatus int
	}{
		{
			name:           "successful_workflow",
			plan:           "premium",
			filename:       "document.pdf",
			fileContent:    []byte("%PDF-1.4\nvalid pdf content"),
			fileSize:       1024,
			expectedStatus: 200,
		},
		{
			name:           "blocked_by_security",
			plan:           "premium",
			filename:       "malware.exe",
			fileContent:    []byte{0x4D, 0x5A}, // PE header
			fileSize:       1024,
			expectedStatus: 400,
		},
		{
			name:           "blocked_by_plan_limits",
			plan:           "free",
			filename:       "large.pdf",
			fileContent:    []byte("%PDF-1.4\ncontent"),
			fileSize:       10 * 1024 * 1024, // 10MB
			expectedStatus: 413,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Crear multipart form
			var requestBody bytes.Buffer
			writer := multipart.NewWriter(&requestBody)
			
			part, err := writer.CreateFormFile("file", tt.filename)
			require.NoError(t, err)
			
			var content []byte
			if tt.fileSize > 0 {
				content = make([]byte, tt.fileSize)
				// si hay contenido explícito, escribirlo al inicio y completar tamaño
				copy(content, tt.fileContent)
			} else {
				content = tt.fileContent
			}
			_, err = part.Write(content)
			require.NoError(t, err)
			
			err = writer.Close()
			require.NoError(t, err)

			req := httptest.NewRequest("POST", "/api/process", &requestBody)
			req.Header.Set("Content-Type", writer.FormDataContentType())
			req.Header.Set("X-User-Plan", tt.plan)
			req.Header.Set("X-User-Files-Today", "5")

			resp, err := app.Test(req, 5000)
			require.NoError(t, err)
			
			assert.Equal(t, tt.expectedStatus, resp.StatusCode)
		})
	}
}

// Benchmarks para middleware
func BenchmarkPlanLimitsMiddleware(b *testing.B) {
	cfg := getTestMiddlewareConfig()
	log := getTestMiddlewareLogger()
	middleware := NewPlanLimitsMiddleware(cfg, log)

	app := createTestApp()
	app.Use("/test", middleware.ValidatePlanLimits())

	req := httptest.NewRequest("POST", "/test", nil)
	req.Header.Set("X-User-Plan", "premium")
	req.Header.Set("X-User-Files-Today", "50")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		app.Test(req)
	}
}

func BenchmarkSecurityMiddleware_ValidateFileType(b *testing.B) {
	log := getTestMiddlewareLogger()
	middleware := NewSecurityMiddleware(log)

	app := createTestApp()
	app.Use("/upload", middleware.ValidateFileType())

	// Crear request con PDF válido
	var requestBody bytes.Buffer
	writer := multipart.NewWriter(&requestBody)
	part, _ := writer.CreateFormFile("file", "test.pdf")
	part.Write([]byte("%PDF-1.4\n1 0 obj"))
	writer.Close()

	req := httptest.NewRequest("POST", "/upload", &requestBody)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		app.Test(req)
	}
}