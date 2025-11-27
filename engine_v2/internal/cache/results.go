package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

const (
	// TTL por defecto para resultados cacheados
	DefaultResultTTL = 24 * time.Hour
	
	// Prefijos de keys Redis
	ResultKeyPrefix = "cache:result:"
	StatsKeyPrefix  = "cache:stats:"
	
	// Límites
	MaxCacheSize = 50 << 20 // 50MB por resultado individual
)

// ResultCache gestiona el cache de resultados procesados
type ResultCache struct {
	redis  *redis.Client
	logger *logger.Logger
	ttl    time.Duration
}

// CachedResult representa un resultado cacheado
type CachedResult struct {
	JobID       string            `json:"job_id"`
	Operation   string            `json:"operation"`
	ResultPath  string            `json:"result_path"`
	ResultData  map[string]any    `json:"result_data,omitempty"`
	FileHash    string            `json:"file_hash"`
	ParamsHash  string            `json:"params_hash"`
	Size        int64             `json:"size"`
	CreatedAt   time.Time         `json:"created_at"`
	ExpiresAt   time.Time         `json:"expires_at"`
	HitCount    int               `json:"hit_count"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// CacheStats estadísticas del cache
type CacheStats struct {
	TotalHits   int64 `json:"total_hits"`
	TotalMisses int64 `json:"total_misses"`
	HitRate     float64 `json:"hit_rate"`
	TotalKeys   int64 `json:"total_keys"`
	TotalSize   int64 `json:"total_size"`
}

// NewResultCache crea una nueva instancia de ResultCache
func NewResultCache(redisClient *redis.Client, log *logger.Logger) *ResultCache {
	return &ResultCache{
		redis:  redisClient,
		logger: log,
		ttl:    DefaultResultTTL,
	}
}

// SetTTL establece el TTL para nuevos resultados cacheados
func (rc *ResultCache) SetTTL(ttl time.Duration) {
	rc.ttl = ttl
}

// GenerateCacheKey genera una key única para el cache
// Key format: cache:result:{file_hash}:{operation}:{params_hash}
func (rc *ResultCache) GenerateCacheKey(fileHash, operation string, params map[string]any) string {
	paramsHash := rc.hashParams(params)
	return fmt.Sprintf("%s%s:%s:%s", ResultKeyPrefix, fileHash, operation, paramsHash)
}

// hashParams genera un hash de los parámetros
func (rc *ResultCache) hashParams(params map[string]any) string {
	if params == nil || len(params) == 0 {
		return "default"
	}
	
	// Serializar params a JSON ordenado
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		return "default"
	}
	
	// Calcular hash SHA256
	hash := sha256.Sum256(paramsJSON)
	return hex.EncodeToString(hash[:8]) // Usar solo primeros 8 bytes
}

// Get obtiene un resultado del cache
func (rc *ResultCache) Get(ctx context.Context, fileHash, operation string, params map[string]any) (*CachedResult, error) {
	key := rc.GenerateCacheKey(fileHash, operation, params)
	
	// Obtener del Redis
	data, err := rc.redis.Get(ctx, key).Bytes()
	if err == redis.Nil {
		// Cache miss
		rc.incrementMisses(ctx)
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("redis get error: %w", err)
	}
	
	// Deserializar resultado
	var result CachedResult
	if err := json.Unmarshal(data, &result); err != nil {
		rc.logger.Error("Failed to unmarshal cached result", "key", key, "error", err)
		return nil, fmt.Errorf("unmarshal error: %w", err)
	}
	
	// Verificar expiración
	if time.Now().After(result.ExpiresAt) {
		rc.Delete(ctx, fileHash, operation, params)
		rc.incrementMisses(ctx)
		return nil, nil
	}
	
	// Incrementar hit count
	result.HitCount++
	rc.incrementHits(ctx)
	
	// Actualizar hit count en Redis (fire and forget)
	go func() {
		data, _ := json.Marshal(result)
		rc.redis.Set(context.Background(), key, data, rc.ttl)
	}()
	
	rc.logger.Debug("Cache hit",
		"key", key,
		"operation", operation,
		"hit_count", result.HitCount,
	)
	
	return &result, nil
}

// Set almacena un resultado en el cache
func (rc *ResultCache) Set(ctx context.Context, result *CachedResult) error {
	if result.Size > MaxCacheSize {
		rc.logger.Warn("Result too large to cache",
			"size", result.Size,
			"max_size", MaxCacheSize,
		)
		return fmt.Errorf("result size %d exceeds max cache size %d", result.Size, MaxCacheSize)
	}
	
	// Establecer timestamps
	result.CreatedAt = time.Now()
	result.ExpiresAt = result.CreatedAt.Add(rc.ttl)
	result.HitCount = 0
	
	// Generar key
	params := make(map[string]any)
	if result.Metadata != nil {
		for k, v := range result.Metadata {
			params[k] = v
		}
	}
	key := rc.GenerateCacheKey(result.FileHash, result.Operation, params)
	
	// Serializar resultado
	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("marshal error: %w", err)
	}
	
	// Guardar en Redis con TTL
	if err := rc.redis.Set(ctx, key, data, rc.ttl).Err(); err != nil {
		return fmt.Errorf("redis set error: %w", err)
	}
	
	rc.logger.Info("Result cached",
		"key", key,
		"operation", result.Operation,
		"size", result.Size,
		"ttl", rc.ttl,
	)
	
	return nil
}

// Delete elimina un resultado del cache
func (rc *ResultCache) Delete(ctx context.Context, fileHash, operation string, params map[string]any) error {
	key := rc.GenerateCacheKey(fileHash, operation, params)
	
	if err := rc.redis.Del(ctx, key).Err(); err != nil {
		return fmt.Errorf("redis del error: %w", err)
	}
	
	rc.logger.Debug("Cache entry deleted", "key", key)
	return nil
}

// InvalidateByFileHash invalida todos los resultados de un archivo
func (rc *ResultCache) InvalidateByFileHash(ctx context.Context, fileHash string) error {
	pattern := fmt.Sprintf("%s%s:*", ResultKeyPrefix, fileHash)
	
	// Buscar todas las keys que coincidan
	iter := rc.redis.Scan(ctx, 0, pattern, 0).Iterator()
	deletedCount := 0
	
	for iter.Next(ctx) {
		if err := rc.redis.Del(ctx, iter.Val()).Err(); err != nil {
			rc.logger.Error("Failed to delete cache key", "key", iter.Val(), "error", err)
			continue
		}
		deletedCount++
	}
	
	if err := iter.Err(); err != nil {
		return fmt.Errorf("scan error: %w", err)
	}
	
	rc.logger.Info("Cache invalidated",
		"file_hash", fileHash,
		"deleted_count", deletedCount,
	)
	
	return nil
}

// InvalidateByOperation invalida todos los resultados de una operación
func (rc *ResultCache) InvalidateByOperation(ctx context.Context, operation string) error {
	pattern := fmt.Sprintf("%s*:%s:*", ResultKeyPrefix, operation)
	
	iter := rc.redis.Scan(ctx, 0, pattern, 0).Iterator()
	deletedCount := 0
	
	for iter.Next(ctx) {
		if err := rc.redis.Del(ctx, iter.Val()).Err(); err != nil {
			rc.logger.Error("Failed to delete cache key", "key", iter.Val(), "error", err)
			continue
		}
		deletedCount++
	}
	
	if err := iter.Err(); err != nil {
		return fmt.Errorf("scan error: %w", err)
	}
	
	rc.logger.Info("Cache invalidated",
		"operation", operation,
		"deleted_count", deletedCount,
	)
	
	return nil
}

// GetStats obtiene estadísticas del cache
func (rc *ResultCache) GetStats(ctx context.Context) (*CacheStats, error) {
	stats := &CacheStats{}
	
	// Obtener hits y misses
	hits, err := rc.redis.Get(ctx, StatsKeyPrefix+"hits").Int64()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get hits: %w", err)
	}
	stats.TotalHits = hits
	
	misses, err := rc.redis.Get(ctx, StatsKeyPrefix+"misses").Int64()
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get misses: %w", err)
	}
	stats.TotalMisses = misses
	
	// Calcular hit rate
	total := stats.TotalHits + stats.TotalMisses
	if total > 0 {
		stats.HitRate = float64(stats.TotalHits) / float64(total) * 100
	}
	
	// Contar keys en cache
	pattern := ResultKeyPrefix + "*"
	iter := rc.redis.Scan(ctx, 0, pattern, 0).Iterator()
	for iter.Next(ctx) {
		stats.TotalKeys++
	}
	
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("scan error: %w", err)
	}
	
	return stats, nil
}

// ResetStats resetea las estadísticas del cache
func (rc *ResultCache) ResetStats(ctx context.Context) error {
	if err := rc.redis.Del(ctx, StatsKeyPrefix+"hits", StatsKeyPrefix+"misses").Err(); err != nil {
		return fmt.Errorf("failed to reset stats: %w", err)
	}
	
	rc.logger.Info("Cache stats reset")
	return nil
}

// CleanupExpired limpia entradas expiradas (ejecutar en background)
func (rc *ResultCache) CleanupExpired(ctx context.Context) error {
	pattern := ResultKeyPrefix + "*"
	iter := rc.redis.Scan(ctx, 0, pattern, 100).Iterator()
	deletedCount := 0
	
	for iter.Next(ctx) {
		key := iter.Val()
		
		// Obtener resultado
		data, err := rc.redis.Get(ctx, key).Bytes()
		if err != nil {
			continue
		}
		
		var result CachedResult
		if err := json.Unmarshal(data, &result); err != nil {
			continue
		}
		
		// Verificar si está expirado
		if time.Now().After(result.ExpiresAt) {
			if err := rc.redis.Del(ctx, key).Err(); err != nil {
				rc.logger.Error("Failed to delete expired key", "key", key, "error", err)
				continue
			}
			deletedCount++
		}
	}
	
	if err := iter.Err(); err != nil {
		return fmt.Errorf("scan error: %w", err)
	}
	
	if deletedCount > 0 {
		rc.logger.Info("Expired cache entries cleaned",
			"deleted_count", deletedCount,
		)
	}
	
	return nil
}

// incrementHits incrementa el contador de cache hits
func (rc *ResultCache) incrementHits(ctx context.Context) {
	rc.redis.Incr(ctx, StatsKeyPrefix+"hits")
}

// incrementMisses incrementa el contador de cache misses
func (rc *ResultCache) incrementMisses(ctx context.Context) {
	rc.redis.Incr(ctx, StatsKeyPrefix+"misses")
}

// ComputeFileHash calcula el hash SHA256 de un archivo
func ComputeFileHash(filePath string) (string, error) {
	// Esta función debería estar en internal/utils pero la incluimos aquí como helper
	// En producción, mover a utils y reutilizar
	return "", fmt.Errorf("not implemented - use utils.ComputeFileHash")
}
