package auth

import (
	"time"

	"github.com/google/uuid"
)

// APIKey representa una clave de API en la base de datos
type APIKey struct {
	// Identificación
	ID        uuid.UUID `gorm:"type:uuid;primary_key;default:gen_random_uuid()" json:"id"`
	UserID    string    `gorm:"type:varchar(255);not null;index" json:"user_id"`
	CompanyID *string   `gorm:"type:varchar(255);index" json:"company_id,omitempty"`

	// API Key (almacenada como hash SHA-256)
	KeyHash   string `gorm:"type:varchar(64);not null;uniqueIndex" json:"-"` // No exponer en JSON
	KeyPrefix string `gorm:"type:varchar(16);not null" json:"key_prefix"`    // tc_XXXXX para identificación

	// Plan y permisos
	Plan string `gorm:"type:varchar(50);not null;default:'free';index" json:"plan"`
	// Valores: 'free', 'premium', 'pro', 'corporate'

	// Estado
	Active        bool       `gorm:"not null;default:true;index" json:"active"`
	Revoked       bool       `gorm:"not null;default:false;index" json:"revoked"`
	RevokedAt     *time.Time `gorm:"type:timestamp" json:"revoked_at,omitempty"`
	RevokedReason *string    `gorm:"type:text" json:"revoked_reason,omitempty"`

	// Metadata
	Name        *string `gorm:"type:varchar(255)" json:"name,omitempty"`
	Description *string `gorm:"type:text" json:"description,omitempty"`

	// Fechas
	CreatedAt  time.Time  `gorm:"not null;default:NOW()" json:"created_at"`
	UpdatedAt  time.Time  `gorm:"not null;default:NOW()" json:"updated_at"`
	ExpiresAt  *time.Time `gorm:"type:timestamp;index" json:"expires_at,omitempty"`
	LastUsedAt *time.Time `gorm:"type:timestamp" json:"last_used_at,omitempty"`

	// Uso
	TotalRequests int64 `gorm:"not null;default:0" json:"total_requests"`
	TotalBytes    int64 `gorm:"not null;default:0" json:"total_bytes"`

	// Restricciones de seguridad
	AllowedIPs         []string `gorm:"type:text[]" json:"allowed_ips,omitempty"`
	AllowedOrigins     []string `gorm:"type:text[]" json:"allowed_origins,omitempty"`
	RateLimitOverride  *int     `gorm:"type:integer" json:"rate_limit_override,omitempty"`

	// Auditoría
	CreatedBy *string `gorm:"type:varchar(255)" json:"created_by,omitempty"`
	UpdatedBy *string `gorm:"type:varchar(255)" json:"updated_by,omitempty"`
}

// TableName especifica el nombre de la tabla
func (APIKey) TableName() string {
	return "api_keys"
}

// IsValid verifica si la API key es válida para uso
func (k *APIKey) IsValid() bool {
	// Debe estar activa
	if !k.Active {
		return false
	}

	// No debe estar revocada
	if k.Revoked {
		return false
	}

	// Verificar expiración si existe
	if k.ExpiresAt != nil && k.ExpiresAt.Before(time.Now()) {
		return false
	}

	return true
}

// IsExpired verifica si la key ha expirado
func (k *APIKey) IsExpired() bool {
	if k.ExpiresAt == nil {
		return false // Sin expiración = nunca expira
	}
	return k.ExpiresAt.Before(time.Now())
}

// CanUseFromIP verifica si la key puede usarse desde una IP específica
func (k *APIKey) CanUseFromIP(ip string) bool {
	// Si no hay restricción de IPs, permitir todas
	if len(k.AllowedIPs) == 0 {
		return true
	}

	// Verificar si la IP está en la lista permitida
	for _, allowedIP := range k.AllowedIPs {
		if allowedIP == ip {
			return true
		}
	}

	return false
}

// CanUseFromOrigin verifica si la key puede usarse desde un origen específico
func (k *APIKey) CanUseFromOrigin(origin string) bool {
	// Si no hay restricción de orígenes, permitir todos
	if len(k.AllowedOrigins) == 0 {
		return true
	}

	// Verificar si el origen está en la lista permitida
	for _, allowedOrigin := range k.AllowedOrigins {
		if allowedOrigin == origin || allowedOrigin == "*" {
			return true
		}
	}

	return false
}

// APIKeyCreateRequest request para crear nueva API key
type APIKeyCreateRequest struct {
	UserID             string    `json:"user_id" binding:"required"`
	CompanyID          *string   `json:"company_id,omitempty"`
	Plan               string    `json:"plan" binding:"required,oneof=free premium pro corporate"`
	Name               *string   `json:"name,omitempty"`
	Description        *string   `json:"description,omitempty"`
	ExpiresAt          *time.Time `json:"expires_at,omitempty"`
	AllowedIPs         []string  `json:"allowed_ips,omitempty"`
	AllowedOrigins     []string  `json:"allowed_origins,omitempty"`
	RateLimitOverride  *int      `json:"rate_limit_override,omitempty"`
}

// APIKeyResponse respuesta con información de API key
type APIKeyResponse struct {
	ID             uuid.UUID  `json:"id"`
	KeyPrefix      string     `json:"key_prefix"`
	UserID         string     `json:"user_id"`
	Plan           string     `json:"plan"`
	Name           *string    `json:"name,omitempty"`
	Active         bool       `json:"active"`
	CreatedAt      time.Time  `json:"created_at"`
	ExpiresAt      *time.Time `json:"expires_at,omitempty"`
	LastUsedAt     *time.Time `json:"last_used_at,omitempty"`
	TotalRequests  int64      `json:"total_requests"`
	TotalBytes     int64      `json:"total_bytes"`
	AllowedIPs     []string   `json:"allowed_ips,omitempty"`
	AllowedOrigins []string   `json:"allowed_origins,omitempty"`
}

// ToResponse convierte APIKey a APIKeyResponse (sin datos sensibles)
func (k *APIKey) ToResponse() APIKeyResponse {
	return APIKeyResponse{
		ID:             k.ID,
		KeyPrefix:      k.KeyPrefix,
		UserID:         k.UserID,
		Plan:           k.Plan,
		Name:           k.Name,
		Active:         k.Active,
		CreatedAt:      k.CreatedAt,
		ExpiresAt:      k.ExpiresAt,
		LastUsedAt:     k.LastUsedAt,
		TotalRequests:  k.TotalRequests,
		TotalBytes:     k.TotalBytes,
		AllowedIPs:     k.AllowedIPs,
		AllowedOrigins: k.AllowedOrigins,
	}
}

// APIKeyCreateResponse respuesta al crear una API key (incluye key en texto plano)
type APIKeyCreateResponse struct {
	APIKey    string            `json:"api_key"` // Solo visible una vez
	KeyInfo   APIKeyResponse    `json:"key_info"`
	Warning   string            `json:"warning"`
}
