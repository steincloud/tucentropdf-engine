package middleware

import (
	"strings"

	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"

	"github.com/tucentropdf/engine-v2/internal/auth"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// AuthMiddleware middleware de autenticación
type AuthMiddleware struct {
	config        *config.Config
	logger        *logger.Logger
	apiKeyManager *auth.APIKeyManager
}

// NewAuthMiddleware crear nuevo middleware de autenticación
func NewAuthMiddleware(cfg *config.Config, log *logger.Logger, db *gorm.DB) *AuthMiddleware {
	return &AuthMiddleware{
		config:        cfg,
		logger:        log,
		apiKeyManager: auth.NewAPIKeyManager(db),
	}
}

// Authenticate middleware de autenticación
func (m *AuthMiddleware) Authenticate() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Obtener token de autenticación
		token := m.extractToken(c)
		if token == "" {
			m.logger.Warn("Falta header de autenticación", "ip", c.IP(), "path", c.Path())
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Falta header de autenticación. Use X-API-Key o Authorization Bearer",
				"code": "MISSING_AUTH",
			})
		}

		// Validar token y obtener información del usuario
		userInfo, err := m.validateToken(token)
		if err != nil {
			m.logger.Warn("Token inválido", 
				"ip", c.IP(), 
				"path", c.Path(),
				"error", err.Error(),
			)
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Token de autenticación inválido",
				"code": "INVALID_AUTH",
			})
		}

		// Establecer información del usuario en el contexto
		c.Locals("userID", userInfo.ID)
		c.Locals("userPlan", userInfo.Plan)
		c.Locals("apiKey", token)

		m.logger.Debug("Autenticación exitosa", 
			"ip", c.IP(), 
			"path", c.Path(),
			"plan", userInfo.Plan,
		)

		return c.Next()
	}
}

// UserInfo información del usuario autenticado
type UserInfo struct {
	ID   string `json:"id"`
	Plan string `json:"plan"`
	Name string `json:"name,omitempty"`
}

// extractToken extraer token de los headers
func (m *AuthMiddleware) extractToken(c *fiber.Ctx) string {
	// Verificar header X-API-Key
	token := c.Get("X-API-Key")
	if token != "" {
		return token
	}

	// Verificar header Authorization Bearer
	auth := c.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	// Verificar ENGINE_SECRET para retrocompatibilidad
	engineSecret := c.Get("X-ENGINE-SECRET")
	if engineSecret != "" {
		return engineSecret
	}

	return ""
}

// validateToken validar token y obtener información del usuario
func (m *AuthMiddleware) validateToken(token string) (*UserInfo, error) {
	// Fallback: ENGINE_SECRET para retrocompatibilidad temporal
	if token == m.config.EngineSecret && m.config.EngineSecret != "" {
		m.logger.Warn("Using ENGINE_SECRET for authentication (deprecated)",
			"warning", "Migrate to API Keys for production")
		return &UserInfo{
			ID:   "admin",
			Plan: "corporate",
			Name: "Admin (Legacy)",
		}, nil
	}

	// Validación REAL con API Keys
	apiKey, err := m.apiKeyManager.ValidateAPIKey(token)
	if err != nil {
		m.logger.Debug("API key validation failed",
			"error", err.Error(),
			"key_prefix", extractKeyPrefix(token))
		return nil, err
	}

	// Verificar IP si hay restricciones
	// Nota: c.IP() no está disponible aquí, se debe pasar desde Authenticate()
	
	// Retornar información del usuario
	userName := "User"
	if apiKey.Name != nil {
		userName = *apiKey.Name
	}

	return &UserInfo{
		ID:   apiKey.UserID,
		Plan: apiKey.Plan,
		Name: userName,
	}, nil
}

// extractKeyPrefix extrae el prefijo de una API key para logging
func extractKeyPrefix(token string) string {
	if len(token) < 8 {
		return "invalid"
	}
	return token[:8]
}