package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/tucentropdf/engine-v2/internal/audit"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// AuditLogger servicio de auditoría y logging de eventos
// Implementa audit.AuditLogger interface
type AuditLogger struct {
	redis   *redis.Client
	logger  *logger.Logger
	config  *config.Config
	enabled bool
}

// NewAuditLogger crea un nuevo logger de auditoría
func NewAuditLogger(redisClient *redis.Client, log *logger.Logger, cfg *config.Config) audit.AuditLogger {
	return &AuditLogger{
		redis:   redisClient,
		logger:  log,
		config:  cfg,
		enabled: true, // Por defecto habilitado
	}
}

// EventType representa el tipo de evento de auditoría - mantenemos para compatibilidad
type EventType = audit.EventType

// Constantes de eventos mantenidas para compatibilidad
const (
	// Eventos de autenticación
	EventLogin           = audit.EventPlanChanged // Se mapean al tipo básico
	EventLogout          = audit.EventPlanChanged
	EventAuthFailure     = audit.EventPlanChanged
	EventAPIKeyCreated   = audit.EventPlanChanged
	EventAPIKeyRevoked   = audit.EventPlanChanged
	
	// Eventos de planes y facturación
	EventPlanChanged     = audit.EventPlanChanged
	EventPlanUpgraded    = audit.EventPlanChanged
	EventPlanDowngraded  = audit.EventPlanChanged
	EventProrationCalculated = audit.EventPlanChanged
	EventPaymentProcessed = audit.EventPlanChanged
	EventPaymentFailed   = audit.EventPlanChanged
	
	// Eventos de cuotas y límites
	EventQuotaExceeded   = audit.EventQuotaReach
	EventLimitReached    = audit.EventQuotaReach
	EventUsageTracked    = audit.EventPlanChanged
	EventCounterReset    = audit.EventPlanChanged
	
	// Eventos de operaciones
	EventFileUploaded    = audit.EventFileAccess
	EventFileProcessed   = audit.EventFileAccess
	EventOperationStarted = audit.EventAPICall
	EventOperationCompleted = audit.EventAPICall
	EventOperationFailed = audit.EventError
	
	// Eventos de sistema
	EventSystemError     = audit.EventError
	EventConfigChanged   = audit.EventAPICall
	EventMaintenanceMode = audit.EventAPICall
	
	// Eventos de webhooks
	EventWebhookSent     = audit.EventAPICall
	EventWebhookFailed   = audit.EventError
)

// AuditEvent representa un evento de auditoría - para compatibilidad usamos el tipo del paquete audit
type AuditEvent = audit.AuditEvent

// LogEvent registra un evento de auditoría general
func (al *AuditLogger) LogEvent(event audit.AuditEvent) {
	if !al.enabled {
		return
	}
	
	// Generar ID único si no se proporciona
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	
	// Guardar en Redis para procesamiento asíncrono
	al.storeEventInRedis(event)
	
	// Log inmediato en logger estructurado
	al.logEventImmediate(event)
}

// LogPlanEvent registra eventos relacionados con planes
func (al *AuditLogger) LogPlanEvent(event audit.AuditEvent) {
	al.LogEvent(event)
}

// LogAuthEvent registra eventos de autenticación
func (al *AuditLogger) LogAuthEvent(event audit.AuditEvent) {
	al.LogEvent(event)
}

// LogQuotaEvent registra eventos de cuotas y límites
func (al *AuditLogger) LogQuotaEvent(event audit.AuditEvent) {
	al.LogEvent(event)
}

// LogUsageEvent registra eventos de uso
func (al *AuditLogger) LogUsageEvent(event audit.AuditEvent) {
	al.LogEvent(event)
}

// LogOperationEvent registra eventos de operaciones
func (al *AuditLogger) LogOperationEvent(event audit.AuditEvent) {
	al.LogEvent(event)
}

// LogSystemEvent registra eventos de sistema
func (al *AuditLogger) LogSystemEvent(event audit.AuditEvent) {
	al.LogEvent(event)
}

// LogWebhookEvent registra eventos de webhooks
func (al *AuditLogger) LogWebhookEvent(event audit.AuditEvent) {
	al.LogEvent(event)
}

// GetUserAuditLog obtiene el log de auditoría de un usuario específico
func (al *AuditLogger) GetUserAuditLog(ctx context.Context, userID string, limit int) ([]audit.AuditEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100 // Límite por defecto
	}
	
	key := al.keyUserAuditLog(userID)
	
	// Obtener eventos desde Redis
	result, err := al.redis.LRange(ctx, key, 0, int64(limit-1)).Result()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get user audit log: %w", err)
	}
	
	events := make([]audit.AuditEvent, 0, len(result))
	for _, eventJSON := range result {
		var event audit.AuditEvent
		if err := json.Unmarshal([]byte(eventJSON), &event); err == nil {
			events = append(events, event)
		}
	}
	
	return events, nil
}

// GetSystemAuditLog obtiene eventos de auditoría del sistema
func (al *AuditLogger) GetSystemAuditLog(ctx context.Context, eventType audit.EventType, limit int) ([]audit.AuditEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	
	key := al.keySystemAuditLog(eventType)
	
	result, err := al.redis.LRange(ctx, key, 0, int64(limit-1)).Result()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get system audit log: %w", err)
	}
	
	events := make([]audit.AuditEvent, 0, len(result))
	for _, eventJSON := range result {
		var event audit.AuditEvent
		if err := json.Unmarshal([]byte(eventJSON), &event); err == nil {
			events = append(events, event)
		}
	}
	
	return events, nil
}

