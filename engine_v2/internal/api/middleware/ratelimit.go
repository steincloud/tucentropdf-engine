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
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// RateLimitMiddleware middleware de rate limiting por plan
type RateLimitMiddleware struct {
	config   *config.Config
	logger   *logger.Logger
	redis    *redis.Client
}

// NewRateLimitMiddleware crear nuevo middleware de rate limiting
func NewRateLimitMiddleware(cfg *config.Config, log *logger.Logger, redisClient *redis.Client) *RateLimitMiddleware {
	return &RateLimitMiddleware{
		config:   cfg,
		logger:   log,
		redis:    redisClient,
	}
}

// RateLimit aplicar rate limiting basado en plan
func (m *RateLimitMiddleware) RateLimit() fiber.Handler {
	return func(c *fiber.Ctx) error {
		if !m.config.Security.EnableRateLimiting {
			return c.Next()
		}

		// Obtener plan del usuario
		userPlan, ok := c.Locals("userPlan").(string)
		if !ok {
			userPlan = "free"
		}

		// Obtener límites del plan
		limits := m.getPlanRateLimits(userPlan)

		// Crear clave única para el usuario
		userKey := m.generateUserKey(c, userPlan)

		// Verificar límites
		allowed, remaining, resetTime, err := m.checkRateLimit(userKey, limits)
		if err != nil {
			m.logger.Error("Rate limit check failed", "error", err.Error())
			// En caso de error, permitir la petición pero loguearlo
			return c.Next()
		}

		// Establecer headers de rate limiting
		c.Set("X-RateLimit-Limit", strconv.Itoa(limits.RequestsPerMinute))
		c.Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Set("X-RateLimit-Reset", strconv.FormatInt(resetTime.Unix(), 10))
		c.Set("X-RateLimit-Plan", userPlan)

		// Verificar si se excede el límite
		if !allowed {
			m.logger.Warn("Rate limit exceeded",
				"plan", userPlan,
				"user_key", userKey,
				"limit", limits.RequestsPerMinute,
				"remaining", remaining,
				"reset_time", resetTime.Format(time.RFC3339),
			)

			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error": fmt.Sprintf("Límite de %d peticiones por minuto excedido. Intenta de nuevo en %v", 
					limits.RequestsPerMinute, 
					time.Until(resetTime).Round(time.Second)),
				"code": "RATE_LIMIT_EXCEEDED",
				"details": fiber.Map{
					"limit":     limits.RequestsPerMinute,
					"remaining": 0,
					"reset_at":  resetTime.Unix(),
					"plan":      userPlan,
				},
			})
		}

		// Log de petición válida
		m.logger.Debug("Rate limit check passed",
			"plan", userPlan,
			"remaining", remaining,
			"limit", limits.RequestsPerMinute,
		)

		return c.Next()
	}
}

// RateLimitConfig configuración de rate limiting por plan
type RateLimitConfig struct {
	RequestsPerMinute int
	BurstLimit       int
	WindowSize       time.Duration
}

// getPlanRateLimits obtener límites de rate limiting por plan
func (m *RateLimitMiddleware) getPlanRateLimits(plan string) *RateLimitConfig {
	switch strings.ToLower(plan) {
	case "free":
		return &RateLimitConfig{
			RequestsPerMinute: 10,
			BurstLimit:       20,
			WindowSize:       time.Minute,
		}
	case "premium":
		return &RateLimitConfig{
			RequestsPerMinute: 60,
			BurstLimit:       120,
			WindowSize:       time.Minute,
		}
	case "pro":
		return &RateLimitConfig{
			RequestsPerMinute: 300,
			BurstLimit:       600,
			WindowSize:       time.Minute,
		}
	default:
		return &RateLimitConfig{
			RequestsPerMinute: 5,
			BurstLimit:       10,
			WindowSize:       time.Minute,
		}
	}
}

