package maintenance

import (
	"strings"
	"time"
)

// CleanupRedis limpieza periódica de Redis
func (s *Service) CleanupRedis() error {
	if s.redis == nil {
		return nil
	}

	s.logger.Info("🧹 Starting Redis cleanup...")

	// 1. Limpiar claves expiradas manualmente
	if err := s.cleanExpiredKeys(); err != nil {
		s.logger.Error("Error cleaning expired keys", "error", err)
	}

	// 2. Ajustar TTLs faltantes
	if err := s.fixMissingTTLs(); err != nil {
		s.logger.Error("Error fixing missing TTLs", "error", err)
	}

	// 3. Limpiar claves huérfanas
	if err := s.cleanOrphanedKeys(); err != nil {
		s.logger.Error("Error cleaning orphaned keys", "error", err)
	}

	s.logger.Info("✅ Redis cleanup completed")
	return nil
}

// DeepCleanupRedis limpieza profunda diaria de Redis
func (s *Service) DeepCleanupRedis() error {
	if s.redis == nil {
		return nil
	}

	s.logger.Info("🔍 Starting deep Redis cleanup...")

	// 1. Resetear contadores diarios antiguos
	if err := s.resetOldDailyCounters(); err != nil {
		s.logger.Error("Error resetting daily counters", "error", err)
	}

	// 2. Resetear contadores mensuales si es nuevo mes
	if err := s.resetMonthlyCountersIfNeeded(); err != nil {
		s.logger.Error("Error resetting monthly counters", "error", err)
	}

	// 3. Limpiar colas de trabajos antiguos
	if err := s.cleanOldJobQueues(); err != nil {
		s.logger.Error("Error cleaning old job queues", "error", err)
	}

	// 4. Optimizar memoria Redis
	if err := s.optimizeRedisMemory(); err != nil {
		s.logger.Error("Error optimizing Redis memory", "error", err)
	}

	s.logger.Info("✅ Deep Redis cleanup completed")
	return nil
}

// cleanExpiredKeys limpia claves expiradas manualmente
func (s *Service) cleanExpiredKeys() error {
	patterns := []string{
		"temp:*",
		"job:*",
		"queue:*",
		"lock:*",
		"session:*",
	}

	for _, pattern := range patterns {
		keys, err := s.redis.Keys(s.ctx, pattern).Result()
		if err != nil {
			continue
		}

		for _, key := range keys {
			// Verificar si la clave ha expirado
			ttl, err := s.redis.TTL(s.ctx, key).Result()
			if err != nil {
				continue
			}

			// Si TTL es -1 (sin expiración) o -2 (ya expirada)
			if ttl == -2 {
				s.redis.Del(s.ctx, key)
				s.logger.Debug("Deleted expired key", "key", key)
			}
		}
	}

	return nil
}

// fixMissingTTLs ajusta TTLs faltantes en claves temporales
func (s *Service) fixMissingTTLs() error {
	today := time.Now().Format("2006-01-02")
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	thisMonth := time.Now().Format("2006-01")
	lastMonth := time.Now().AddDate(0, -1, 0).Format("2006-01")

	// Patrones de claves que deben tener TTL
	tempPatterns := map[string]time.Duration{
		"user:*:daily_count:" + today:     25 * time.Hour,
		"tool:*:daily_count:" + today:     25 * time.Hour,
		"user:*:monthly_count:" + thisMonth: 32 * 24 * time.Hour,
		"tool:*:monthly_count:" + thisMonth: 32 * 24 * time.Hour,
		"temp:*":                         12 * time.Hour,
		"job:*":                          6 * time.Hour,
		"queue:*":                        1 * time.Hour,
		"lock:*":                         30 * time.Minute,
	}

	for pattern, ttl := range tempPatterns {
		keys, err := s.redis.Keys(s.ctx, pattern).Result()
		if err != nil {
			continue
		}

		for _, key := range keys {
			// Verificar si tiene TTL
			currentTTL, err := s.redis.TTL(s.ctx, key).Result()
			if err != nil {
				continue
			}

			// Si no tiene TTL (-1), asignar uno
			if currentTTL == -1 {
				s.redis.Expire(s.ctx, key, ttl)
				s.logger.Debug("Fixed missing TTL", "key", key, "ttl", ttl)
			}
		}
	}

	// Limpiar contadores de días anteriores
	oldDailyKeys, _ := s.redis.Keys(s.ctx, "*:daily_count:"+yesterday).Result()
	if len(oldDailyKeys) > 0 {
		s.redis.Del(s.ctx, oldDailyKeys...)
		s.logger.Info("Deleted old daily counters", "count", len(oldDailyKeys))
	}

	// Limpiar contadores de meses anteriores
	oldMonthlyKeys, _ := s.redis.Keys(s.ctx, "*:monthly_count:"+lastMonth).Result()
	if len(oldMonthlyKeys) > 0 {
		s.redis.Del(s.ctx, oldMonthlyKeys...)
		s.logger.Info("Deleted old monthly counters", "count", len(oldMonthlyKeys))
	}

	return nil
}

