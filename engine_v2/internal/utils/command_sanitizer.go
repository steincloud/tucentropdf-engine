package utils

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// CommandArgSanitizer sanitiza argumentos para comandos externos
type CommandArgSanitizer struct {
	// Caracteres peligrosos que pueden causar command injection
	dangerousChars *regexp.Regexp
}

// NewCommandArgSanitizer crea un nuevo sanitizador
func NewCommandArgSanitizer() *CommandArgSanitizer {
	// Detecta caracteres que pueden usarse para command injection
	dangerousPattern := regexp.MustCompile(`[;&|$()<>\x00\n\r'"` + "`" + `\\]`)
	
	return &CommandArgSanitizer{
		dangerousChars: dangerousPattern,
	}
}

// SanitizeCommandArg sanitiza un argumento de comando
func (cas *CommandArgSanitizer) SanitizeCommandArg(arg string) string {
	// Remover caracteres peligrosos
	sanitized := cas.dangerousChars.ReplaceAllString(arg, "")
	
	// Limpiar espacios múltiples
	sanitized = strings.TrimSpace(sanitized)
	
	return sanitized
}

// IsValidPath verifica si un path es seguro para usar en comandos
func (cas *CommandArgSanitizer) IsValidPath(path string) bool {
	if path == "" {
		return false
	}
	
	// Verificar que no contenga caracteres peligrosos
	if cas.dangerousChars.MatchString(path) {
		return false
	}
	
	// Verificar que no contenga secuencias de path traversal
	if strings.Contains(path, "..") {
		return false
	}
	
	// Verificar que no contenga null bytes
	if strings.Contains(path, "\x00") {
		return false
	}
	
	// Limpiar el path y verificar que sea el mismo
	cleaned := filepath.Clean(path)
	if cleaned != path && !strings.HasSuffix(cleaned, filepath.Base(path)) {
		return false
	}
	
	return true
}

// IsValidFileID verifica si un file ID es seguro
func IsValidFileID(fileID string) bool {
	if fileID == "" {
		return false
	}
	
	// File ID solo debe contener caracteres alfanuméricos, guiones y puntos
	validPattern := regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	if !validPattern.MatchString(fileID) {
		return false
	}
	
	// No debe contener path traversal
	if strings.Contains(fileID, "..") {
		return false
	}
	
	// No debe empezar con / o \ (paths absolutos)
	if strings.HasPrefix(fileID, "/") || strings.HasPrefix(fileID, "\\") {
		return false
	}
	
	return true
}

// SanitizeFilePath sanitiza completamente un file path
func SanitizeFilePath(basePath, userPath string) (string, error) {
	// Limpiar el user path
	cleaned := filepath.Clean(userPath)
	
	// Verificar path traversal
	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("path traversal detected")
	}
	
	// Construir path completo
	fullPath := filepath.Join(basePath, cleaned)
	
	// Verificar que el resultado esté dentro del basePath
	absBase, err := filepath.Abs(basePath)
	if err != nil {
		return "", fmt.Errorf("invalid base path: %w", err)
	}
	
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	
	// Verificar que absPath esté contenido en absBase
	if !strings.HasPrefix(absPath, absBase) {
		return "", fmt.Errorf("path outside base directory")
	}
	
	return fullPath, nil
}

// ValidateCommandOutput verifica que el output de un comando sea seguro
func ValidateCommandOutput(output string, maxLength int) error {
	if len(output) > maxLength {
		return fmt.Errorf("command output exceeds maximum length: %d > %d", len(output), maxLength)
	}
	
	// Verificar que no contenga null bytes
	if strings.Contains(output, "\x00") {
		return fmt.Errorf("command output contains null bytes")
	}
	
	return nil
}
