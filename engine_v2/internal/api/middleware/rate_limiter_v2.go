package middleware

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/redis/go-redis/v9"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

const (
	// Prefijos de keys Redis
	RateLimitKeyPrefix = "ratelimit:"
	AbuseKeyPrefix     = "abuse:"
	
	// Ventana de tiempo para sliding window
	WindowSize = 1 * time.Minute
	
	// Penalty por abuse (multiplicador de rate limit)
	AbusePenaltyMultiplier = 0.5 // Reducir límite al 50%
	AbusePenaltyDuration   = 15 * time.Minute
)

// RateLimiterV2 implementa rate limiting avanzado con Redis
type RateLimiterV2 struct {
	redis  *redis.Client
	logger *logger.Logger
	config *config.Config
}

// PlanLimits límites por plan
type PlanRateLimits struct {
	RequestsPerMinute int           // Límite base
	BurstAllowance    int           // Burst adicional permitido
	MaxConcurrent     int           // Máximo concurrent requests
	CooldownPeriod    time.Duration // Cooldown después de alcanzar límite
}

// NewRateLimiterV2 crea una nueva instancia
func NewRateLimiterV2(redisClient *redis.Client, cfg *config.Config, log *logger.Logger) *RateLimiterV2 {
	return &RateLimiterV2{
		redis:  redisClient,
		config: cfg,
		logger: log,
	}
}

// Middleware retorna el middleware de Fiber
func (rl *RateLimiterV2) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Obtener identificador del usuario
		userID := rl.getUserID(c)
		if userID == "" {
			userID = c.IP() // Fallback a IP
		}
		
		// Obtener plan del usuario
		plan := rl.getUserPlan(c)
		
		// Obtener límites del plan
		limits := rl.getPlanLimits(plan)
		
		// Verificar si está en abuse penalty
		penaltyMultiplier, err := rl.checkAbusePenalty(c.Context(), userID)
		if err != nil {
			rl.logger.Error("Failed to check abuse penalty", "error", err)
		}
		
		// Aplicar penalty si existe
		effectiveLimit := limits.RequestsPerMinute
		if penaltyMultiplier < 1.0 {
			effectiveLimit = int(float64(effectiveLimit) * penaltyMultiplier)
			rl.logger.Warn("Abuse penalty active",
				"user_id", userID,
				"multiplier", penaltyMultiplier,
				"effective_limit", effectiveLimit,
			)
		}
		
		// Verificar rate limit con sliding window
		allowed, remaining, resetAt, err := rl.checkRateLimit(
			c.Context(),
			userID,
			effectiveLimit,
			limits.BurstAllowance,
		)
		
		if err != nil {
			rl.logger.Error("Rate limit check failed", "error", err)
			// En caso de error, permitir request (fail open)
			return c.Next()
		}
		
		// Establecer headers de rate limit
		c.Set("X-RateLimit-Limit", strconv.Itoa(effectiveLimit))
		c.Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		c.Set("X-RateLimit-Reset", strconv.FormatInt(resetAt.Unix(), 10))
		
		if !allowed {
			// Verificar si es abuse (múltiples intentos después del límite)
			if err := rl.recordAbuse(c.Context(), userID); err != nil {
				rl.logger.Error("Failed to record abuse", "error", err)
			}
			
			// Calcular tiempo de cooldown
			retryAfter := int(resetAt.Sub(time.Now()).Seconds())
			if retryAfter < 0 {
				retryAfter = 60
			}
			
			c.Set("Retry-After", strconv.Itoa(retryAfter))
			
			rl.logger.Warn("Rate limit exceeded",
				"user_id", userID,
				"plan", plan,
				"limit", effectiveLimit,
				"retry_after", retryAfter,
			)
			
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   "RATE_LIMIT_EXCEEDED",
				"message": fmt.Sprintf("Rate limit exceeded. Try again in %d seconds.", retryAfter),
				"limit":   effectiveLimit,
				"reset_at": resetAt.Unix(),
			})
		}
		
		// Verificar concurrent requests
		concurrent, err := rl.incrementConcurrent(c.Context(), userID)
		if err != nil {
			rl.logger.Error("Failed to increment concurrent", "error", err)
		} else if concurrent > limits.MaxConcurrent {
			rl.decrementConcurrent(c.Context(), userID)
			
			rl.logger.Warn("Concurrent limit exceeded",
				"user_id", userID,
				"concurrent", concurrent,
				"limit", limits.MaxConcurrent,
			)
			
			return c.Status(fiber.StatusTooManyRequests).JSON(fiber.Map{
				"error":   "CONCURRENT_LIMIT_EXCEEDED",
				"message": fmt.Sprintf("Too many concurrent requests. Max: %d", limits.MaxConcurrent),
			})
		}
		
		// Decrementar concurrent al finalizar request
		defer rl.decrementConcurrent(c.Context(), userID)
		
		return c.Next()
	}
}

