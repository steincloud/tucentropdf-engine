package audit

import "time"

// EventType define el tipo de evento de auditoría
type EventType string

const (
	EventPlanChanged  EventType = "plan_changed"
	EventUserLogin    EventType = "user_login"
	EventLogin        EventType = "login"
	EventAuthFailure  EventType = "auth_failure"
	EventFileAccess   EventType = "file_access"
	EventAPICall      EventType = "api_call"
	EventQuotaReach   EventType = "quota_reached"
	EventError        EventType = "error"
)

// AuditEvent representa un evento de auditoría
type AuditEvent struct {
	EventType EventType              `json:"event_type"`
	UserID    string                 `json:"user_id"`
	IPAddress string                 `json:"ip_address"`
	UserAgent string                 `json:"user_agent"`
	Data      map[string]interface{} `json:"data"`
	Timestamp time.Time              `json:"timestamp"`
}

// AuditLogger interface para el logging de auditoría
type AuditLogger interface {
	LogEvent(event AuditEvent)
	LogAuthEvent(event AuditEvent)
	LogPlanEvent(event AuditEvent)
}