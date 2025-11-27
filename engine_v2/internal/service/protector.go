package service

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// ServiceProtector servicio de protección del sistema con límites invisibles
type ServiceProtector struct {
	config *config.Config
	logger *logger.Logger
	redis  *redis.Client
}

// NewServiceProtector crear nuevo protector de servicios
func NewServiceProtector(cfg *config.Config, log *logger.Logger, redisClient *redis.Client) *ServiceProtector {
	return &ServiceProtector{
		config: cfg,
		logger: log,
		redis:  redisClient,
	}
}

// ProtectorError error de protección del sistema
type ProtectorError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *ProtectorError) Error() string {
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// ValidateGlobalQueue valida que la cola global no esté saturada (INVISIBLE)
func (sp *ServiceProtector) ValidateGlobalQueue() error {
	if sp.redis == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Contar jobs en cola
	queueKey := "global:job_queue"
	queueSize, err := sp.redis.LLen(ctx, queueKey).Result()
	if err != nil && err != redis.Nil {
		sp.logger.Warn("Failed to check queue size", "error", err)
		return nil // No bloquear por error de Redis
	}

	// Límite invisible: máximo 50 jobs globales simultáneos
	const maxGlobalJobs = 50
	
	if queueSize >= maxGlobalJobs {
		sp.logger.Warn("Global queue saturated",
			"queue_size", queueSize,
			"max_allowed", maxGlobalJobs,
		)
		
		// Activar modo protector por 5 minutos
		sp.ActivateProtectorMode(5 * time.Minute)
		
		return &ProtectorError{
			Code:    "GLOBAL_QUEUE_SATURATED",
			Message: "Sistema temporalmente sobrecargado. Intenta de nuevo en unos minutos.",
		}
	}

	return nil
}

// ActivateProtectorMode activa el modo protector por tiempo especificado
func (sp *ServiceProtector) ActivateProtectorMode(duration time.Duration) {
	if sp.redis == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	protectorKey := "system:protector_mode"
	
	err := sp.redis.Set(ctx, protectorKey, time.Now().Unix(), duration).Err()
	if err != nil {
		sp.logger.Error("Failed to activate protector mode", "error", err)
		return
	}

	sp.logger.Warn("Protector mode activated",
		"duration_minutes", duration.Minutes(),
		"reason", "system_overload_protection",
	)

	// Notificar a todos los workers
	sp.redis.Publish(ctx, "system:alerts", "PROTECTOR_MODE_ACTIVATED")
}

// IsProtectorModeActive verifica si el modo protector está activo
func (sp *ServiceProtector) IsProtectorModeActive() bool {
	if sp.redis == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	protectorKey := "system:protector_mode"
	
	exists, err := sp.redis.Exists(ctx, protectorKey).Result()
	if err != nil {
		sp.logger.Warn("Failed to check protector mode", "error", err)
		return false
	}

	return exists > 0
}

// RestrictLargeFiles restringe archivos grandes temporalmente si el sistema está sobrecargado
func (sp *ServiceProtector) RestrictLargeFiles(maxSizeMB int) error {
	if sp.redis == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	restrictionKey := "system:large_files_restricted"
	
	err := sp.redis.Set(ctx, restrictionKey, maxSizeMB, 10*time.Minute).Err()
	if err != nil {
		sp.logger.Error("Failed to set file restriction", "error", err)
		return err
	}

	sp.logger.Warn("Large files restricted temporarily",
		"max_size_mb", maxSizeMB,
		"duration_minutes", 10,
	)

	return nil
}

// GetCurrentFileRestriction obtiene la restricción actual de archivos
func (sp *ServiceProtector) GetCurrentFileRestriction() (int, bool) {
	if sp.redis == nil {
		return 0, false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	restrictionKey := "system:large_files_restricted"
	
	maxSize, err := sp.redis.Get(ctx, restrictionKey).Int()
	if err != nil {
		if err != redis.Nil {
			sp.logger.Warn("Failed to get file restriction", "error", err)
		}
		return 0, false
	}

	return maxSize, true
}

// PauseProcessingForPlan pausa procesamiento para un plan específico
func (sp *ServiceProtector) PauseProcessingForPlan(plan string, duration time.Duration) {
	if sp.redis == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pauseKey := fmt.Sprintf("system:pause_plan:%s", plan)
	
	err := sp.redis.Set(ctx, pauseKey, time.Now().Unix(), duration).Err()
	if err != nil {
		sp.logger.Error("Failed to pause plan processing", "plan", plan, "error", err)
		return
	}

	sp.logger.Warn("Plan processing paused",
		"plan", plan,
		"duration_minutes", duration.Minutes(),
	)
}

// IsPlanPaused verifica si el procesamiento está pausado para un plan
func (sp *ServiceProtector) IsPlanPaused(plan string) bool {
	if sp.redis == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	pauseKey := fmt.Sprintf("system:pause_plan:%s", plan)
	
	exists, err := sp.redis.Exists(ctx, pauseKey).Result()
	if err != nil {
		sp.logger.Warn("Failed to check plan pause status", "plan", plan, "error", err)
		return false
	}

	return exists > 0
}

// AdjustPriorityBasedOnLoad ajusta prioridades basado en la carga del sistema
func (sp *ServiceProtector) AdjustPriorityBasedOnLoad() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Obtener métricas de sistema
	queueSize := sp.getQueueSize(ctx)
	activeJobs := sp.getActiveJobs(ctx)
	
	sp.logger.Info("System load check",
		"queue_size", queueSize,
		"active_jobs", activeJobs,
	)

	// Si la carga es alta, bajar prioridad FREE y subir PRO
	if queueSize > 20 || activeJobs > 15 {
		sp.logger.Warn("High system load detected, adjusting priorities")
		
		// Bajar prioridad de FREE por 5 minutos
		sp.PauseProcessingForPlan("free", 5*time.Minute)
		
		// Restringir archivos grandes temporalmente
		sp.RestrictLargeFiles(50) // 50MB máximo durante alta carga
		
		// Notificar modo de carga alta
		sp.redis.Publish(ctx, "system:alerts", "HIGH_LOAD_DETECTED")
	}

	// Si la carga es muy alta, activar modo protector
	if queueSize > 35 || activeJobs > 25 {
		sp.logger.Error("Critical system load detected, activating protector mode")
		sp.ActivateProtectorMode(10 * time.Minute)
	}
}

// MonitorAndProtect ejecuta monitoreo continuo y protección automática
func (sp *ServiceProtector) MonitorAndProtect() {
	ticker := time.NewTicker(30 * time.Second) // Cada 30 segundos
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			sp.AdjustPriorityBasedOnLoad()
			sp.cleanupExpiredJobs()
			sp.checkWorkerHealth()
		}
	}
}

