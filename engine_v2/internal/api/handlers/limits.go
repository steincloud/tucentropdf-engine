package handlers

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/internal/service"
	"github.com/tucentropdf/engine-v2/internal/utils"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// LimitsHandler maneja endpoints relacionados con límites y estadísticas
type LimitsHandler struct {
	config           *config.Config
	logger           *logger.Logger
	usageService     *service.UsageService
	serviceProtector *service.ServiceProtector
	resourceMonitor  *utils.ResourceMonitor
}

// NewLimitsHandler crear nuevo handler de límites
func NewLimitsHandler(
	cfg *config.Config,
	log *logger.Logger,
	usageService *service.UsageService,
	serviceProtector *service.ServiceProtector,
	resourceMonitor *utils.ResourceMonitor,
) *LimitsHandler {
	return &LimitsHandler{
		config:           cfg,
		logger:           log,
		usageService:     usageService,
		serviceProtector: serviceProtector,
		resourceMonitor:  resourceMonitor,
	}
}

// GetPlanLimits obtiene los límites detallados de un plan específico
// @Summary Límites por plan
// @Description Obtiene información detallada de límites visibles para un plan
// @Tags public
// @Accept json
// @Produce json
// @Param plan path string true "Plan" Enums(free, premium, pro, corporate)
// @Success 200 {object} response.Response
// @Router /api/v2/limits/plan/{plan} [get]
func (lh *LimitsHandler) GetPlanLimits(c *fiber.Ctx) error {
	plan := config.Plan(strings.ToLower(c.Params("plan")))
	
	if !plan.IsValid() {
		return c.Status(400).JSON(fiber.Map{
			"success": false,
			"error": "INVALID_PLAN",
			"message": "Plan debe ser uno de: free, premium, pro, corporate",
		})
	}

	planConfig := config.GetDefaultPlanConfiguration()
	limits := planConfig.GetPlanLimits(plan)
	pricing := planConfig.GetPlanPricing(plan)

	// Preparar respuesta con límites visibles SOLAMENTE
	visibleLimits := map[string]interface{}{
		"plan": string(plan),
		"pricing": pricing,
		"file_limits": map[string]interface{}{
			"max_file_size_mb":     limits.MaxFileSizeMB,
			"max_pages_per_file":   limits.MaxPages,
			"max_files_per_day":    limits.MaxFilesPerDay,
			"max_files_per_month":  limits.MaxFilesPerMonth,
			"max_concurrent_files": limits.MaxConcurrentFiles,
		},
		"operation_limits": map[string]interface{}{
			"daily_operations":   limits.DailyOperations,
			"monthly_operations": limits.MonthlyOperations,
			"rate_limit_per_min": limits.RateLimit,
			"priority_level":     limits.Priority,
			"speed_level":        limits.SpeedLevel,
			"timeout_seconds":    int(limits.ProcessingTimeout.Seconds()),
		},
		"ocr_limits": map[string]interface{}{
			"ocr_pages_per_day":    limits.OCRPagesPerDay,
			"ocr_pages_per_month":  limits.OCRPagesPerMonth,
			"ai_ocr_pages_per_day": limits.AIOCRPagesPerDay,
			"ai_ocr_pages_per_month": limits.AIOCRPagesPerMonth,
			"ai_ocr_enabled":       limits.EnableAIOCR,
		},
		"office_limits": map[string]interface{}{
			"office_pages_per_day":   limits.OfficePagesPerDay,
			"office_pages_per_month": limits.OfficePagesPerMonth,
			"office_has_watermark":   limits.OfficeHasWatermark,
		},
		"transfer_limits": map[string]interface{}{
			"max_bytes_per_day_mb":   limits.MaxBytesPerDay / (1024 * 1024),
			"max_bytes_per_month_mb": limits.MaxBytesPerMonth / (1024 * 1024),
		},
		"features": map[string]interface{}{
			"ai_ocr_enabled":      limits.EnableAIOCR,
			"priority_processing": limits.EnablePriority,
			"analytics_enabled":   limits.EnableAnalytics,
			"team_access":         limits.EnableTeamAccess,
			"api_access":          limits.EnableAPI,
			"advanced_features":   limits.EnableAdvancedFeats,
			"has_watermark":       limits.HasWatermark,
			"has_ads":             limits.HasAds,
			"support_level":       limits.SupportLevel,
			"max_team_users":      limits.MaxTeamUsers,
		},
		"display_info": map[string]interface{}{
			"recommended_for": lh.getRecommendationText(plan),
			"upgrade_benefits": lh.getUpgradeBenefits(plan),
		},
	}

	return c.JSON(fiber.Map{"success": true, "message": "Límites del plan obtenidos exitosamente", "data": visibleLimits})
}

