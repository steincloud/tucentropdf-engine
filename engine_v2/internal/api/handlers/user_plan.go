package handlers

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/tucentropdf/engine-v2/internal/api/middleware"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/internal/storage"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// UserPlanHandler maneja endpoints relacionados con usuarios y planes
type UserPlanHandler struct {
	logger         *logger.Logger
	userManager    *storage.UserManager
	usageTracker   *storage.UsageTracker
	auditLogger    *middleware.AuditLogger
	webhookManager *storage.WebhookEventManager
}

// NewUserPlanHandler crea un nuevo handler de usuarios y planes
func NewUserPlanHandler(
	log *logger.Logger,
	userManager *storage.UserManager,
	usageTracker *storage.UsageTracker,
	auditLogger *middleware.AuditLogger,
	webhookManager *storage.WebhookEventManager,
) *UserPlanHandler {
	return &UserPlanHandler{
		logger:         log,
		userManager:    userManager,
		usageTracker:   usageTracker,
		auditLogger:    auditLogger,
		webhookManager: webhookManager,
	}
}

// GetUserProfile obtiene el perfil del usuario actual
func (h *UserPlanHandler) GetUserProfile(c *fiber.Ctx) error {
	user, ok := c.Locals("user").(*config.User)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"success": false, "error": "USER_REQUIRED", "message": "Usuario autenticado requerido"})
	}

	// Obtener estadísticas de uso
	usage, err := h.usageTracker.GetUserUsage(c.Context(), user.ID)
	if err != nil {
		h.logger.Warn("Failed to get user usage", "user_id", user.ID, "error", err)
		usage = &config.UserUsageStats{} // Estadísticas vacías como fallback
	}

	// Obtener límites del plan
	planLimits := user.GetCurrentPlanLimits()
	planPricing := user.GetCurrentPlanPricing()

	profile := map[string]interface{}{
		"user": map[string]interface{}{
			"id":           user.ID,
			"email":        user.Email,
			"name":         user.Name,
			"plan":         user.Plan,
			"status":       user.Status,
			"created_at":   user.CreatedAt.Format(time.RFC3339),
			"updated_at":   user.UpdatedAt.Format(time.RFC3339),
			"billing_cycle": user.BillingCycle,
		},
		"plan": map[string]interface{}{
			"name":    user.Plan,
			"limits":  planLimits,
			"pricing": planPricing,
		},
		"usage": usage,
		"subscription": map[string]interface{}{
			"subscription_id": user.SubscriptionID,
			"last_payment":    user.LastPayment,
			"next_payment":    user.NextPayment,
		},
	}

	return c.JSON(fiber.Map{"success": true, "message": "User profile retrieved successfully", "data": profile})
}

// GetUserUsage obtiene las estadísticas de uso del usuario
func (h *UserPlanHandler) GetUserUsage(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	usage, err := h.usageTracker.GetUserUsage(c.Context(), userID)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "USAGE_ERROR", "message": "Error al obtener estadísticas de uso"})
	}

	return c.JSON(fiber.Map{"success": true, "message": "Usage statistics retrieved successfully", "data": usage})
}

