package storage

import (
	"crypto/sha256"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/internal/utils"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// Constantes de seguridad
const (
	MaxFileSize     = 100 << 20 // 100 MB
	MaxFilenameLn   = 255
	CleanupInterval = 30 * time.Minute
	FileExpiryTime  = 2 * time.Hour
)

// MIME types permitidos por categoría
var (
	PDFMimes = map[string]bool{
		"application/pdf": true,
	}
	
	ImageMimes = map[string]bool{
		"image/jpeg":    true,
		"image/jpg":     true,
		"image/png":     true,
		"image/tiff":    true,
		"image/bmp":     true,
		"image/webp":    true,
	}
	
	OfficeMimes = map[string]bool{
		"application/msword":                                                         true,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   true,
		"application/vnd.ms-excel":                                                   true,
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
		"application/vnd.ms-powerpoint":                                              true,
		"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
		"application/rtf":                                                            true,
		"text/plain":                                                                 true,
	}
)

// Expresión regular para sanitización de nombres
var filenameRegex = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

// Service interface para gestión de archivos
type Service interface {
	SaveUpload(file *multipart.FileHeader) (*FileInfo, error)
	SaveTemp(data []byte, extension string) (*FileInfo, error)
	GetPath(fileID string) (string, error)
	Delete(fileID string) error
	DeletePath(path string) error
	ReadFile(path string) ([]byte, error)
	GenerateOutputPath(filename string) string
	CleanupOld() error
	ValidateFile(fileInfo *FileInfo, plan string) error
	ValidateMimeType(mimeType string, category string) error
	SanitizeFilename(filename string) string
	GetPlanLimits(plan string) *PlanLimits
}

// PlanLimits límites por plan de usuario
type PlanLimits struct {
	MaxFileSize   int64 `json:"max_file_size"`
	MaxFiles      int   `json:"max_files"`
	AllowedMimes  []string `json:"allowed_mimes"`
	DailyUploads  int   `json:"daily_uploads"`
	StorageQuota  int64 `json:"storage_quota"`
}

// FileInfo información de archivo almacenado
type FileInfo struct {
	ID          string            `json:"id"`
	OriginalName string           `json:"original_name"`
	Path        string            `json:"path"`
	Size        int64             `json:"size"`
	MimeType    string            `json:"mime_type"`
	Extension   string            `json:"extension"`
	Hash        string            `json:"hash"`
	CreatedAt   time.Time         `json:"created_at"`
	Metadata    map[string]string `json:"metadata"`
}

// LocalStorage implementación de storage local
type LocalStorage struct {
	config    *config.Config
	logger    *logger.Logger
	tempDir   string
	cleanupMu sync.Mutex
}

// NewService crea una instancia del servicio de storage
func NewService(cfg *config.Config, log *logger.Logger) Service {
	storage := &LocalStorage{
		config:  cfg,
		logger:  log,
		tempDir: cfg.Storage.TempDir,
	}

	// Crear directorio temporal si no existe
	if err := os.MkdirAll(storage.tempDir, 0755); err != nil {
		log.Error("Failed to create temp directory", "dir", storage.tempDir, "error", err)
	}

	// Iniciar rutina de limpieza
	go storage.startCleanupRoutine()

	return storage
}

// GetTempDir returns the temporary directory path
func (s *LocalStorage) GetTempDir() string {
	return s.tempDir
}

func (s *LocalStorage) SaveUpload(file *multipart.FileHeader) (*FileInfo, error) {
	s.logger.Info("💾 Saving uploaded file",
		"filename", file.Filename,
		"size", file.Size,
	)

	// Validar espacio en disco antes de guardar
	diskChecker := utils.NewDiskSpaceChecker(s.logger)
	if err := diskChecker.CheckSpaceForFile(s.tempDir, file.Size, 20); err != nil {
		s.logger.Error("Insufficient disk space for upload", "error", err, "file_size", file.Size)
		return nil, fmt.Errorf("insufficient disk space: %w", err)
	}

	// Generar ID único
	fileID := generateFileID(file.Filename)

	// Obtener extensión
	ext := strings.ToLower(filepath.Ext(file.Filename))
	
	// Validar que el fileID sea seguro (prevenir path traversal)
	if !utils.IsValidFileID(fileID) {
		s.logger.Error("Invalid file ID generated", "file_id", fileID)
		return nil, fmt.Errorf("invalid file ID for security reasons")
	}
	
	// Crear path de destino con validación
	destPath, err := utils.SanitizeFilePath(s.tempDir, fileID+ext)
	if err != nil {
		return nil, fmt.Errorf("path sanitization failed: %w", err)
	}

	// Abrir archivo uploaded
	src, err := file.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open uploaded file: %w", err)
	}
	defer src.Close()

	// Crear archivo de destino
	dst, err := os.Create(destPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	// Copiar contenido calculando hash SHA-256
	hasher := sha256.New()
	writer := io.MultiWriter(dst, hasher)
	
	size, err := io.Copy(writer, src)
	if err != nil {
		os.Remove(destPath) // Limpiar en caso de error
		return nil, fmt.Errorf("failed to copy file: %w", err)
	}

	// Detectar MIME type real del archivo
	mimeType, err := detectMimeType(destPath)
	if err != nil {
		s.logger.Warn("Failed to detect MIME type", "file", fileID, "error", err)
		mimeType = "application/octet-stream"
	}

	// Sanitizar nombre original
	sanitizedName := s.SanitizeFilename(file.Filename)

	fileInfo := &FileInfo{
		ID:          fileID,
		OriginalName: sanitizedName,
		Path:        destPath,
		Size:        size,
		MimeType:    mimeType,
		Extension:   ext,
		Hash:        fmt.Sprintf("%x", hasher.Sum(nil)),
		CreatedAt:   time.Now(),
		Metadata:    make(map[string]string),
	}

	// Validar límites de seguridad básicos
	if err := s.validateBasicSecurity(fileInfo); err != nil {
		os.Remove(destPath)
		return nil, fmt.Errorf("security validation failed: %w", err)
	}

	s.logger.Info("✅ File saved successfully",
		"file_id", fileID,
		"size", size,
		"mime_type", mimeType,
		"hash", fileInfo.Hash[:8]+"...",
	)

	return fileInfo, nil
}

func (s *LocalStorage) SaveTemp(data []byte, extension string) (*FileInfo, error) {
	// Validar espacio en disco antes de guardar
	diskChecker := utils.NewDiskSpaceChecker(s.logger)
	if err := diskChecker.CheckSpaceForFile(s.tempDir, int64(len(data)), 20); err != nil {
		s.logger.Error("Insufficient disk space for temp file", "error", err, "data_size", len(data))
		return nil, fmt.Errorf("insufficient disk space: %w", err)
	}

	// Generar ID único
	fileID := generateTempID()
	
	if !strings.HasPrefix(extension, ".") {
		extension = "." + extension
	}

	// Crear path de destino
	destPath := filepath.Join(s.tempDir, fileID+extension)

	// Escribir datos
	err := os.WriteFile(destPath, data, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to write temp file: %w", err)
	}

	// Calcular hash SHA-256
	hasher := sha256.New()
	hasher.Write(data)

	// Detectar MIME type
	mimeType, err := detectMimeType(destPath)
	if err != nil {
		mimeType = "application/octet-stream"
	}

	fileInfo := &FileInfo{
		ID:        fileID,
		OriginalName: fmt.Sprintf("temp%s", extension),
		Path:      destPath,
		Size:      int64(len(data)),
		MimeType:  mimeType,
		Extension: extension,
		Hash:      fmt.Sprintf("%x", hasher.Sum(nil)),
		CreatedAt: time.Now(),
		Metadata:  make(map[string]string),
	}

	return fileInfo, nil
}

func (s *LocalStorage) GetPath(fileID string) (string, error) {
	// Buscar archivo en directorio temporal
	pattern := filepath.Join(s.tempDir, fileID+".*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", err
	}

	if len(matches) == 0 {
		return "", fmt.Errorf("file not found: %s", fileID)
	}

	return matches[0], nil
}

func (s *LocalStorage) Delete(fileID string) error {
	path, err := s.GetPath(fileID)
	if err != nil {
		return err
	}

	err = os.Remove(path)
	if err != nil {
		s.logger.Warn("Failed to delete file", "file_id", fileID, "error", err)
		return err
	}

	s.logger.Debug("🗑️ File deleted", "file_id", fileID)
	return nil
}

func (s *LocalStorage) CleanupOld() error {
	s.cleanupMu.Lock()
	defer s.cleanupMu.Unlock()

	s.logger.Info("🧹 Starting cleanup of old files")

	maxAge := time.Duration(s.config.Storage.MaxTempAge) * time.Second
	cutoff := time.Now().Add(-maxAge)
	
	deleted := 0
	err := filepath.Walk(s.tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if info.ModTime().Before(cutoff) {
			if err := os.Remove(path); err != nil {
				s.logger.Warn("Failed to delete old file", "path", path, "error", err)
			} else {
				deleted++
			}
		}

		return nil
	})

	if err != nil {
		s.logger.Error("Cleanup failed", "error", err)
		return err
	}

	s.logger.Info("✅ Cleanup completed", "files_deleted", deleted)
	return nil
}

