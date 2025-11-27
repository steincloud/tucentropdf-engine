package handlers

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/internal/storage"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// setupTestHandlers crea handlers para testing
func setupTestHandlers(t *testing.T) *Handlers {
	cfg := &config.Config{
		Environment:  "test",
		EngineSecret: "test-secret-key-for-testing-purposes-12345",
		Storage: config.StorageConfig{
			TempDir:         t.TempDir(),
			CleanupInterval: 0, // Disable cleanup in tests
		},
	}

	log := logger.New("info", "console")
	store := storage.NewService(cfg, log)

	return New(cfg, log, store)
}

// setupTestApp crea app Fiber para testing
func setupTestApp(handlers *Handlers) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			return c.Status(500).JSON(fiber.Map{
				"error": err.Error(),
			})
		},
	})

	return app
}

// TestGetHealth verifica endpoint de health check
func TestGetHealth(t *testing.T) {
	h := setupTestHandlers(t)
	app := setupTestApp(h)

	app.Get("/health", h.GetHealth)

	req := httptest.NewRequest("GET", "/health", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.True(t, result["success"].(bool))
	data := result["data"].(map[string]interface{})
	assert.Equal(t, "healthy", data["status"])
	assert.Equal(t, "2.0.0", data["version"])
}

// TestGetInfo verifica endpoint de información
func TestGetInfo(t *testing.T) {
	h := setupTestHandlers(t)
	app := setupTestApp(h)

	app.Get("/info", h.GetInfo)

	req := httptest.NewRequest("GET", "/info", nil)
	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode)

	var result map[string]interface{}
	err = json.NewDecoder(resp.Body).Decode(&result)
	require.NoError(t, err)

	assert.True(t, result["success"].(bool))
	data := result["data"].(map[string]interface{})
	assert.Equal(t, "TuCentroPDF Engine V2", data["name"])
	assert.Equal(t, "2.0.0", data["version"])
}

// TestMergePDF verifica merge de PDFs
func TestMergePDF(t *testing.T) {
	h := setupTestHandlers(t)
	app := setupTestApp(h)

	// Middleware simulado de autenticación
	app.Use(func(c *fiber.Ctx) error {
		c.Locals("userID", "test-user")
		c.Locals("userPlan", "premium")
		return c.Next()
	})

	app.Post("/pdf/merge", h.MergePDF)

	// Crear PDFs de prueba
	pdf1 := createTestPDF(t, "test1.pdf")
	pdf2 := createTestPDF(t, "test2.pdf")
	defer os.Remove(pdf1)
	defer os.Remove(pdf2)

	// Crear multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Agregar archivos
	addFileToForm(t, writer, "files", pdf1)
	addFileToForm(t, writer, "files", pdf2)

	writer.Close()

	req := httptest.NewRequest("POST", "/pdf/merge", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := app.Test(req, -1) // -1 = sin timeout
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode)
}

// TestSplitPDF verifica split de PDF
func TestSplitPDF(t *testing.T) {
	h := setupTestHandlers(t)
	app := setupTestApp(h)

	app.Use(func(c *fiber.Ctx) error {
		c.Locals("userID", "test-user")
		c.Locals("userPlan", "premium")
		return c.Next()
	})

	app.Post("/pdf/split", h.SplitPDF)

	pdf := createTestPDF(t, "test.pdf")
	defer os.Remove(pdf)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	addFileToForm(t, writer, "file", pdf)
	writer.WriteField("mode", "pages")
	writer.WriteField("range", "1")
	writer.Close()

	req := httptest.NewRequest("POST", "/pdf/split", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode)
}

// TestOptimizePDF verifica optimización de PDF
func TestOptimizePDF(t *testing.T) {
	h := setupTestHandlers(t)
	app := setupTestApp(h)

	app.Use(func(c *fiber.Ctx) error {
		c.Locals("userID", "test-user")
		c.Locals("userPlan", "premium")
		return c.Next()
	})

	app.Post("/pdf/optimize", h.OptimizePDF)

	pdf := createTestPDF(t, "test.pdf")
	defer os.Remove(pdf)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	addFileToForm(t, writer, "file", pdf)
	writer.WriteField("level", "medium")
	writer.Close()

	req := httptest.NewRequest("POST", "/pdf/optimize", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode)
}

// TestRotatePDF verifica rotación de PDF
func TestRotatePDF(t *testing.T) {
	h := setupTestHandlers(t)
	app := setupTestApp(h)

	app.Use(func(c *fiber.Ctx) error {
		c.Locals("userID", "test-user")
		c.Locals("userPlan", "premium")
		return c.Next()
	})

	app.Post("/pdf/rotate", h.RotatePDF)

	pdf := createTestPDF(t, "test.pdf")
	defer os.Remove(pdf)

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	addFileToForm(t, writer, "file", pdf)
	writer.WriteField("angle", "90")
	writer.Close()

	req := httptest.NewRequest("POST", "/pdf/rotate", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := app.Test(req, -1)
	require.NoError(t, err)

	assert.Equal(t, 200, resp.StatusCode)
	
	// Leer respuesta para liberar el archivo
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	
	// Esperar breve para que el sistema operativo libere el archivo
	time.Sleep(100 * time.Millisecond)
}

// TestValidateCaptcha verifica validación de captcha
func TestValidateCaptcha(t *testing.T) {
	h := setupTestHandlers(t)
	h.config.Captcha = config.CaptchaConfig{
		Enabled:   true,
		SecretKey: "test-secret",
	}

	app := setupTestApp(h)
	app.Post("/captcha/validate", h.ValidateCaptcha)

	// Test con token vacío
	reqBody := map[string]string{"token": ""}
	bodyBytes, _ := json.Marshal(reqBody)

	req := httptest.NewRequest("POST", "/captcha/validate", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	require.NoError(t, err)

	assert.Equal(t, 400, resp.StatusCode)
}

// Helper functions

func createTestPDF(t *testing.T, filename string) string {
	// Crear un PDF mínimo válido
	pdfContent := `%PDF-1.4
1 0 obj
<<
/Type /Catalog
/Pages 2 0 R
>>
endobj
2 0 obj
<<
/Type /Pages
/Kids [3 0 R]
/Count 1
>>
endobj
3 0 obj
<<
/Type /Page
/Parent 2 0 R
/MediaBox [0 0 612 792]
/Contents 4 0 R
/Resources <<
/Font <<
/F1 <<
/Type /Font
/Subtype /Type1
/BaseFont /Helvetica
>>
>>
>>
>>
endobj
4 0 obj
<<
/Length 44
>>
stream
BT
/F1 12 Tf
100 700 Td
(Test PDF) Tj
ET
endstream
endobj
xref
0 5
0000000000 65535 f
0000000009 00000 n
0000000058 00000 n
0000000115 00000 n
0000000317 00000 n
trailer
<<
/Size 5
/Root 1 0 R
>>
startxref
410
%%EOF`

	tempDir := t.TempDir()
	path := filepath.Join(tempDir, filename)
	err := os.WriteFile(path, []byte(pdfContent), 0644)
	require.NoError(t, err)

	return path
}

func addFileToForm(t *testing.T, writer *multipart.Writer, fieldName, filePath string) {
	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer file.Close()

	part, err := writer.CreateFormFile(fieldName, filepath.Base(filePath))
	require.NoError(t, err)

	_, err = io.Copy(part, file)
	require.NoError(t, err)
}
