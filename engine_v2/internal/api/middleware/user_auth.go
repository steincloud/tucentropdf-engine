package middleware

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	audit "github.com/tucentropdf/engine-v2/internal/audit"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/internal/storage"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// UserAuthMiddleware middleware de autenticación con gestión de usuarios
type UserAuthMiddleware struct {
	config      *config.Config
	logger      *logger.Logger
	userManager *storage.UserManager
	auditLogger *AuditLogger
}

// NewUserAuthMiddleware crear nuevo middleware de autenticación de usuarios
func NewUserAuthMiddleware(
	cfg *config.Config,
	log *logger.Logger,
	userManager *storage.UserManager,
	auditLogger *AuditLogger,
) *UserAuthMiddleware {
	return &UserAuthMiddleware{
		config:      cfg,
		logger:      log,
		userManager: userManager,
		auditLogger: auditLogger,
	}
}

// AuthenticateUser middleware principal de autenticación
func (m *UserAuthMiddleware) AuthenticateUser() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Obtener token de autenticación
		token := m.extractToken(c)
		if token == "" {
			m.logFailedAuth(c, "missing_token", "No authentication token provided")
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Token de autenticación requerido. Use X-API-Key o Authorization Bearer",
				"code": "MISSING_AUTH",
			})
		}

		// Validar token y obtener información del usuario
		user, err := m.validateTokenAndGetUser(c.Context(), token)
		if err != nil {
			m.logFailedAuth(c, "invalid_token", err.Error())
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Token de autenticación inválido",
				"code": "INVALID_AUTH",
			})
		}

		// Verificar que el usuario esté activo
		if user.Status != config.UserStatusActive {
			m.logFailedAuth(c, "user_inactive", "User account is not active")
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Cuenta de usuario inactiva",
				"code": "USER_INACTIVE",
			})
		}

		// Obtener límites del plan del usuario
		planLimits := user.GetCurrentPlanLimits()

		// Establecer información del usuario en el contexto
		c.Locals("user", user)
		c.Locals("userID", user.ID)
		c.Locals("userPlan", user.Plan)
		c.Locals("planLimits", planLimits)
		c.Locals("apiKey", token)

		// Log de autenticación exitosa
		m.auditLogger.LogAuthEvent(audit.AuditEvent{
			EventType: audit.EventLogin,
			UserID:    user.ID,
			IPAddress: c.IP(),
			UserAgent: c.Get("User-Agent"),
			Data: map[string]interface{}{
				"plan":   user.Plan,
				"method": "api_key",
				"path":   c.Path(),
			},
			Timestamp: time.Now(),
		})

		m.logger.Debug("User authenticated successfully", 
			"user_id", user.ID,
			"plan", user.Plan,
			"ip", c.IP(),
			"path", c.Path(),
		)

		return c.Next()
	}
}

// OptionalAuth middleware de autenticación opcional (para endpoints públicos con límites)
func (m *UserAuthMiddleware) OptionalAuth() fiber.Handler {
	return func(c *fiber.Ctx) error {
		token := m.extractToken(c)
		
		if token == "" {
			// Sin token, usar usuario anónimo con plan free
			m.setAnonymousUser(c)
			return c.Next()
		}

		// Intentar autenticar si hay token
		user, err := m.validateTokenAndGetUser(c.Context(), token)
		if err != nil {
			// Token inválido, usar usuario anónimo
			m.logger.Warn("Invalid token in optional auth", "error", err.Error(), "ip", c.IP())
			m.setAnonymousUser(c)
			return c.Next()
		}

		// Usuario autenticado exitosamente
		planLimits := user.GetCurrentPlanLimits()
		c.Locals("user", user)
		c.Locals("userID", user.ID)
		c.Locals("userPlan", user.Plan)
		c.Locals("planLimits", planLimits)
		c.Locals("authenticated", true)

		return c.Next()
	}
}

