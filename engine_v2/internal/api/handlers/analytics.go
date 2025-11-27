package handlers

import (
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/tucentropdf/engine-v2/internal/analytics"
	"github.com/tucentropdf/engine-v2/internal/analytics/models"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// AnalyticsHandler handler para endpoints de analytics
type AnalyticsHandler struct {
	service  *analytics.Service
	config   *config.Config
	logger   *logger.Logger
}

// NewAnalyticsHandler crea nuevo handler de analytics
func NewAnalyticsHandler(service *analytics.Service, cfg *config.Config, log *logger.Logger) *AnalyticsHandler {
	return &AnalyticsHandler{
		service:  service,
		config:   cfg,
		logger:   log,
	}
}

// GetOverview obtiene vista general de analytics
// @Summary Vista general de analytics
// @Description Obtiene métricas generales del sistema (admin/corporate)
// @Tags analytics
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} response.Response
// @Router /api/v2/analytics/overview [get]
func (h *AnalyticsHandler) GetOverview(c *fiber.Ctx) error {
	h.logger.Info("📊 Analytics overview requested", "ip", c.IP())

	// Validar permisos (solo admin o corporate)
	if !h.hasAnalyticsPermissions(c) {
		return c.Status(403).JSON(fiber.Map{
			"success": false,
			"error":   "UNAUTHORIZED",
			"message": "Analytics access requires admin or corporate plan",
		})
	}

	overview, err := h.service.GetGlobalUsage()
	if err != nil {
		h.logger.Error("Error getting analytics overview", "error", err)
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "ANALYTICS_ERROR",
			"message": "Failed to get analytics overview",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Analytics overview retrieved successfully",
		"data":    overview,
	})
}

// GetTools obtiene estadísticas de herramientas
// @Summary Estadísticas de herramientas
// @Description Obtiene estadísticas detalladas por herramienta
// @Tags analytics
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param period query string false "Período: daily, weekly, monthly, yearly" default(weekly)
// @Param limit query int false "Límite de resultados" default(10)
// @Success 200 {object} response.Response
// @Router /api/v2/analytics/tools [get]
func (h *AnalyticsHandler) GetTools(c *fiber.Ctx) error {
	h.logger.Info("🔧 Analytics tools requested", "ip", c.IP())

	if !h.hasAnalyticsPermissions(c) {
		return c.Status(403).JSON(fiber.Map{
			"success": false,
			"error":   "UNAUTHORIZED",
			"message": "Analytics access requires admin or corporate plan",
		})
	}

	period := c.Query("period", "weekly")
	limit, _ := strconv.Atoi(c.Query("limit", "10"))

	mostUsed, err := h.service.GetMostUsedTools(period, limit)
	if err != nil {
		h.logger.Error("Error getting most used tools", "error", err)
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "ANALYTICS_ERROR",
			"message": "Failed to get tool statistics",
			"details": err.Error(),
		})
	}

	leastUsed, err := h.service.GetLeastUsedTools(period, limit)
	if err != nil {
		h.logger.Error("Error getting least used tools", "error", err)
		// No fallar completamente, continuar sin least used
		leastUsed = []models.ToolStats{}
	}

	result := map[string]interface{}{
		"period":     period,
		"most_used":  mostUsed,
		"least_used": leastUsed,
		"generated_at": time.Now(),
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Tool statistics retrieved successfully",
		"data":    result,
	})
}

// GetMostUsedTools obtiene herramientas más utilizadas
// @Summary Herramientas más utilizadas
// @Description Obtiene ranking de herramientas más utilizadas
// @Tags analytics
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param period query string false "Período" default(weekly)
// @Param limit query int false "Límite" default(10)
// @Success 200 {object} response.Response
// @Router /api/v2/analytics/tools/most-used [get]
func (h *AnalyticsHandler) GetMostUsedTools(c *fiber.Ctx) error {
	if !h.hasAnalyticsPermissions(c) {
		return c.Status(403).JSON(fiber.Map{
			"success": false,
			"error":   "UNAUTHORIZED",
			"message": "Analytics access requires admin or corporate plan",
		})
	}

	period := c.Query("period", "weekly")
	limit, _ := strconv.Atoi(c.Query("limit", "10"))

	stats, err := h.service.GetMostUsedTools(period, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "ANALYTICS_ERROR",
			"message": "Failed to get most used tools",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Most used tools retrieved successfully",
		"data": map[string]interface{}{
			"period": period,
			"tools":  stats,
			"generated_at": time.Now(),
		},
	})
}