func (s *LocalStorage) ValidateFile(fileInfo *FileInfo, plan string) error {
	// Obtener límites por plan
	var maxSize int64
	switch plan {
	case "free":
		maxSize = int64(s.config.Limits.Free.MaxFileSizeMB) * 1024 * 1024
	case "premium":
		maxSize = int64(s.config.Limits.Premium.MaxFileSizeMB) * 1024 * 1024
	case "pro":
		maxSize = int64(s.config.Limits.Pro.MaxFileSizeMB) * 1024 * 1024
	default:
		maxSize = int64(s.config.Limits.Free.MaxFileSizeMB) * 1024 * 1024
	}

	// Validar tamaño
	if fileInfo.Size > maxSize {
		return fmt.Errorf("file too large: %d bytes (max %d bytes for plan %s)", 
			fileInfo.Size, maxSize, plan)
	}

	// Validar MIME type
	if err := validateMimeType(fileInfo.MimeType); err != nil {
		return err
	}

	return nil
}

func (s *LocalStorage) startCleanupRoutine() {
	// Skip cleanup routine in test mode (when interval is 0)
	if s.config.Storage.CleanupInterval <= 0 {
		s.logger.Debug("Cleanup routine disabled (interval <= 0)")
		return
	}
	
	interval := time.Duration(s.config.Storage.CleanupInterval) * time.Second
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := s.CleanupOld(); err != nil {
				s.logger.Error("Scheduled cleanup failed", "error", err)
			}
		}
	}
}