// RequireActivePlan middleware que requiere un plan específico o superior
func (m *UserAuthMiddleware) RequireActivePlan(minimumPlan config.Plan) fiber.Handler {
	return func(c *fiber.Ctx) error {
		user, ok := c.Locals("user").(*config.User)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Usuario autenticado requerido",
				"code": "USER_REQUIRED",
			})
		}

		if !m.hasRequiredPlan(user.Plan, minimumPlan) {
			m.auditLogger.LogEvent(audit.AuditEvent{
				EventType: audit.EventAuthFailure,
				UserID:    user.ID,
				Data: map[string]interface{}{
					"reason":          "insufficient_plan",
					"current_plan":    user.Plan,
					"required_plan":   minimumPlan,
					"attempted_path":  c.Path(),
				},
				Timestamp: time.Now(),
			})

			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Plan insuficiente para acceder a este recurso",
				"code": "INSUFFICIENT_PLAN",
				"details": fiber.Map{
					"current_plan":  user.Plan,
					"required_plan": minimumPlan,
					"upgrade_url":   "/upgrade",
				},
			})
		}

		return c.Next()
	}
}

// RequireFeature middleware que requiere una característica específica habilitada
func (m *UserAuthMiddleware) RequireFeature(feature string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		user, ok := c.Locals("user").(*config.User)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Usuario autenticado requerido",
				"code": "USER_REQUIRED",
			})
		}

		planLimits := user.GetCurrentPlanLimits()
		
		if !m.hasFeatureEnabled(feature, planLimits) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "Característica no disponible en tu plan actual",
				"code": "FEATURE_NOT_AVAILABLE",
				"details": fiber.Map{
					"feature":     feature,
					"current_plan": user.Plan,
				},
			})
		}

		return c.Next()
	}
}

// extractToken extrae el token de autenticación de los headers
func (m *UserAuthMiddleware) extractToken(c *fiber.Ctx) string {
	// Verificar header X-API-Key (preferido)
	if token := c.Get("X-API-Key"); token != "" {
		return token
	}

	// Verificar header Authorization Bearer
	if auth := c.Get("Authorization"); auth != "" {
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer ")
		}
	}

	// Verificar ENGINE_SECRET para retrocompatibilidad
	if engineSecret := c.Get("X-ENGINE-SECRET"); engineSecret != "" {
		return engineSecret
	}

	return ""
}

// validateTokenAndGetUser valida el token y obtiene el usuario
func (m *UserAuthMiddleware) validateTokenAndGetUser(ctx context.Context, token string) (*config.User, error) {
	// Para ENGINE_SECRET (admin)
	if token == m.config.EngineSecret {
		// Crear o obtener usuario admin
		return m.getOrCreateAdminUser(ctx)
	}

	// Para tokens normales, extraer userID del token
	// En una implementación real, aquí buscarías el token en la base de datos
	// Por ahora, usaremos el prefijo del token para simular diferentes usuarios
	userID := m.extractUserIDFromToken(token)
	if userID == "" {
		return nil, fmt.Errorf("invalid token format")
	}

	// Obtener usuario desde el manager
	user, err := m.userManager.GetUser(ctx, userID)
	if err != nil {
		// Si el usuario no existe, crearlo automáticamente para testing
		if strings.Contains(err.Error(), "not found") {
			return m.createUserFromToken(ctx, token, userID)
		}
		return nil, err
	}

	return user, nil
}

// extractUserIDFromToken extrae el userID del token (implementación simplificada)
func (m *UserAuthMiddleware) extractUserIDFromToken(token string) string {
	// Implementación simplificada basada en prefijos
	switch {
	case strings.HasPrefix(token, "free_"):
		return "user_" + token[5:] // Remover prefijo "free_"
	case strings.HasPrefix(token, "premium_"):
		return "user_" + token[8:] // Remover prefijo "premium_"
	case strings.HasPrefix(token, "pro_"):
		return "user_" + token[4:] // Remover prefijo "pro_"
	case len(token) >= 10:
		// Token genérico, usar los últimos caracteres como ID
		return "user_" + token[len(token)-8:]
	default:
		return ""
	}
}