// checkRateLimit verifica el rate limit usando sliding window algorithm
func (rl *RateLimiterV2) checkRateLimit(ctx context.Context, userID string, limit int, burst int) (bool, int, time.Time, error) {
	now := time.Now()
	windowStart := now.Add(-WindowSize)
	
	key := fmt.Sprintf("%s%s", RateLimitKeyPrefix, userID)
	
	// Lua script para sliding window atómico
	script := `
		local key = KEYS[1]
		local now = tonumber(ARGV[1])
		local window_start = tonumber(ARGV[2])
		local limit = tonumber(ARGV[3])
		local burst = tonumber(ARGV[4])
		local window_size = tonumber(ARGV[5])
		
		-- Eliminar requests antiguos fuera de la ventana
		redis.call('ZREMRANGEBYSCORE', key, '-inf', window_start)
		
		-- Contar requests en la ventana actual
		local count = redis.call('ZCARD', key)
		
		-- Calcular límite efectivo (base + burst)
		local effective_limit = limit + burst
		
		if count < effective_limit then
			-- Agregar request actual
			redis.call('ZADD', key, now, now)
			redis.call('EXPIRE', key, window_size)
			return {1, effective_limit - count - 1}
		else
			return {0, 0}
		end
	`
	
	result, err := rl.redis.Eval(ctx, script, []string{key},
		now.UnixNano(),
		windowStart.UnixNano(),
		limit,
		burst,
		int(WindowSize.Seconds())+10, // TTL con buffer
	).Result()
	
	if err != nil {
		return false, 0, time.Time{}, fmt.Errorf("redis eval failed: %w", err)
	}
	
	resultSlice, ok := result.([]interface{})
	if !ok || len(resultSlice) != 2 {
		return false, 0, time.Time{}, fmt.Errorf("invalid result format")
	}
	
	allowed := resultSlice[0].(int64) == 1
	remaining := int(resultSlice[1].(int64))
	resetAt := now.Add(WindowSize)
	
	return allowed, remaining, resetAt, nil
}

// incrementConcurrent incrementa contador de requests concurrentes
func (rl *RateLimiterV2) incrementConcurrent(ctx context.Context, userID string) (int, error) {
	key := fmt.Sprintf("%sconcurrent:%s", RateLimitKeyPrefix, userID)
	
	count, err := rl.redis.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	
	// Establecer TTL de 5 minutos por seguridad
	rl.redis.Expire(ctx, key, 5*time.Minute)
	
	return int(count), nil
}

// decrementConcurrent decrementa contador de requests concurrentes
func (rl *RateLimiterV2) decrementConcurrent(ctx context.Context, userID string) {
	key := fmt.Sprintf("%sconcurrent:%s", RateLimitKeyPrefix, userID)
	rl.redis.Decr(ctx, key)
}

// checkAbusePenalty verifica si el usuario tiene penalty por abuse
func (rl *RateLimiterV2) checkAbusePenalty(ctx context.Context, userID string) (float64, error) {
	key := fmt.Sprintf("%s%s", AbuseKeyPrefix, userID)
	
	exists, err := rl.redis.Exists(ctx, key).Result()
	if err != nil {
		return 1.0, err
	}
	
	if exists > 0 {
		// Usuario tiene penalty activo
		return AbusePenaltyMultiplier, nil
	}
	
	return 1.0, nil
}

