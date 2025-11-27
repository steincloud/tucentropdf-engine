package utils

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

// MIME type categories
const (
	CategoryPDF    = "pdf"
	CategoryImage  = "image"
	CategoryOffice = "office"
	CategoryAll    = "all"
)

// Allowed MIME types por categoría (definición estricta)
var (
	PDFMimeTypes = map[string]bool{
		"application/pdf": true,
	}

	ImageMimeTypes = map[string]bool{
		"image/jpeg": true,
		"image/jpg":  true,
		"image/png":  true,
		"image/tiff": true,
		"image/bmp":  true,
		"image/webp": true,
		"image/gif":  true,
	}

	OfficeMimeTypes = map[string]bool{
		"application/msword":                                                         true, // .doc
		"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   true, // .docx
		"application/vnd.ms-excel":                                                   true, // .xls
		"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true, // .xlsx
		"application/vnd.ms-powerpoint":                                              true, // .ppt
		"application/vnd.openxmlformats-officedocument.presentationml.presentation": true, // .pptx
		"application/rtf":                                                            true, // .rtf
		"text/plain":                                                                 true, // .txt
		"application/vnd.oasis.opendocument.text":                                    true, // .odt
		"application/vnd.oasis.opendocument.spreadsheet":                             true, // .ods
		"application/vnd.oasis.opendocument.presentation":                            true, // .odp
	}

	AllAllowedMimeTypes = mergeMimeMaps(PDFMimeTypes, ImageMimeTypes, OfficeMimeTypes)
)

// ValidateMimeType valida que el MIME type pertenezca a una categoría permitida
func ValidateMimeType(mimeType string, category string) error {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	
	// Remover parámetros (ej: "text/plain; charset=utf-8" -> "text/plain")
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}

	switch strings.ToLower(category) {
	case CategoryPDF:
		if !PDFMimeTypes[mimeType] {
			return fmt.Errorf("invalid PDF MIME type: %s (expected: application/pdf)", mimeType)
		}
	case CategoryImage:
		if !ImageMimeTypes[mimeType] {
			return fmt.Errorf("invalid image MIME type: %s", mimeType)
		}
	case CategoryOffice:
		if !OfficeMimeTypes[mimeType] {
			return fmt.Errorf("invalid office document MIME type: %s", mimeType)
		}
	case CategoryAll:
		if !AllAllowedMimeTypes[mimeType] {
			return fmt.Errorf("MIME type not allowed: %s", mimeType)
		}
	default:
		return fmt.Errorf("unknown category: %s", category)
	}

	return nil
}

// IsAllowedMimeType verifica si un MIME type está permitido (cualquier categoría)
func IsAllowedMimeType(mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}
	return AllAllowedMimeTypes[mimeType]
}

// DetectMimeType detecta el MIME type de un archivo usando magic bytes + extensión
func DetectMimeType(filePath string) (string, error) {
	// Abrir archivo
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Leer primeros 512 bytes para detección
	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	// Detectar MIME type usando http.DetectContentType
	detectedMime := http.DetectContentType(buffer[:n])

	// Si la detección es genérica, usar extensión como fallback
	if detectedMime == "application/octet-stream" || detectedMime == "text/plain; charset=utf-8" {
		ext := strings.ToLower(filepath.Ext(filePath))
		if fallbackMime := mimeFromExtension(ext); fallbackMime != "" {
			return fallbackMime, nil
		}
	}

	// Limpiar MIME type (remover charset)
	if idx := strings.Index(detectedMime, ";"); idx != -1 {
		detectedMime = strings.TrimSpace(detectedMime[:idx])
	}

	return detectedMime, nil
}

// DetectMimeTypeFromBytes detecta MIME type desde bytes en memoria
func DetectMimeTypeFromBytes(data []byte, filename string) string {
	// Usar máximo 512 bytes
	sampleSize := len(data)
	if sampleSize > 512 {
		sampleSize = 512
	}

	detectedMime := http.DetectContentType(data[:sampleSize])

	// Fallback a extensión si es genérico
	if detectedMime == "application/octet-stream" || strings.HasPrefix(detectedMime, "text/plain") {
		ext := strings.ToLower(filepath.Ext(filename))
		if fallbackMime := mimeFromExtension(ext); fallbackMime != "" {
			return fallbackMime
		}
	}

	// Limpiar charset
	if idx := strings.Index(detectedMime, ";"); idx != -1 {
		detectedMime = strings.TrimSpace(detectedMime[:idx])
	}

	return detectedMime
}

// ValidateFileExtensionMatch verifica consistencia entre MIME type y extensión
func ValidateFileExtensionMatch(mimeType, filename string) error {
	ext := strings.ToLower(filepath.Ext(filename))
	expectedMime := mimeFromExtension(ext)

	if expectedMime == "" {
		return fmt.Errorf("unknown file extension: %s", ext)
	}

	// Normalizar MIME types
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}

	// Verificar coincidencia (con aliases comunes)
	if mimeType != expectedMime {
		// Aliases aceptables
		if (mimeType == "image/jpg" && expectedMime == "image/jpeg") ||
			(mimeType == "image/jpeg" && expectedMime == "image/jpg") {
			return nil // jpg/jpeg son intercambiables
		}
		return fmt.Errorf("MIME type mismatch: file has '%s' but extension suggests '%s'", mimeType, expectedMime)
	}

	return nil
}

// GetCategoryForMimeType devuelve la categoría de un MIME type
func GetCategoryForMimeType(mimeType string) string {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	if idx := strings.Index(mimeType, ";"); idx != -1 {
		mimeType = strings.TrimSpace(mimeType[:idx])
	}

	if PDFMimeTypes[mimeType] {
		return CategoryPDF
	}
	if ImageMimeTypes[mimeType] {
		return CategoryImage
	}
	if OfficeMimeTypes[mimeType] {
		return CategoryOffice
	}
	return ""
}

// Helper functions

func mimeFromExtension(ext string) string {
	// Mapeo extensión -> MIME type
	extMap := map[string]string{
		".pdf":  "application/pdf",
		".jpg":  "image/jpeg",
		".jpeg": "image/jpeg",
		".png":  "image/png",
		".gif":  "image/gif",
		".bmp":  "image/bmp",
		".tiff": "image/tiff",
		".tif":  "image/tiff",
		".webp": "image/webp",
		".doc":  "application/msword",
		".docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		".xls":  "application/vnd.ms-excel",
		".xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		".ppt":  "application/vnd.ms-powerpoint",
		".pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
		".rtf":  "application/rtf",
		".txt":  "text/plain",
		".odt":  "application/vnd.oasis.opendocument.text",
		".ods":  "application/vnd.oasis.opendocument.spreadsheet",
		".odp":  "application/vnd.oasis.opendocument.presentation",
	}

	if mimeType, ok := extMap[ext]; ok {
		return mimeType
	}

	// Fallback a mime.TypeByExtension
	return mime.TypeByExtension(ext)
}

func mergeMimeMaps(maps ...map[string]bool) map[string]bool {
	result := make(map[string]bool)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}