// Helper functions
func generateFileID(filename string) string {
	timestamp := time.Now().Unix()
	hasher := sha256.New()
	hasher.Write([]byte(fmt.Sprintf("%s_%d", filename, timestamp)))
	return fmt.Sprintf("%x", hasher.Sum(nil))[:16]
}

func generateTempID() string {
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("temp_%d", timestamp)
}

func detectMimeType(filePath string) (string, error) {
	// Usar el detector robusto del validator
	return utils.DetectMimeType(filePath)
}

func validateMimeType(mimeType string) error {
	allowedTypes := map[string]bool{
		// PDF
		"application/pdf": true,
		
		// Images
		"image/jpeg":    true,
		"image/png":     true,
		"image/gif":     true,
		"image/bmp":     true,
		"image/tiff":    true,
		"image/webp":    true,
		
		// Microsoft Office
		"application/msword": true,
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document":     true,
		"application/vnd.ms-excel": true,
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":          true,
		"application/vnd.ms-powerpoint": true,
		"application/vnd.openxmlformats-officedocument.presentationml.presentation":  true,
		
		// OpenDocument
		"application/vnd.oasis.opendocument.text":         true,
		"application/vnd.oasis.opendocument.spreadsheet":  true,
		"application/vnd.oasis.opendocument.presentation": true,
		
		// Text
		"text/plain": true,
		"text/rtf":   true,
	}

	if !allowedTypes[mimeType] {
		return fmt.Errorf("MIME type not allowed: %s", mimeType)
	}

	return nil
}

// Nuevos métodos requeridos por la interface
func (s *LocalStorage) DeletePath(path string) error {
	s.logger.Info("🗑️ Deleting file by path", "path", filepath.Base(path))
	
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	
	return nil
}

func (s *LocalStorage) ReadFile(path string) ([]byte, error) {
	s.logger.Info("📝 Reading file", "path", filepath.Base(path))
	
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	
	return data, nil
}

func (s *LocalStorage) GenerateOutputPath(filename string) string {
	timestamp := time.Now().Unix()
	sanitizedName := s.SanitizeFilename(filename)
	return filepath.Join(s.tempDir, fmt.Sprintf("output_%d_%s", timestamp, sanitizedName))
}

