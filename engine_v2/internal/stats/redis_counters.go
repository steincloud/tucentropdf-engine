package stats

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"

	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// RedisCounters servicio de contadores rápidos en Redis
type RedisCounters struct {
	redis  *redis.Client
	logger *logger.Logger
	ctx    context.Context
}

// NewRedisCounters crea nueva instancia de contadores Redis
func NewRedisCounters(redisClient *redis.Client, log *logger.Logger) *RedisCounters {
	return &RedisCounters{
		redis:  redisClient,
		logger: log,
		ctx:    context.Background(),
	}
}

// CounterData datos de contador
type CounterData struct {
	Key   string `json:"key"`
	Value int64  `json:"value"`
	TTL   time.Duration `json:"ttl"`
}

// IncrementToolUsage incrementa uso de herramienta
func (rc *RedisCounters) IncrementToolUsage(tool string, period string) error {
	key := rc.getToolKey(tool, period)
	ttl := rc.getTTLForPeriod(period)

	pipe := rc.redis.Pipeline()
	pipe.Incr(rc.ctx, key)
	pipe.Expire(rc.ctx, key, ttl)

	_, err := pipe.Exec(rc.ctx)
	return err
}

// IncrementUserUsage incrementa uso de usuario
func (rc *RedisCounters) IncrementUserUsage(userID string, tool string, period string) error {
	key := rc.getUserToolKey(userID, tool, period)
	userKey := rc.getUserKey(userID, period)
	ttl := rc.getTTLForPeriod(period)

	pipe := rc.redis.Pipeline()
	pipe.Incr(rc.ctx, key)
	pipe.Incr(rc.ctx, userKey)
	pipe.Expire(rc.ctx, key, ttl)
	pipe.Expire(rc.ctx, userKey, ttl)

	_, err := pipe.Exec(rc.ctx)
	return err
}

// IncrementPlanUsage incrementa uso por plan
func (rc *RedisCounters) IncrementPlanUsage(plan string, tool string, period string) error {
	planKey := rc.getPlanKey(plan, period)
	planToolKey := rc.getPlanToolKey(plan, tool, period)
	ttl := rc.getTTLForPeriod(period)

	pipe := rc.redis.Pipeline()
	pipe.Incr(rc.ctx, planKey)
	pipe.Incr(rc.ctx, planToolKey)
	pipe.Expire(rc.ctx, planKey, ttl)
	pipe.Expire(rc.ctx, planToolKey, ttl)

	_, err := pipe.Exec(rc.ctx)
	return err
}

// IncrementSuccessCount incrementa contador de éxitos
func (rc *RedisCounters) IncrementSuccessCount(tool string, period string) error {
	key := rc.getSuccessKey(tool, period)
	ttl := rc.getTTLForPeriod(period)

	pipe := rc.redis.Pipeline()
	pipe.Incr(rc.ctx, key)
	pipe.Expire(rc.ctx, key, ttl)

	_, err := pipe.Exec(rc.ctx)
	return err
}

// IncrementFailureCount incrementa contador de fallos
func (rc *RedisCounters) IncrementFailureCount(tool string, reason string, period string) error {
	failKey := rc.getFailureKey(tool, period)
	reasonKey := rc.getFailureReasonKey(reason, period)
	ttl := rc.getTTLForPeriod(period)

	pipe := rc.redis.Pipeline()
	pipe.Incr(rc.ctx, failKey)
	pipe.Incr(rc.ctx, reasonKey)
	pipe.Expire(rc.ctx, failKey, ttl)
	pipe.Expire(rc.ctx, reasonKey, ttl)

	_, err := pipe.Exec(rc.ctx)
	return err
}

// AddProcessingTime agrega tiempo de procesamiento (lista para promedio móvil)
func (rc *RedisCounters) AddProcessingTime(tool string, durationMS int64) error {
	key := rc.getProcessingTimeKey(tool)

	pipe := rc.redis.Pipeline()
	pipe.LPush(rc.ctx, key, durationMS)
	pipe.LTrim(rc.ctx, key, 0, 999) // Mantener últimas 1000 mediciones
	pipe.Expire(rc.ctx, key, 7*24*time.Hour) // Expirar en 7 días

	_, err := pipe.Exec(rc.ctx)
	return err
}

