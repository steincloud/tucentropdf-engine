package audit

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// EnhancedAuditLogger logger de auditoría con cumplimiento compliance
type EnhancedAuditLogger struct {
	logger       *logger.Logger
	db           *gorm.DB
	redis        *redis.Client
	hmacKey      []byte
	enableTamperProof bool
}

// NewEnhancedAuditLogger crea un nuevo logger de auditoría
func NewEnhancedAuditLogger(
	log *logger.Logger,
	db *gorm.DB,
	redisClient *redis.Client,
	hmacKey string,
) *EnhancedAuditLogger {
	return &EnhancedAuditLogger{
		logger:            log,
		db:                db,
		redis:             redisClient,
		hmacKey:           []byte(hmacKey),
		enableTamperProof: true,
	}
}

// AuditEvent evento de auditoría
type AuditRecord struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	EventID     string    `gorm:"uniqueIndex;not null" json:"event_id"`
	Timestamp   time.Time `gorm:"index;not null" json:"timestamp"`
	EventType   string    `gorm:"index;not null" json:"event_type"` // security, access, data, system
	Category    string    `gorm:"index;not null" json:"category"`   // auth, file_access, api_call, config_change
	Severity    string    `gorm:"index;not null" json:"severity"`   // low, medium, high, critical
	UserID      string    `gorm:"index" json:"user_id,omitempty"`
	SessionID   string    `gorm:"index" json:"session_id,omitempty"`
	IPAddress   string    `gorm:"index" json:"ip_address,omitempty"`
	UserAgent   string    `json:"user_agent,omitempty"`
	Resource    string    `gorm:"index" json:"resource,omitempty"` // /api/v2/ocr/ai
	Action      string    `gorm:"index" json:"action,omitempty"`   // create, read, update, delete
	Result      string    `gorm:"index" json:"result"`             // success, failure
	Message     string    `json:"message"`
	Details     JSON      `gorm:"type:jsonb" json:"details,omitempty"`
	HMAC        string    `gorm:"not null" json:"hmac"` // Tamper-proof signature
	CreatedAt   time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// JSON custom type para JSONB
type JSON map[string]interface{}

// TableName nombre de la tabla
func (AuditRecord) TableName() string {
	return "audit_events"
}

// LogEvent registra un evento de auditoría
func (eal *EnhancedAuditLogger) LogEvent(ctx context.Context, event *AuditRecord) error {
	// Generar event ID si no existe
	if event.EventID == "" {
		event.EventID = eal.generateEventID()
	}

	// Timestamp
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Calcular HMAC para tamper-proof
	if eal.enableTamperProof {
		event.HMAC = eal.calculateHMAC(event)
	}

	// Guardar en DB
	if err := eal.db.WithContext(ctx).Create(event).Error; err != nil {
		eal.logger.Error("Failed to save audit event", "error", err, "event_id", event.EventID)
		return err
	}

	// Cache en Redis para acceso rápido
	if err := eal.cacheEvent(ctx, event); err != nil {
		eal.logger.Warn("Failed to cache audit event", "error", err, "event_id", event.EventID)
	}

	// Log estructurado
	eal.logger.Info("Audit event logged",
		"event_id", event.EventID,
		"type", event.EventType,
		"category", event.Category,
		"severity", event.Severity,
		"user_id", event.UserID,
		"result", event.Result,
	)

	return nil
}

// LogSecurityEvent registra un evento de seguridad
func (eal *EnhancedAuditLogger) LogSecurityEvent(ctx context.Context, category, severity, message string, details JSON) error {
	event := &AuditRecord{
		EventType: "security",
		Category:  category,
		Severity:  severity,
		Message:   message,
		Details:   details,
	}

	return eal.LogEvent(ctx, event)
}

// LogAuthEvent registra un evento de autenticación
func (eal *EnhancedAuditLogger) LogAuthEvent(ctx context.Context, userID, action, result, ip string, details JSON) error {
	event := &AuditRecord{
		EventType: "access",
		Category:  "auth",
		Severity:  eal.getSeverityForAuthResult(result),
		UserID:    userID,
		Action:    action,
		Result:    result,
		IPAddress: ip,
		Message:   fmt.Sprintf("Authentication %s: %s", action, result),
		Details:   details,
	}

	return eal.LogEvent(ctx, event)
}

// LogAPIAccess registra acceso a API
func (eal *EnhancedAuditLogger) LogAPIAccess(ctx context.Context, userID, resource, action, result, ip, userAgent string) error {
	event := &AuditRecord{
		EventType: "access",
		Category:  "api_call",
		Severity:  "low",
		UserID:    userID,
		Resource:  resource,
		Action:    action,
		Result:    result,
		IPAddress: ip,
		UserAgent: userAgent,
		Message:   fmt.Sprintf("API %s %s: %s", action, resource, result),
	}

	return eal.LogEvent(ctx, event)
}

