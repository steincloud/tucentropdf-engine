package middleware

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/go-redis/redis/v8"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/internal/service"
	"github.com/tucentropdf/engine-v2/internal/storage"
	"github.com/tucentropdf/engine-v2/internal/utils"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// LimitsMiddleware middleware unificado para validar límites visibles e invisibles
type LimitsMiddleware struct {
	config           *config.Config
	logger           *logger.Logger
	redis            *redis.Client
	usageTracker     *storage.UsageTracker
	planConfig       *config.PlanConfiguration
	resourceMonitor  *utils.ResourceMonitor
	serviceProtector *service.ServiceProtector
	usageService     *service.UsageService
}

// NewLimitsMiddleware crear middleware de límites unificado
func NewLimitsMiddleware(
	cfg *config.Config,
	log *logger.Logger,
	redisClient *redis.Client,
	usageTracker *storage.UsageTracker,
) *LimitsMiddleware {
	return &LimitsMiddleware{
		config:           cfg,
		logger:           log,
		redis:            redisClient,
		usageTracker:     usageTracker,
		planConfig:       config.GetDefaultPlanConfiguration(),
		resourceMonitor:  utils.NewResourceMonitor(log, redisClient),
		serviceProtector: service.NewServiceProtector(cfg, log, redisClient),
		usageService:     service.NewUsageService(cfg, log, redisClient, usageTracker),
	}
}

// LimitError error estructurado para límites
type LimitError struct {
	Code         string      `json:"code"`
	Message      string      `json:"message"`
	RequiredPlan string      `json:"required_plan,omitempty"`
	CurrentUsage interface{} `json:"current_usage,omitempty"`
	Limits       interface{} `json:"limits,omitempty"`
	ResetTime    *time.Time  `json:"reset_time,omitempty"`
}

// Error implementa error interface
func (e *LimitError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ValidateLimits middleware principal de validación de límites
func (l *LimitsMiddleware) ValidateLimits() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// 1. Verificar límites internos invisibles PRIMERO
		if err := l.validateInternalLimits(c); err != nil {
			return l.handleInternalLimitError(c, err)
		}

		// 2. Obtener información del usuario y plan
		userID := l.getUserID(c)
		userPlan := l.getUserPlan(c)
		
		// 3. Validar límites visibles del plan
		if err := l.validatePlanLimits(c, userID, userPlan); err != nil {
			return l.handlePlanLimitError(c, err)
		}

		// 4. Preparar operación para tracking posterior
		if err := l.prepareOperationTracking(c, userID, userPlan); err != nil {
			l.logger.Warn("Failed to prepare operation tracking", "error", err)
		}

		return c.Next()
	}
}

// validateInternalLimits valida límites invisibles de protección del servidor
func (l *LimitsMiddleware) validateInternalLimits(c *fiber.Ctx) error {
	// 1. Verificar tamaño absoluto de archivo
	if err := l.validateAbsoluteFileSize(c); err != nil {
		return err
	}

	// 2. Verificar cola global de jobs
	if err := l.serviceProtector.ValidateGlobalQueue(); err != nil {
		return err
	}

	// 3. Verificar recursos del sistema (CPU/RAM)
	if err := l.resourceMonitor.ValidateSystemResources(); err != nil {
		return err
	}

	// 4. Verificar modo protector
	if l.serviceProtector.IsProtectorModeActive() {
		return &LimitError{
			Code:    "PROTECTOR_MODE_ACTIVE",
			Message: "Sistema en modo protector temporalmente. Intenta de nuevo en unos minutos.",
		}
	}

	return nil
}