// GetUserUsage obtiene el uso actual del usuario autenticado
// @Summary Uso del usuario
// @Description Obtiene estadísticas de uso actual del usuario
// @Tags user
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} response.Response
// @Router /api/v2/limits/usage [get]
func (lh *LimitsHandler) GetUserUsage(c *fiber.Ctx) error {
	userID := lh.getUserID(c)
	if userID == "" {
		return c.Status(401).JSON(fiber.Map{
			"success": false,
			"error": "AUTHENTICATION_REQUIRED",
			"message": "Autenticación requerida para ver estadísticas",
		})
	}

	userPlan := lh.getUserPlan(c)
	
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	usage, err := lh.usageService.GetUsageSummary(ctx, userID, userPlan)
	if err != nil {
		lh.logger.Error("Failed to get usage summary", "user_id", userID, "error", err)
		return c.Status(500).JSON(fiber.Map{
			"success": false,
			"error": "USAGE_FETCH_ERROR",
			"message": "Error obteniendo estadísticas de uso",
		})
	}

	return c.JSON(fiber.Map{"success": true, "message": "Estadísticas de uso obtenidas exitosamente", "data": usage})
}

// GetSystemStatus obtiene el estado del sistema (admin endpoint)
// @Summary Estado del sistema
// @Description Obtiene métricas del sistema y estado de protecciones (admin)
// @Tags admin
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Success 200 {object} response.Response
// @Router /api/v2/admin/limits/system [get]
func (lh *LimitsHandler) GetSystemStatus(c *fiber.Ctx) error {
	// TODO: Verificar permisos de admin
	
	systemStatus := map[string]interface{}{
		"timestamp": time.Now(),
		"protector": lh.serviceProtector.GetSystemStatus(),
		"resources": lh.resourceMonitor.CheckContainerLimits(),
		"metrics":   lh.resourceMonitor.GetMetricsHistory(),
	}

	return c.JSON(fiber.Map{"success": true, "message": "Estado del sistema obtenido exitosamente", "data": systemStatus})
}

// GetPlanComparison obtiene comparación entre todos los planes
// @Summary Comparación de planes
// @Description Obtiene una comparación detallada entre todos los planes disponibles
// @Tags public
// @Accept json
// @Produce json
// @Success 200 {object} response.Response
// @Router /api/v2/limits/plans/compare [get]
func (lh *LimitsHandler) GetPlanComparison(c *fiber.Ctx) error {
	planConfig := config.GetDefaultPlanConfiguration()

	comparison := map[string]interface{}{
		"plans": map[string]interface{}{},
		"features_matrix": lh.buildFeaturesMatrix(),
		"recommendations": map[string]interface{}{
			"free":      "Ideal para uso ocasional y pruebas",
			"premium":   "Perfecto para profesionales independientes",
			"pro":       "Ideal para equipos pequeños y uso intensivo",
			"corporate": "Solución empresarial con SLA y soporte dedicado",
		},
	}

	// Agregar cada plan a la comparación
	for plan := range planConfig.Plans {
		limits := planConfig.GetPlanLimits(plan)
		pricing := planConfig.GetPlanPricing(plan)

		comparison["plans"].(map[string]interface{})[string(plan)] = map[string]interface{}{
			"name":     lh.getPlanDisplayName(plan),
			"pricing":  pricing,
			"summary":  lh.getPlanSummary(plan, limits),
			"features": lh.getPlanFeatureList(plan, limits),
		}
	}

	return c.JSON(fiber.Map{"success": true, "message": "Comparación de planes obtenida exitosamente", "data": comparison})
}

// Helper methods

func (lh *LimitsHandler) getUserID(c *fiber.Ctx) string {
	if userID, ok := c.Locals("userID").(string); ok {
		return userID
	}
	return ""
}

func (lh *LimitsHandler) getUserPlan(c *fiber.Ctx) config.Plan {
	if plan, ok := c.Locals("userPlan").(string); ok {
		return config.Plan(plan)
	}
	return config.PlanFree
}

func (lh *LimitsHandler) getPlanDisplayName(plan config.Plan) string {
	switch plan {
	case config.PlanFree:
		return "Gratuito"
	case config.PlanPremium:
		return "Premium"
	case config.PlanPro:
		return "Profesional"
	case config.PlanCorporate:
		return "Corporativo"
	default:
		return string(plan)
	}
}

func (lh *LimitsHandler) getRecommendationText(plan config.Plan) string {
	switch plan {
	case config.PlanFree:
		return "Ideal para pruebas y uso ocasional. Perfecto para conocer la plataforma."
	case config.PlanPremium:
		return "Perfecto para profesionales independientes con necesidades moderadas."
	case config.PlanPro:
		return "Ideal para equipos y uso profesional intensivo con IA avanzada."
	case config.PlanCorporate:
		return "Solución empresarial completa con soporte dedicado y SLA."
	default:
		return ""
	}
}