// LogDataAccess registra acceso a datos sensibles
func (eal *EnhancedAuditLogger) LogDataAccess(ctx context.Context, userID, resource, action, result string, details JSON) error {
	event := &AuditRecord{
		EventType: "data",
		Category:  "file_access",
		Severity:  "medium",
		UserID:    userID,
		Resource:  resource,
		Action:    action,
		Result:    result,
		Message:   fmt.Sprintf("Data access: %s %s", action, resource),
		Details:   details,
	}

	return eal.LogEvent(ctx, event)
}

// LogConfigChange registra cambios de configuración
func (eal *EnhancedAuditLogger) LogConfigChange(ctx context.Context, userID, configKey, oldValue, newValue string) error {
	event := &AuditRecord{
		EventType: "system",
		Category:  "config_change",
		Severity:  "high",
		UserID:    userID,
		Action:    "update",
		Result:    "success",
		Message:   fmt.Sprintf("Configuration changed: %s", configKey),
		Details: JSON{
			"config_key": configKey,
			"old_value":  oldValue,
			"new_value":  newValue,
		},
	}

	return eal.LogEvent(ctx, event)
}

// LogSuspiciousActivity registra actividad sospechosa
func (eal *EnhancedAuditLogger) LogSuspiciousActivity(ctx context.Context, userID, ip, activity string, details JSON) error {
	event := &AuditRecord{
		EventType: "security",
		Category:  "suspicious_activity",
		Severity:  "critical",
		UserID:    userID,
		IPAddress: ip,
		Result:    "failure",
		Message:   fmt.Sprintf("Suspicious activity detected: %s", activity),
		Details:   details,
	}

	// Enviar alerta inmediata
	eal.sendSecurityAlert(ctx, event)

	return eal.LogEvent(ctx, event)
}

// VerifyEventIntegrity verifica la integridad de un evento
func (eal *EnhancedAuditLogger) VerifyEventIntegrity(event *AuditRecord) bool {
	if !eal.enableTamperProof {
		return true
	}

	expectedHMAC := eal.calculateHMAC(event)
	return hmac.Equal([]byte(event.HMAC), []byte(expectedHMAC))
}

// calculateHMAC calcula el HMAC del evento
func (eal *EnhancedAuditLogger) calculateHMAC(event *AuditRecord) string {
	// Construir cadena canónica
	data := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%s|%s|%s",
		event.EventID,
		event.Timestamp.Format(time.RFC3339Nano),
		event.EventType,
		event.Category,
		event.UserID,
		event.Action,
		event.Result,
		event.Message,
		jsonString(event.Details),
	)

	// Calcular HMAC-SHA256
	h := hmac.New(sha256.New, eal.hmacKey)
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

// generateEventID genera un ID único para el evento
func (eal *EnhancedAuditLogger) generateEventID() string {
	return fmt.Sprintf("evt_%d_%s", time.Now().UnixNano(), randomString(8))
}

// cacheEvent cachea evento en Redis para acceso rápido
func (eal *EnhancedAuditLogger) cacheEvent(ctx context.Context, event *AuditRecord) error {
	key := fmt.Sprintf("audit:event:%s", event.EventID)
	data, _ := json.Marshal(event)
	return eal.redis.Set(ctx, key, data, 24*time.Hour).Err()
}

// GetEvent obtiene un evento por ID
func (eal *EnhancedAuditLogger) GetEvent(ctx context.Context, eventID string) (*AuditRecord, error) {
	// Buscar en cache primero
	key := fmt.Sprintf("audit:event:%s", eventID)
	data, err := eal.redis.Get(ctx, key).Result()
	if err == nil {
		var event AuditRecord
		if err := json.Unmarshal([]byte(data), &event); err == nil {
			return &event, nil
		}
	}

	// Buscar en DB
	var event AuditRecord
	if err := eal.db.WithContext(ctx).Where("event_id = ?", eventID).First(&event).Error; err != nil {
		return nil, err
	}

	return &event, nil
}

// QueryEvents consulta eventos con filtros
func (eal *EnhancedAuditLogger) QueryEvents(ctx context.Context, filters *EventFilters) ([]AuditRecord, error) {
	query := eal.db.WithContext(ctx)

	if filters.EventType != "" {
		query = query.Where("event_type = ?", filters.EventType)
	}
	if filters.Category != "" {
		query = query.Where("category = ?", filters.Category)
	}
	if filters.Severity != "" {
		query = query.Where("severity = ?", filters.Severity)
	}
	if filters.UserID != "" {
		query = query.Where("user_id = ?", filters.UserID)
	}
	if filters.Result != "" {
		query = query.Where("result = ?", filters.Result)
	}
	if !filters.StartTime.IsZero() {
		query = query.Where("timestamp >= ?", filters.StartTime)
	}
	if !filters.EndTime.IsZero() {
		query = query.Where("timestamp <= ?", filters.EndTime)
	}

	query = query.Order("timestamp DESC").Limit(filters.Limit)

	var events []AuditRecord
	if err := query.Find(&events).Error; err != nil {
		return nil, err
	}

	return events, nil
}

// EventFilters filtros para consultas
type EventFilters struct {
	EventType string
	Category  string
	Severity  string
	UserID    string
	Result    string
	StartTime time.Time
	EndTime   time.Time
	Limit     int
}

