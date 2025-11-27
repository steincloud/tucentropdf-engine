package config

import (
	"encoding/json"
	"fmt"
	"time"
)

// User representa un usuario del sistema
type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email,omitempty"`
	Name      string    `json:"name,omitempty"`
	Plan      Plan      `json:"plan"`
	Status    UserStatus `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	
	// Información de facturación
	BillingCycle    BillingCycle `json:"billing_cycle,omitempty"`
	SubscriptionID  string       `json:"subscription_id,omitempty"`
	LastPayment     *time.Time   `json:"last_payment,omitempty"`
	NextPayment     *time.Time   `json:"next_payment,omitempty"`
	
	// API Keys asociadas
	APIKeys []APIKey `json:"api_keys,omitempty"`
	
	// Configuración específica del usuario
	Settings UserSettings `json:"settings"`
	
	// Metadatos adicionales
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// UserStatus representa el estado de un usuario
type UserStatus string

const (
	UserStatusActive    UserStatus = "active"
	UserStatusInactive  UserStatus = "inactive"
	UserStatusSuspended UserStatus = "suspended"
	UserStatusCanceled  UserStatus = "canceled"
)

// String implementa Stringer interface
func (u UserStatus) String() string {
	return string(u)
}

// APIKey representa una clave API de usuario
type APIKey struct {
	Key       string     `json:"key"`
	Name      string     `json:"name"`
	IsActive  bool       `json:"is_active"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	LastUsed  *time.Time `json:"last_used,omitempty"`
	
	// Permisos específicos de la API key
	Permissions []string `json:"permissions,omitempty"`
	
	// Límites específicos para esta API key (opcional)
	CustomLimits *PlanLimits `json:"custom_limits,omitempty"`
}

// UserSettings configuración específica del usuario
type UserSettings struct {
	// Preferencias de notificaciones
	EmailNotifications bool `json:"email_notifications"`
	WebhookURL         string `json:"webhook_url,omitempty"`
	
	// Preferencias de procesamiento
	DefaultLanguage    string `json:"default_language"`
	DefaultQuality     string `json:"default_quality"`
	AutoOptimize       bool   `json:"auto_optimize"`
	
	// Configuración de límites personalizados (para usuarios enterprise)
	CustomRateLimit    *int `json:"custom_rate_limit,omitempty"`
	CustomPriority     *int `json:"custom_priority,omitempty"`
	
	// Timezone para resetear contadores
	Timezone string `json:"timezone"`
}

// UserUsageStats estadísticas de uso del usuario
type UserUsageStats struct {
	UserID  string `json:"user_id"`
	Plan    Plan   `json:"plan"`
	
	// Contadores diarios (se resetean cada día)
	DailyStats DailyUsageStats `json:"daily_stats"`
	
	// Contadores mensuales (se resetean cada mes)
	MonthlyStats MonthlyUsageStats `json:"monthly_stats"`
	
	// Última actualización y reseteos
	LastUpdated     time.Time `json:"last_updated"`
	LastDailyReset  time.Time `json:"last_daily_reset"`
	LastMonthlyReset time.Time `json:"last_monthly_reset"`
}

// DailyUsageStats contadores diarios
type DailyUsageStats struct {
	Operations     int   `json:"operations"`
	FilesProcessed int   `json:"files_processed"`
	PagesProcessed int   `json:"pages_processed"`
	BytesProcessed int64 `json:"bytes_processed"`
	
	// Contadores específicos por tipo
	OCRPages    int `json:"ocr_pages"`
	AIOCRPages  int `json:"ai_ocr_pages"`
	OfficePages int `json:"office_pages"`
	
	// Estadísticas adicionales
	Errors      int   `json:"errors"`
	APIRequests int   `json:"api_requests"`
	TotalTime   int64 `json:"total_time_ms"` // Tiempo total de procesamiento en ms
}

// MonthlyUsageStats contadores mensuales
type MonthlyUsageStats struct {
	Operations     int   `json:"operations"`
	FilesProcessed int   `json:"files_processed"`
	PagesProcessed int   `json:"pages_processed"`
	BytesProcessed int64 `json:"bytes_processed"`
	
	// Contadores específicos por tipo
	OCRPages    int `json:"ocr_pages"`
	AIOCRPages  int `json:"ai_ocr_pages"`
	OfficePages int `json:"office_pages"`
	
	// Estadísticas adicionales
	Errors      int   `json:"errors"`
	APIRequests int   `json:"api_requests"`
	TotalTime   int64 `json:"total_time_ms"` // Tiempo total de procesamiento en ms
}

// GetCurrentPlanLimits obtiene los límites actuales del plan del usuario
func (u *User) GetCurrentPlanLimits() PlanLimits {
	config := GetDefaultPlanConfiguration()
	return config.GetPlanLimits(u.Plan)
}

// GetCurrentPlanPricing obtiene el precio actual del plan del usuario
func (u *User) GetCurrentPlanPricing() PlanPricing {
	config := GetDefaultPlanConfiguration()
	return config.GetPlanPricing(u.Plan)
}

// CanUpgradeToPlan verifica si el usuario puede actualizar a un plan específico
func (u *User) CanUpgradeToPlan(targetPlan Plan) bool {
	if !targetPlan.IsValid() {
		return false
	}
	
	// No se puede "actualizar" al mismo plan
	if u.Plan == targetPlan {
		return false
	}
	
	// Solo usuarios activos pueden actualizar
	if u.Status != UserStatusActive {
		return false
	}
	
	// Verificar que sea una actualización (no downgrade)
	return u.isPlanUpgrade(targetPlan)
}

// isPlanUpgrade verifica si el cambio es una actualización
func (u *User) isPlanUpgrade(targetPlan Plan) bool {
	planHierarchy := map[Plan]int{
		PlanFree:    1,
		PlanPremium: 2,
		PlanPro:     3,
	}
	
	currentLevel := planHierarchy[u.Plan]
	targetLevel := planHierarchy[targetPlan]
	
	return targetLevel > currentLevel
}

// ToJSON convierte el usuario a JSON
func (u *User) ToJSON() ([]byte, error) {
	return json.Marshal(u)
}

// FromJSON carga datos del usuario desde JSON
func (u *User) FromJSON(data []byte) error {
	return json.Unmarshal(data, u)
}

// Validate valida los datos del usuario
func (u *User) Validate() error {
	if u.ID == "" {
		return fmt.Errorf("user ID is required")
	}
	
	if !u.Plan.IsValid() {
		return fmt.Errorf("invalid plan: %s", u.Plan)
	}
	
	if u.Email != "" {
		// Aquí podrías agregar validación de email
		// Por simplicidad, solo verificamos que no esté vacío si se proporciona
	}
	
	// Validar timezone si se especifica
	if u.Settings.Timezone != "" {
		if _, err := time.LoadLocation(u.Settings.Timezone); err != nil {
			return fmt.Errorf("invalid timezone: %s", u.Settings.Timezone)
		}
	}
	
	return nil
}

// NewUser crea un nuevo usuario con valores por defecto
func NewUser(id, email string) *User {
	now := time.Now()
	
	return &User{
		ID:        id,
		Email:     email,
		Plan:      PlanFree, // Plan por defecto
		Status:    UserStatusActive,
		CreatedAt: now,
		UpdatedAt: now,
		
		Settings: UserSettings{
			EmailNotifications: true,
			DefaultLanguage:    "es",
			DefaultQuality:     "medium",
			AutoOptimize:       true,
			Timezone:          "America/Mexico_City",
		},
		
		Metadata: make(map[string]interface{}),
	}
}