// validateAbsoluteFileSize valida el límite absoluto de archivo (350MB)
func (l *LimitsMiddleware) validateAbsoluteFileSize(c *fiber.Ctx) error {
	contentLength := c.Get("Content-Length")
	if contentLength == "" {
		return nil // No hay contenido o se validará después
	}

	size, err := strconv.ParseInt(contentLength, 10, 64)
	if err != nil {
		return nil // Error de parsing, continuar
	}

	// Límite absoluto invisible: 350MB
	const absoluteMaxSize = 350 * 1024 * 1024
	
	if size > absoluteMaxSize {
		l.logger.Warn("Archivo excede límite absoluto invisible",
			"size_mb", size/(1024*1024),
			"max_mb", absoluteMaxSize/(1024*1024),
			"user_ip", c.IP(),
		)
		
		return &LimitError{
			Code:    "FILE_TOO_LARGE_INTERNAL",
			Message: "El archivo es demasiado grande según las políticas internas del sistema.",
		}
	}

	return nil
}

// validatePlanLimits valida límites visibles según el plan del usuario
func (l *LimitsMiddleware) validatePlanLimits(c *fiber.Ctx, userID string, plan config.Plan) error {
	planLimits := l.planConfig.GetPlanLimits(plan)

	// 1. Validar tamaño de archivo según plan
	if err := l.validatePlanFileSize(c, planLimits); err != nil {
		return err
	}

	// 2. Validar límites de uso diario/mensual
	if err := l.validateUsageLimits(c, userID, planLimits); err != nil {
		return err
	}

	// 3. Validar archivos concurrentes
	if err := l.validateConcurrentFiles(c, userID, planLimits); err != nil {
		return err
	}

	// 4. Validar rate limiting
	if err := l.validateRateLimit(c, userID, planLimits); err != nil {
		return err
	}

	return nil
}

// validatePlanFileSize valida tamaño según límites del plan
func (l *LimitsMiddleware) validatePlanFileSize(c *fiber.Ctx, limits config.PlanLimits) error {
	contentLength := c.Get("Content-Length")
	if contentLength == "" {
		return nil
	}

	size, err := strconv.ParseInt(contentLength, 10, 64)
	if err != nil {
		return nil
	}

	maxSizeBytes := int64(limits.MaxFileSizeMB) * 1024 * 1024
	
	if size > maxSizeBytes {
		return &LimitError{
			Code:         "OVER_FILE_SIZE_LIMIT",
			Message:      fmt.Sprintf("El archivo excede el límite de %dMB para tu plan.", limits.MaxFileSizeMB),
			RequiredPlan: l.getRequiredPlanForFileSize(size),
			Limits: map[string]interface{}{
				"max_file_size_mb": limits.MaxFileSizeMB,
				"current_size_mb":  size / (1024 * 1024),
			},
		}
	}

	return nil
}

