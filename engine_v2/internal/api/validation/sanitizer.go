package validation

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"io"
	"mime/multipart"
	"path/filepath"
	"strings"

	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// Sanitizer valida y sanitiza inputs
type Sanitizer struct {
	logger *logger.Logger
}

// NewSanitizer crea un nuevo sanitizer
func NewSanitizer(log *logger.Logger) *Sanitizer {
	return &Sanitizer{
		logger: log,
	}
}

// FileValidationResult contiene el resultado de la validación
type FileValidationResult struct {
	Valid          bool
	Error          string
	Warning        string
	DetectedType   string
	DetectedSize   int64
	IsMalicious    bool
	MaliciousScore float64
}

// FileValidationOptions configura la validación
type FileValidationOptions struct {
	MaxSize           int64    // Bytes
	AllowedExtensions []string // [".pdf", ".docx", etc]
	AllowedMIMETypes  []string // ["application/pdf", etc]
	CheckMagicBytes   bool     // Validar magic bytes
	ScanMalware       bool     // Escanear malware
	SanitizeFilename  bool     // Limpiar filename
}

// DefaultFileValidationOptions retorna opciones por defecto
func DefaultFileValidationOptions() *FileValidationOptions {
	return &FileValidationOptions{
		MaxSize: 100 * 1024 * 1024, // 100MB
		AllowedExtensions: []string{
			".pdf", ".docx", ".xlsx", ".pptx", ".doc", ".xls", ".ppt",
			".odt", ".ods", ".odp", ".txt", ".rtf", ".jpg", ".jpeg", ".png",
		},
		AllowedMIMETypes: []string{
			"application/pdf",
			"application/vnd.openxmlformats-officedocument.wordprocessingml.document",
			"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
			"application/vnd.openxmlformats-officedocument.presentationml.presentation",
			"application/msword",
			"application/vnd.ms-excel",
			"application/vnd.ms-powerpoint",
			"application/vnd.oasis.opendocument.text",
			"application/vnd.oasis.opendocument.spreadsheet",
			"application/vnd.oasis.opendocument.presentation",
			"text/plain",
			"application/rtf",
			"image/jpeg",
			"image/png",
		},
		CheckMagicBytes:  true,
		ScanMalware:      true,
		SanitizeFilename: true,
	}
}

// ValidateFile valida un archivo completo
func (s *Sanitizer) ValidateFile(file *multipart.FileHeader, options *FileValidationOptions) *FileValidationResult {
	if options == nil {
		options = DefaultFileValidationOptions()
	}

	result := &FileValidationResult{
		Valid:          true,
		DetectedSize:   file.Size,
		MaliciousScore: 0.0,
	}

	// 1. Validar tamaño
	if file.Size > options.MaxSize {
		result.Valid = false
		result.Error = fmt.Sprintf("File too large: %d bytes (max: %d bytes)", file.Size, options.MaxSize)
		return result
	}

	if file.Size == 0 {
		result.Valid = false
		result.Error = "Empty file"
		return result
	}

	// 2. Sanitizar filename
	cleanFilename := file.Filename
	if options.SanitizeFilename {
		cleanFilename = s.SanitizeFilename(file.Filename)
		if cleanFilename != file.Filename {
			result.Warning = "Filename was sanitized"
		}
	}

	// 3. Validar extensión
	ext := strings.ToLower(filepath.Ext(cleanFilename))
	if !s.isAllowedExtension(ext, options.AllowedExtensions) {
		result.Valid = false
		result.Error = fmt.Sprintf("Invalid file extension: %s", ext)
		return result
	}

	// 4. Abrir archivo para validación de contenido
	f, err := file.Open()
	if err != nil {
		result.Valid = false
		result.Error = fmt.Sprintf("Cannot open file: %v", err)
		return result
	}
	defer f.Close()

	// 5. Validar magic bytes
	if options.CheckMagicBytes {
		header := make([]byte, 512)
		n, err := f.Read(header)
		if err != nil && err != io.EOF {
			result.Valid = false
			result.Error = fmt.Sprintf("Cannot read file header: %v", err)
			return result
		}

		detectedType := s.DetectFileType(header[:n])
		result.DetectedType = detectedType

		if !s.isAllowedMIMEType(detectedType, options.AllowedMIMETypes) {
			result.Valid = false
			result.Error = fmt.Sprintf("File type mismatch: detected %s, expected %s", detectedType, ext)
			return result
		}

		// Resetear reader
		f.Seek(0, 0)
	}

	// 6. Escanear malware
	if options.ScanMalware {
		content, err := io.ReadAll(f)
		if err != nil {
			result.Valid = false
			result.Error = fmt.Sprintf("Cannot read file content: %v", err)
			return result
		}

		isMalicious, score := s.ScanMalware(content)
		result.IsMalicious = isMalicious
		result.MaliciousScore = score

		if isMalicious {
			result.Valid = false
			result.Error = fmt.Sprintf("Malicious content detected (score: %.2f)", score)
			s.logger.Warn("Malicious file detected",
				"filename", file.Filename,
				"size", file.Size,
				"score", score,
			)
			return result
		}
	}

	return result
}

