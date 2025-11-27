package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// APIKeyManager gestiona API keys
type APIKeyManager struct {
	db *gorm.DB
}

// NewAPIKeyManager crea un nuevo gestor de API keys
func NewAPIKeyManager(db *gorm.DB) *APIKeyManager {
	return &APIKeyManager{
		db: db,
	}
}

// GenerateAPIKey genera una nueva API key segura
// Formato: tc_XXXXX_YYYYYYYYYYYYYYYYYYYYYYYYYYYY (8 + 32 caracteres)
func (m *APIKeyManager) GenerateAPIKey() (string, error) {
	// Prefijo: tc_
	prefix := "tc_"

	// Identificador corto: 5 caracteres aleatorios
	shortID := make([]byte, 5)
	if _, err := rand.Read(shortID); err != nil {
		return "", fmt.Errorf("failed to generate short ID: %w", err)
	}
	shortIDStr := fmt.Sprintf("%X", shortID)[:5]

	// Parte secreta: 32 bytes (64 caracteres hex)
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		return "", fmt.Errorf("failed to generate secret: %w", err)
	}
	secretStr := hex.EncodeToString(secret)

	// API Key completa: tc_XXXXX_YYYYYYYYYYYY...
	apiKey := fmt.Sprintf("%s%s_%s", prefix, shortIDStr, secretStr)

	return apiKey, nil
}

// HashAPIKey genera el hash SHA-256 de una API key
func (m *APIKeyManager) HashAPIKey(apiKey string) string {
	hash := sha256.Sum256([]byte(apiKey))
	return hex.EncodeToString(hash[:])
}

// ExtractKeyPrefix extrae el prefijo de una API key (tc_XXXXX)
func (m *APIKeyManager) ExtractKeyPrefix(apiKey string) string {
	if len(apiKey) < 8 {
		return apiKey
	}
	return apiKey[:8]
}

// CreateAPIKey crea una nueva API key en la base de datos
func (m *APIKeyManager) CreateAPIKey(req APIKeyCreateRequest) (*APIKeyCreateResponse, error) {
	// Generar API key
	apiKey, err := m.GenerateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	// Hash de la key
	keyHash := m.HashAPIKey(apiKey)
	keyPrefix := m.ExtractKeyPrefix(apiKey)

	// Crear registro en DB
	dbKey := APIKey{
		ID:                uuid.New(),
		UserID:            req.UserID,
		CompanyID:         req.CompanyID,
		KeyHash:           keyHash,
		KeyPrefix:         keyPrefix,
		Plan:              req.Plan,
		Name:              req.Name,
		Description:       req.Description,
		Active:            true,
		Revoked:           false,
		ExpiresAt:         req.ExpiresAt,
		AllowedIPs:        req.AllowedIPs,
		AllowedOrigins:    req.AllowedOrigins,
		RateLimitOverride: req.RateLimitOverride,
		TotalRequests:     0,
		TotalBytes:        0,
	}

	if err := m.db.Create(&dbKey).Error; err != nil {
		return nil, fmt.Errorf("failed to create API key in database: %w", err)
	}

	// Respuesta con la key en texto plano (solo visible una vez)
	response := &APIKeyCreateResponse{
		APIKey:  apiKey,
		KeyInfo: dbKey.ToResponse(),
		Warning: "⚠️ Guarda esta API key en un lugar seguro. No podrás verla de nuevo.",
	}

	return response, nil
}

// ValidateAPIKey valida una API key y retorna la información asociada
func (m *APIKeyManager) ValidateAPIKey(apiKey string) (*APIKey, error) {
	// Hash de la key recibida
	keyHash := m.HashAPIKey(apiKey)

	// Buscar en DB
	var dbKey APIKey
	err := m.db.Where("key_hash = ?", keyHash).First(&dbKey).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid API key")
		}
		return nil, fmt.Errorf("database error: %w", err)
	}

	// Validar estado
	if !dbKey.IsValid() {
		if !dbKey.Active {
			return nil, errors.New("API key is inactive")
		}
		if dbKey.Revoked {
			return nil, errors.New("API key has been revoked")
		}
		if dbKey.IsExpired() {
			return nil, errors.New("API key has expired")
		}
		return nil, errors.New("API key is not valid")
	}

	return &dbKey, nil
}

// TrackUsage registra el uso de una API key
func (m *APIKeyManager) TrackUsage(keyHash string, bytes int64) error {
	return m.db.Model(&APIKey{}).
		Where("key_hash = ?", keyHash).
		Updates(map[string]interface{}{
			"total_requests": gorm.Expr("total_requests + ?", 1),
			"total_bytes":    gorm.Expr("total_bytes + ?", bytes),
			"last_used_at":   time.Now(),
		}).Error
}

// RevokeAPIKey revoca una API key
func (m *APIKeyManager) RevokeAPIKey(keyHash string, reason string, revokedBy string) error {
	now := time.Now()
	return m.db.Model(&APIKey{}).
		Where("key_hash = ? AND active = ? AND revoked = ?", keyHash, true, false).
		Updates(map[string]interface{}{
			"revoked":        true,
			"revoked_at":     now,
			"revoked_reason": reason,
			"updated_by":     revokedBy,
		}).Error
}

