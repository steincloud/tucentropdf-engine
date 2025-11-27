package service

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/internal/storage"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// UsageService servicio de gestión de uso con límites visibles según plan
type UsageService struct {
	config       *config.Config
	logger       *logger.Logger
	redis        *redis.Client
	usageTracker *storage.UsageTracker
	planConfig   *config.PlanConfiguration
}

// NewUsageService crear nuevo servicio de uso
func NewUsageService(
	cfg *config.Config,
	log *logger.Logger,
	redisClient *redis.Client,
	usageTracker *storage.UsageTracker,
) *UsageService {
	return &UsageService{
		config:       cfg,
		logger:       log,
		redis:        redisClient,
		usageTracker: usageTracker,
		planConfig:   config.GetDefaultPlanConfiguration(),
	}
}

// UsageLimitCheck resultado de validación de límites de uso
type UsageLimitCheck struct {
	Allowed      bool                   `json:"allowed"`
	LimitType    string                 `json:"limit_type"`
	CurrentUsage map[string]interface{} `json:"current_usage"`
	Limits       map[string]interface{} `json:"limits"`
	ResetTime    *time.Time             `json:"reset_time,omitempty"`
	Message      string                 `json:"message,omitempty"`
}

// ValidateUsageForOperation valida si el usuario puede realizar una operación específica
func (us *UsageService) ValidateUsageForOperation(
	ctx context.Context,
	userID string,
	plan config.Plan,
	operationType storage.OperationType,
	fileSizeMB int,
	pages int,
) (*UsageLimitCheck, error) {
	
	if userID == "" {
		return &UsageLimitCheck{Allowed: true}, nil // Usuario anónimo
	}

	planLimits := us.planConfig.GetPlanLimits(plan)

	// Obtener uso actual del usuario
	usage, err := us.usageTracker.GetUserUsage(ctx, userID)
	if err != nil {
		us.logger.Warn("Failed to get usage for validation", "user_id", userID, "error", err)
		// En caso de error de Redis, permitir operación para no bloquear
		return &UsageLimitCheck{Allowed: true}, nil
	}

	// Validar según tipo de operación
	switch operationType {
	case storage.OpTypeOCR:
		return us.validateOCRUsage(usage, planLimits, pages)
	
	case storage.OpTypeAIOCR:
		return us.validateAIOCRUsage(usage, planLimits, pages)
	
	case storage.OpTypeOffice:
		return us.validateOfficeUsage(usage, planLimits, pages)
	
	case storage.OpTypeUpload:
		return us.validateUploadUsage(usage, planLimits, fileSizeMB)
	
	default:
		return us.validateGeneralUsage(usage, planLimits)
	}
}

// validateOCRUsage valida límites de OCR básico
func (us *UsageService) validateOCRUsage(
	usage *config.UserUsageStats,
	limits config.PlanLimits,
	pages int,
) (*UsageLimitCheck, error) {
	
	// Verificar límite diario de páginas OCR
	if usage.DailyStats.AIOCRPages+pages > limits.OCRPagesPerDay {
		resetTime := time.Now().Add(24 * time.Hour)
		return &UsageLimitCheck{
			Allowed:   false,
			LimitType: "daily_ocr_pages",
			CurrentUsage: map[string]interface{}{
				"daily_ocr_pages": usage.DailyStats.AIOCRPages,
				"requested_pages": pages,
				"total_would_be":  usage.DailyStats.AIOCRPages + pages,
			},
			Limits: map[string]interface{}{
				"max_daily_ocr_pages": limits.OCRPagesPerDay,
			},
			ResetTime: &resetTime,
			Message:   fmt.Sprintf("Límite diario de OCR alcanzado (%d de %d páginas). Se resetea mañana.", usage.DailyStats.AIOCRPages, limits.OCRPagesPerDay),
		}, nil
	}

	// Verificar límite mensual de páginas OCR
	if usage.MonthlyStats.AIOCRPages+pages > limits.OCRPagesPerMonth {
		resetTime := time.Now().AddDate(0, 1, 0) // Próximo mes
		return &UsageLimitCheck{
			Allowed:   false,
			LimitType: "monthly_ocr_pages",
			CurrentUsage: map[string]interface{}{
				"monthly_ocr_pages": usage.MonthlyStats.AIOCRPages,
				"requested_pages":   pages,
				"total_would_be":    usage.MonthlyStats.AIOCRPages + pages,
			},
			Limits: map[string]interface{}{
				"max_monthly_ocr_pages": limits.OCRPagesPerMonth,
			},
			ResetTime: &resetTime,
			Message:   fmt.Sprintf("Límite mensual de OCR alcanzado (%d de %d páginas). Se resetea el próximo mes.", usage.MonthlyStats.AIOCRPages, limits.OCRPagesPerMonth),
		}, nil
	}

	return &UsageLimitCheck{
		Allowed: true,
		CurrentUsage: map[string]interface{}{
			"daily_ocr_pages":    usage.DailyStats.AIOCRPages,
			"monthly_ocr_pages":  usage.MonthlyStats.AIOCRPages,
			"remaining_daily":    limits.OCRPagesPerDay - usage.DailyStats.AIOCRPages,
			"remaining_monthly":  limits.OCRPagesPerMonth - usage.MonthlyStats.AIOCRPages,
		},
	}, nil
}