// GetAuditStatistics obtiene estadísticas de auditoría
func (al *AuditLogger) GetAuditStatistics(ctx context.Context, userID string, since time.Time) (*AuditStatistics, error) {
	key := al.keyUserAuditLog(userID)
	
	// Obtener todos los eventos desde la fecha especificada
	result, err := al.redis.LRange(ctx, key, 0, -1).Result()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get audit statistics: %w", err)
	}
	
	stats := &AuditStatistics{
		UserID:     userID,
		Since:      since,
		EventCounts: make(map[audit.EventType]int),
	}
	
	for _, eventJSON := range result {
		var event audit.AuditEvent
		if err := json.Unmarshal([]byte(eventJSON), &event); err == nil {
			// Solo contar eventos desde la fecha especificada
			if event.Timestamp.After(since) {
				stats.TotalEvents++
				stats.EventCounts[event.EventType]++
				
				// Actualizar último evento
				if event.Timestamp.After(stats.LastEventTime) {
					stats.LastEventTime = event.Timestamp
					stats.LastEventType = event.EventType
				}
			}
		}
	}
	
	return stats, nil
}

// AuditStatistics estadísticas de eventos de auditoría
type AuditStatistics struct {
	UserID        string             `json:"user_id"`
	Since         time.Time          `json:"since"`
	TotalEvents   int                `json:"total_events"`
	ErrorEvents   int                `json:"error_events"`
	WarningEvents int                `json:"warning_events"`
	EventCounts   map[audit.EventType]int  `json:"event_counts"`
	LastEventTime time.Time          `json:"last_event_time"`
	LastEventType audit.EventType          `json:"last_event_type"`
}

// storeEventInRedis almacena el evento en Redis
func (al *AuditLogger) storeEventInRedis(event audit.AuditEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	eventJSON, err := json.Marshal(event)
	if err != nil {
		al.logger.Error("Failed to marshal audit event", "error", err)
		return
	}
	
	pipe := al.redis.Pipeline()
	
	// Guardar en log del usuario (si aplica)
	if event.UserID != "" {
		userKey := al.keyUserAuditLog(event.UserID)
		pipe.LPush(ctx, userKey, eventJSON)
		pipe.LTrim(ctx, userKey, 0, 999) // Mantener solo 1000 eventos por usuario
		pipe.Expire(ctx, userKey, 90*24*time.Hour) // TTL de 90 días
	}
	
	// Guardar en log del sistema por tipo de evento
	systemKey := al.keySystemAuditLog(event.EventType)
	pipe.LPush(ctx, systemKey, eventJSON)
	pipe.LTrim(ctx, systemKey, 0, 4999) // Mantener 5000 eventos por tipo
	pipe.Expire(ctx, systemKey, 30*24*time.Hour) // TTL de 30 días
	
	// Guardar en log general del sistema
	allEventsKey := al.keyAllAuditLogs()
	pipe.LPush(ctx, allEventsKey, eventJSON)
	pipe.LTrim(ctx, allEventsKey, 0, 9999) // Mantener 10000 eventos generales
	pipe.Expire(ctx, allEventsKey, 30*24*time.Hour)
	
	_, err = pipe.Exec(ctx)
	if err != nil {
		al.logger.Error("Failed to store audit event in Redis", "error", err)
	}
}

// logEventImmediate registra el evento inmediatamente en el logger estructurado
func (al *AuditLogger) logEventImmediate(event audit.AuditEvent) {
	fields := []interface{}{
		"event_type", event.EventType,
		"timestamp", event.Timestamp,
	}
	
	if event.UserID != "" {
		fields = append(fields, "user_id", event.UserID)
	}
	
	if event.Data != nil {
		for k, v := range event.Data {
			fields = append(fields, "data_"+k, v)
		}
	}
	
	message := fmt.Sprintf("AUDIT: %s", event.EventType)
	
	// Convert slice to individual args for SugaredLogger
	args := make([]interface{}, len(fields))
	for i, v := range fields {
		args[i] = v
	}
	al.logger.Infow(message, args...)
}

// Helper methods para generar keys de Redis
func (al *AuditLogger) keyUserAuditLog(userID string) string {
	return fmt.Sprintf("audit:user:%s", userID)
}

func (al *AuditLogger) keySystemAuditLog(eventType audit.EventType) string {
	return fmt.Sprintf("audit:system:%s", eventType)
}

func (al *AuditLogger) keyAllAuditLogs() string {
	return "audit:all"
}

func (al *AuditLogger) keyCriticalEvents() string {
	return "audit:critical"
}

// Enable habilita el logging de auditoría
func (al *AuditLogger) Enable() {
	al.enabled = true
	al.logger.Info("Audit logging enabled")
}

// Disable deshabilita el logging de auditoría
func (al *AuditLogger) Disable() {
	al.enabled = false
	al.logger.Info("Audit logging disabled")
}

// IsEnabled verifica si el logging de auditoría está habilitado
func (al *AuditLogger) IsEnabled() bool {
	return al.enabled
}