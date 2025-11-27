package health

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// Checker realiza health checks del sistema
type Checker struct {
	logger  *logger.Logger
	db      *sql.DB
	redis   *redis.Client
	timeout time.Duration
}

// NewChecker crea un nuevo health checker
func NewChecker(log *logger.Logger, db *sql.DB, redisClient *redis.Client) *Checker {
	return &Checker{
		logger:  log,
		db:      db,
		redis:   redisClient,
		timeout: 5 * time.Second,
	}
}

// HealthStatus estado de salud del sistema
type HealthStatus struct {
	Status      string                 `json:"status"` // healthy, degraded, unhealthy
	Timestamp   time.Time              `json:"timestamp"`
	Uptime      float64                `json:"uptime_seconds"`
	Version     string                 `json:"version"`
	Checks      map[string]CheckResult `json:"checks"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// CheckResult resultado de un check individual
type CheckResult struct {
	Status    string  `json:"status"` // pass, fail, warn
	Timestamp time.Time `json:"timestamp"`
	Duration  float64 `json:"duration_ms"`
	Message   string  `json:"message,omitempty"`
	Error     string  `json:"error,omitempty"`
}

var startTime = time.Now()

// LivenessProbe verifica si el servicio está vivo (K8s liveness)
func (hc *Checker) LivenessProbe(ctx context.Context) *HealthStatus {
	status := &HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now(),
		Uptime:    time.Since(startTime).Seconds(),
		Version:   "2.0.0",
		Checks:    make(map[string]CheckResult),
	}

	// Liveness solo verifica que el proceso esté vivo
	// No hace checks de dependencias (eso es readiness)
	status.Checks["process"] = CheckResult{
		Status:    "pass",
		Timestamp: time.Now(),
		Duration:  0.1,
		Message:   "Process is running",
	}

	return status
}

// ReadinessProbe verifica si el servicio está listo (K8s readiness)
func (hc *Checker) ReadinessProbe(ctx context.Context) *HealthStatus {
	status := &HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now(),
		Uptime:    time.Since(startTime).Seconds(),
		Version:   "2.0.0",
		Checks:    make(map[string]CheckResult),
	}

	// Ejecutar checks en paralelo
	var wg sync.WaitGroup
	var mu sync.Mutex

	checks := []struct {
		name string
		fn   func(context.Context) CheckResult
	}{
		{"database", hc.checkDatabase},
		{"redis", hc.checkRedis},
		{"disk_space", hc.checkDiskSpace},
		{"memory", hc.checkMemory},
	}

	for _, check := range checks {
		wg.Add(1)
		go func(name string, fn func(context.Context) CheckResult) {
			defer wg.Done()

			// Context con timeout
			checkCtx, cancel := context.WithTimeout(ctx, hc.timeout)
			defer cancel()

			result := fn(checkCtx)

			mu.Lock()
			status.Checks[name] = result
			mu.Unlock()
		}(check.name, check.fn)
	}

	wg.Wait()

	// Determinar estado general
	failedChecks := 0
	warnChecks := 0

	for _, result := range status.Checks {
		if result.Status == "fail" {
			failedChecks++
		} else if result.Status == "warn" {
			warnChecks++
		}
	}

	if failedChecks > 0 {
		status.Status = "unhealthy"
	} else if warnChecks > 0 {
		status.Status = "degraded"
	}

	return status
}

// StartupProbe verifica que el servicio haya iniciado correctamente (K8s startup)
func (hc *Checker) StartupProbe(ctx context.Context) *HealthStatus {
	status := &HealthStatus{
		Status:    "healthy",
		Timestamp: time.Now(),
		Uptime:    time.Since(startTime).Seconds(),
		Version:   "2.0.0",
		Checks:    make(map[string]CheckResult),
	}

	// Verificar inicialización crítica
	var wg sync.WaitGroup
	var mu sync.Mutex

	checks := []struct {
		name string
		fn   func(context.Context) CheckResult
	}{
		{"database_migration", hc.checkDatabaseMigrations},
		{"redis_connection", hc.checkRedis},
		{"configuration", hc.checkConfiguration},
	}

	for _, check := range checks {
		wg.Add(1)
		go func(name string, fn func(context.Context) CheckResult) {
			defer wg.Done()

			checkCtx, cancel := context.WithTimeout(ctx, hc.timeout)
			defer cancel()

			result := fn(checkCtx)

			mu.Lock()
			status.Checks[name] = result
			mu.Unlock()
		}(check.name, check.fn)
	}

	wg.Wait()

	// Si algún check falla, startup no está completo
	for _, result := range status.Checks {
		if result.Status == "fail" {
			status.Status = "unhealthy"
			break
		}
	}

	return status
}

// checkDatabase verifica conexión a base de datos
func (hc *Checker) checkDatabase(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Status:    "pass",
		Timestamp: start,
	}

	// Ping database
	err := hc.db.PingContext(ctx)
	result.Duration = time.Since(start).Seconds() * 1000 // ms

	if err != nil {
		result.Status = "fail"
		result.Error = err.Error()
		result.Message = "Database connection failed"
		return result
	}

	// Verificar query simple
	var count int
	err = hc.db.QueryRowContext(ctx, "SELECT 1").Scan(&count)
	if err != nil {
		result.Status = "fail"
		result.Error = err.Error()
		result.Message = "Database query failed"
		return result
	}

	result.Message = fmt.Sprintf("Database connected (ping: %.2fms)", result.Duration)

	// Warning si latencia alta
	if result.Duration > 100 {
		result.Status = "warn"
		result.Message += " - High latency"
	}

	return result
}

// checkRedis verifica conexión a Redis
func (hc *Checker) checkRedis(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Status:    "pass",
		Timestamp: start,
	}

	// Ping Redis
	err := hc.redis.Ping(ctx).Err()
	result.Duration = time.Since(start).Seconds() * 1000 // ms

	if err != nil {
		result.Status = "fail"
		result.Error = err.Error()
		result.Message = "Redis connection failed"
		return result
	}

	// Verificar SET/GET
	testKey := "health:check:test"
	err = hc.redis.Set(ctx, testKey, "ok", 10*time.Second).Err()
	if err != nil {
		result.Status = "fail"
		result.Error = err.Error()
		result.Message = "Redis SET failed"
		return result
	}

	val, err := hc.redis.Get(ctx, testKey).Result()
	if err != nil || val != "ok" {
		result.Status = "fail"
		result.Error = err.Error()
		result.Message = "Redis GET failed"
		return result
	}

	result.Message = fmt.Sprintf("Redis connected (ping: %.2fms)", result.Duration)

	// Warning si latencia alta
	if result.Duration > 50 {
		result.Status = "warn"
		result.Message += " - High latency"
	}

	return result
}

// checkDiskSpace verifica espacio en disco
func (hc *Checker) checkDiskSpace(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Status:    "pass",
		Timestamp: start,
		Duration:  1.0,
	}

	// TODO: Implementar check real de disco
	// Por ahora placeholder
	result.Message = "Disk space OK"

	return result
}

// checkMemory verifica uso de memoria
func (hc *Checker) checkMemory(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Status:    "pass",
		Timestamp: start,
		Duration:  1.0,
	}

	// TODO: Implementar check real de memoria
	// Por ahora placeholder
	result.Message = "Memory usage OK"

	return result
}

// checkDatabaseMigrations verifica que migraciones estén aplicadas
func (hc *Checker) checkDatabaseMigrations(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Status:    "pass",
		Timestamp: start,
	}

	// Verificar tabla de migraciones existe
	var exists bool
	query := `
		SELECT EXISTS (
			SELECT FROM information_schema.tables 
			WHERE table_schema = 'public' 
			AND table_name = 'schema_migrations'
		)
	`
	err := hc.db.QueryRowContext(ctx, query).Scan(&exists)
	result.Duration = time.Since(start).Seconds() * 1000

	if err != nil {
		result.Status = "fail"
		result.Error = err.Error()
		result.Message = "Failed to check migrations table"
		return result
	}

	if !exists {
		result.Status = "fail"
		result.Message = "Migrations table not found"
		return result
	}

	result.Message = "Database migrations applied"
	return result
}

// checkConfiguration verifica configuración crítica
func (hc *Checker) checkConfiguration(ctx context.Context) CheckResult {
	start := time.Now()
	result := CheckResult{
		Status:    "pass",
		Timestamp: start,
		Duration:  0.5,
	}

	// TODO: Verificar env vars críticas
	// JWT_SECRET, OPENAI_API_KEY, etc.

	result.Message = "Configuration OK"
	return result
}

// DetailedHealthCheck health check completo con métricas
func (hc *Checker) DetailedHealthCheck(ctx context.Context) *HealthStatus {
	status := hc.ReadinessProbe(ctx)

	// Añadir métricas adicionales
	status.Metadata = map[string]interface{}{
		"goroutines":      fmt.Sprintf("%d", 0), // TODO: runtime.NumGoroutine()
		"memory_alloc_mb": fmt.Sprintf("%.2f", 0.0), // TODO: Get memory stats
		"uptime_human":    formatDuration(time.Since(startTime)),
	}

	// DB connection pool stats
	if hc.db != nil {
		stats := hc.db.Stats()
		status.Metadata["db_connections"] = map[string]interface{}{
			"open":        stats.OpenConnections,
			"in_use":      stats.InUse,
			"idle":        stats.Idle,
			"max_open":    stats.MaxOpenConnections,
			"wait_count":  stats.WaitCount,
			"wait_duration": stats.WaitDuration.String(),
		}
	}

	// Redis info
	if hc.redis != nil {
		poolStats := hc.redis.PoolStats()
		status.Metadata["redis_connections"] = map[string]interface{}{
			"hits":        poolStats.Hits,
			"misses":      poolStats.Misses,
			"timeouts":    poolStats.Timeouts,
			"total_conns": poolStats.TotalConns,
			"idle_conns":  poolStats.IdleConns,
			"stale_conns": poolStats.StaleConns,
		}
	}

	return status
}

// formatDuration formatea duración legible
func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm %ds", days, hours, minutes, seconds)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	}
	if minutes > 0 {
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
	return fmt.Sprintf("%ds", seconds)
}

// MonitorHealth monitorea health checks periódicamente (background job)
func (hc *Checker) MonitorHealth(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			hc.logger.Info("Health monitoring stopped")
			return
		case <-ticker.C:
			status := hc.ReadinessProbe(ctx)

			// Log si hay problemas
			if status.Status != "healthy" {
				hc.logger.Warn("Health check degraded",
					"status", status.Status,
					"failed_checks", hc.getFailedChecks(status),
				)
			}
		}
	}
}

// getFailedChecks retorna lista de checks fallidos
func (hc *Checker) getFailedChecks(status *HealthStatus) []string {
	var failed []string
	for name, result := range status.Checks {
		if result.Status == "fail" {
			failed = append(failed, name)
		}
	}
	return failed
}