// validateUsageLimits valida límites de uso diario/mensual
func (l *LimitsMiddleware) validateUsageLimits(c *fiber.Ctx, userID string, limits config.PlanLimits) error {
	if userID == "" {
		return nil // Usuario anónimo, skip
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	usage, err := l.usageTracker.GetUserUsage(ctx, userID)
	if err != nil {
		l.logger.Warn("Failed to get user usage", "user_id", userID, "error", err)
		return nil // No bloquear por error de Redis
	}

	// Validar archivos por día
	if usage.DailyStats.FilesProcessed >= limits.MaxFilesPerDay {
		resetTime := time.Now().Add(24 * time.Hour)
		return &LimitError{
			Code:    "OVER_DAILY_FILES_LIMIT",
			Message: fmt.Sprintf("Has alcanzado el límite diario de %d archivos.", limits.MaxFilesPerDay),
			CurrentUsage: map[string]interface{}{
				"daily_files": usage.DailyStats.FilesProcessed,
				"max_files":   limits.MaxFilesPerDay,
			},
			ResetTime: &resetTime,
		}
	}

	// Validar operaciones por día
	if usage.DailyStats.Operations >= limits.DailyOperations {
		resetTime := time.Now().Add(24 * time.Hour)
		return &LimitError{
			Code:    "OVER_DAILY_OPERATIONS_LIMIT",
			Message: fmt.Sprintf("Has alcanzado el límite diario de %d operaciones.", limits.DailyOperations),
			CurrentUsage: map[string]interface{}{
				"daily_operations": usage.DailyStats.Operations,
				"max_operations":   limits.DailyOperations,
			},
			ResetTime: &resetTime,
		}
	}

	return nil
}

// validateConcurrentFiles valida archivos en procesamiento simultáneo
func (l *LimitsMiddleware) validateConcurrentFiles(c *fiber.Ctx, userID string, limits config.PlanLimits) error {
	if userID == "" || l.redis == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	concurrentKey := fmt.Sprintf("user:%s:concurrent_files", userID)
	current, err := l.redis.Get(ctx, concurrentKey).Int()
	if err != nil && err != redis.Nil {
		l.logger.Warn("Failed to get concurrent files count", "user_id", userID, "error", err)
		return nil
	}

	if current >= limits.MaxConcurrentFiles {
		return &LimitError{
			Code:    "OVER_CONCURRENT_FILES_LIMIT",
			Message: fmt.Sprintf("Máximo %d archivos simultáneos permitidos. Espera a que terminen los actuales.", limits.MaxConcurrentFiles),
			CurrentUsage: map[string]interface{}{
				"concurrent_files": current,
				"max_concurrent":   limits.MaxConcurrentFiles,
			},
		}
	}

	// Incrementar contador de archivos concurrentes
	pipe := l.redis.Pipeline()
	pipe.Incr(ctx, concurrentKey)
	pipe.Expire(ctx, concurrentKey, 1*time.Hour) // TTL por si se cuelga
	
	if _, err := pipe.Exec(ctx); err != nil {
		l.logger.Warn("Failed to increment concurrent files", "user_id", userID, "error", err)
	}

	// Almacenar para decrementar después
	c.Locals("concurrentFileKey", concurrentKey)

	return nil
}

// validateRateLimit valida límites de velocidad
func (l *LimitsMiddleware) validateRateLimit(c *fiber.Ctx, userID string, limits config.PlanLimits) error {
	if l.redis == nil {
		return nil // Sin Redis, skip rate limiting
	}

	userKey := fmt.Sprintf("rate_limit:user:%s", userID)
	if userID == "" {
		userKey = fmt.Sprintf("rate_limit:ip:%s", c.IP())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Sliding window rate limiting
	now := time.Now()
	window := time.Minute
	
	pipe := l.redis.Pipeline()
	pipe.ZRemRangeByScore(ctx, userKey, "-inf", strconv.FormatInt(now.Add(-window).Unix(), 10))
	countCmd := pipe.ZCard(ctx, userKey)
	pipe.ZAdd(ctx, userKey, &redis.Z{
		Score:  float64(now.Unix()),
		Member: fmt.Sprintf("%d:%d", now.Unix(), now.Nanosecond()),
	})
	pipe.Expire(ctx, userKey, window*2)
	
	if _, err := pipe.Exec(ctx); err != nil {
		l.logger.Warn("Failed to check rate limit", "user_key", userKey, "error", err)
		return nil // No bloquear por error de Redis
	}

	current, _ := countCmd.Result()
	
	if int(current) > limits.RateLimit {
		resetTime := now.Add(window)
		return &LimitError{
			Code:    "RATE_LIMIT_EXCEEDED",
			Message: fmt.Sprintf("Demasiadas peticiones. Límite: %d por minuto.", limits.RateLimit),
			CurrentUsage: map[string]interface{}{
				"current_requests": current,
				"limit_per_minute": limits.RateLimit,
			},
			ResetTime: &resetTime,
		}
	}

	return nil
}

// prepareOperationTracking prepara información para tracking posterior
func (l *LimitsMiddleware) prepareOperationTracking(c *fiber.Ctx, userID string, plan config.Plan) error {
	operation := &storage.UsageOperation{
		UserID:        userID,
		OperationType: l.getOperationType(c),
		Timestamp:     time.Now(),
	}

	c.Locals("pendingOperation", operation)
	return nil
}

// PostProcessingCleanup middleware para limpiar después del procesamiento
func (l *LimitsMiddleware) PostProcessingCleanup() fiber.Handler {
	return func(c *fiber.Ctx) error {
		defer func() {
			// Decrementar contador de archivos concurrentes
			if key, ok := c.Locals("concurrentFileKey").(string); ok && l.redis != nil {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				
				l.redis.Decr(ctx, key)
			}

			// Track operation si fue exitosa
			if operation, ok := c.Locals("pendingOperation").(*storage.UsageOperation); ok {
				operation.Success = c.Response().StatusCode() < 400
				operation.ProcessingTime = time.Since(operation.Timestamp).Milliseconds()
				
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				
				if err := l.usageTracker.TrackUsage(ctx, operation); err != nil {
					l.logger.Error("Failed to track usage", "error", err)
				}
			}
		}()

		return c.Next()
	}
}

// Helper methods

func (l *LimitsMiddleware) getUserID(c *fiber.Ctx) string {
	if userID, ok := c.Locals("userID").(string); ok {
		return userID
	}
	return ""
}

func (l *LimitsMiddleware) getUserPlan(c *fiber.Ctx) config.Plan {
	if plan, ok := c.Locals("userPlan").(string); ok {
		return config.Plan(plan)
	}
	return config.PlanFree
}

func (l *LimitsMiddleware) getOperationType(c *fiber.Ctx) storage.OperationType {
	path := strings.ToLower(c.Path())
	
	switch {
	case strings.Contains(path, "/ocr/ai"):
		return storage.OpTypeAIOCR
	case strings.Contains(path, "/ocr"):
		return storage.OpTypeOCR
	case strings.Contains(path, "/office"):
		return storage.OpTypeOffice
	case strings.Contains(path, "/upload"):
		return storage.OpTypeUpload
	default:
		return storage.OpTypePDF
	}
}

func (l *LimitsMiddleware) getRequiredPlanForFileSize(size int64) string {
	sizeMB := size / (1024 * 1024)
	
	if sizeMB <= 50 {
		return "premium"
	} else if sizeMB <= 200 {
		return "pro"
	}
	return "corporate"
}

func (l *LimitsMiddleware) handleInternalLimitError(c *fiber.Ctx, err error) error {
	if limitErr, ok := err.(*LimitError); ok {
		l.logger.Warn("Internal limit exceeded",
			"code", limitErr.Code,
			"user_ip", c.IP(),
			"user_agent", c.Get("User-Agent"),
		)
		
		return c.Status(503).JSON(fiber.Map{
			"success": false,
			"error": fiber.Map{
				"code":    limitErr.Code,
				"message": limitErr.Message,
				"type":    "internal_limit",
			},
		})
	}
	
	return c.Status(503).JSON(fiber.Map{
		"success": false,
		"error": fiber.Map{
			"code":    "INTERNAL_LIMIT_ERROR",
			"message": "Error de límites internos del sistema",
			"type":    "internal_limit",
		},
	})
}

func (l *LimitsMiddleware) handlePlanLimitError(c *fiber.Ctx, err error) error {
	if limitErr, ok := err.(*LimitError); ok {
		status := 429 // Too Many Requests
		
		if limitErr.Code == "OVER_FILE_SIZE_LIMIT" {
			status = 413 // Payload Too Large
		}
		
		return c.Status(status).JSON(fiber.Map{
			"success": false,
			"error": fiber.Map{
				"code":          limitErr.Code,
				"message":       limitErr.Message,
				"type":          "plan_limit",
				"required_plan": limitErr.RequiredPlan,
				"current_usage": limitErr.CurrentUsage,
				"limits":        limitErr.Limits,
				"reset_time":    limitErr.ResetTime,
			},
		})
	}
	
	return c.Status(429).JSON(fiber.Map{
		"success": false,
		"error": fiber.Map{
			"code":    "PLAN_LIMIT_ERROR",
			"message": "Error de límites del plan",
			"type":    "plan_limit",
		},
	})
}