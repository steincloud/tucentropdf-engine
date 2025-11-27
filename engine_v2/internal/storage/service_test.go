package storage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// setupTestStorage crea storage service para testing
func setupTestStorage(t *testing.T) Service {
	// Use default plan configuration to ensure limits are available in tests
	planCfg := config.GetDefaultPlanConfiguration()

	cfg := &config.Config{
		Environment: "test",
		Storage: config.StorageConfig{
			TempDir:         t.TempDir(),
			CleanupInterval: 0, // Disable cleanup in tests
		},
		Limits: config.LimitsConfig{
			Free:      planCfg.Plans[config.PlanFree],
			Premium:   planCfg.Plans[config.PlanPremium],
			Pro:       planCfg.Plans[config.PlanPro],
			Corporate: planCfg.Plans[config.PlanCorporate],
		},
	}

	log := logger.New("info", "console")
	return NewService(cfg, log)
}

// TestSaveTemp verifica guardado de datos temporales
func TestSaveTemp(t *testing.T) {
	store := setupTestStorage(t)

	data := []byte("test content")
	fileInfo, err := store.SaveTemp(data, ".txt")

	require.NoError(t, err)
	assert.NotEmpty(t, fileInfo.ID)
	assert.Equal(t, int64(len(data)), fileInfo.Size)
	assert.Equal(t, ".txt", fileInfo.Extension)
	assert.FileExists(t, fileInfo.Path)

	// Cleanup
	store.Delete(fileInfo.ID)
}

// TestGetPath verifica obtención de path
func TestGetPath(t *testing.T) {
	store := setupTestStorage(t)

	data := []byte("test")
	fileInfo, err := store.SaveTemp(data, ".txt")
	require.NoError(t, err)
	defer store.Delete(fileInfo.ID)

	path, err := store.GetPath(fileInfo.ID)
	require.NoError(t, err)
	assert.Equal(t, fileInfo.Path, path)
}

// TestDelete verifica eliminación de archivo
func TestDelete(t *testing.T) {
	store := setupTestStorage(t)

	data := []byte("test")
	fileInfo, err := store.SaveTemp(data, ".txt")
	require.NoError(t, err)

	// Verificar que existe
	assert.FileExists(t, fileInfo.Path)

	// Eliminar
	err = store.Delete(fileInfo.ID)
	require.NoError(t, err)

	// Verificar que no existe
	_, err = os.Stat(fileInfo.Path)
	assert.True(t, os.IsNotExist(err))
}

// TestValidateFile verifica validación de archivos
func TestValidateFile(t *testing.T) {
	store := setupTestStorage(t)

	// Create a temp file slightly over the Free plan limit but under Premium
	freeLimits := store.GetPlanLimits("free")
	premiumLimits := store.GetPlanLimits("premium")

	// File sizes
	smallSize := int64(1024 * 1024)                                // 1MB
	freeMax := freeLimits.MaxFileSize
	premiumMax := premiumLimits.MaxFileSize

	// Create a file under free limit (use a PDF header so MIME detection works)
	pdfHeader := []byte("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
	dataSmall := make([]byte, smallSize)
	copy(dataSmall, pdfHeader)
	fSmall, err := store.SaveTemp(dataSmall, ".pdf")
	require.NoError(t, err)
	defer store.Delete(fSmall.ID)

	// Should pass for free plan
	err = store.ValidateFile(fSmall, "free")
	assert.NoError(t, err)

	// Create a file over free limit (if freeMax > smallSize)
	overFreeSize := freeMax + (1 << 20) // freeMax + 1MB
	if overFreeSize <= premiumMax {
		dataOverFree := make([]byte, overFreeSize)
		copy(dataOverFree, pdfHeader)
		fOver, err := store.SaveTemp(dataOverFree, ".pdf")
		require.NoError(t, err)
		defer store.Delete(fOver.ID)

		// Should fail for free plan
		err = store.ValidateFile(fOver, "free")
		assert.Error(t, err)

		// Should pass for premium plan
		err = store.ValidateFile(fOver, "premium")
		assert.NoError(t, err)
	} else {
		t.Log("free plan max is already larger than premium test assumptions; skipping overFree checks")
	}
}

// TestSanitizeFilename verifica sanitización de nombres
func TestSanitizeFilename(t *testing.T) {
	store := setupTestStorage(t)

	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "normal-file.pdf",
			expected: "normal-file.pdf",
		},
		{
			input:    "file with spaces.pdf",
			expected: "file_with_spaces.pdf",
		},
		{
			input:    "../../../etc/passwd",
			expected: ".._.._.._etc_passwd",
		},
		{
			input:    "file<>:\"|?*.txt",
			expected: "file_______.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := store.SanitizeFilename(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestValidateMimeType verifica validación de MIME types
func TestValidateMimeType(t *testing.T) {
	store := setupTestStorage(t)

	tests := []struct {
		mimeType  string
		category  string
		expectErr bool
	}{
		{"application/pdf", "pdf", false},
		{"image/jpeg", "image", false},
		{"image/png", "image", false},
		{"application/msword", "office", false},
		{"application/evil", "pdf", true},
		{"text/html", "pdf", true},
	}

	for _, tt := range tests {
		t.Run(tt.mimeType, func(t *testing.T) {
			err := store.ValidateMimeType(tt.mimeType, tt.category)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestGetPlanLimits verifica obtención de límites por plan
func TestGetPlanLimits(t *testing.T) {
	store := setupTestStorage(t)

	tests := []struct {
		plan           string
		expectedMaxMB  int64
	}{
		{"free", 25},       // Default value
		{"premium", 50},
		{"pro", 100},
		{"corporate", 25},  // Falls back to default
	}

	for _, tt := range tests {
		t.Run(tt.plan, func(t *testing.T) {
			limits := store.GetPlanLimits(tt.plan)
			assert.NotNil(t, limits)
			expectedBytes := tt.expectedMaxMB * 1024 * 1024
			assert.Equal(t, expectedBytes, limits.MaxFileSize)
		})
	}
}

// TestGenerateOutputPath verifica generación de paths de salida
func TestGenerateOutputPath(t *testing.T) {
	store := setupTestStorage(t)

	filename := "output.pdf"
	path := store.GenerateOutputPath(filename)

	assert.NotEmpty(t, path)
	assert.Contains(t, path, filename)
	assert.True(t, filepath.IsAbs(path))
}

// TestReadFile verifica lectura de archivos
func TestReadFile(t *testing.T) {
	store := setupTestStorage(t)

	content := []byte("test content for reading")
	fileInfo, err := store.SaveTemp(content, ".txt")
	require.NoError(t, err)
	defer store.Delete(fileInfo.ID)

	readContent, err := store.ReadFile(fileInfo.Path)
	require.NoError(t, err)
	assert.Equal(t, content, readContent)
}

// TestDeletePath verifica eliminación por path
func TestDeletePath(t *testing.T) {
	store := setupTestStorage(t)

	content := []byte("test")
	fileInfo, err := store.SaveTemp(content, ".txt")
	require.NoError(t, err)

	assert.FileExists(t, fileInfo.Path)

	err = store.DeletePath(fileInfo.Path)
	require.NoError(t, err)

	_, err = os.Stat(fileInfo.Path)
	assert.True(t, os.IsNotExist(err))
}