// recordAbuse registra un intento de abuse (exceder rate limit múltiples veces)
func (rl *RateLimiterV2) recordAbuse(ctx context.Context, userID string) error {
	key := fmt.Sprintf("%s%s", AbuseKeyPrefix, userID)
	counterKey := fmt.Sprintf("%scount:%s", AbuseKeyPrefix, userID)
	
	// Incrementar contador de abuse
	count, err := rl.redis.Incr(ctx, counterKey).Result()
	if err != nil {
		return err
	}
	
	rl.redis.Expire(ctx, counterKey, 5*time.Minute)
	
	// Si excede 10 intentos en 5 minutos, aplicar penalty
	if count >= 10 {
		if err := rl.redis.Set(ctx, key, "1", AbusePenaltyDuration).Err(); err != nil {
			return err
		}
		
		rl.logger.Warn("Abuse penalty applied",
			"user_id", userID,
			"attempts", count,
			"duration", AbusePenaltyDuration,
		)
		
		// Resetear contador
		rl.redis.Del(ctx, counterKey)
	}
	
	return nil
}

// getPlanLimits obtiene límites según el plan
func (rl *RateLimiterV2) getPlanLimits(plan string) PlanRateLimits {
	switch plan {
	case "pro":
		return PlanRateLimits{
			RequestsPerMinute: 300,
			BurstAllowance:    50,
			MaxConcurrent:     20,
			CooldownPeriod:    30 * time.Second,
		}
	case "premium":
		return PlanRateLimits{
			RequestsPerMinute: 100,
			BurstAllowance:    20,
			MaxConcurrent:     10,
			CooldownPeriod:    1 * time.Minute,
		}
	default: // free
		return PlanRateLimits{
			RequestsPerMinute: 30,
			BurstAllowance:    5,
			MaxConcurrent:     3,
			CooldownPeriod:    2 * time.Minute,
		}
	}
}

// getUserID obtiene el ID del usuario del contexto
func (rl *RateLimiterV2) getUserID(c *fiber.Ctx) string {
	if userID, ok := c.Locals("userID").(string); ok {
		return userID
	}
	return ""
}

// getUserPlan obtiene el plan del usuario del contexto
func (rl *RateLimiterV2) getUserPlan(c *fiber.Ctx) string {
	if plan, ok := c.Locals("userPlan").(string); ok {
		return plan
	}
	return "free"
}

// GetStats obtiene estadísticas de rate limiting
func (rl *RateLimiterV2) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	// Contar keys de rate limit activos
	pattern := RateLimitKeyPrefix + "*"
	iter := rl.redis.Scan(ctx, 0, pattern, 100).Iterator()
	
	activeUsers := 0
	for iter.Next(ctx) {
		activeUsers++
	}
	
	if err := iter.Err(); err != nil {
		return nil, err
	}
	
	// Contar usuarios con abuse penalty
	abusePattern := AbuseKeyPrefix + "*"
	abuseIter := rl.redis.Scan(ctx, 0, abusePattern, 100).Iterator()
	
	abusedUsers := 0
	for abuseIter.Next(ctx) {
		abusedUsers++
	}
	
	if err := abuseIter.Err(); err != nil {
		return nil, err
	}
	
	stats["active_users"] = activeUsers
	stats["abused_users"] = abusedUsers
	stats["window_size_seconds"] = int(WindowSize.Seconds())
	
	return stats, nil
}

// ResetUserLimit resetea el rate limit de un usuario (admin only)
func (rl *RateLimiterV2) ResetUserLimit(ctx context.Context, userID string) error {
	key := fmt.Sprintf("%s%s", RateLimitKeyPrefix, userID)
	abuseKey := fmt.Sprintf("%s%s", AbuseKeyPrefix, userID)
	
	if err := rl.redis.Del(ctx, key, abuseKey).Err(); err != nil {
		return err
	}
	
	rl.logger.Info("User rate limit reset", "user_id", userID)
	return nil
}
