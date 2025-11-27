package routes

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/adaptor/v2"
	"github.com/go-redis/redis/v8"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"gorm.io/gorm"

	"github.com/tucentropdf/engine-v2/internal/analytics"
	"github.com/tucentropdf/engine-v2/internal/api/handlers"
	"github.com/tucentropdf/engine-v2/internal/api/middleware"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/internal/maintenance"
	"github.com/tucentropdf/engine-v2/internal/monitor"
	"github.com/tucentropdf/engine-v2/internal/service"
	"github.com/tucentropdf/engine-v2/internal/storage"
	"github.com/tucentropdf/engine-v2/internal/utils"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// Setup configura todas las rutas de la API
func Setup(app *fiber.App, cfg *config.Config, log *logger.Logger, redisClient *redis.Client, db *gorm.DB) {
	// Crear servicio de storage
	storageService := storage.NewService(cfg, log)
	
	// Crear instancia de handlers
	h := handlers.New(cfg, log, storageService)

	// Crear servicios de analytics
	analyticsService := analytics.NewService(db, redisClient, cfg, log)
	analyticsHandler := handlers.NewAnalyticsHandler(analyticsService, cfg, log)

	// Crear servicio de limits
	usageTracker := storage.NewUsageTracker(redisClient, cfg, log)
	usageService := service.NewUsageService(cfg, log, redisClient, usageTracker)
	serviceProtector := service.NewServiceProtector(cfg, log, redisClient)
	resourceMonitor := utils.NewResourceMonitor(log, redisClient)
	limitsHandler := handlers.NewLimitsHandler(cfg, log, usageService, serviceProtector, resourceMonitor)

	// Crear servicio de mantenimiento
	maintenanceService := maintenance.NewService(db, redisClient, cfg, log)
	maintenanceHandler := handlers.NewMaintenanceHandlers(maintenanceService, log)

	// Crear servicio de monitoreo interno
	monitorService := monitor.NewService(db, redisClient, cfg, log)
	healthHandler := handlers.NewHealthHandlers(monitorService)

	// Iniciar servicios de mantenimiento y monitoreo automático
	maintenanceService.Start()
	monitorService.Start()

	// Crear middleware de analytics
	analyticsMiddleware := analytics.NewMiddleware(analyticsService, log)

	// Middleware globales
	auth := middleware.NewAuthMiddleware(cfg, log, db)
	planLimits := middleware.NewPlanLimitsMiddleware(cfg, log)
	rateLimit := middleware.NewRateLimitMiddleware(cfg, log, redisClient)

	// API V1 Group (retrocompatibilidad)
	v1 := app.Group("/api/v1")
	setupV1Routes(v1, h, auth, planLimits, rateLimit, analyticsMiddleware)

	// API V2 Group (nueva implementación con IA y analytics)
	v2 := app.Group("/api/v2")
	setupV2Routes(v2, h, limitsHandler, analyticsHandler, maintenanceHandler, healthHandler, auth, planLimits, rateLimit, analyticsMiddleware)
}

// setupV1Routes configura rutas de API V1 (retrocompatibilidad)
func setupV1Routes(api fiber.Router, h *handlers.Handlers, auth *middleware.AuthMiddleware, planLimits *middleware.PlanLimitsMiddleware, rateLimit *middleware.RateLimitMiddleware, analytics *analytics.Middleware) {
	// Rutas públicas
	setupPublicRoutes(api, h)

	// Rutas protegidas con middleware básico + analytics
	protected := api.Group("", 
		auth.Authenticate(),
		rateLimit.RateLimit(),
		analytics.Capture(), // Capturar analytics en V1
	)
	setupProtectedRoutes(protected, h)
}