// GetLeastUsedTools obtiene herramientas menos utilizadas
// @Summary Herramientas menos utilizadas
// @Description Obtiene herramientas con menor uso para identificar oportunidades de mejora
// @Tags analytics
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param period query string false "Período" default(weekly)
// @Param limit query int false "Límite" default(10)
// @Success 200 {object} response.Response
// @Router /api/v2/analytics/tools/least-used [get]
func (h *AnalyticsHandler) GetLeastUsedTools(c *fiber.Ctx) error {
	if !h.hasAnalyticsPermissions(c) {
		return c.Status(403).JSON(fiber.Map{
			"success": false,
			"error":   "UNAUTHORIZED",
			"message": "Analytics access requires admin or corporate plan",
		})
	}

	period := c.Query("period", "weekly")
	limit, _ := strconv.Atoi(c.Query("limit", "10"))

	stats, err := h.service.GetLeastUsedTools(period, limit)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "ANALYTICS_ERROR",
			"message": "Failed to get least used tools",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Least used tools retrieved successfully",
		"data": map[string]interface{}{
			"period": period,
			"tools":  stats,
			"insights": []string{
				"Consider improving UX for low-usage tools",
				"Evaluate if these tools meet user needs",
				"Potential candidates for optimization or removal",
			},
			"generated_at": time.Now(),
		},
	})
}

// GetUserAnalytics obtiene analytics de un usuario específico
// @Summary Analytics de usuario
// @Description Obtiene estadísticas detalladas de un usuario (admin/corporate/own data)
// @Tags analytics
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param id path string true "User ID"
// @Success 200 {object} response.Response
// @Router /api/v2/analytics/users/{id} [get]
func (h *AnalyticsHandler) GetUserAnalytics(c *fiber.Ctx) error {
	userID := c.Params("id")
	currentUserID := h.extractUserID(c)

	// Validar permisos: admin/corporate o datos propios
	if !h.hasAnalyticsPermissions(c) && userID != currentUserID {
		return c.Status(403).JSON(fiber.Map{
			"success": false,
			"error":   "UNAUTHORIZED",
			"message": "Cannot access other user's analytics",
		})
	}

	userStats, err := h.service.GetUserToolBreakdown(userID)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "ANALYTICS_ERROR",
			"message": "Failed to get user analytics",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "User analytics retrieved successfully",
		"data":    userStats,
	})
}

// GetPlans obtiene estadísticas por plan
// @Summary Estadísticas por plan
// @Description Obtiene breakdown de uso por plan de suscripción
// @Tags analytics
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} response.Response
// @Router /api/v2/analytics/plans [get]
func (h *AnalyticsHandler) GetPlans(c *fiber.Ctx) error {
	if !h.hasAnalyticsPermissions(c) {
		return c.Status(403).JSON(fiber.Map{
			"success": false,
			"error":   "UNAUTHORIZED",
			"message": "Analytics access requires admin or corporate plan",
		})
	}

	plans := []string{"free", "premium", "pro", "corporate"}
	result := make(map[string]interface{})

	for _, plan := range plans {
		stats, err := h.service.GetPlanUsageBreakdown(plan)
		if err != nil {
			h.logger.Error("Error getting plan stats", "plan", plan, "error", err)
			continue
		}
		result[plan] = stats
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Plan analytics retrieved successfully",
		"data":    result,
	})
}

// GetFailures obtiene estadísticas de fallos
// @Summary Estadísticas de fallos
// @Description Obtiene análisis de fallos y errores del sistema
// @Tags analytics
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param period query string false "Período" default(weekly)
// @Success 200 {object} response.Response
// @Router /api/v2/analytics/failures [get]
func (h *AnalyticsHandler) GetFailures(c *fiber.Ctx) error {
	if !h.hasAnalyticsPermissions(c) {
		return c.Status(403).JSON(fiber.Map{
			"success": false,
			"error":   "UNAUTHORIZED",
			"message": "Analytics access requires admin or corporate plan",
		})
	}

	period := c.Query("period", "weekly")

	failReasons, err := h.service.GetTopFailReasons(period)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error":   "ANALYTICS_ERROR",
			"message": "Failed to get failure statistics",
			"details": err.Error(),
		})
	}

	result := map[string]interface{}{
		"period":       period,
		"fail_reasons": failReasons,
		"recommendations": []string{
			"Monitor top failure reasons closely",
			"Implement automated retry logic",
			"Improve error handling and user feedback",
		},
		"generated_at": time.Now(),
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Failure statistics retrieved successfully",
		"data":    result,
	})
}