// SanitizeFilename limpia un filename de caracteres peligrosos
func (s *Sanitizer) SanitizeFilename(filename string) string {
	// Eliminar path traversal
	filename = filepath.Base(filename)

	// Reemplazar caracteres peligrosos
	dangerousChars := []string{
		"../", "..\\", "<", ">", ":", "\"", "|", "?", "*",
		"\x00", "\n", "\r", "\t",
	}

	for _, char := range dangerousChars {
		filename = strings.ReplaceAll(filename, char, "_")
	}

	// Limitar longitud
	if len(filename) > 255 {
		ext := filepath.Ext(filename)
		basename := filename[:255-len(ext)]
		filename = basename + ext
	}

	// Asegurar que no empiece con punto (archivos ocultos)
	if strings.HasPrefix(filename, ".") {
		filename = "_" + filename
	}

	return filename
}

// DetectFileType detecta el tipo de archivo por magic bytes
func (s *Sanitizer) DetectFileType(header []byte) string {
	if len(header) < 4 {
		return "unknown"
	}

	// PDF
	if bytes.HasPrefix(header, []byte{0x25, 0x50, 0x44, 0x46}) { // %PDF
		return "application/pdf"
	}

	// PNG
	if bytes.HasPrefix(header, []byte{0x89, 0x50, 0x4E, 0x47}) {
		return "image/png"
	}

	// JPEG
	if bytes.HasPrefix(header, []byte{0xFF, 0xD8, 0xFF}) {
		return "image/jpeg"
	}

	// ZIP-based (DOCX, XLSX, PPTX, ODT, etc)
	if bytes.HasPrefix(header, []byte{0x50, 0x4B, 0x03, 0x04}) || // PK..
		bytes.HasPrefix(header, []byte{0x50, 0x4B, 0x05, 0x06}) { // PK.. (empty zip)
		// Detectar subtipo por contenido interno
		if len(header) > 30 {
			content := string(header[:min(512, len(header))])
			if strings.Contains(content, "word/") {
				return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
			}
			if strings.Contains(content, "xl/") {
				return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
			}
			if strings.Contains(content, "ppt/") {
				return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
			}
		}
		return "application/zip"
	}

	// MS Office legacy (DOC, XLS, PPT)
	if bytes.HasPrefix(header, []byte{0xD0, 0xCF, 0x11, 0xE0}) {
		return "application/msword" // También podría ser XLS/PPT
	}

	// RTF
	if bytes.HasPrefix(header, []byte{0x7B, 0x5C, 0x72, 0x74}) { // {\rt
		return "application/rtf"
	}

	// Plain text (heuristic)
	if s.isTextContent(header) {
		return "text/plain"
	}

	return "unknown"
}