// GetSecurityAlerts obtiene alertas de seguridad recientes
func (eal *EnhancedAuditLogger) GetSecurityAlerts(ctx context.Context, hours int) ([]AuditRecord, error) {
	startTime := time.Now().Add(-time.Duration(hours) * time.Hour)

	return eal.QueryEvents(ctx, &EventFilters{
		EventType: "security",
		Severity:  "critical",
		StartTime: startTime,
		Limit:     100,
	})
}

// GetFailedAuthAttempts obtiene intentos fallidos de autenticación
func (eal *EnhancedAuditLogger) GetFailedAuthAttempts(ctx context.Context, userID string, minutes int) (int, error) {
	startTime := time.Now().Add(-time.Duration(minutes) * time.Minute)

	var count int64
	err := eal.db.WithContext(ctx).
		Model(&AuditRecord{}).
		Where("event_type = ? AND category = ? AND user_id = ? AND result = ? AND timestamp >= ?",
			"access", "auth", userID, "failure", startTime).
		Count(&count).Error

	return int(count), err
}

// GetUserActivity obtiene actividad de un usuario
func (eal *EnhancedAuditLogger) GetUserActivity(ctx context.Context, userID string, limit int) ([]AuditRecord, error) {
	var events []AuditRecord
	err := eal.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("timestamp DESC").
		Limit(limit).
		Find(&events).Error

	return events, err
}

// GenerateComplianceReport genera reporte de compliance
func (eal *EnhancedAuditLogger) GenerateComplianceReport(ctx context.Context, startDate, endDate time.Time) (*ComplianceReport, error) {
	report := &ComplianceReport{
		StartDate: startDate,
		EndDate:   endDate,
	}

	// Total de eventos
	eal.db.WithContext(ctx).
		Model(&AuditRecord{}).
		Where("timestamp BETWEEN ? AND ?", startDate, endDate).
		Count(&report.TotalEvents)

	// Eventos por tipo
	eal.db.WithContext(ctx).
		Model(&AuditRecord{}).
		Select("event_type, COUNT(*) as count").
		Where("timestamp BETWEEN ? AND ?", startDate, endDate).
		Group("event_type").
		Scan(&report.EventsByType)

	// Eventos críticos
	eal.db.WithContext(ctx).
		Model(&AuditRecord{}).
		Where("timestamp BETWEEN ? AND ? AND severity = ?", startDate, endDate, "critical").
		Count(&report.CriticalEvents)

	// Fallos de autenticación
	eal.db.WithContext(ctx).
		Model(&AuditRecord{}).
		Where("timestamp BETWEEN ? AND ? AND category = ? AND result = ?", startDate, endDate, "auth", "failure").
		Count(&report.FailedAuthAttempts)

	// Accesos a datos sensibles
	eal.db.WithContext(ctx).
		Model(&AuditRecord{}).
		Where("timestamp BETWEEN ? AND ? AND category = ?", startDate, endDate, "file_access").
		Count(&report.DataAccessEvents)

	return report, nil
}

// ComplianceReport reporte de compliance
type ComplianceReport struct {
	StartDate          time.Time
	EndDate            time.Time
	TotalEvents        int64
	EventsByType       []EventTypeCount
	CriticalEvents     int64
	FailedAuthAttempts int64
	DataAccessEvents   int64
}

// EventTypeCount contador por tipo de evento
type EventTypeCount struct {
	EventType string `json:"event_type"`
	Count     int64  `json:"count"`
}

// sendSecurityAlert envía alerta de seguridad
func (eal *EnhancedAuditLogger) sendSecurityAlert(ctx context.Context, event *AuditRecord) {
	// TODO: Integrar con sistema de alertas (email, Slack, PagerDuty)
	eal.logger.Warn("SECURITY ALERT",
		"event_id", event.EventID,
		"severity", event.Severity,
		"message", event.Message,
		"user_id", event.UserID,
		"ip", event.IPAddress,
	)
}

// getSeverityForAuthResult determina severidad según resultado de auth
func (eal *EnhancedAuditLogger) getSeverityForAuthResult(result string) string {
	if result == "failure" {
		return "high"
	}
	return "low"
}

// jsonString convierte JSON a string
func jsonString(data JSON) string {
	if data == nil {
		return ""
	}
	b, _ := json.Marshal(data)
	return string(b)
}

// randomString genera string aleatorio
func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(b)
}

// ArchiveOldEvents archiva eventos antiguos
func (eal *EnhancedAuditLogger) ArchiveOldEvents(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoffDate := time.Now().Add(-olderThan)

	// TODO: Mover a tabla de archivo o exportar a S3 antes de eliminar
	result := eal.db.WithContext(ctx).
		Where("timestamp < ?", cutoffDate).
		Delete(&AuditEvent{})

	if result.Error != nil {
		return 0, result.Error
	}

	eal.logger.Info("Archived old audit events",
		"cutoff_date", cutoffDate,
		"count", result.RowsAffected,
	)

	return result.RowsAffected, nil
}