// GetWorkers obtiene estadísticas de workers
// @Summary Estadísticas de workers
// @Description Obtiene rendimiento y estadísticas de workers
// @Tags analytics
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} response.Response
// @Router /api/v2/analytics/workers [get]
func (h *AnalyticsHandler) GetWorkers(c *fiber.Ctx) error {
	if !h.hasAnalyticsPermissions(c) {
		return c.Status(403).JSON(fiber.Map{"success": false, "error": "UNAUTHORIZED", "message": "Analytics access requires admin or corporate plan"})
	}

	workers := []string{"api", "ocr-worker", "office-worker"}
	result := make(map[string]interface{})

	for _, worker := range workers {
		stats, err := h.service.GetWorkerPerformance(worker)
		if err != nil {
			h.logger.Error("Error getting worker stats", "worker", worker, "error", err)
			continue
		}
		result[worker] = stats
	}

	return c.JSON(fiber.Map{"success": true, "message": "Worker statistics retrieved successfully", "data": result})
}

// GetPerformance obtiene métricas de rendimiento
// @Summary Métricas de rendimiento
// @Description Obtiene métricas de rendimiento del sistema
// @Tags analytics
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} response.Response
// @Router /api/v2/analytics/performance [get]
func (h *AnalyticsHandler) GetPerformance(c *fiber.Ctx) error {
	if !h.hasAnalyticsPermissions(c) {
		return c.Status(403).JSON(fiber.Map{"success": false, "error": "UNAUTHORIZED", "message": "Analytics access requires admin or corporate plan"})
	}

	processingTimes, err := h.service.GetProcessingTimesByTool()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": "ANALYTICS_ERROR", "message": "Failed to get processing times", "details": err.Error()})
	}

	fileSizeDistribution, err := h.service.GetFileSizeDistribution()
	if err != nil {
		// No fallar completamente
		fileSizeDistribution = make(map[string]int64)
	}

	result := map[string]interface{}{
		"processing_times":     processingTimes,
		"file_size_distribution": fileSizeDistribution,
		"generated_at":        time.Now(),
	}

	return c.JSON(fiber.Map{"success": true, "message": "Performance metrics retrieved successfully", "data": result})
}

// GetUsageTrends obtiene tendencias de uso
// @Summary Tendencias de uso
// @Description Obtiene tendencias y patrones de uso del sistema
// @Tags analytics
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param tool query string false "Herramienta específica para tendencias"
// @Success 200 {object} response.Response
// @Router /api/v2/analytics/usage/trends [get]
func (h *AnalyticsHandler) GetUsageTrends(c *fiber.Ctx) error {
	if !h.hasAnalyticsPermissions(c) {
		return c.Status(403).JSON(fiber.Map{"success": false, "error": "UNAUTHORIZED", "message": "Analytics access requires admin or corporate plan"})
	}

	tool := c.Query("tool")
	var result map[string]interface{}

	if tool != "" {
		// Tendencias para herramienta específica
		trends, err := h.service.GetToolGrowthTrend(tool)
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "error": "ANALYTICS_ERROR", "message": "Failed to get tool trends", "details": err.Error()})
		}

		result = map[string]interface{}{
			"tool":   tool,
			"trends": trends,
		}
	} else {
		// Picos de uso general
		peaks, err := h.service.GetUsagePeaks()
		if err != nil {
			return c.Status(500).JSON(fiber.Map{"success": false, "error": "ANALYTICS_ERROR", "message": "Failed to get usage peaks", "details": err.Error()})
		}

		result = map[string]interface{}{
			"usage_peaks": peaks,
			"insights": []string{
				"Monitor peak usage times for capacity planning",
				"Consider auto-scaling during high demand periods",
			},
		}
	}

	result["generated_at"] = time.Now()
	return c.JSON(fiber.Map{"success": true, "message": "Usage trends retrieved successfully", "data": result})
}