// isTextContent verifica si el contenido parece texto
func (s *Sanitizer) isTextContent(data []byte) bool {
	// Verificar si mayoría de bytes son ASCII printable
	printable := 0
	for _, b := range data {
		if (b >= 32 && b <= 126) || b == 9 || b == 10 || b == 13 {
			printable++
		}
	}

	return float64(printable)/float64(len(data)) > 0.85
}

// ScanMalware escanea contenido malicioso (heurístico simple)
func (s *Sanitizer) ScanMalware(content []byte) (bool, float64) {
	score := 0.0

	// 1. Detección de ejecutables embebidos
	if s.containsExecutable(content) {
		score += 50.0
	}

	// 2. Detección de scripts embebidos
	if s.containsScript(content) {
		score += 30.0
	}

	// 3. Detección de macros sospechosas
	if s.containsMacros(content) {
		score += 20.0
	}

	// 4. Detección de URLs sospechosas
	if s.containsSuspiciousURLs(content) {
		score += 25.0
	}

	// 5. Detección de shellcode patterns
	if s.containsShellcode(content) {
		score += 60.0
	}

	// 6. Detección de ofuscación excesiva
	if s.isExcessivelyObfuscated(content) {
		score += 15.0
	}

	// Score > 50 = malicioso
	return score >= 50.0, score
}

// containsExecutable detecta ejecutables embebidos
func (s *Sanitizer) containsExecutable(content []byte) bool {
	// PE header (Windows executables)
	if bytes.Contains(content, []byte{0x4D, 0x5A}) { // MZ
		return true
	}

	// ELF header (Linux executables)
	if bytes.Contains(content, []byte{0x7F, 0x45, 0x4C, 0x46}) { // .ELF
		return true
	}

	// Mach-O header (macOS executables)
	if bytes.Contains(content, []byte{0xFE, 0xED, 0xFA, 0xCE}) {
		return true
	}

	return false
}

// containsScript detecta scripts embebidos
func (s *Sanitizer) containsScript(content []byte) bool {
	contentStr := strings.ToLower(string(content))

	suspiciousPatterns := []string{
		"<script",
		"javascript:",
		"vbscript:",
		"eval(",
		"exec(",
		"system(",
		"powershell",
		"cmd.exe",
		"/bin/sh",
		"/bin/bash",
	}

	for _, pattern := range suspiciousPatterns {
		if strings.Contains(contentStr, pattern) {
			return true
		}
	}

	return false
}

// containsMacros detecta macros VBA
func (s *Sanitizer) containsMacros(content []byte) bool {
	contentStr := string(content)

	macroIndicators := []string{
		"VBA",
		"VBAProject",
		"Module",
		"ThisDocument",
		"AutoOpen",
		"AutoExec",
		"Document_Open",
		"Workbook_Open",
	}

	count := 0
	for _, indicator := range macroIndicators {
		if strings.Contains(contentStr, indicator) {
			count++
		}
	}

	// Si tiene 3+ indicadores, probablemente tiene macros
	return count >= 3
}

// containsSuspiciousURLs detecta URLs sospechosas
func (s *Sanitizer) containsSuspiciousURLs(content []byte) bool {
	contentStr := strings.ToLower(string(content))

	suspiciousDomains := []string{
		"bit.ly",
		"tinyurl",
		"short.link",
		".tk", // Dominio free sospechoso
		".ml",
		".ga",
		"iplogger",
		"grabify",
	}

	for _, domain := range suspiciousDomains {
		if strings.Contains(contentStr, domain) {
			return true
		}
	}

	return false
}

