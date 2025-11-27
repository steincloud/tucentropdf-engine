package utils

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidateMimeType verifica validación de MIME types
func TestValidateMimeType(t *testing.T) {
	tests := []struct {
		name      string
		mimeType  string
		category  string
		expectErr bool
	}{
		// PDF tests
		{"pdf válido", "application/pdf", CategoryPDF, false},
		{"pdf inválido", "text/plain", CategoryPDF, true},
		
		// Image tests
		{"jpeg válido", "image/jpeg", CategoryImage, false},
		{"png válido", "image/png", CategoryImage, false},
		{"gif válido", "image/gif", CategoryImage, false},
		{"image inválido", "application/pdf", CategoryImage, true},
		
		// Office tests
		{"docx válido", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", CategoryOffice, false},
		{"xlsx válido", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", CategoryOffice, false},
		{"txt válido", "text/plain", CategoryOffice, false},
		{"office inválido", "image/jpeg", CategoryOffice, true},
		
		// All category
		{"pdf en all", "application/pdf", CategoryAll, false},
		{"image en all", "image/jpeg", CategoryAll, false},
		{"evil en all", "application/evil", CategoryAll, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateMimeType(tt.mimeType, tt.category)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestIsAllowedMimeType verifica si MIME type está permitido
func TestIsAllowedMimeType(t *testing.T) {
	tests := []struct {
		mimeType string
		expected bool
	}{
		{"application/pdf", true},
		{"image/jpeg", true},
		{"image/png", true},
		{"text/plain", true},
		{"application/msword", true},
		{"application/evil", false},
		{"text/html", false},
		{"application/x-executable", false},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			result := IsAllowedMimeType(tt.mimeType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDetectMimeTypeFromBytes verifica detección desde bytes
func TestDetectMimeTypeFromBytes(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		filename string
		expected string
	}{
		{
			name:     "PDF",
			data:     []byte("%PDF-1.4\n"),
			filename: "test.pdf",
			expected: "application/pdf",
		},
		{
			name:     "JPEG",
			data:     []byte{0xFF, 0xD8, 0xFF, 0xE0},
			filename: "test.jpg",
			expected: "image/jpeg",
		},
		{
			name:     "PNG",
			data:     []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A},
			filename: "test.png",
			expected: "image/png",
		},
		{
			name:     "texto plano",
			data:     []byte("Hello World"),
			filename: "test.txt",
			expected: "text/plain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DetectMimeTypeFromBytes(tt.data, tt.filename)
			assert.Contains(t, result, tt.expected)
		})
	}
}

// TestValidateFileExtensionMatch verifica consistencia extensión/MIME
func TestValidateFileExtensionMatch(t *testing.T) {
	tests := []struct {
		name      string
		mimeType  string
		filename  string
		expectErr bool
	}{
		{"pdf correcto", "application/pdf", "file.pdf", false},
		{"jpeg correcto", "image/jpeg", "file.jpg", false},
		{"png correcto", "image/png", "file.png", false},
		{"mismatch pdf", "image/jpeg", "file.pdf", true},
		{"mismatch jpg", "application/pdf", "file.jpg", true},
		{"jpg/jpeg alias", "image/jpg", "file.jpeg", false},
		{"jpeg/jpg alias", "image/jpeg", "file.jpg", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFileExtensionMatch(tt.mimeType, tt.filename)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestGetCategoryForMimeType verifica categorización
func TestGetCategoryForMimeType(t *testing.T) {
	tests := []struct {
		mimeType string
		expected string
	}{
		{"application/pdf", CategoryPDF},
		{"image/jpeg", CategoryImage},
		{"image/png", CategoryImage},
		{"application/msword", CategoryOffice},
		{"text/plain", CategoryOffice},
		{"application/unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			result := GetCategoryForMimeType(tt.mimeType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestDetectMimeType verifica detección desde archivo
func TestDetectMimeType(t *testing.T) {
	tempDir := t.TempDir()

	// Crear archivo PDF de prueba
	pdfPath := filepath.Join(tempDir, "test.pdf")
	pdfContent := []byte("%PDF-1.4\ntest content")
	err := os.WriteFile(pdfPath, pdfContent, 0644)
	require.NoError(t, err)

	mimeType, err := DetectMimeType(pdfPath)
	require.NoError(t, err)
	assert.Equal(t, "application/pdf", mimeType)

	// Crear archivo de texto
	txtPath := filepath.Join(tempDir, "test.txt")
	txtContent := []byte("Hello World")
	err = os.WriteFile(txtPath, txtContent, 0644)
	require.NoError(t, err)

	mimeType, err = DetectMimeType(txtPath)
	require.NoError(t, err)
	assert.Contains(t, mimeType, "text/plain")
}

// TestMimeTypeNormalization verifica normalización de MIME types
func TestMimeTypeNormalization(t *testing.T) {
	tests := []struct {
		input    string
		category string
		valid    bool
	}{
		{"application/pdf", CategoryPDF, true},
		{"application/pdf; charset=utf-8", CategoryPDF, true},
		{"APPLICATION/PDF", CategoryPDF, true},
		{"  application/pdf  ", CategoryPDF, true},
		{"image/jpeg", CategoryImage, true},
		{"image/jpeg; quality=80", CategoryImage, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			err := ValidateMimeType(tt.input, tt.category)
			if tt.valid {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
