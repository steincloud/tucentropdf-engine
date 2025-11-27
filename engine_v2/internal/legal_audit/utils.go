package legal_audit

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// GenerateAuditID genera ID único para registro de auditoría
func GenerateAuditID() uuid.UUID {
	return uuid.New()
}

// GenerateSecureToken genera token seguro para operaciones críticas
func GenerateSecureToken(length int) (string, error) {
	if length <= 0 {
		length = 32
	}

	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure token: %w", err)
	}

	return hex.EncodeToString(bytes), nil
}

// ValidateAuditEvent valida si un evento de auditoría es válido
func ValidateAuditEvent(event *AuditEvent) error {
	if event == nil {
		return fmt.Errorf("event cannot be nil")
	}

	if event.Tool == "" {
		return fmt.Errorf("tool is required")
	}

	if event.Action == "" {
		return fmt.Errorf("action is required")
	}

	if event.UserID == nil || *event.UserID <= 0 {
		return fmt.Errorf("valid user_id is required")
	}

	if event.IP == "" {
		return fmt.Errorf("ip address is required")
	}

	// Validar herramientas permitidas
	validTools := []string{
		"pdf-merge", "pdf-split", "pdf-compress", "pdf-extract",
		"pdf-convert", "pdf-encrypt", "pdf-decrypt", "pdf-rotate",
		"pdf-watermark", "pdf-stamp", "pdf-optimize", "office-convert",
		"ocr-text", "ocr-image", "image-convert", "image-compress",
	}

	if !contains(validTools, event.Tool) {
		return fmt.Errorf("invalid tool: %s", event.Tool)
	}

	// Validar acciones permitidas
	validActions := []string{
		"process", "upload", "download", "convert", "merge", "split",
		"compress", "extract", "encrypt", "decrypt", "rotate", "watermark",
		"stamp", "optimize", "ocr", "preview", "validate",
	}

	if !contains(validActions, event.Action) {
		return fmt.Errorf("invalid action: %s", event.Action)
	}

	// Validar planes permitidos
	validPlans := []string{
		"free", "basic", "professional", "enterprise", "api",
	}

	if !contains(validPlans, event.Plan) {
		return fmt.Errorf("invalid plan: %s", event.Plan)
	}

	// Validar tamaño de archivo si se especifica
	if event.FileSize != nil && *event.FileSize < 0 {
		return fmt.Errorf("file size cannot be negative")
	}

	// Validar estado
	validStatuses := []string{
		"success", "error", "failed", "timeout", "cancelled", "processing",
	}

	if event.Status != "" && !contains(validStatuses, event.Status) {
		return fmt.Errorf("invalid status: %s", event.Status)
	}

	return nil
}

// SanitizeUserAgent sanitiza user agent para almacenamiento seguro
func SanitizeUserAgent(userAgent string) string {
	if len(userAgent) > 500 {
		userAgent = userAgent[:500]
	}

	// Remover caracteres peligrosos
	userAgent = strings.ReplaceAll(userAgent, "\n", " ")
	userAgent = strings.ReplaceAll(userAgent, "\r", " ")
	userAgent = strings.ReplaceAll(userAgent, "\t", " ")

	// Comprimir espacios múltiples
	for strings.Contains(userAgent, "  ") {
		userAgent = strings.ReplaceAll(userAgent, "  ", " ")
	}

	return strings.TrimSpace(userAgent)
}

// SanitizeIP sanitiza dirección IP
func SanitizeIP(ip string) string {
	ip = strings.TrimSpace(ip)
	
	// Remover puerto si está presente
	if colonIndex := strings.LastIndex(ip, ":"); colonIndex != -1 {
		// Verificar si es IPv6 o IPv4 con puerto
		if strings.Count(ip, ":") == 1 {
			ip = ip[:colonIndex] // IPv4 con puerto
		}
	}

	// Validación básica de longitud
	if len(ip) > 45 { // IPv6 máximo es 39 caracteres
		ip = ip[:45]
	}

	return ip
}

// DetectAbusiveActivity detecta actividad abusiva basada en patrones
func DetectAbusiveActivity(event *AuditEvent, recentEvents []AuditEvent) bool {
	if event == nil {
		return false
	}

	// Verificar límites de rate
	if isRateLimited(event, recentEvents) {
		return true
	}

	// Verificar patrones sospechosos en user agent
	if isSuspiciousUserAgent(event.UserAgent) {
		return true
	}

	// Verificar tamaño de archivo sospechoso
	if isSuspiciousFileSize(event.FileSize) {
		return true
	}

	// Verificar patterns de herramientas
	if isSuspiciousToolUsage(event, recentEvents) {
		return true
	}

	return false
}