// ChangePlan cambia el plan del usuario
func (h *UserPlanHandler) ChangePlan(c *fiber.Ctx) error {
	user, ok := c.Locals("user").(*config.User)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"success": false, "error": "USER_REQUIRED", "message": "Usuario autenticado requerido"})
	}

	// Parsear request
	var req struct {
		NewPlan      string `json:"new_plan" validate:"required"`
		BillingCycle string `json:"billing_cycle" validate:"required"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "INVALID_REQUEST", "message": "Formato de solicitud inválido"})
	}

	// Validar nuevo plan
	newPlan := config.Plan(req.NewPlan)
	if !newPlan.IsValid() {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "INVALID_PLAN", "message": "Plan especificado no es válido"})
	}

	// Validar ciclo de facturación
	billingCycle := config.BillingCycle(req.BillingCycle)
	if billingCycle != config.BillingMonthly && billingCycle != config.BillingYearly {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "INVALID_BILLING_CYCLE", "message": "Ciclo de facturación inválido"})
	}

	// Verificar que puede cambiar al nuevo plan
	if !user.CanUpgradeToPlan(newPlan) {
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"success": false,
			"error": "INVALID_PLAN_CHANGE",
			"message": "No se puede cambiar al plan especificado",
			"details": map[string]interface{}{
				"current_plan": user.Plan,
				"requested_plan": newPlan,
			},
		})
	}

	// Cambiar plan con cálculo de prorrateo
	prorationCalc, err := h.userManager.ChangePlan(c.Context(), user.ID, newPlan, billingCycle)
	if err != nil {
		h.logger.Error("Failed to change plan", "user_id", user.ID, "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "PLAN_CHANGE_ERROR", "message": "Error al cambiar el plan"})
	}

	result := map[string]interface{}{
		"success": true,
		"old_plan": user.Plan,
		"new_plan": newPlan,
		"billing_cycle": billingCycle,
		"effective_date": time.Now().Format(time.RFC3339),
	}

	if prorationCalc != nil {
		result["proration"] = prorationCalc
	}

	return c.JSON(fiber.Map{"success": true, "message": "Plan changed successfully", "data": result})
}

// GetPlanComparison obtiene comparación de planes disponibles
func (h *UserPlanHandler) GetPlanComparison(c *fiber.Ctx) error {
	planConfig := config.GetDefaultPlanConfiguration()

	comparison := map[string]interface{}{
		"plans": map[string]interface{}{
			"free": map[string]interface{}{
				"name":     "Gratuito",
				"price":    planConfig.Pricing[config.PlanFree],
				"limits":   planConfig.Plans[config.PlanFree],
				"features": h.getPlanFeatures(config.PlanFree),
			},
			"premium": map[string]interface{}{
				"name":     "Premium",
				"price":    planConfig.Pricing[config.PlanPremium],
				"limits":   planConfig.Plans[config.PlanPremium],
				"features": h.getPlanFeatures(config.PlanPremium),
			},
			"pro": map[string]interface{}{
				"name":     "Pro",
				"price":    planConfig.Pricing[config.PlanPro],
				"limits":   planConfig.Plans[config.PlanPro],
				"features": h.getPlanFeatures(config.PlanPro),
			},
		},
		"recommended": config.PlanPremium, // Plan recomendado
	}

	return c.JSON(fiber.Map{"success": true, "message": "Plan comparison retrieved successfully", "data": comparison})
}

// CalculateProration calcula el prorrateo para un cambio de plan
func (h *UserPlanHandler) CalculateProration(c *fiber.Ctx) error {
	user, ok := c.Locals("user").(*config.User)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"success": false, "error": "USER_REQUIRED", "message": "Usuario autenticado requerido"})
	}

	// Parsear parámetros
	newPlanStr := c.Query("new_plan")
	billingCycleStr := c.Query("billing_cycle", string(user.BillingCycle))

	if newPlanStr == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "MISSING_PLAN", "message": "Parámetro new_plan requerido"})
	}

	newPlan := config.Plan(newPlanStr)
	if !newPlan.IsValid() {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "INVALID_PLAN", "message": "Plan especificado no es válido"})
	}

	billingCycle := config.BillingCycle(billingCycleStr)
	if billingCycle != config.BillingMonthly && billingCycle != config.BillingYearly {
		billingCycle = config.BillingMonthly // Default
	}

	// Calcular prorrateo
	prorationService := config.NewProrationService()
	
	// Estimar tiempo usado (en un sistema real esto vendría de la base de datos)
	timeUsed := time.Now().Sub(user.CreatedAt)
	var cycleDuration time.Duration
	if billingCycle == config.BillingMonthly {
		cycleDuration = 30 * 24 * time.Hour
	} else {
		cycleDuration = 365 * 24 * time.Hour
	}

	// Ajustar timeUsed si excede la duración del ciclo
	if timeUsed > cycleDuration {
		timeUsed = cycleDuration
	}

	prorationCalc, err := prorationService.CalcularProrrateo(
		user.Plan,
		newPlan,
		billingCycle,
		timeUsed,
		cycleDuration,
	)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "PRORATION_ERROR", "message": "Error al calcular prorrateo"})
	}

	return c.JSON(fiber.Map{"success": true, "message": "Proration calculated successfully", "data": prorationCalc})
}

// GetAuditLog obtiene el log de auditoría del usuario
func (h *UserPlanHandler) GetAuditLog(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	// Parsear límite
	limitStr := c.Query("limit", "50")
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 || limit > 1000 {
		limit = 50
	}

	// Obtener log de auditoría
	auditLog, err := h.auditLogger.GetUserAuditLog(c.Context(), userID, limit)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "AUDIT_LOG_ERROR", "message": "Error al obtener log de auditoría"})
	}

	return c.JSON(fiber.Map{"success": true, "message": "Audit log retrieved successfully", "data": map[string]interface{}{
		"user_id": userID,
		"events":  auditLog,
		"total":   len(auditLog),
	}})
}

// GetAuditStatistics obtiene estadísticas de auditoría del usuario
func (h *UserPlanHandler) GetAuditStatistics(c *fiber.Ctx) error {
	userID := c.Locals("userID").(string)

	// Parsear fecha desde cuando obtener estadísticas
	sinceStr := c.Query("since", "7d") // Default: última semana
	since := h.parseDuration(sinceStr)

	stats, err := h.auditLogger.GetAuditStatistics(c.Context(), userID, since)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "AUDIT_STATS_ERROR", "message": "Error al obtener estadísticas de auditoría"})
	}

	return c.JSON(fiber.Map{"success": true, "message": "Audit statistics retrieved successfully", "data": stats})
}

// UpdateUserSettings actualiza la configuración del usuario
func (h *UserPlanHandler) UpdateUserSettings(c *fiber.Ctx) error {
	user, ok := c.Locals("user").(*config.User)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{"success": false, "error": "USER_REQUIRED", "message": "Usuario autenticado requerido"})
	}

	// Parsear nuevas configuraciones
	var req struct {
		EmailNotifications *bool   `json:"email_notifications,omitempty"`
		WebhookURL         *string `json:"webhook_url,omitempty"`
		DefaultLanguage    *string `json:"default_language,omitempty"`
		DefaultQuality     *string `json:"default_quality,omitempty"`
		AutoOptimize       *bool   `json:"auto_optimize,omitempty"`
		Timezone           *string `json:"timezone,omitempty"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "INVALID_REQUEST", "message": "Formato de solicitud inválido"})
	}

	// Aplicar cambios
	oldSettings := user.Settings
	if req.EmailNotifications != nil {
		user.Settings.EmailNotifications = *req.EmailNotifications
	}
	if req.WebhookURL != nil {
		user.Settings.WebhookURL = *req.WebhookURL
	}
	if req.DefaultLanguage != nil {
		user.Settings.DefaultLanguage = *req.DefaultLanguage
	}
	if req.DefaultQuality != nil {
		user.Settings.DefaultQuality = *req.DefaultQuality
	}
	if req.AutoOptimize != nil {
		user.Settings.AutoOptimize = *req.AutoOptimize
	}
	if req.Timezone != nil {
		user.Settings.Timezone = *req.Timezone
	}

	// Actualizar usuario
	if err := h.userManager.UpdateUser(c.Context(), user); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "UPDATE_ERROR", "message": "Error al actualizar configuraciones"})
	}

	// Log de auditoría
	h.auditLogger.LogEvent(middleware.AuditEvent{
		EventType: middleware.EventConfigChanged,
		UserID:    user.ID,
		Data: map[string]interface{}{
			"old_settings": oldSettings,
			"new_settings": user.Settings,
		},
		Timestamp: time.Now(),
	})

	return c.JSON(fiber.Map{"success": true, "message": "User settings updated successfully", "data": map[string]interface{}{
		"settings": user.Settings,
	}})
}