// validateAIOCRUsage valida límites de OCR con IA
func (us *UsageService) validateAIOCRUsage(
	usage *config.UserUsageStats,
	limits config.PlanLimits,
	pages int,
) (*UsageLimitCheck, error) {
	
	// Verificar si el plan tiene IA OCR habilitado
	if !limits.EnableAIOCR {
		return &UsageLimitCheck{
			Allowed:   false,
			LimitType: "ai_ocr_not_enabled",
			Message:   "OCR con IA no está disponible en tu plan. Actualiza a Premium o Pro.",
		}, nil
	}

	// Verificar límite diario de páginas AI OCR
	if usage.DailyStats.AIOCRPages+pages > limits.AIOCRPagesPerDay {
		resetTime := time.Now().Add(24 * time.Hour)
		return &UsageLimitCheck{
			Allowed:   false,
			LimitType: "daily_ai_ocr_pages",
			CurrentUsage: map[string]interface{}{
				"daily_ai_ocr_pages": usage.DailyStats.AIOCRPages,
				"requested_pages":    pages,
				"total_would_be":     usage.DailyStats.AIOCRPages + pages,
			},
			Limits: map[string]interface{}{
				"max_daily_ai_ocr_pages": limits.AIOCRPagesPerDay,
			},
			ResetTime: &resetTime,
			Message:   fmt.Sprintf("Límite diario de OCR con IA alcanzado (%d de %d páginas). Se resetea mañana.", usage.DailyStats.AIOCRPages, limits.AIOCRPagesPerDay),
		}, nil
	}

	// Verificar límite mensual de páginas AI OCR
	if usage.MonthlyStats.AIOCRPages+pages > limits.AIOCRPagesPerMonth {
		resetTime := time.Now().AddDate(0, 1, 0)
		return &UsageLimitCheck{
			Allowed:   false,
			LimitType: "monthly_ai_ocr_pages",
			CurrentUsage: map[string]interface{}{
				"monthly_ai_ocr_pages": usage.MonthlyStats.AIOCRPages,
				"requested_pages":      pages,
				"total_would_be":       usage.MonthlyStats.AIOCRPages + pages,
			},
			Limits: map[string]interface{}{
				"max_monthly_ai_ocr_pages": limits.AIOCRPagesPerMonth,
			},
			ResetTime: &resetTime,
			Message:   fmt.Sprintf("Límite mensual de OCR con IA alcanzado (%d de %d páginas). Se resetea el próximo mes.", usage.MonthlyStats.AIOCRPages, limits.AIOCRPagesPerMonth),
		}, nil
	}

	return &UsageLimitCheck{
		Allowed: true,
		CurrentUsage: map[string]interface{}{
			"daily_ai_ocr_pages":    usage.DailyStats.AIOCRPages,
			"monthly_ai_ocr_pages":  usage.MonthlyStats.AIOCRPages,
			"remaining_daily":       limits.AIOCRPagesPerDay - usage.DailyStats.AIOCRPages,
			"remaining_monthly":     limits.AIOCRPagesPerMonth - usage.MonthlyStats.AIOCRPages,
		},
	}, nil
}