// Helper methods

func (sp *ServiceProtector) getQueueSize(ctx context.Context) int64 {
	if sp.redis == nil {
		return 0
	}

	size, err := sp.redis.LLen(ctx, "global:job_queue").Result()
	if err != nil {
		return 0
	}
	return size
}

func (sp *ServiceProtector) getActiveJobs(ctx context.Context) int64 {
	if sp.redis == nil {
		return 0
	}

	active, err := sp.redis.SCard(ctx, "global:active_jobs").Result()
	if err != nil {
		return 0
	}
	return active
}

func (sp *ServiceProtector) cleanupExpiredJobs() {
	if sp.redis == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Limpiar jobs expirados (más de 2 horas)
	cutoff := time.Now().Add(-2 * time.Hour).Unix()
	
	// Limpiar de la cola de jobs activos
	pipe := sp.redis.Pipeline()
	pipe.ZRemRangeByScore(ctx, "global:jobs_by_time", "-inf", fmt.Sprintf("%d", cutoff))
	pipe.ZRemRangeByScore(ctx, "global:completed_jobs", "-inf", fmt.Sprintf("%d", cutoff))
	
	results, err := pipe.Exec(ctx)
	if err != nil {
		sp.logger.Warn("Failed to cleanup expired jobs", "error", err)
		return
	}

	if len(results) > 0 {
		if cmd, ok := results[0].(*redis.IntCmd); ok {
			cleaned, _ := cmd.Result()
			if cleaned > 0 {
				sp.logger.Info("Cleaned up expired jobs", "count", cleaned)
			}
		}
	}
}

func (sp *ServiceProtector) checkWorkerHealth() {
	if sp.redis == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Verificar workers que no han enviado heartbeat en 2 minutos
	cutoff := time.Now().Add(-2 * time.Minute).Unix()
	
	staleWorkers, err := sp.redis.ZRangeByScore(ctx, "workers:heartbeat", &redis.ZRangeBy{
		Min: "-inf",
		Max: fmt.Sprintf("%d", cutoff),
	}).Result()

	if err != nil {
		sp.logger.Warn("Failed to check worker health", "error", err)
		return
	}

	if len(staleWorkers) > 0 {
		sp.logger.Warn("Stale workers detected",
			"count", len(staleWorkers),
			"workers", staleWorkers,
		)

		// Remover workers obsoletos
		for _, worker := range staleWorkers {
			sp.redis.ZRem(ctx, "workers:heartbeat", worker)
		}

		// Si hay muchos workers colgados, activar protector
		if len(staleWorkers) > 3 {
			sp.logger.Error("Multiple worker failures detected, activating protector mode")
			sp.ActivateProtectorMode(5 * time.Minute)
		}
	}
}

// WorkerHeartbeat registra que un worker está vivo
func (sp *ServiceProtector) WorkerHeartbeat(workerID string) {
	if sp.redis == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sp.redis.ZAdd(ctx, "workers:heartbeat", &redis.Z{
		Score:  float64(time.Now().Unix()),
		Member: workerID,
	})
}

// GetSystemStatus obtiene el estado actual del sistema de protección
func (sp *ServiceProtector) GetSystemStatus() map[string]interface{} {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	status := map[string]interface{}{
		"protector_mode_active": sp.IsProtectorModeActive(),
		"queue_size":           sp.getQueueSize(ctx),
		"active_jobs":          sp.getActiveJobs(ctx),
	}

	if restriction, active := sp.GetCurrentFileRestriction(); active {
		status["file_restriction_mb"] = restriction
	}

	// Verificar pausas por plan
	for _, plan := range []string{"free", "premium", "pro", "corporate"} {
		if sp.IsPlanPaused(plan) {
			status[fmt.Sprintf("plan_%s_paused", plan)] = true
		}
	}

	return status
}