// GetAPIKeyByHash obtiene una API key por su hash
func (m *APIKeyManager) GetAPIKeyByHash(keyHash string) (*APIKey, error) {
	var dbKey APIKey
	err := m.db.Where("key_hash = ?", keyHash).First(&dbKey).Error
	if err != nil {
		return nil, err
	}
	return &dbKey, nil
}

// ListAPIKeysByUser lista todas las API keys de un usuario
func (m *APIKeyManager) ListAPIKeysByUser(userID string) ([]APIKey, error) {
	var keys []APIKey
	err := m.db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&keys).Error
	return keys, err
}

// ListActiveAPIKeys lista todas las API keys activas
func (m *APIKeyManager) ListActiveAPIKeys() ([]APIKey, error) {
	var keys []APIKey
	err := m.db.Where("active = ? AND revoked = ?", true, false).
		Where("expires_at IS NULL OR expires_at > ?", time.Now()).
		Order("created_at DESC").
		Find(&keys).Error
	return keys, err
}

// DeleteAPIKey elimina permanentemente una API key (usar con precaución)
func (m *APIKeyManager) DeleteAPIKey(keyHash string) error {
	return m.db.Where("key_hash = ?", keyHash).Delete(&APIKey{}).Error
}

// DeactivateAPIKey desactiva una API key (sin eliminarla)
func (m *APIKeyManager) DeactivateAPIKey(keyHash string, updatedBy string) error {
	return m.db.Model(&APIKey{}).
		Where("key_hash = ?", keyHash).
		Updates(map[string]interface{}{
			"active":     false,
			"updated_by": updatedBy,
		}).Error
}

// ReactivateAPIKey reactiva una API key desactivada
func (m *APIKeyManager) ReactivateAPIKey(keyHash string, updatedBy string) error {
	return m.db.Model(&APIKey{}).
		Where("key_hash = ? AND revoked = ?", keyHash, false).
		Updates(map[string]interface{}{
			"active":     true,
			"updated_by": updatedBy,
		}).Error
}

// UpdateAPIKeyPlan actualiza el plan de una API key
func (m *APIKeyManager) UpdateAPIKeyPlan(keyHash string, newPlan string, updatedBy string) error {
	// Validar plan
	validPlans := map[string]bool{
		"free":      true,
		"premium":   true,
		"pro":       true,
		"corporate": true,
	}

	if !validPlans[newPlan] {
		return fmt.Errorf("invalid plan: %s", newPlan)
	}

	return m.db.Model(&APIKey{}).
		Where("key_hash = ?", keyHash).
		Updates(map[string]interface{}{
			"plan":       newPlan,
			"updated_by": updatedBy,
		}).Error
}

// GetAPIKeyStats obtiene estadísticas de uso de una API key
func (m *APIKeyManager) GetAPIKeyStats(keyHash string) (map[string]interface{}, error) {
	var dbKey APIKey
	err := m.db.Where("key_hash = ?", keyHash).First(&dbKey).Error
	if err != nil {
		return nil, err
	}

	stats := map[string]interface{}{
		"key_prefix":      dbKey.KeyPrefix,
		"user_id":         dbKey.UserID,
		"plan":            dbKey.Plan,
		"total_requests":  dbKey.TotalRequests,
		"total_bytes":     dbKey.TotalBytes,
		"total_bytes_mb":  float64(dbKey.TotalBytes) / (1024 * 1024),
		"created_at":      dbKey.CreatedAt,
		"last_used_at":    dbKey.LastUsedAt,
		"expires_at":      dbKey.ExpiresAt,
		"is_valid":        dbKey.IsValid(),
		"is_expired":      dbKey.IsExpired(),
		"active":          dbKey.Active,
		"revoked":         dbKey.Revoked,
	}

	return stats, nil
}

// CleanupExpiredKeys elimina keys expiradas (tarea de mantenimiento)
func (m *APIKeyManager) CleanupExpiredKeys() (int64, error) {
	result := m.db.Where("expires_at IS NOT NULL AND expires_at < ?", time.Now()).
		Delete(&APIKey{})
	return result.RowsAffected, result.Error
}

// GetAPIKeyCount obtiene el conteo de API keys por estado
func (m *APIKeyManager) GetAPIKeyCount() (map[string]int64, error) {
	counts := make(map[string]int64)

	// Total
	var total int64
	m.db.Model(&APIKey{}).Count(&total)
	counts["total"] = total

	// Activas
	var active int64
	m.db.Model(&APIKey{}).Where("active = ?", true).Count(&active)
	counts["active"] = active

	// Revocadas
	var revoked int64
	m.db.Model(&APIKey{}).Where("revoked = ?", true).Count(&revoked)
	counts["revoked"] = revoked

	// Expiradas
	var expired int64
	m.db.Model(&APIKey{}).
		Where("expires_at IS NOT NULL AND expires_at < ?", time.Now()).
		Count(&expired)
	counts["expired"] = expired

	// Por plan
	var planCounts []struct {
		Plan  string
		Count int64
	}
	m.db.Model(&APIKey{}).
		Select("plan, COUNT(*) as count").
		Where("active = ?", true).
		Group("plan").
		Scan(&planCounts)

	for _, pc := range planCounts {
		counts["plan_"+pc.Plan] = pc.Count
	}

	return counts, nil
}