func (lh *LimitsHandler) getUpgradeBenefits(plan config.Plan) []string {
	switch plan {
	case config.PlanFree:
		return []string{
			"Sin marca de agua (Premium+)",
			"OCR con IA (Premium+)",
			"Sin publicidad (Premium+)",
			"Mayor límite de archivos (Premium+)",
			"Soporte prioritario (Pro+)",
		}
	case config.PlanPremium:
		return []string{
			"OCR con IA ilimitado (Pro+)",
			"Acceso de equipo (Pro+)",
			"API y webhooks (Pro+)",
			"Soporte prioritario (Pro+)",
			"Features avanzadas (Pro+)",
		}
	case config.PlanPro:
		return []string{
			"Límites \"ilimitados\" (Corporate)",
			"Equipos grandes (Corporate)",
			"SLA garantizado (Corporate)",
			"Soporte dedicado 24/7 (Corporate)",
			"Workers dedicados (Corporate)",
		}
	case config.PlanCorporate:
		return []string{
			"Ya tienes el plan más completo disponible",
		}
	default:
		return []string{}
	}
}

func (lh *LimitsHandler) getPlanSummary(plan config.Plan, limits config.PlanLimits) map[string]interface{} {
	return map[string]interface{}{
		"max_file_mb":       limits.MaxFileSizeMB,
		"files_per_day":     limits.MaxFilesPerDay,
		"concurrent_files":  limits.MaxConcurrentFiles,
		"ai_ocr_enabled":    limits.EnableAIOCR,
		"has_watermark":     limits.HasWatermark,
		"has_ads":           limits.HasAds,
		"support_level":     limits.SupportLevel,
		"team_users":        limits.MaxTeamUsers,
	}
}

func (lh *LimitsHandler) getPlanFeatureList(plan config.Plan, limits config.PlanLimits) []string {
	features := []string{
		"Procesamiento de PDFs básico",
	}

	if limits.EnableAIOCR {
		features = append(features, "OCR con Inteligencia Artificial")
	}

	if !limits.HasWatermark {
		features = append(features, "Sin marca de agua")
	}

	if !limits.HasAds {
		features = append(features, "Sin publicidad")
	}

	if limits.EnablePriority {
		features = append(features, "Procesamiento prioritario")
	}

	if limits.EnableAnalytics {
		features = append(features, "Analytics y estadísticas")
	}

	if limits.EnableTeamAccess {
		features = append(features, "Acceso de equipo")
	}

	if limits.EnableAPI {
		features = append(features, "API y webhooks")
	}

	switch limits.SupportLevel {
	case "email":
		features = append(features, "Soporte por email")
	case "priority":
		features = append(features, "Soporte prioritario (1 hora)")
	case "dedicated":
		features = append(features, "Soporte dedicado 24/7 + SLA")
	}

	return features
}

func (lh *LimitsHandler) buildFeaturesMatrix() map[string]interface{} {
	return map[string]interface{}{
		"file_processing": map[string]interface{}{
			"free":      "Básico",
			"premium":   "Completo",
			"pro":       "Avanzado",
			"corporate": "Empresarial",
		},
		"ocr_basic": map[string]interface{}{
			"free":      "2 páginas/día",
			"premium":   "50 páginas/día",
			"pro":       "200 páginas/día",
			"corporate": "1000 páginas/día",
		},
		"ocr_ai": map[string]interface{}{
			"free":      "❌ No disponible",
			"premium":   "5 páginas/día",
			"pro":       "50 páginas/día",
			"corporate": "500 páginas/día",
		},
		"office_conversion": map[string]interface{}{
			"free":      "3 páginas/día + marca de agua",
			"premium":   "50 páginas/día",
			"pro":       "200 páginas/día + turbo",
			"corporate": "1000 páginas/día + turbo",
		},
		"file_size": map[string]interface{}{
			"free":      "10MB",
			"premium":   "50MB",
			"pro":       "200MB",
			"corporate": "500MB",
		},
		"concurrent_files": map[string]interface{}{
			"free":      "1 archivo",
			"premium":   "3 archivos",
			"pro":       "10 archivos",
			"corporate": "50 archivos",
		},
		"support": map[string]interface{}{
			"free":      "Automático",
			"premium":   "Email",
			"pro":       "Prioritario (1h)",
			"corporate": "Dedicado 24/7 + SLA",
		},
		"team_access": map[string]interface{}{
			"free":      "❌ No",
			"premium":   "❌ No",
			"pro":       "✅ 5 usuarios",
			"corporate": "✅ 100 usuarios",
		},
		"api_webhooks": map[string]interface{}{
			"free":      "❌ No",
			"premium":   "❌ No",
			"pro":       "✅ Completo",
			"corporate": "✅ Completo + dedicado",
		},
	}
}