// isRateLimited verifica si hay demasiadas requests
func isRateLimited(event *AuditEvent, recentEvents []AuditEvent) bool {
	if event.UserID == nil {
		return false
	}

	// userID := *event.UserID
	// now := time.Now()
	// oneMinuteAgo := now.Add(-1 * time.Minute)
	// oneHourAgo := now.Add(-1 * time.Hour)

	var countLastMinute, countLastHour int

	for _, recentEvent := range recentEvents {
		// Note: AuditEvent no tiene Timestamp, usar current time para comparaciones
		// Este código necesita ser refactorizado para usar LegalAuditLog
		_ = recentEvent // Evitar warning de variable no usada
	}

	// Límites basados en plan
	limits := getRateLimits(event.Plan)
	
	return countLastMinute > limits.PerMinute || countLastHour > limits.PerHour
}

// RateLimits define límites por plan
type RateLimits struct {
	PerMinute int
	PerHour   int
}

// getRateLimits obtiene límites según plan
func getRateLimits(plan string) RateLimits {
	switch plan {
	case "free":
		return RateLimits{PerMinute: 10, PerHour: 100}
	case "basic":
		return RateLimits{PerMinute: 30, PerHour: 500}
	case "professional":
		return RateLimits{PerMinute: 60, PerHour: 2000}
	case "enterprise":
		return RateLimits{PerMinute: 120, PerHour: 10000}
	case "api":
		return RateLimits{PerMinute: 300, PerHour: 50000}
	default:
		return RateLimits{PerMinute: 10, PerHour: 100}
	}
}

// isSuspiciousUserAgent verifica user agents sospechosos
func isSuspiciousUserAgent(userAgent string) bool {
	suspiciousPatterns := []string{
		"bot", "crawler", "spider", "scraper", "curl", "wget",
		"python", "go-http", "okhttp", "java", "perl", "ruby",
		"automated", "script", "tool", "hack", "exploit",
	}

	userAgentLower := strings.ToLower(userAgent)

	for _, pattern := range suspiciousPatterns {
		if strings.Contains(userAgentLower, pattern) {
			return true
		}
	}

	// User agents muy cortos o muy largos son sospechosos
	if len(userAgent) < 10 || len(userAgent) > 500 {
		return true
	}

	return false
}

// isSuspiciousFileSize verifica tamaños de archivo sospechosos
func isSuspiciousFileSize(fileSize *int64) bool {
	if fileSize == nil {
		return false
	}

	size := *fileSize

	// Archivos extremadamente grandes (>500MB) o negativos
	if size < 0 || size > 500*1024*1024 {
		return true
	}

	// Archivos de exactamente 0 bytes pueden ser sospechosos
	if size == 0 {
		return true
	}

	return false
}

// isSuspiciousToolUsage verifica patrones sospechosos en uso de herramientas
func isSuspiciousToolUsage(event *AuditEvent, recentEvents []AuditEvent) bool {
	if event.UserID == nil {
		return false
	}

	// userID := *event.UserID
	// now := time.Now()
	// fiveMinutesAgo := now.Add(-5 * time.Minute)

	// Contar herramientas diferentes usadas en últimos 5 minutos
	toolsUsed := make(map[string]bool)
	
	// Note: AuditEvent no tiene campo Timestamp, esta funcionalidad necesita refactoring
	// para trabajar con LegalAuditLog que sí tiene Timestamp
	for _, recentEvent := range recentEvents {
		if recentEvent.UserID != nil {
			// Sin timestamp, asumimos todos los eventos son recientes
			toolsUsed[recentEvent.Tool] = true
		}
	}

	// Si usa más de 5 herramientas diferentes en 5 minutos, es sospechoso
	return len(toolsUsed) > 5
}

// FormatDuration formatea duración en formato legible
func FormatDuration(duration time.Duration) string {
	if duration < time.Second {
		return fmt.Sprintf("%dms", duration.Milliseconds())
	}
	if duration < time.Minute {
		return fmt.Sprintf("%.1fs", duration.Seconds())
	}
	if duration < time.Hour {
		return fmt.Sprintf("%.1fm", duration.Minutes())
	}
	return fmt.Sprintf("%.1fh", duration.Hours())
}

// GetFileExtension obtiene extensión de archivo de forma segura
func GetFileExtension(filename string) string {
	ext := filepath.Ext(filename)
	if len(ext) > 0 && ext[0] == '.' {
		ext = ext[1:]
	}
	return strings.ToLower(ext)
}

// IsValidFileType verifica si tipo de archivo es válido para procesamiento
func IsValidFileType(filename string) bool {
	validExtensions := []string{
		"pdf", "doc", "docx", "xls", "xlsx", "ppt", "pptx",
		"jpg", "jpeg", "png", "gif", "tiff", "bmp", "webp",
		"txt", "rtf", "odt", "ods", "odp",
	}

	ext := GetFileExtension(filename)
	return contains(validExtensions, ext)
}

// GetRetentionDate calcula fecha de retención basada en regulaciones
func GetRetentionDate() time.Time {
	// 3 años de retención según regulaciones legales
	return time.Now().AddDate(3, 0, 0)
}