// validateOfficeUsage valida límites de conversión de Office
func (us *UsageService) validateOfficeUsage(
	usage *config.UserUsageStats,
	limits config.PlanLimits,
	pages int,
) (*UsageLimitCheck, error) {
	
	// Verificar límite diario de páginas Office
	if usage.DailyStats.OfficePages+pages > limits.OfficePagesPerDay {
		resetTime := time.Now().Add(24 * time.Hour)
		return &UsageLimitCheck{
			Allowed:   false,
			LimitType: "daily_office_pages",
			CurrentUsage: map[string]interface{}{
				"daily_office_pages": usage.DailyStats.OfficePages,
				"requested_pages":    pages,
				"total_would_be":     usage.DailyStats.OfficePages + pages,
				"will_have_watermark": limits.OfficeHasWatermark,
			},
			Limits: map[string]interface{}{
				"max_daily_office_pages": limits.OfficePagesPerDay,
				"has_watermark":          limits.OfficeHasWatermark,
			},
			ResetTime: &resetTime,
			Message:   fmt.Sprintf("Límite diario de conversión Office alcanzado (%d de %d páginas). Se resetea mañana.", usage.DailyStats.OfficePages, limits.OfficePagesPerDay),
		}, nil
	}

	// Verificar límite mensual de páginas Office
	if usage.MonthlyStats.OfficePages+pages > limits.OfficePagesPerMonth {
		resetTime := time.Now().AddDate(0, 1, 0)
		return &UsageLimitCheck{
			Allowed:   false,
			LimitType: "monthly_office_pages",
			CurrentUsage: map[string]interface{}{
				"monthly_office_pages": usage.MonthlyStats.OfficePages,
				"requested_pages":      pages,
				"total_would_be":       usage.MonthlyStats.OfficePages + pages,
				"will_have_watermark":  limits.OfficeHasWatermark,
			},
			Limits: map[string]interface{}{
				"max_monthly_office_pages": limits.OfficePagesPerMonth,
				"has_watermark":            limits.OfficeHasWatermark,
			},
			ResetTime: &resetTime,
			Message:   fmt.Sprintf("Límite mensual de conversión Office alcanzado (%d de %d páginas). Se resetea el próximo mes.", usage.MonthlyStats.OfficePages, limits.OfficePagesPerMonth),
		}, nil
	}

	return &UsageLimitCheck{
		Allowed: true,
		CurrentUsage: map[string]interface{}{
			"daily_office_pages":    usage.DailyStats.OfficePages,
			"monthly_office_pages":  usage.MonthlyStats.OfficePages,
			"remaining_daily":       limits.OfficePagesPerDay - usage.DailyStats.OfficePages,
			"remaining_monthly":     limits.OfficePagesPerMonth - usage.MonthlyStats.OfficePages,
			"will_have_watermark":   limits.OfficeHasWatermark,
		},
	}, nil
}

// validateUploadUsage valida límites de subida de archivos
func (us *UsageService) validateUploadUsage(
	usage *config.UserUsageStats,
	limits config.PlanLimits,
	fileSizeMB int,
) (*UsageLimitCheck, error) {
	
	// Verificar límite diario de archivos
	if usage.DailyStats.FilesProcessed >= limits.MaxFilesPerDay {
		resetTime := time.Now().Add(24 * time.Hour)
		return &UsageLimitCheck{
			Allowed:   false,
			LimitType: "daily_files",
			CurrentUsage: map[string]interface{}{
				"daily_files": usage.DailyStats.FilesProcessed,
			},
			Limits: map[string]interface{}{
				"max_daily_files": limits.MaxFilesPerDay,
			},
			ResetTime: &resetTime,
			Message:   fmt.Sprintf("Límite diario de archivos alcanzado (%d de %d archivos). Se resetea mañana.", usage.DailyStats.FilesProcessed, limits.MaxFilesPerDay),
		}, nil
	}

	// Verificar límite mensual de archivos
	if usage.MonthlyStats.FilesProcessed >= limits.MaxFilesPerMonth {
		resetTime := time.Now().AddDate(0, 1, 0)
		return &UsageLimitCheck{
			Allowed:   false,
			LimitType: "monthly_files",
			CurrentUsage: map[string]interface{}{
				"monthly_files": usage.MonthlyStats.FilesProcessed,
			},
			Limits: map[string]interface{}{
				"max_monthly_files": limits.MaxFilesPerMonth,
			},
			ResetTime: &resetTime,
			Message:   fmt.Sprintf("Límite mensual de archivos alcanzado (%d de %d archivos). Se resetea el próximo mes.", usage.MonthlyStats.FilesProcessed, limits.MaxFilesPerMonth),
		}, nil
	}

	// Verificar límite diario de bytes
	fileSizeBytes := int64(fileSizeMB) * 1024 * 1024
	if usage.DailyStats.BytesProcessed+fileSizeBytes > limits.MaxBytesPerDay {
		resetTime := time.Now().Add(24 * time.Hour)
		return &UsageLimitCheck{
			Allowed:   false,
			LimitType: "daily_bytes",
			CurrentUsage: map[string]interface{}{
				"daily_bytes_mb": usage.DailyStats.BytesProcessed / (1024 * 1024),
				"file_size_mb":   fileSizeMB,
				"total_would_be_mb": (usage.DailyStats.BytesProcessed + fileSizeBytes) / (1024 * 1024),
			},
			Limits: map[string]interface{}{
				"max_daily_bytes_mb": limits.MaxBytesPerDay / (1024 * 1024),
			},
			ResetTime: &resetTime,
			Message:   fmt.Sprintf("Límite diario de transferencia alcanzado (%dMB de %dMB). Se resetea mañana.", usage.DailyStats.BytesProcessed/(1024*1024), limits.MaxBytesPerDay/(1024*1024)),
		}, nil
	}

	return &UsageLimitCheck{
		Allowed: true,
		CurrentUsage: map[string]interface{}{
			"daily_files":       usage.DailyStats.FilesProcessed,
			"monthly_files":     usage.MonthlyStats.FilesProcessed,
			"remaining_daily":   limits.MaxFilesPerDay - usage.DailyStats.FilesProcessed,
			"remaining_monthly": limits.MaxFilesPerMonth - usage.MonthlyStats.FilesProcessed,
			"daily_bytes_mb":    usage.DailyStats.BytesProcessed / (1024 * 1024),
			"monthly_bytes_mb": usage.MonthlyStats.BytesProcessed / (1024 * 1024),
		},
	}, nil
}

