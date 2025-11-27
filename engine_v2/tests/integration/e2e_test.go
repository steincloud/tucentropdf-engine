package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	baseURL = "http://localhost:8080"
	apiKey  = "test-api-key"
)

// TestE2E_PDFWorkflow test completo de workflow PDF
func TestE2E_PDFWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	// 1. Health check
	t.Run("Health Check", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/v2/health")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		assert.True(t, result["success"].(bool))
	})

	// 2. Obtener información
	t.Run("Get Info", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/v1/info")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		
		data := result["data"].(map[string]interface{})
		assert.Equal(t, "TuCentroPDF Engine V2", data["name"])
	})

	// 3. Upload y merge PDFs
	t.Run("Merge PDFs", func(t *testing.T) {
		pdf1 := createTestPDFFile(t, "merge1.pdf")
		pdf2 := createTestPDFFile(t, "merge2.pdf")
		defer os.Remove(pdf1)
		defer os.Remove(pdf2)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		addFileToMultipart(t, writer, "files", pdf1)
		addFileToMultipart(t, writer, "files", pdf2)
		writer.Close()

		req, err := http.NewRequest("POST", baseURL+"/api/v1/pdf/merge", body)
		require.NoError(t, err)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("X-API-Key", apiKey)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)
	})

	// 4. Split PDF
	t.Run("Split PDF", func(t *testing.T) {
		pdf := createTestPDFFile(t, "split.pdf")
		defer os.Remove(pdf)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		addFileToMultipart(t, writer, "file", pdf)
		writer.WriteField("mode", "pages")
		writer.WriteField("range", "1")
		writer.Close()

		req, err := http.NewRequest("POST", baseURL+"/api/v1/pdf/split", body)
		require.NoError(t, err)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("X-API-Key", apiKey)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)
	})

	// 5. Optimize PDF
	t.Run("Optimize PDF", func(t *testing.T) {
		pdf := createTestPDFFile(t, "optimize.pdf")
		defer os.Remove(pdf)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		addFileToMultipart(t, writer, "file", pdf)
		writer.WriteField("level", "medium")
		writer.Close()

		req, err := http.NewRequest("POST", baseURL+"/api/v1/pdf/optimize", body)
		require.NoError(t, err)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("X-API-Key", apiKey)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)
	})

	// 6. Rotate PDF
	t.Run("Rotate PDF", func(t *testing.T) {
		pdf := createTestPDFFile(t, "rotate.pdf")
		defer os.Remove(pdf)

		body := &bytes.Buffer{}
		writer := multipart.NewWriter(body)

		addFileToMultipart(t, writer, "file", pdf)
		writer.WriteField("angle", "90")
		writer.Close()

		req, err := http.NewRequest("POST", baseURL+"/api/v1/pdf/rotate", body)
		require.NoError(t, err)
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("X-API-Key", apiKey)

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)
	})
}

// TestE2E_Authentication test de autenticación
func TestE2E_Authentication(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	t.Run("Sin API Key", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/v1/pdf/merge")
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 401, resp.StatusCode)
	})

	t.Run("Con API Key inválida", func(t *testing.T) {
		req, err := http.NewRequest("GET", baseURL+"/api/v1/pdf/merge", nil)
		require.NoError(t, err)
		req.Header.Set("X-API-Key", "invalid-key")

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 401, resp.StatusCode)
	})
}

// TestE2E_RateLimiting test de rate limiting
func TestE2E_RateLimiting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	t.Run("Múltiples requests rápidos", func(t *testing.T) {
		client := &http.Client{}
		successCount := 0
		rateLimitedCount := 0

		for i := 0; i < 100; i++ {
			req, _ := http.NewRequest("GET", baseURL+"/api/v1/info", nil)
			req.Header.Set("X-API-Key", apiKey)

			resp, err := client.Do(req)
			if err != nil {
				continue
			}

			if resp.StatusCode == 200 {
				successCount++
			} else if resp.StatusCode == 429 {
				rateLimitedCount++
			}
			resp.Body.Close()
		}

		t.Logf("Success: %d, Rate Limited: %d", successCount, rateLimitedCount)
	})
}

// TestE2E_StorageEndpoints test de endpoints de storage
func TestE2E_StorageEndpoints(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	t.Run("List Files", func(t *testing.T) {
		req, err := http.NewRequest("GET", baseURL+"/api/v2/storage/files", nil)
		require.NoError(t, err)
		req.Header.Set("X-API-Key", apiKey)

		client := &http.Client{}
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		assert.Equal(t, 200, resp.StatusCode)
	})
}

// Helper functions

func createTestPDFFile(t *testing.T, filename string) string {
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
(Integration Test) Tj
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

	tempDir := os.TempDir()
	path := filepath.Join(tempDir, filename)
	err := os.WriteFile(path, []byte(pdfContent), 0644)
	require.NoError(t, err)

	return path
}

func addFileToMultipart(t *testing.T, writer *multipart.Writer, fieldName, filePath string) {
	file, err := os.Open(filePath)
	require.NoError(t, err)
	defer file.Close()

	part, err := writer.CreateFormFile(fieldName, filepath.Base(filePath))
	require.NoError(t, err)

	_, err = io.Copy(part, file)
	require.NoError(t, err)
}