// GetUpgradeOpportunities obtiene oportunidades de upgrade
// @Summary Oportunidades de upgrade
// @Description Detecta usuarios candidatos para upgrade de plan
// @Tags analytics
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} response.Response
// @Router /api/v2/analytics/upgrade-opportunities [get]
func (h *AnalyticsHandler) GetUpgradeOpportunities(c *fiber.Ctx) error {
	if !h.hasAnalyticsPermissions(c) {
		return c.Status(403).JSON(fiber.Map{"success": false, "error": "UNAUTHORIZED", "message": "Analytics access requires admin or corporate plan"})
	}

	opportunities, err := h.service.GetUpgradeOpportunities()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": "ANALYTICS_ERROR", "message": "Failed to get upgrade opportunities", "details": err.Error()})
	}

	heavyUsers, err := h.service.GetHeavyUsers()
	if err != nil {
		// No fallar completamente
		heavyUsers = []models.UserStats{}
	}

	// Calcular revenue potencial total
	totalPotentialRevenue := 0.0
	for _, opp := range opportunities {
		totalPotentialRevenue += opp.PotentialRevenue
	}

	result := map[string]interface{}{
		"upgrade_opportunities": opportunities,
		"heavy_users":          heavyUsers,
		"total_potential_revenue": totalPotentialRevenue,
		"recommendations": []string{
			"Contact high-confidence upgrade candidates",
			"Offer targeted promotions to heavy users",
			"Analyze usage patterns for product improvements",
		},
		"generated_at": time.Now(),
	}

	return c.JSON(fiber.Map{"success": true, "message": "Upgrade opportunities retrieved successfully", "data": result})
}

// GetBusinessInsights obtiene insights automáticos de negocio
// @Summary Business insights
// @Description Obtiene insights automáticos para optimización de negocio
// @Tags analytics
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} response.Response
// @Router /api/v2/analytics/business-insights [get]
func (h *AnalyticsHandler) GetBusinessInsights(c *fiber.Ctx) error {
	if !h.hasAnalyticsPermissions(c) {
		return c.Status(403).JSON(fiber.Map{"success": false, "error": "UNAUTHORIZED", "message": "Analytics access requires admin or corporate plan"})
	}

	insights, err := h.service.GenerateBusinessInsights()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"success": false, "error": "ANALYTICS_ERROR", "message": "Failed to generate business insights", "details": err.Error()})
	}

	// Agrupar insights por tipo y severidad
	groupedInsights := map[string][]interface{}{
		"critical": {},
		"high":     {},
		"medium":   {},
		"low":      {},
	}

	for _, insight := range insights {
		groupedInsights[insight.Severity] = append(groupedInsights[insight.Severity], insight)
	}

	result := map[string]interface{}{
		"insights":        insights,
		"grouped_insights": groupedInsights,
		"summary": map[string]int{
			"total":    len(insights),
			"critical": len(groupedInsights["critical"]),
			"high":     len(groupedInsights["high"]),
			"medium":   len(groupedInsights["medium"]),
			"low":      len(groupedInsights["low"]),
		},
		"generated_at": time.Now(),
	}

	return c.JSON(fiber.Map{"success": true, "message": "Business insights generated successfully", "data": result})
}

// hasAnalyticsPermissions verifica si el usuario tiene permisos para analytics
func (h *AnalyticsHandler) hasAnalyticsPermissions(c *fiber.Ctx) bool {
	// Verificar si es admin
	if isAdmin := c.Get("X-Is-Admin"); isAdmin == "true" {
		return true
	}

	// Verificar si es plan corporate
	userPlan := h.extractUserPlan(c)
	if userPlan == "corporate" {
		return true
	}

	return false
}

// extractUserID extrae el ID del usuario
func (h *AnalyticsHandler) extractUserID(c *fiber.Ctx) string {
	if userID := c.Get("X-User-ID"); userID != "" {
		return userID
	}
	if userID := c.Locals("userID"); userID != nil {
		if uid, ok := userID.(string); ok {
			return uid
		}
	}
	return "anonymous_" + c.IP()
}

// extractUserPlan extrae el plan del usuario
func (h *AnalyticsHandler) extractUserPlan(c *fiber.Ctx) string {
	if plan := c.Get("X-User-Plan"); plan != "" {
		return plan
	}
	if plan := c.Locals("userPlan"); plan != nil {
		if p, ok := plan.(string); ok {
			return p
		}
	}
	return "free"
}