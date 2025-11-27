package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// WebhookEventManager gestiona eventos para webhooks
type WebhookEventManager struct {
	redis  *redis.Client
	logger *logger.Logger
	config *config.Config
}

// NewWebhookEventManager crea un nuevo gestor de eventos webhook
func NewWebhookEventManager(redisClient *redis.Client, log *logger.Logger, cfg *config.Config) *WebhookEventManager {
	return &WebhookEventManager{
		redis:  redisClient,
		logger: log,
		config: cfg,
	}
}

// WebhookEventType tipos de eventos de webhook
type WebhookEventType string

const (
	// Eventos de planes
	WebhookPlanChanged      WebhookEventType = "PLAN_CHANGED"
	WebhookPlanUpgraded     WebhookEventType = "PLAN_UPGRADED"
	WebhookPlanDowngraded   WebhookEventType = "PLAN_DOWNGRADED"
	WebhookUpgradeProrated  WebhookEventType = "UPGRADE_PRORATED"
	
	// Eventos de cuotas
	WebhookOverQuota        WebhookEventType = "OVER_QUOTA"
	WebhookQuotaWarning     WebhookEventType = "QUOTA_WARNING" // 80% del límite
	WebhookQuotaReset       WebhookEventType = "QUOTA_RESET"
	
	// Eventos de operaciones
	WebhookOperationCompleted WebhookEventType = "OPERATION_COMPLETED"
	WebhookOperationFailed   WebhookEventType = "OPERATION_FAILED"
	WebhookFileProcessed     WebhookEventType = "FILE_PROCESSED"
	
	// Eventos de facturación
	WebhookPaymentSucceeded  WebhookEventType = "PAYMENT_SUCCEEDED"
	WebhookPaymentFailed     WebhookEventType = "PAYMENT_FAILED"
	WebhookInvoiceCreated    WebhookEventType = "INVOICE_CREATED"
	
	// Eventos de usuario
	WebhookUserCreated       WebhookEventType = "USER_CREATED"
	WebhookUserUpdated       WebhookEventType = "USER_UPDATED"
	WebhookUserDeactivated   WebhookEventType = "USER_DEACTIVATED"
)

// WebhookEvent representa un evento para webhook
type WebhookEvent struct {
	ID          string                 `json:"id"`
	Type        WebhookEventType       `json:"type"`
	UserID      string                 `json:"user_id"`
	Timestamp   time.Time              `json:"timestamp"`
	Data        map[string]interface{} `json:"data"`
	Attempts    int                    `json:"attempts"`
	MaxAttempts int                    `json:"max_attempts"`
	NextAttempt *time.Time             `json:"next_attempt,omitempty"`
	Status      WebhookStatus          `json:"status"`
	ErrorMsg    string                 `json:"error_msg,omitempty"`
	WebhookURL  string                 `json:"webhook_url,omitempty"`
	
	// Headers personalizados para el webhook
	Headers map[string]string `json:"headers,omitempty"`
	
	// Signature para verificación
	Signature string `json:"signature,omitempty"`
}

// WebhookStatus estado del webhook
type WebhookStatus string

const (
	WebhookStatusPending  WebhookStatus = "pending"
	WebhookStatusSent     WebhookStatus = "sent"
	WebhookStatusFailed   WebhookStatus = "failed"
	WebhookStatusExpired  WebhookStatus = "expired"
)

// QueueEvent encola un evento para envío por webhook
func (wem *WebhookEventManager) QueueEvent(ctx context.Context, event *WebhookEvent) error {
	// Establecer valores por defecto
	if event.ID == "" {
		event.ID = wem.generateEventID()
	}
	
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	
	if event.MaxAttempts == 0 {
		event.MaxAttempts = 5 // Máximo 5 intentos por defecto
	}
	
	if event.Status == "" {
		event.Status = WebhookStatusPending
	}
	
	// Serializar evento
	eventJSON, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal webhook event: %w", err)
	}
	
	// Encolar en Redis
	pipe := wem.redis.Pipeline()
	
	// Agregar a la cola general de webhooks
	pipe.LPush(ctx, wem.keyWebhookQueue(), eventJSON)
	
	// Agregar a la cola específica del usuario
	pipe.LPush(ctx, wem.keyUserWebhookQueue(event.UserID), eventJSON)
	
	// Agregar a la cola específica del tipo de evento
	pipe.LPush(ctx, wem.keyTypeWebhookQueue(event.Type), eventJSON)
	
	// Establecer TTL para auto-limpieza
	ttl := 7 * 24 * time.Hour // 7 días
	pipe.Expire(ctx, wem.keyUserWebhookQueue(event.UserID), ttl)
	pipe.Expire(ctx, wem.keyTypeWebhookQueue(event.Type), ttl)
	
	// Incrementar contador de eventos pendientes
	pipe.Incr(ctx, wem.keyPendingEventsCount())
	pipe.Expire(ctx, wem.keyPendingEventsCount(), ttl)
	
	_, err = pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to queue webhook event: %w", err)
	}
	
	wem.logger.Info("Webhook event queued",
		"event_id", event.ID,
		"event_type", event.Type,
		"user_id", event.UserID,
	)
	
	return nil
}