func (s *LocalStorage) ValidateMimeType(mimeType string, category string) error {
	// Usar el validator centralizado más robusto
	return utils.ValidateMimeType(mimeType, category)
}

func (s *LocalStorage) SanitizeFilename(filename string) string {
	// Remover caracteres peligrosos
	sanitized := filenameRegex.ReplaceAllString(filename, "_")
	
	// Limitar longitud
	if len(sanitized) > MaxFilenameLn {
		ext := filepath.Ext(sanitized)
		name := sanitized[:MaxFilenameLn-len(ext)-10] // Dejar espacio para timestamp
		timestamp := time.Now().Unix()
		sanitized = fmt.Sprintf("%s_%d%s", name, timestamp, ext)
	}
	
	// Evitar nombres reservados
	reserved := map[string]bool{
		"CON": true, "PRN": true, "AUX": true, "NUL": true,
		"COM1": true, "COM2": true, "LPT1": true, "LPT2": true,
	}
	
	basename := strings.ToUpper(strings.TrimSuffix(sanitized, filepath.Ext(sanitized)))
	if reserved[basename] {
		ext := filepath.Ext(sanitized)
		sanitized = fmt.Sprintf("file_%s%s", basename, ext)
	}
	
	return sanitized
}

func (s *LocalStorage) GetPlanLimits(plan string) *PlanLimits {
	switch strings.ToLower(plan) {
	case "free":
		return &PlanLimits{
			MaxFileSize:  25 << 20, // 25 MB
			MaxFiles:     10,
			DailyUploads: 50,
			StorageQuota: 100 << 20, // 100 MB
			AllowedMimes: []string{
				"application/pdf",
				"image/jpeg", "image/png",
				"application/msword",
				"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			},
		}
	case "premium":
		return &PlanLimits{
			MaxFileSize:  50 << 20, // 50 MB
			MaxFiles:     25,
			DailyUploads: 200,
			StorageQuota: 500 << 20, // 500 MB
			AllowedMimes: []string{
				"application/pdf",
				"image/jpeg", "image/png", "image/tiff", "image/webp",
				"application/msword",
				"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
				"application/vnd.ms-excel",
				"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
				"application/vnd.ms-powerpoint",
				"application/vnd.openxmlformats-officedocument.presentationml.presentation",
			},
		}
	case "pro":
		return &PlanLimits{
			MaxFileSize:  100 << 20, // 100 MB
			MaxFiles:     100,
			DailyUploads: 1000,
			StorageQuota: 2 << 30, // 2 GB
			AllowedMimes: []string{
				"application/pdf",
				"image/jpeg", "image/png", "image/tiff", "image/webp", "image/bmp",
				"application/msword",
				"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
				"application/vnd.ms-excel",
				"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
				"application/vnd.ms-powerpoint",
				"application/vnd.openxmlformats-officedocument.presentationml.presentation",
				"application/rtf", "text/plain",
			},
		}
	default:
		return s.GetPlanLimits("free")
	}
}

func (s *LocalStorage) validateBasicSecurity(fileInfo *FileInfo) error {
	// Validar tamaño máximo absoluto
	if fileInfo.Size > MaxFileSize {
		return fmt.Errorf("file too large: %d bytes (max: %d)", fileInfo.Size, MaxFileSize)
	}
	
	// Verificar que el archivo no esté vacío
	if fileInfo.Size == 0 {
		return fmt.Errorf("empty file not allowed")
	}
	
	// Validar MIME type usando validator robusto
	if !utils.IsAllowedMimeType(fileInfo.MimeType) {
		return fmt.Errorf("MIME type not allowed: %s", fileInfo.MimeType)
	}
	
	// Verificar consistencia extensión vs MIME type
	if err := utils.ValidateFileExtensionMatch(fileInfo.MimeType, fileInfo.OriginalName); err != nil {
		s.logger.Warn("MIME type mismatch detected", 
			"file", fileInfo.OriginalName,
			"mime", fileInfo.MimeType,
			"error", err,
		)
		// No fallar, pero registrar advertencia (algunos navegadores reportan mal el MIME)
	}
	
	return nil
}

// validateExtensionMimeConsistency removed - now using utils.ValidateFileExtensionMatch