// cleanOrphanedKeys limpia claves huérfanas
func (s *Service) cleanOrphanedKeys() error {
	// Buscar claves sin patron específico que puedan ser huérfanas
	orphanPatterns := []string{
		"analytics:*:*:*:*", // Claves con demasiados niveles
		"user:*:*:*:*",     // Claves de usuario malformadas
		"tool:*:*:*:*",     // Claves de herramienta malformadas
	}

	for _, pattern := range orphanPatterns {
		keys, err := s.redis.Keys(s.ctx, pattern).Result()
		if err != nil {
			continue
		}

		// Verificar si son realmente huérfanas
		for _, key := range keys {
			// Si la clave tiene más de 7 días de antigüedad
			typeResult, err := s.redis.Type(s.ctx, key).Result()
			if err != nil {
				continue
			}

			// Si es un tipo inesperado o está corrupta
			if typeResult == "none" {
				s.redis.Del(s.ctx, key)
				s.logger.Debug("Deleted orphaned key", "key", key)
			}
		}
	}

	return nil
}

// resetOldDailyCounters resetea contadores diarios antiguos
func (s *Service) resetOldDailyCounters() error {
	yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	twoDaysAgo := time.Now().AddDate(0, 0, -2).Format("2006-01-02")

	oldPatterns := []string{
		"user:*:daily_count:" + yesterday,
		"tool:*:daily_count:" + yesterday,
		"user:*:daily_count:" + twoDaysAgo,
		"tool:*:daily_count:" + twoDaysAgo,
		"plan:*:daily_count:" + yesterday,
		"plan:*:daily_count:" + twoDaysAgo,
	}

	totalDeleted := 0
	for _, pattern := range oldPatterns {
		keys, err := s.redis.Keys(s.ctx, pattern).Result()
		if err != nil {
			continue
		}

		if len(keys) > 0 {
			s.redis.Del(s.ctx, keys...)
			totalDeleted += len(keys)
		}
	}

	if totalDeleted > 0 {
		s.logger.Info("Reset old daily counters", "deleted_keys", totalDeleted)
	}

	return nil
}

// resetMonthlyCountersIfNeeded resetea contadores mensuales si es necesario
func (s *Service) resetMonthlyCountersIfNeeded() error {
	now := time.Now()
	
	// Si es el primer día del mes
	if now.Day() == 1 {
		lastMonth := now.AddDate(0, -1, 0).Format("2006-01")
		twoMonthsAgo := now.AddDate(0, -2, 0).Format("2006-01")

		oldPatterns := []string{
			"user:*:monthly_count:" + lastMonth,
			"tool:*:monthly_count:" + lastMonth,
			"plan:*:monthly_count:" + lastMonth,
			"user:*:monthly_count:" + twoMonthsAgo,
			"tool:*:monthly_count:" + twoMonthsAgo,
			"plan:*:monthly_count:" + twoMonthsAgo,
		}

		totalDeleted := 0
		for _, pattern := range oldPatterns {
			keys, err := s.redis.Keys(s.ctx, pattern).Result()
			if err != nil {
				continue
			}

			if len(keys) > 0 {
				s.redis.Del(s.ctx, keys...)
				totalDeleted += len(keys)
			}
		}

		if totalDeleted > 0 {
			s.logger.Info("Reset old monthly counters", "deleted_keys", totalDeleted)
		}
	}

	return nil
}

// cleanOldJobQueues limpia colas de trabajos antiguas
func (s *Service) cleanOldJobQueues() error {
	jobPatterns := []string{
		"queue:pdf:*",
		"queue:ocr:*",
		"queue:office:*",
		"job:*",
		"worker:*:status",
	}

	totalCleaned := 0
	for _, pattern := range jobPatterns {
		keys, err := s.redis.Keys(s.ctx, pattern).Result()
		if err != nil {
			continue
		}

		// Verificar cada clave individualmente
		for _, key := range keys {
			// Si la clave no tiene TTL, asignar uno corto
			ttl, err := s.redis.TTL(s.ctx, key).Result()
			if err != nil {
				continue
			}

			if ttl == -1 {
				// Asignar TTL de 6 horas para trabajos
				s.redis.Expire(s.ctx, key, 6*time.Hour)
				totalCleaned++
			}
		}
	}

	if totalCleaned > 0 {
		s.logger.Info("Cleaned old job queues", "keys_fixed", totalCleaned)
	}

	return nil
}

// optimizeRedisMemory optimiza el uso de memoria de Redis
func (s *Service) optimizeRedisMemory() error {
	// Ejecutar MEMORY PURGE para liberar memoria no utilizada
	_, err := s.redis.Do(s.ctx, "MEMORY", "PURGE").Result()
	if err != nil {
		s.logger.Debug("Could not purge Redis memory", "error", err)
	}

	// Obtener estadísticas de memoria
	memInfo, err := s.redis.Info(s.ctx, "memory").Result()
	if err == nil {
		s.logger.Debug("Redis memory info", "info", memInfo)
	}

	return nil
}

// getRedisInfo obtiene información de Redis
func (s *Service) getRedisInfo() (*RedisInfo, error) {
	if s.redis == nil {
		return &RedisInfo{}, nil
	}

	// Obtener número total de claves
	dbSize, err := s.redis.DBSize(s.ctx).Result()
	if err != nil {
		return nil, err
	}

	// Obtener información de memoria
	memInfo, err := s.redis.Info(s.ctx, "memory").Result()
	var memoryUsage string
	if err != nil {
		memoryUsage = "unknown"
	} else {
		// Parsear used_memory_human de la respuesta
		lines := strings.Split(memInfo, "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "used_memory_human:") {
				memoryUsage = strings.TrimPrefix(line, "used_memory_human:")
				memoryUsage = strings.TrimSpace(memoryUsage)
				break
			}
		}
		if memoryUsage == "" {
			memoryUsage = "unknown"
		}
	}

	return &RedisInfo{
		Keys:   dbSize,
		Memory: memoryUsage,
	}, nil
}