// CreatePlanChangedEvent crea un evento de cambio de plan
func (wem *WebhookEventManager) CreatePlanChangedEvent(userID string, oldPlan, newPlan config.Plan, effectiveDate time.Time) *WebhookEvent {
	return &WebhookEvent{
		Type:   WebhookPlanChanged,
		UserID: userID,
		Data: map[string]interface{}{
			"old_plan":       oldPlan,
			"new_plan":       newPlan,
			"effective_date": effectiveDate.Format(time.RFC3339),
			"is_upgrade":     wem.isPlanUpgrade(oldPlan, newPlan),
		},
	}
}

// CreateOverQuotaEvent crea un evento de cuota excedida
func (wem *WebhookEventManager) CreateOverQuotaEvent(userID string, quotaType string, currentUsage, limit interface{}) *WebhookEvent {
	return &WebhookEvent{
		Type:   WebhookOverQuota,
		UserID: userID,
		Data: map[string]interface{}{
			"quota_type":    quotaType,
			"current_usage": currentUsage,
			"limit":         limit,
			"exceeded_at":   time.Now().Format(time.RFC3339),
		},
	}
}

// CreateUpgradeProratedEvent crea un evento de upgrade prorrateado
func (wem *WebhookEventManager) CreateUpgradeProratedEvent(userID string, prorationData *config.ProrationCalculation) *WebhookEvent {
	return &WebhookEvent{
		Type:   WebhookUpgradeProrated,
		UserID: userID,
		Data: map[string]interface{}{
			"current_plan":    prorationData.CurrentPlan,
			"new_plan":        prorationData.NewPlan,
			"credit_amount":   prorationData.Credit,
			"charge_amount":   prorationData.ChargeAmount,
			"effective_price": prorationData.EffectivePrice,
			"billing_cycle":   prorationData.BillingCycle,
			"time_remaining":  prorationData.TimeRemaining.String(),
		},
	}
}

// CreateOperationCompletedEvent crea un evento de operación completada
func (wem *WebhookEventManager) CreateOperationCompletedEvent(userID, operationID string, operationType OperationType, result map[string]interface{}) *WebhookEvent {
	return &WebhookEvent{
		Type:   WebhookOperationCompleted,
		UserID: userID,
		Data: map[string]interface{}{
			"operation_id":   operationID,
			"operation_type": operationType,
			"completed_at":   time.Now().Format(time.RFC3339),
			"result":         result,
		},
	}
}

// CreateQuotaWarningEvent crea un evento de advertencia de cuota
func (wem *WebhookEventManager) CreateQuotaWarningEvent(userID string, quotaType string, usagePercent float64, limit interface{}) *WebhookEvent {
	return &WebhookEvent{
		Type:   WebhookQuotaWarning,
		UserID: userID,
		Data: map[string]interface{}{
			"quota_type":     quotaType,
			"usage_percent":  usagePercent,
			"limit":          limit,
			"warning_at":     time.Now().Format(time.RFC3339),
			"threshold":      80.0, // 80% threshold
		},
	}
}

// GetPendingEvents obtiene eventos pendientes para envío
func (wem *WebhookEventManager) GetPendingEvents(ctx context.Context, limit int) ([]*WebhookEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 10 // Límite por defecto
	}
	
	// Obtener eventos de la cola principal
	result, err := wem.redis.LRange(ctx, wem.keyWebhookQueue(), 0, int64(limit-1)).Result()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get pending webhook events: %w", err)
	}
	
	events := make([]*WebhookEvent, 0, len(result))
	for _, eventJSON := range result {
		var event WebhookEvent
		if err := json.Unmarshal([]byte(eventJSON), &event); err == nil {
			// Solo incluir eventos pendientes y que no hayan expirado
			if event.Status == WebhookStatusPending && event.Attempts < event.MaxAttempts {
				events = append(events, &event)
			}
		}
	}
	
	return events, nil
}

// MarkEventAsSent marca un evento como enviado exitosamente
func (wem *WebhookEventManager) MarkEventAsSent(ctx context.Context, eventID string) error {
	return wem.updateEventStatus(ctx, eventID, WebhookStatusSent, "")
}

// MarkEventAsFailed marca un evento como fallido
func (wem *WebhookEventManager) MarkEventAsFailed(ctx context.Context, eventID string, errorMsg string, shouldRetry bool) error {
	status := WebhookStatusFailed
	if shouldRetry {
		status = WebhookStatusPending // Mantener como pending para reintentar
	}
	
	return wem.updateEventStatus(ctx, eventID, status, errorMsg)
}