// setupV2Routes configura rutas de API V2 (con IA y límites avanzados)
func setupV2Routes(api fiber.Router, h *handlers.Handlers, limitsHandler *handlers.LimitsHandler, analyticsHandler *handlers.AnalyticsHandler, maintenanceHandler *handlers.MaintenanceHandlers, healthHandler *handlers.HealthHandlers, auth *middleware.AuthMiddleware, planLimits *middleware.PlanLimitsMiddleware, rateLimit *middleware.RateLimitMiddleware, analyticsMiddleware *analytics.Middleware) {
	// Rutas públicas V2
	api.Get("/health", healthHandler.GetHealthCheck)          // Health check completo para Nginx
	api.Get("/health/basic", healthHandler.GetBasicHealth)    // Health check básico (más rápido)
	api.Get("/health/workers", healthHandler.GetWorkerHealth) // Estado específico de workers
	api.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler())) // Métricas Prometheus
	api.Get("/info", h.GetInfo)
	api.Get("/plans", h.GetPlans)
	
	// Nuevos endpoints de límites (públicos)
	api.Get("/limits/plan/:plan", limitsHandler.GetPlanLimits)
	api.Get("/limits/plans/compare", limitsHandler.GetPlanComparison)

	// Middleware completo para rutas protegidas V2
	protected := api.Group("",
		auth.Authenticate(),
		rateLimit.RateLimit(),
		planLimits.ValidatePlanLimits(),
		analyticsMiddleware.Capture(), // Capturar analytics automáticamente
	)
	
	// Endpoints de usuario autenticado
	protected.Get("/limits/usage", limitsHandler.GetUserUsage)

	// PDF Operations V2
	pdf := protected.Group("/pdf")
	pdf.Post("/merge", h.MergePDF)
	pdf.Post("/split", h.SplitPDF)
	pdf.Post("/optimize", h.OptimizePDF)
	pdf.Post("/watermark", h.WatermarkPDF)
	pdf.Post("/info", h.PDFInfo)

	// OCR Operations V2 (con AI)
	ocr := protected.Group("/ocr")
	ocr.Post("/classic", h.ClassicOCR)  // OCR tradicional
	ocr.Post("/ai", h.AIOCR)           // OCR con IA (Premium/Pro)

	// Office Operations V2
	office := protected.Group("/office")
	office.Post("/convert", h.OfficeConvert) // Solo Premium/Pro

	// Storage Operations V2
	storage := protected.Group("/storage")
	storage.Get("/files", h.GetFiles)
	storage.Get("/download/:filename", h.DownloadFile)
	storage.Delete("/files/:filename", h.DeleteFile)
	
	// Analytics Endpoints V2 (solo admin/corporate)
	analytics := api.Group("/analytics")
	analytics.Get("/overview", analyticsHandler.GetOverview)
	analytics.Get("/tools", analyticsHandler.GetTools)
	analytics.Get("/tools/most-used", analyticsHandler.GetMostUsedTools)
	analytics.Get("/tools/least-used", analyticsHandler.GetLeastUsedTools)
	analytics.Get("/users/:id", analyticsHandler.GetUserAnalytics)
	analytics.Get("/plans", analyticsHandler.GetPlans)
	analytics.Get("/failures", analyticsHandler.GetFailures)
	analytics.Get("/workers", analyticsHandler.GetWorkers)
	analytics.Get("/performance", analyticsHandler.GetPerformance)
	analytics.Get("/usage/trends", analyticsHandler.GetUsageTrends)
	analytics.Get("/upgrade-opportunities", analyticsHandler.GetUpgradeOpportunities)
	analytics.Get("/business-insights", analyticsHandler.GetBusinessInsights)
	
	// Maintenance Endpoints (solo admin/corporate)
	maintenance := api.Group("/maintenance")
	maintenance.Get("/status", maintenanceHandler.GetSystemStatus)
	maintenance.Get("/config", maintenanceHandler.GetMaintenanceConfig)
	maintenance.Post("/trigger", maintenanceHandler.TriggerMaintenance)
	
	// Monitoring Endpoints (solo admin/corporate)
	monitoring := api.Group("/monitoring")
	monitoring.Get("/status", healthHandler.GetMonitoringStatus)
	monitoring.Get("/incidents", healthHandler.GetSystemIncidents)
	monitoring.Get("/protection", func(c *fiber.Ctx) error {
		// Endpoint para obtener estado del modo protector
		return c.JSON(fiber.Map{
			"message": "Protection status endpoint - implement in healthHandler",
		})
	})
	
	// Admin endpoints (requieren permisos especiales)
	admin := api.Group("/admin")
	admin.Get("/limits/system", limitsHandler.GetSystemStatus) // Estado del sistema
}

// setupPublicRoutes configura rutas públicas
func setupPublicRoutes(api fiber.Router, h *handlers.Handlers) {
	// Info general
	api.Get("/info", h.GetInfo)
	api.Get("/status", h.GetStatus)
	
	// Captcha validation (público pero puede requerir rate limiting)
	api.Post("/captcha/validate", h.ValidateCaptcha) 
	api.Get("/health", h.GetHealth)
	api.Get("/limits/:plan", h.GetPlanLimits)
}

// setupProtectedRoutes configura rutas V1 que requieren autenticación
func setupProtectedRoutes(api fiber.Router, h *handlers.Handlers) {
	// PDF Operations V1
	pdf := api.Group("/pdf")
	pdf.Post("/merge", h.MergePDF)
	pdf.Post("/split", h.SplitPDF)
	pdf.Post("/compress", h.CompressPDF)
	pdf.Post("/rotate", h.RotatePDF)
	pdf.Post("/watermark", h.WatermarkPDF)
	pdf.Post("/info", h.PDFInfo)

	// OCR Operations V1
	ocr := api.Group("/ocr")
	ocr.Post("/classic", h.ClassicOCR)

	// Office Operations V1 (limitado)
	office := api.Group("/office")
	office.Post("/convert", h.OfficeConvert)

	// Utility Operations
	utils := api.Group("/utils")
	utils.Post("/validate", h.ValidateFile)
	utils.Get("/formats", h.GetSupportedFormats)
}