// generateUserKey generar clave única para el usuario
func (m *RateLimitMiddleware) generateUserKey(c *fiber.Ctx, plan string) string {
	// Usar API key si está disponible, sino usar IP
	apiKey := c.Get("X-API-Key")
	if apiKey == "" {
		apiKey = c.Get("Authorization")
		if strings.HasPrefix(apiKey, "Bearer ") {
			apiKey = apiKey[7:]
		}
	}

	if apiKey != "" {
		return fmt.Sprintf("ratelimit:api:%s", apiKey)
	}

	// Fallback a IP si no hay API key
	ip := c.IP()
	return fmt.Sprintf("ratelimit:ip:%s:%s", ip, plan)
}

// checkRateLimit verificar límites usando algoritmo sliding window con Redis
func (m *RateLimitMiddleware) checkRateLimit(userKey string, limits *RateLimitConfig) (allowed bool, remaining int, resetTime time.Time, err error) {
	if m.redis == nil {
		// Sin Redis, usar límites en memoria básicos (fallback)
		return m.checkRateLimitMemory(userKey, limits)
	}

	ctx := context.Background()
	now := time.Now()
	window := limits.WindowSize
	
	// Usar sliding window con Redis
	pipeline := m.redis.Pipeline()
	
	// Limpiar entradas antiguas
	pipeline.ZRemRangeByScore(ctx, userKey, "-inf", strconv.FormatInt(now.Add(-window).Unix(), 10))
	
	// Contar peticiones en la ventana actual
	countCmd := pipeline.ZCard(ctx, userKey)
	
	// Agregar petición actual
	pipeline.ZAdd(ctx, userKey, &redis.Z{
		Score:  float64(now.Unix()),
		Member: fmt.Sprintf("%d:%d", now.Unix(), now.Nanosecond()),
	})
	
	// Establecer TTL
	pipeline.Expire(ctx, userKey, window*2)
	
	// Ejecutar pipeline
	_, err = pipeline.Exec(ctx)
	if err != nil {
		return false, 0, time.Time{}, fmt.Errorf("error ejecutando pipeline Redis: %v", err)
	}

	// Obtener resultado del conteo
	currentCount, err := countCmd.Result()
	if err != nil {
		return false, 0, time.Time{}, fmt.Errorf("error obteniendo conteo: %v", err)
	}

	// Calcular valores de retorno
	allowed = int(currentCount) <= limits.RequestsPerMinute
	remaining = limits.RequestsPerMinute - int(currentCount)
	if remaining < 0 {
		remaining = 0
	}
	
	resetTime = now.Add(window)

	return allowed, remaining, resetTime, nil
}

// checkRateLimitMemory fallback para rate limiting en memoria
func (m *RateLimitMiddleware) checkRateLimitMemory(userKey string, limits *RateLimitConfig) (allowed bool, remaining int, resetTime time.Time, err error) {
	// Implementación simple en memoria para fallback
	// NOTA: Esto no funciona en múltiples instancias, solo para desarrollo
	now := time.Now()
	
	// Por simplicidad, asumimos que está permitido en modo fallback
	// En producción se debe usar Redis
	m.logger.Warn("Using memory-based rate limiting fallback", 
		"user_key", userKey,
		"note", "This is not suitable for production with multiple instances")
	
	return true, limits.RequestsPerMinute - 1, now.Add(limits.WindowSize), nil
}

// GetUserStats obtener estadísticas de uso del usuario
func (m *RateLimitMiddleware) GetUserStats(userKey string, plan string) map[string]interface{} {
	limits := m.getPlanRateLimits(plan)
	
	stats := map[string]interface{}{
		"plan":                plan,
		"requests_per_minute": limits.RequestsPerMinute,
		"burst_limit":        limits.BurstLimit,
		"window_size_seconds": int(limits.WindowSize.Seconds()),
	}

	if m.redis != nil {
		ctx := context.Background()
		now := time.Now()
		window := limits.WindowSize
		
		// Contar peticiones en la ventana actual
		count, err := m.redis.ZCount(ctx, userKey, 
			strconv.FormatInt(now.Add(-window).Unix(), 10), 
			strconv.FormatInt(now.Unix(), 10)).Result()
		
		if err == nil {
			stats["current_usage"] = count
			stats["remaining"] = limits.RequestsPerMinute - int(count)
		}
	}

	return stats
}