// validateGeneralUsage valida límites generales
func (us *UsageService) validateGeneralUsage(
	usage *config.UserUsageStats,
	limits config.PlanLimits,
) (*UsageLimitCheck, error) {
	
	// Verificar límite diario de operaciones
	if usage.DailyStats.Operations >= limits.DailyOperations {
		resetTime := time.Now().Add(24 * time.Hour)
		return &UsageLimitCheck{
			Allowed:   false,
			LimitType: "daily_operations",
			CurrentUsage: map[string]interface{}{
				"daily_operations": usage.DailyStats.Operations,
			},
			Limits: map[string]interface{}{
				"max_daily_operations": limits.DailyOperations,
			},
			ResetTime: &resetTime,
			Message:   fmt.Sprintf("Límite diario de operaciones alcanzado (%d de %d operaciones). Se resetea mañana.", usage.DailyStats.Operations, limits.DailyOperations),
		}, nil
	}

	return &UsageLimitCheck{
		Allowed: true,
		CurrentUsage: map[string]interface{}{
			"daily_operations":     usage.DailyStats.Operations,
			"monthly_operations":   usage.MonthlyStats.Operations,
			"remaining_daily_ops":  limits.DailyOperations - usage.DailyStats.Operations,
			"remaining_monthly_ops": limits.MonthlyOperations - usage.MonthlyStats.Operations,
		},
	}, nil
}