// createUserFromToken crea un usuario basado en el token (para testing/desarrollo)
func (m *UserAuthMiddleware) createUserFromToken(ctx context.Context, token, userID string) (*config.User, error) {
	// Determinar plan basado en el prefijo del token
	var plan config.Plan
	switch {
	case strings.HasPrefix(token, "free_"):
		plan = config.PlanFree
	case strings.HasPrefix(token, "premium_"):
		plan = config.PlanPremium
	case strings.HasPrefix(token, "pro_"):
		plan = config.PlanPro
	default:
		plan = config.PlanFree
	}

	// Crear usuario
	user := config.NewUser(userID, "")
	user.Plan = plan
	user.Name = fmt.Sprintf("User %s", userID)

	// Crear usuario en el sistema
	if err := m.userManager.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	return user, nil
}

// getOrCreateAdminUser obtiene o crea el usuario administrador
func (m *UserAuthMiddleware) getOrCreateAdminUser(ctx context.Context) (*config.User, error) {
	adminID := "admin"
	
	// Intentar obtener usuario admin existente
	admin, err := m.userManager.GetUser(ctx, adminID)
	if err == nil {
		return admin, nil
	}

	// Crear usuario admin si no existe
	admin = config.NewUser(adminID, "admin@tucentropdf.com")
	admin.Plan = config.PlanPro // Admin siempre tiene plan Pro
	admin.Name = "Administrator"

	if err := m.userManager.CreateUser(ctx, admin); err != nil {
		return nil, fmt.Errorf("failed to create admin user: %w", err)
	}

	return admin, nil
}

// setAnonymousUser establece un usuario anónimo con plan free
func (m *UserAuthMiddleware) setAnonymousUser(c *fiber.Ctx) {
	anonymousUser := config.NewUser("anonymous", "")
	anonymousUser.Plan = config.PlanFree
	
	planLimits := anonymousUser.GetCurrentPlanLimits()
	
	c.Locals("user", anonymousUser)
	c.Locals("userID", "anonymous")
	c.Locals("userPlan", config.PlanFree)
	c.Locals("planLimits", planLimits)
	c.Locals("authenticated", false)
}

// hasRequiredPlan verifica si el plan actual cumple con el mínimo requerido
func (m *UserAuthMiddleware) hasRequiredPlan(currentPlan, requiredPlan config.Plan) bool {
	planLevels := map[config.Plan]int{
		config.PlanFree:    1,
		config.PlanPremium: 2,
		config.PlanPro:     3,
	}
	
	currentLevel := planLevels[currentPlan]
	requiredLevel := planLevels[requiredPlan]
	
	return currentLevel >= requiredLevel
}

// hasFeatureEnabled verifica si una característica está habilitada
func (m *UserAuthMiddleware) hasFeatureEnabled(feature string, limits config.PlanLimits) bool {
	switch feature {
	case "ai_ocr":
		return limits.EnableAIOCR
	case "priority_processing":
		return limits.EnablePriority
	case "analytics":
		return limits.EnableAnalytics
	case "no_watermark":
		return !limits.HasWatermark
	default:
		return true // Características desconocidas son permitidas por defecto
	}
}

// logFailedAuth registra intentos de autenticación fallidos
func (m *UserAuthMiddleware) logFailedAuth(c *fiber.Ctx, reason, details string) {
	m.auditLogger.LogAuthEvent(audit.AuditEvent{
		EventType: audit.EventAuthFailure,
		IPAddress: c.IP(),
		UserAgent: c.Get("User-Agent"),
		Data: map[string]interface{}{
			"reason":  reason,
			"details": details,
			"path":    c.Path(),
			"method":  c.Method(),
		},
		Timestamp: time.Now(),
	})

	m.logger.Warn("Authentication failed",
		"reason", reason,
		"details", details,
		"ip", c.IP(),
		"path", c.Path(),
	)
}