// Helper methods

// getPlanFeatures obtiene las características de un plan
func (h *UserPlanHandler) getPlanFeatures(plan config.Plan) []string {
	switch plan {
	case config.PlanFree:
		return []string{
			"Procesamiento básico de PDFs",
			"OCR básico (sin IA)",
			"Conversión de documentos Office limitada",
			"Marca de agua en resultados",
			"Soporte por email",
		}
	case config.PlanPremium:
		return []string{
			"Todo lo del plan Gratuito",
			"OCR con IA (5 páginas/día)",
			"Sin marcas de agua",
			"Procesamiento prioritario",
			"Análisis y estadísticas",
			"Soporte prioritario",
		}
	case config.PlanPro:
		return []string{
			"Todo lo del plan Premium",
			"OCR con IA ilimitado",
			"Procesamiento de alta prioridad",
			"API webhooks",
			"Análisis avanzados",
			"Soporte dedicado 24/7",
		}
	default:
		return []string{}
	}
}

// parseDuration parsea una duración desde string (ej: "7d", "30d", "1h")
func (h *UserPlanHandler) parseDuration(durationStr string) time.Time {
	var duration time.Duration
	var err error

	switch {
	case durationStr == "1h":
		duration = 1 * time.Hour
	case durationStr == "24h" || durationStr == "1d":
		duration = 24 * time.Hour
	case durationStr == "7d":
		duration = 7 * 24 * time.Hour
	case durationStr == "30d":
		duration = 30 * 24 * time.Hour
	case durationStr == "90d":
		duration = 90 * 24 * time.Hour
	default:
		// Intentar parsear como duración estándar
		duration, err = time.ParseDuration(durationStr)
		if err != nil {
			duration = 7 * 24 * time.Hour // Default: 1 semana
		}
	}

	return time.Now().Add(-duration)
}