// GetUsageSummary obtiene un resumen completo del uso del usuario
func (us *UsageService) GetUsageSummary(ctx context.Context, userID string, plan config.Plan) (map[string]interface{}, error) {
	if userID == "" {
		return map[string]interface{}{"error": "user_id required"}, nil
	}

	usage, err := us.usageTracker.GetUserUsage(ctx, userID)
	if err != nil {
		return nil, err
	}

	planLimits := us.planConfig.GetPlanLimits(plan)

	summary := map[string]interface{}{
		"user_id": userID,
		"plan":    string(plan),
		"current_usage": map[string]interface{}{
			"daily": map[string]interface{}{
				"operations":     usage.DailyStats.Operations,
				"files":          usage.DailyStats.FilesProcessed,
				"bytes_mb":       usage.DailyStats.BytesProcessed / (1024 * 1024),
				"pages":          usage.DailyStats.PagesProcessed,
				"ocr_pages":      usage.DailyStats.AIOCRPages,
				"ai_ocr_pages":   usage.DailyStats.AIOCRPages,
				"office_pages":   usage.DailyStats.OfficePages,
			},
			"monthly": map[string]interface{}{
				"operations":     usage.MonthlyStats.Operations,
				"files":          usage.MonthlyStats.FilesProcessed,
				"bytes_mb":       usage.MonthlyStats.BytesProcessed / (1024 * 1024),
				"pages":          usage.MonthlyStats.PagesProcessed,
				"ocr_pages":      usage.MonthlyStats.AIOCRPages,
				"ai_ocr_pages":   usage.MonthlyStats.AIOCRPages,
				"office_pages":   usage.MonthlyStats.OfficePages,
			},
		},
		"limits": map[string]interface{}{
			"daily": map[string]interface{}{
				"operations":     planLimits.DailyOperations,
				"files":          planLimits.MaxFilesPerDay,
				"bytes_mb":       planLimits.MaxBytesPerDay / (1024 * 1024),
				"ocr_pages":      planLimits.OCRPagesPerDay,
				"ai_ocr_pages":   planLimits.AIOCRPagesPerDay,
				"office_pages":   planLimits.OfficePagesPerDay,
				"file_size_mb":   planLimits.MaxFileSizeMB,
				"concurrent_files": planLimits.MaxConcurrentFiles,
			},
			"monthly": map[string]interface{}{
				"operations":     planLimits.MonthlyOperations,
				"files":          planLimits.MaxFilesPerMonth,
				"bytes_mb":       planLimits.MaxBytesPerMonth / (1024 * 1024),
				"ocr_pages":      planLimits.OCRPagesPerMonth,
				"ai_ocr_pages":   planLimits.AIOCRPagesPerMonth,
				"office_pages":   planLimits.OfficePagesPerMonth,
			},
		},
		"remaining": map[string]interface{}{
			"daily": map[string]interface{}{
				"operations":     max(0, planLimits.DailyOperations-usage.DailyStats.Operations),
				"files":          max(0, planLimits.MaxFilesPerDay-usage.DailyStats.FilesProcessed),
				"bytes_mb":       max(0, int((planLimits.MaxBytesPerDay-usage.DailyStats.BytesProcessed)/(1024*1024))),
				"ocr_pages":      max(0, planLimits.OCRPagesPerDay-usage.DailyStats.AIOCRPages),
				"ai_ocr_pages":   max(0, planLimits.AIOCRPagesPerDay-usage.DailyStats.AIOCRPages),
				"office_pages":   max(0, planLimits.OfficePagesPerDay-usage.DailyStats.OfficePages),
			},
			"monthly": map[string]interface{}{
				"operations":     max(0, planLimits.MonthlyOperations-usage.MonthlyStats.Operations),
				"files":          max(0, planLimits.MaxFilesPerMonth-usage.MonthlyStats.FilesProcessed),
				"bytes_mb":       usage.MonthlyStats.BytesProcessed / (1024 * 1024),
				"ocr_pages":      max(0, planLimits.OCRPagesPerMonth-usage.MonthlyStats.AIOCRPages),
				"ai_ocr_pages":   max(0, planLimits.AIOCRPagesPerMonth-usage.MonthlyStats.AIOCRPages),
				"office_pages":   max(0, planLimits.OfficePagesPerMonth-usage.MonthlyStats.OfficePages),
			},
		},
		"percentages": map[string]interface{}{
			"daily_operations":  us.calculatePercentage(usage.DailyStats.Operations, planLimits.DailyOperations),
			"daily_files":      us.calculatePercentage(usage.DailyStats.FilesProcessed, planLimits.MaxFilesPerDay),
			"daily_bytes":      us.calculatePercentage(int(usage.DailyStats.BytesProcessed), int(planLimits.MaxBytesPerDay)),
			"monthly_operations": us.calculatePercentage(usage.MonthlyStats.Operations, planLimits.MonthlyOperations),
			"monthly_files":    us.calculatePercentage(usage.MonthlyStats.FilesProcessed, planLimits.MaxFilesPerMonth),
						"monthly_bytes":     us.calculatePercentage(int(usage.MonthlyStats.BytesProcessed), int(planLimits.MaxBytesPerMonth)),
		},
		"features": map[string]interface{}{
			"ai_ocr_enabled":     planLimits.EnableAIOCR,
			"priority_enabled":   planLimits.EnablePriority,
			"analytics_enabled":  planLimits.EnableAnalytics,
			"team_access":        planLimits.EnableTeamAccess,
			"api_access":         planLimits.EnableAPI,
			"has_watermark":      planLimits.HasWatermark,
			"has_ads":            planLimits.HasAds,
			"support_level":      planLimits.SupportLevel,
			"max_team_users":     planLimits.MaxTeamUsers,
		},
		"timestamp": time.Now(),
	}

	return summary, nil
}

// Helper methods

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (us *UsageService) calculatePercentage(current, limit int) float64 {
	if limit == 0 {
		return 0.0
	}
	percentage := (float64(current) / float64(limit)) * 100
	if percentage > 100 {
		return 100.0
	}
	return percentage
}

// ResetUserCounters resetea manualmente los contadores de un usuario (admin)
func (us *UsageService) ResetUserCounters(ctx context.Context, userID string, resetType string) error {
	switch resetType {
	case "daily":
		return us.usageTracker.ResetDailyCounters(ctx, userID)
	case "monthly":
		return us.usageTracker.ResetMonthlyCounters(ctx, userID)
	default:
		return fmt.Errorf("invalid reset type: %s", resetType)
	}
}