// AddFileSize agrega tamaño de archivo
func (rc *RedisCounters) AddFileSize(tool string, sizeBytes int64) error {
	key := rc.getFileSizeKey(tool)

	pipe := rc.redis.Pipeline()
	pipe.LPush(rc.ctx, key, sizeBytes)
	pipe.LTrim(rc.ctx, key, 0, 999) // Mantener últimas 1000 mediciones
	pipe.Expire(rc.ctx, key, 7*24*time.Hour)

	_, err := pipe.Exec(rc.ctx)
	return err
}

// GetToolUsage obtiene uso de herramienta
func (rc *RedisCounters) GetToolUsage(tool string, period string) (int64, error) {
	key := rc.getToolKey(tool, period)
	result, err := rc.redis.Get(rc.ctx, key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	return strconv.ParseInt(result, 10, 64)
}

// GetUserUsage obtiene uso de usuario
func (rc *RedisCounters) GetUserUsage(userID string, period string) (int64, error) {
	key := rc.getUserKey(userID, period)
	result, err := rc.redis.Get(rc.ctx, key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	return strconv.ParseInt(result, 10, 64)
}

// GetUserToolUsage obtiene uso de herramienta por usuario
func (rc *RedisCounters) GetUserToolUsage(userID string, tool string) (int64, error) {
	key := rc.getUserToolKey(userID, tool, "total")
	result, err := rc.redis.Get(rc.ctx, key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	return strconv.ParseInt(result, 10, 64)
}

// GetPlanUsage obtiene uso por plan
func (rc *RedisCounters) GetPlanUsage(plan string, period string) (int64, error) {
	key := rc.getPlanKey(plan, period)
	result, err := rc.redis.Get(rc.ctx, key).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	return strconv.ParseInt(result, 10, 64)
}

// GetSuccessRate obtiene tasa de éxito de herramienta
func (rc *RedisCounters) GetSuccessRate(tool string, period string) (float64, error) {
	total, err := rc.GetToolUsage(tool, period)
	if err != nil || total == 0 {
		return 0, err
	}

	successKey := rc.getSuccessKey(tool, period)
	successResult, err := rc.redis.Get(rc.ctx, successKey).Result()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}

	successes, err := strconv.ParseInt(successResult, 10, 64)
	if err != nil {
		return 0, err
	}

	return float64(successes) / float64(total) * 100, nil
}

// GetAvgProcessingTime obtiene tiempo promedio de procesamiento
func (rc *RedisCounters) GetAvgProcessingTime(tool string) (float64, error) {
	key := rc.getProcessingTimeKey(tool)
	times, err := rc.redis.LRange(rc.ctx, key, 0, -1).Result()
	if err != nil || len(times) == 0 {
		return 0, err
	}

	total := int64(0)
	count := int64(0)

	for _, timeStr := range times {
		if time, err := strconv.ParseInt(timeStr, 10, 64); err == nil {
			total += time
			count++
		}
	}

	if count == 0 {
		return 0, nil
	}

	return float64(total) / float64(count), nil
}

// GetAvgFileSize obtiene tamaño promedio de archivo
func (rc *RedisCounters) GetAvgFileSize(tool string) (float64, error) {
	key := rc.getFileSizeKey(tool)
	sizes, err := rc.redis.LRange(rc.ctx, key, 0, -1).Result()
	if err != nil || len(sizes) == 0 {
		return 0, err
	}

	total := int64(0)
	count := int64(0)

	for _, sizeStr := range sizes {
		if size, err := strconv.ParseInt(sizeStr, 10, 64); err == nil {
			total += size
			count++
		}
	}

	if count == 0 {
		return 0, nil
	}

	return float64(total) / float64(count), nil
}

// GetTopFailureReasons obtiene principales razones de fallo
func (rc *RedisCounters) GetTopFailureReasons(period string, limit int) (map[string]int64, error) {
	pattern := rc.getFailureReasonPattern(period)
	keys, err := rc.redis.Keys(rc.ctx, pattern).Result()
	if err != nil {
		return nil, err
	}

	results := make(map[string]int64)

	for _, key := range keys {
		val, err := rc.redis.Get(rc.ctx, key).Result()
		if err == nil {
			count, _ := strconv.ParseInt(val, 10, 64)
			reason := rc.extractReasonFromKey(key)
			results[reason] = count
		}
	}

	return results, nil
}

// GetAllToolStats obtiene estadísticas de todas las herramientas
func (rc *RedisCounters) GetAllToolStats(period string) (map[string]map[string]int64, error) {
	pattern := rc.getToolPattern(period)
	keys, err := rc.redis.Keys(rc.ctx, pattern).Result()
	if err != nil {
		return nil, err
	}

	results := make(map[string]map[string]int64)

	for _, key := range keys {
		val, err := rc.redis.Get(rc.ctx, key).Result()
		if err == nil {
			count, _ := strconv.ParseInt(val, 10, 64)
			tool := rc.extractToolFromKey(key)
			if results[tool] == nil {
				results[tool] = make(map[string]int64)
			}
			results[tool]["usage"] = count
		}
	}

	return results, nil
}

// DeleteExpiredCounters limpia contadores expirados manualmente
func (rc *RedisCounters) DeleteExpiredCounters() error {
	// Redis maneja la expiración automáticamente, pero esta función
	// puede usarse para limpieza manual si es necesario
	return nil
}

// Helper functions para generar keys

func (rc *RedisCounters) getToolKey(tool, period string) string {
	return fmt.Sprintf("tool:%s:%s_count:%s", tool, period, rc.getCurrentPeriod(period))
}

func (rc *RedisCounters) getUserKey(userID, period string) string {
	return fmt.Sprintf("user:%s:%s_count:%s", userID, period, rc.getCurrentPeriod(period))
}

func (rc *RedisCounters) getUserToolKey(userID, tool, period string) string {
	if period == "total" {
		return fmt.Sprintf("user:%s:tool_usage:%s", userID, tool)
	}
	return fmt.Sprintf("user:%s:tool:%s:%s_count:%s", userID, tool, period, rc.getCurrentPeriod(period))
}

func (rc *RedisCounters) getPlanKey(plan, period string) string {
	return fmt.Sprintf("plan:%s:%s_count:%s", plan, period, rc.getCurrentPeriod(period))
}

func (rc *RedisCounters) getPlanToolKey(plan, tool, period string) string {
	return fmt.Sprintf("plan:%s:tool:%s:%s_count:%s", plan, tool, period, rc.getCurrentPeriod(period))
}

func (rc *RedisCounters) getSuccessKey(tool, period string) string {
	return fmt.Sprintf("tool:%s:success_count:%s", tool, rc.getCurrentPeriod(period))
}

func (rc *RedisCounters) getFailureKey(tool, period string) string {
	return fmt.Sprintf("tool:%s:fail_count:%s", tool, rc.getCurrentPeriod(period))
}

func (rc *RedisCounters) getFailureReasonKey(reason, period string) string {
	return fmt.Sprintf("fail_reason:%s:%s", reason, rc.getCurrentPeriod(period))
}

func (rc *RedisCounters) getProcessingTimeKey(tool string) string {
	return fmt.Sprintf("tool:%s:durations", tool)
}

func (rc *RedisCounters) getFileSizeKey(tool string) string {
	return fmt.Sprintf("tool:%s:filesizes", tool)
}

func (rc *RedisCounters) getToolPattern(period string) string {
	return fmt.Sprintf("tool:*:%s_count:%s", period, rc.getCurrentPeriod(period))
}

func (rc *RedisCounters) getFailureReasonPattern(period string) string {
	return fmt.Sprintf("fail_reason:*:%s", rc.getCurrentPeriod(period))
}

func (rc *RedisCounters) getCurrentPeriod(period string) string {
	now := time.Now()
	switch strings.ToLower(period) {
	case "daily", "day":
		return now.Format("2006-01-02")
	case "weekly", "week":
		year, week := now.ISOWeek()
		return fmt.Sprintf("%d-W%02d", year, week)
	case "monthly", "month":
		return now.Format("2006-01")
	case "yearly", "year":
		return now.Format("2006")
	default:
		return now.Format("2006-01-02")
	}
}

func (rc *RedisCounters) getTTLForPeriod(period string) time.Duration {
	switch strings.ToLower(period) {
	case "daily", "day":
		return 25 * time.Hour
	case "weekly", "week":
		return 8 * 24 * time.Hour
	case "monthly", "month":
		return 32 * 24 * time.Hour
	case "yearly", "year":
		return 366 * 24 * time.Hour
	default:
		return 25 * time.Hour
	}
}

func (rc *RedisCounters) extractToolFromKey(key string) string {
	parts := strings.Split(key, ":")
	if len(parts) >= 2 {
		return parts[1]
	}
	return "unknown"
}

func (rc *RedisCounters) extractReasonFromKey(key string) string {
	parts := strings.Split(key, ":")
	if len(parts) >= 2 {
		return parts[1]
	}
	return "unknown"
}