// IsExpired verifica si un registro ha expirado
func IsExpired(createdAt time.Time) bool {
	retentionPeriod := 3 * 365 * 24 * time.Hour // 3 años
	expirationDate := createdAt.Add(retentionPeriod)
	return time.Now().After(expirationDate)
}

// GetArchiveFileName genera nombre de archivo para archivado
func GetArchiveFileName(date time.Time) string {
	return fmt.Sprintf("legal_audit_archive_%s.tar.gz", 
		date.Format("2006_01_02"))
}

// ValidateIPAddress valida formato de dirección IP
func ValidateIPAddress(ip string) bool {
	parts := strings.Split(ip, ".")
	
	// Validación básica IPv4
	if len(parts) == 4 {
		for _, part := range parts {
			if len(part) == 0 || len(part) > 3 {
				return false
			}
			// Verificar que sean números
			for _, char := range part {
				if char < '0' || char > '9' {
					return false
				}
			}
		}
		return true
	}

	// Validación básica IPv6 (simplificada)
	if strings.Contains(ip, ":") && len(ip) <= 39 {
		return true
	}

	return false
}

// MaskSensitiveData enmascara datos sensibles para logs
func MaskSensitiveData(data string) string {
	if len(data) <= 4 {
		return strings.Repeat("*", len(data))
	}

	visible := 2
	masked := strings.Repeat("*", len(data)-visible*2)
	return data[:visible] + masked + data[len(data)-visible:]
}

// GetAuditEventPriority calcula prioridad de evento para procesamiento
func GetAuditEventPriority(event *AuditEvent) int {
	priority := 5 // Prioridad normal

	// Eventos de alta prioridad
	if event.Status == "error" || event.Status == "failed" {
		priority = 8
	}

	// Eventos de seguridad
	if event.Abuse {
		priority = 9
	}

	// Eventos críticos del sistema
	if event.Tool == "pdf-encrypt" || event.Tool == "pdf-decrypt" {
		priority = 7
	}

	// Eventos de usuarios enterprise
	if event.Plan == "enterprise" || event.Plan == "api" {
		priority = 6
	}

	return priority
}

// contains verifica si slice contiene elemento
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// TimeAgo calcula tiempo transcurrido en formato legible
func TimeAgo(t time.Time) string {
	duration := time.Since(t)

	if duration < time.Minute {
		return "menos de un minuto"
	}
	if duration < time.Hour {
		minutes := int(duration.Minutes())
		if minutes == 1 {
			return "1 minuto"
		}
		return fmt.Sprintf("%d minutos", minutes)
	}
	if duration < 24*time.Hour {
		hours := int(duration.Hours())
		if hours == 1 {
			return "1 hora"
		}
		return fmt.Sprintf("%d horas", hours)
	}

	days := int(duration.Hours() / 24)
	if days == 1 {
		return "1 día"
	}
	if days < 30 {
		return fmt.Sprintf("%d días", days)
	}
	
	months := days / 30
	if months == 1 {
		return "1 mes"
	}
	if months < 12 {
		return fmt.Sprintf("%d meses", months)
	}

	years := months / 12
	if years == 1 {
		return "1 año"
	}
	return fmt.Sprintf("%d años", years)
}

// BatchSize define tamaño óptimo de lote para operaciones
const (
	DefaultBatchSize     = 100
	LargeBatchSize      = 500
	ExportBatchSize     = 1000
	IntegrityBatchSize  = 50
)

// GetOptimalBatchSize calcula tamaño óptimo de lote según operación
func GetOptimalBatchSize(operation string) int {
	switch operation {
	case "export":
		return ExportBatchSize
	case "integrity":
		return IntegrityBatchSize
	case "archive":
		return LargeBatchSize
	default:
		return DefaultBatchSize
	}
}

// CompressString comprime string para almacenamiento eficiente
func CompressString(data string) string {
	// Implementación básica - en producción usar compresión real
	if len(data) > 1000 {
		return data[:500] + "..." + data[len(data)-100:]
	}
	return data
}

// CreateAuditSummary crea resumen de múltiples eventos
func CreateAuditSummary(events []AuditEvent, timeRange string) AuditSummary {
	summary := AuditSummary{
		TimeRange:    timeRange,
		TotalEvents:  len(events),
		ToolUsage:    make(map[string]int),
		StatusCounts: make(map[string]int),
		CreatedAt:    time.Now(),
	}

	for _, event := range events {
		summary.ToolUsage[event.Tool]++
		summary.StatusCounts[event.Status]++

		if event.Abuse {
			summary.AbuseDetected++
		}
	}

	return summary
}

// AuditSummary resume actividad de auditoría
type AuditSummary struct {
	TimeRange     string         `json:"time_range"`
	TotalEvents   int            `json:"total_events"`
	AbuseDetected int            `json:"abuse_detected"`
	ToolUsage     map[string]int `json:"tool_usage"`
	StatusCounts  map[string]int `json:"status_counts"`
	CreatedAt     time.Time      `json:"created_at"`
}