// updateEventStatus actualiza el estado de un evento
func (wem *WebhookEventManager) updateEventStatus(ctx context.Context, eventID string, status WebhookStatus, errorMsg string) error {
	// Esta implementación es simplificada. En un sistema real, 
	// necesitarías buscar y actualizar el evento específico en las colas.
	// Por simplicidad, registramos el cambio de estado.
	
	updateData := map[string]interface{}{
		"event_id":   eventID,
		"new_status": status,
		"updated_at": time.Now().Format(time.RFC3339),
	}
	
	if errorMsg != "" {
		updateData["error_msg"] = errorMsg
	}
	
	// Guardar actualización de estado en Redis
	statusJSON, _ := json.Marshal(updateData)
	pipe := wem.redis.Pipeline()
	pipe.LPush(ctx, wem.keyWebhookStatusUpdates(), statusJSON)
	pipe.LTrim(ctx, wem.keyWebhookStatusUpdates(), 0, 999) // Mantener últimas 1000 actualizaciones
	pipe.Expire(ctx, wem.keyWebhookStatusUpdates(), 24*time.Hour)
	
	_, err := pipe.Exec(ctx)
	if err != nil {
		wem.logger.Error("Failed to update webhook event status",
			"event_id", eventID,
			"status", status,
			"error", err,
		)
		return err
	}
	
	wem.logger.Info("Webhook event status updated",
		"event_id", eventID,
		"status", status,
		"error_msg", errorMsg,
	)
	
	return nil
}

// GetEventsByUser obtiene eventos de webhook por usuario
func (wem *WebhookEventManager) GetEventsByUser(ctx context.Context, userID string, limit int) ([]*WebhookEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	
	result, err := wem.redis.LRange(ctx, wem.keyUserWebhookQueue(userID), 0, int64(limit-1)).Result()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get user webhook events: %w", err)
	}
	
	events := make([]*WebhookEvent, 0, len(result))
	for _, eventJSON := range result {
		var event WebhookEvent
		if err := json.Unmarshal([]byte(eventJSON), &event); err == nil {
			events = append(events, &event)
		}
	}
	
	return events, nil
}

// GetEventsByType obtiene eventos de webhook por tipo
func (wem *WebhookEventManager) GetEventsByType(ctx context.Context, eventType WebhookEventType, limit int) ([]*WebhookEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	
	result, err := wem.redis.LRange(ctx, wem.keyTypeWebhookQueue(eventType), 0, int64(limit-1)).Result()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get webhook events by type: %w", err)
	}
	
	events := make([]*WebhookEvent, 0, len(result))
	for _, eventJSON := range result {
		var event WebhookEvent
		if err := json.Unmarshal([]byte(eventJSON), &event); err == nil {
			events = append(events, &event)
		}
	}
	
	return events, nil
}

// GetPendingEventsCount obtiene el número de eventos pendientes
func (wem *WebhookEventManager) GetPendingEventsCount(ctx context.Context) (int, error) {
	count, err := wem.redis.Get(ctx, wem.keyPendingEventsCount()).Int()
	if err != nil && err != redis.Nil {
		return 0, fmt.Errorf("failed to get pending events count: %w", err)
	}
	return count, nil
}

// CleanupExpiredEvents limpia eventos expirados
func (wem *WebhookEventManager) CleanupExpiredEvents(ctx context.Context) error {
	// Esta es una implementación simplificada
	// En un sistema real, necesitarías iterar a través de todas las colas
	// y remover eventos expirados basado en timestamp y número de intentos
	
	wem.logger.Info("Starting webhook events cleanup")
	
	// Por ahora, solo registramos la operación de limpieza
	cleanupData := map[string]interface{}{
		"cleanup_at": time.Now().Format(time.RFC3339),
		"action":     "expired_events_cleanup",
	}
	
	cleanupJSON, _ := json.Marshal(cleanupData)
	wem.redis.LPush(ctx, "webhook:cleanup_log", cleanupJSON)
	
	return nil
}

// Helper methods para generar keys de Redis
func (wem *WebhookEventManager) keyWebhookQueue() string {
	return "webhook:queue"
}

func (wem *WebhookEventManager) keyUserWebhookQueue(userID string) string {
	return fmt.Sprintf("webhook:user:%s", userID)
}

func (wem *WebhookEventManager) keyTypeWebhookQueue(eventType WebhookEventType) string {
	return fmt.Sprintf("webhook:type:%s", eventType)
}

func (wem *WebhookEventManager) keyPendingEventsCount() string {
	return "webhook:pending_count"
}

func (wem *WebhookEventManager) keyWebhookStatusUpdates() string {
	return "webhook:status_updates"
}

// generateEventID genera un ID único para el evento
func (wem *WebhookEventManager) generateEventID() string {
	return fmt.Sprintf("wh_%d_%d", time.Now().UnixNano(), time.Now().UnixMilli()%1000)
}

// isPlanUpgrade verifica si es un upgrade de plan
func (wem *WebhookEventManager) isPlanUpgrade(oldPlan, newPlan config.Plan) bool {
	planLevels := map[config.Plan]int{
		config.PlanFree:    1,
		config.PlanPremium: 2,
		config.PlanPro:     3,
	}
	
	return planLevels[newPlan] > planLevels[oldPlan]
}