// containsShellcode detecta patrones de shellcode
func (s *Sanitizer) containsShellcode(content []byte) bool {
	// Detectar NOP sled (común en exploits)
	nopCount := 0
	for i := 0; i < len(content)-10; i++ {
		if content[i] == 0x90 { // NOP instruction
			nopCount++
			if nopCount >= 20 { // 20+ NOPs consecutivos = sospechoso
				return true
			}
		} else {
			nopCount = 0
		}
	}

	// Detectar syscalls comunes en shellcode
	syscallPatterns := [][]byte{
		{0xCD, 0x80},             // int 0x80 (Linux syscall)
		{0x0F, 0x05},             // syscall (x64)
		{0xFF, 0xD0},             // call eax (common in Windows shellcode)
		{0xEB, 0xFE},             // jmp $ (infinite loop)
		{0x31, 0xC0},             // xor eax, eax
		{0x31, 0xDB},             // xor ebx, ebx
		{0x31, 0xC9},             // xor ecx, ecx
		{0x31, 0xD2},             // xor edx, edx
		{0xB0, 0x0B},             // mov al, 0xb (execve)
	}

	matchCount := 0
	for _, pattern := range syscallPatterns {
		if bytes.Contains(content, pattern) {
			matchCount++
		}
	}

	// Si tiene 3+ patrones de syscall = probable shellcode
	return matchCount >= 3
}

// isExcessivelyObfuscated detecta ofuscación excesiva
func (s *Sanitizer) isExcessivelyObfuscated(content []byte) bool {
	// Calcular entropía (alta entropía = posible ofuscación)
	entropy := s.calculateEntropy(content)

	// Entropía > 7.5 (de 8.0) = altamente aleatorio/ofuscado
	return entropy > 7.5
}

// calculateEntropy calcula entropía de Shannon
func (s *Sanitizer) calculateEntropy(data []byte) float64 {
	if len(data) == 0 {
		return 0.0
	}

	// Contar frecuencias
	freq := make(map[byte]int)
	for _, b := range data {
		freq[b]++
	}

	// Calcular entropía
	var entropy float64
	length := float64(len(data))

	for _, count := range freq {
		p := float64(count) / length
		if p > 0 {
			entropy -= p * (float64(count) / length)
		}
	}

	return entropy
}

// isAllowedExtension verifica si la extensión está permitida
func (s *Sanitizer) isAllowedExtension(ext string, allowed []string) bool {
	ext = strings.ToLower(ext)
	for _, allowedExt := range allowed {
		if strings.ToLower(allowedExt) == ext {
			return true
		}
	}
	return false
}

// isAllowedMIMEType verifica si el MIME type está permitido
func (s *Sanitizer) isAllowedMIMEType(mimeType string, allowed []string) bool {
	mimeType = strings.ToLower(mimeType)
	for _, allowedMIME := range allowed {
		if strings.ToLower(allowedMIME) == mimeType {
			return true
		}
	}
	return false
}

// SanitizeString limpia una cadena de caracteres peligrosos
func (s *Sanitizer) SanitizeString(input string, maxLength int) string {
	// Eliminar null bytes
	input = strings.ReplaceAll(input, "\x00", "")

	// Eliminar control characters (excepto tab, newline, carriage return)
	result := strings.Builder{}
	for _, r := range input {
		if r >= 32 || r == '\t' || r == '\n' || r == '\r' {
			result.WriteRune(r)
		}
	}

	sanitized := result.String()

	// Limitar longitud
	if maxLength > 0 && len(sanitized) > maxLength {
		sanitized = sanitized[:maxLength]
	}

	return sanitized
}

// ValidateBase64 valida y decodifica base64
func (s *Sanitizer) ValidateBase64(input string, maxDecodedSize int) ([]byte, error) {
	// Eliminar whitespace
	input = strings.TrimSpace(input)

	// Decodificar
	decoded, err := base64.StdEncoding.DecodeString(input)
	if err != nil {
		return nil, fmt.Errorf("invalid base64: %w", err)
	}

	// Validar tamaño
	if maxDecodedSize > 0 && len(decoded) > maxDecodedSize {
		return nil, fmt.Errorf("decoded size too large: %d bytes (max: %d)", len(decoded), maxDecodedSize)
	}

	return decoded, nil
}

// min retorna el mínimo de dos enteros
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
