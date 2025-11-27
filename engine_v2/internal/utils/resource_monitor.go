package utils

import (
	"context"
	"fmt"
	"runtime"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// ResourceMonitor monitorea recursos del sistema (CPU, RAM) y aplica límites invisibles
type ResourceMonitor struct {
	logger *logger.Logger
	redis  *redis.Client
}

// NewResourceMonitor crear nuevo monitor de recursos
func NewResourceMonitor(log *logger.Logger, redisClient *redis.Client) *ResourceMonitor {
	return &ResourceMonitor{
		logger: log,
		redis:  redisClient,
	}
}

// SystemResources representa el estado actual de recursos del sistema
type SystemResources struct {
	CPUPercent    float64 `json:"cpu_percent"`
	RAMUsedMB     int64   `json:"ram_used_mb"`
	RAMTotalMB    int64   `json:"ram_total_mb"`
	RAMPercent    float64 `json:"ram_percent"`
	GoRoutines    int     `json:"go_routines"`
	HeapSizeMB    int64   `json:"heap_size_mb"`
	Timestamp     time.Time `json:"timestamp"`
}

// ResourceError error de recursos del sistema
type ResourceError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Current float64 `json:"current"`
	Limit   float64 `json:"limit"`
}

func (e *ResourceError) Error() string {
	return fmt.Sprintf("%s: %s (%.1f%% > %.1f%%)", e.Code, e.Message, e.Current, e.Limit)
}

// ValidateSystemResources valida que los recursos del sistema estén dentro de límites seguros
func (rm *ResourceMonitor) ValidateSystemResources() error {
	resources := rm.GetCurrentResources()

	// Límite invisible: 85% CPU máximo
	if resources.CPUPercent > 85.0 {
		rm.logger.Warn("High CPU usage detected",
			"cpu_percent", resources.CPUPercent,
			"limit", 85.0,
		)
		
		return &ResourceError{
			Code:    "HIGH_CPU_USAGE",
			Message: "Sistema temporalmente sobrecargado (CPU alto)",
			Current: resources.CPUPercent,
			Limit:   85.0,
		}
	}

	// Límite invisible: 80% RAM máximo
	if resources.RAMPercent > 80.0 {
		rm.logger.Warn("High RAM usage detected",
			"ram_percent", resources.RAMPercent,
			"ram_used_mb", resources.RAMUsedMB,
			"limit", 80.0,
		)
		
		return &ResourceError{
			Code:    "HIGH_RAM_USAGE",
			Message: "Sistema temporalmente sobrecargado (RAM alta)",
			Current: resources.RAMPercent,
			Limit:   80.0,
		}
	}

	// Verificar Goroutines excesivas (indicador de memory leaks)
	if resources.GoRoutines > 10000 {
		rm.logger.Warn("Excessive goroutines detected",
			"goroutines", resources.GoRoutines,
			"limit", 10000,
		)
		
		return &ResourceError{
			Code:    "EXCESSIVE_GOROUTINES",
			Message: "Sistema con demasiadas goroutines activas",
			Current: float64(resources.GoRoutines),
			Limit:   10000.0,
		}
	}

	return nil
}

// GetCurrentResources obtiene las métricas actuales de recursos del sistema
func (rm *ResourceMonitor) GetCurrentResources() *SystemResources {
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	resources := &SystemResources{
		CPUPercent:  rm.getCPUUsage(),
		RAMUsedMB:   int64(mem.Sys) / (1024 * 1024),
		HeapSizeMB:  int64(mem.HeapSys) / (1024 * 1024),
		GoRoutines:  runtime.NumGoroutine(),
		Timestamp:   time.Now(),
	}

	// Estimar RAM total (aproximación básica)
	resources.RAMTotalMB = rm.getEstimatedTotalRAM()
	if resources.RAMTotalMB > 0 {
		resources.RAMPercent = (float64(resources.RAMUsedMB) / float64(resources.RAMTotalMB)) * 100
	}

	return resources
}

// getCPUUsage obtiene el uso actual de CPU (aproximación simple)
func (rm *ResourceMonitor) getCPUUsage() float64 {
	// Método simple: medir tiempo de CPU entre dos puntos
	start := time.Now()
	
	// Hacer un trabajo mínimo de CPU
	for i := 0; i < 1000; i++ {
		_ = fmt.Sprintf("%d", i)
	}
	
	elapsed := time.Since(start)
	
	// Aproximación muy básica del uso de CPU
	// En producción se debería usar librerías especializadas como gopsutil
	cpuUsage := float64(elapsed.Nanoseconds()) / 1000000.0 // Convertir a ms
	
	// Normalizar a porcentaje (0-100)
	if cpuUsage > 100 {
		cpuUsage = 100
	}
	
	return cpuUsage
}

// getEstimatedTotalRAM estima la RAM total del sistema
func (rm *ResourceMonitor) getEstimatedTotalRAM() int64 {
	// Aproximación básica basada en límites de contenedor Docker
	// En producción se debería usar syscalls o gopsutil
	
	// Valores típicos por tipo de plan (de docker-compose)
	return 2048 // 2GB por defecto
}

// StartMonitoring inicia el monitoreo continuo de recursos
func (rm *ResourceMonitor) StartMonitoring() {
	ticker := time.NewTicker(10 * time.Second) // Cada 10 segundos
	defer ticker.Stop()

	rm.logger.Info("Resource monitoring started")

	for {
		select {
		case <-ticker.C:
			rm.collectAndStoreMetrics()
		}
	}
}

// collectAndStoreMetrics recolecta y almacena métricas en Redis
func (rm *ResourceMonitor) collectAndStoreMetrics() {
	resources := rm.GetCurrentResources()
	
	// Log métricas si hay niveles altos
	if resources.CPUPercent > 70 || resources.RAMPercent > 70 {
		rm.logger.Warn("High resource usage",
			"cpu_percent", resources.CPUPercent,
			"ram_percent", resources.RAMPercent,
			"goroutines", resources.GoRoutines,
		)
	}

	// Almacenar en Redis para dashboard y alertas
	if rm.redis != nil {
		rm.storeMetricsInRedis(resources)
	}

	// Tomar acciones automáticas si es necesario
	rm.handleHighResourceUsage(resources)
}

// storeMetricsInRedis almacena métricas en Redis
func (rm *ResourceMonitor) storeMetricsInRedis(resources *SystemResources) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pipe := rm.redis.Pipeline()
	
	// Almacenar métricas actuales
	metricsKey := "system:metrics:current"
	pipe.HMSet(ctx, metricsKey, map[string]interface{}{
		"cpu_percent":  resources.CPUPercent,
		"ram_percent":  resources.RAMPercent,
		"ram_used_mb":  resources.RAMUsedMB,
		"heap_size_mb": resources.HeapSizeMB,
		"goroutines":   resources.GoRoutines,
		"timestamp":    resources.Timestamp.Unix(),
	})
	pipe.Expire(ctx, metricsKey, 5*time.Minute)

	// Almacenar histórico de CPU y RAM (últimas 100 muestras)
	cpuHistoryKey := "system:metrics:cpu_history"
	ramHistoryKey := "system:metrics:ram_history"
	
	pipe.LPush(ctx, cpuHistoryKey, resources.CPUPercent)
	pipe.LTrim(ctx, cpuHistoryKey, 0, 99) // Mantener solo 100
	pipe.Expire(ctx, cpuHistoryKey, 1*time.Hour)
	
	pipe.LPush(ctx, ramHistoryKey, resources.RAMPercent)
	pipe.LTrim(ctx, ramHistoryKey, 0, 99) // Mantener solo 100
	pipe.Expire(ctx, ramHistoryKey, 1*time.Hour)

	if _, err := pipe.Exec(ctx); err != nil {
		rm.logger.Warn("Failed to store metrics in Redis", "error", err)
	}
}

// handleHighResourceUsage toma acciones automáticas cuando los recursos son altos
func (rm *ResourceMonitor) handleHighResourceUsage(resources *SystemResources) {
	// Si CPU > 90%, pausar procesamiento heavy por 30 segundos
	if resources.CPUPercent > 90 {
		rm.logger.Error("Critical CPU usage, pausing heavy operations")
		rm.pauseHeavyOperations(30 * time.Second)
	}

	// Si RAM > 90%, forzar garbage collection
	if resources.RAMPercent > 90 {
		rm.logger.Error("Critical RAM usage, forcing garbage collection")
		runtime.GC()
		runtime.GC() // Doble GC para ser más agresivo
	}

	// Si goroutines > 15000, algo está mal
	if resources.GoRoutines > 15000 {
		rm.logger.Error("Excessive goroutines, potential memory leak detected")
		
		// En un caso real, aquí se podría:
		// 1. Alertar al equipo de ops
		// 2. Reiniciar workers problemáticos
		// 3. Activar modo de emergencia
	}
}

// pauseHeavyOperations pausa operaciones pesadas temporalmente
func (rm *ResourceMonitor) pauseHeavyOperations(duration time.Duration) {
	if rm.redis == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	pauseKey := "system:pause_heavy_ops"
	
	err := rm.redis.Set(ctx, pauseKey, time.Now().Unix(), duration).Err()
	if err != nil {
		rm.logger.Error("Failed to pause heavy operations", "error", err)
		return
	}

	rm.logger.Warn("Heavy operations paused",
		"duration_seconds", duration.Seconds(),
		"reason", "high_resource_usage",
	)

	// Notificar a workers
	rm.redis.Publish(ctx, "system:alerts", "HEAVY_OPS_PAUSED")
}

// IsHeavyOperationsPaused verifica si las operaciones pesadas están pausadas
func (rm *ResourceMonitor) IsHeavyOperationsPaused() bool {
	if rm.redis == nil {
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	pauseKey := "system:pause_heavy_ops"
	
	exists, err := rm.redis.Exists(ctx, pauseKey).Result()
	if err != nil {
		return false
	}

	return exists > 0
}

// GetMetricsHistory obtiene el histórico de métricas
func (rm *ResourceMonitor) GetMetricsHistory() map[string]interface{} {
	if rm.redis == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	pipe := rm.redis.Pipeline()
	
	currentCmd := pipe.HGetAll(ctx, "system:metrics:current")
	cpuHistoryCmd := pipe.LRange(ctx, "system:metrics:cpu_history", 0, 99)
	ramHistoryCmd := pipe.LRange(ctx, "system:metrics:ram_history", 0, 99)
	
	_, err := pipe.Exec(ctx)
	if err != nil {
		rm.logger.Warn("Failed to get metrics history", "error", err)
		return nil
	}

	current, _ := currentCmd.Result()
	cpuHistory, _ := cpuHistoryCmd.Result()
	ramHistory, _ := ramHistoryCmd.Result()

	return map[string]interface{}{
		"current":     current,
		"cpu_history": cpuHistory,
		"ram_history": ramHistory,
		"heavy_ops_paused": rm.IsHeavyOperationsPaused(),
	}
}

// GetResourcesForPlan obtiene recursos específicos recomendados por plan
func (rm *ResourceMonitor) GetResourcesForPlan(plan string) map[string]interface{} {
	switch plan {
	case "free":
		return map[string]interface{}{
			"cpu_limit":    0.5,
			"ram_limit_mb": 300,
			"cpu_reserve":  0.1,
			"ram_reserve_mb": 64,
			"priority":     1,
		}
	case "premium":
		return map[string]interface{}{
			"cpu_limit":    1.0,
			"ram_limit_mb": 512,
			"cpu_reserve":  0.2,
			"ram_reserve_mb": 128,
			"priority":     5,
		}
	case "pro":
		return map[string]interface{}{
			"cpu_limit":    2.0,
			"ram_limit_mb": 1024,
			"cpu_reserve":  0.5,
			"ram_reserve_mb": 256,
			"priority":     10,
		}
	case "corporate":
		return map[string]interface{}{
			"cpu_limit":    4.0,
			"ram_limit_mb": 2048,
			"cpu_reserve":  1.0,
			"ram_reserve_mb": 512,
			"priority":     10,
		}
	default:
		return rm.GetResourcesForPlan("free")
	}
}

// CheckContainerLimits verifica si los contenedores están dentro de sus límites
func (rm *ResourceMonitor) CheckContainerLimits() map[string]interface{} {
	resources := rm.GetCurrentResources()
	
	status := map[string]interface{}{
		"current_resources": resources,
		"within_limits":     true,
		"warnings":          []string{},
	}

	warnings := []string{}
	withinLimits := true

	// Verificar límites de CPU por contenedor
	if resources.CPUPercent > 85 {
		warnings = append(warnings, "CPU usage above safe limit (85%)")
		withinLimits = false
	}

	// Verificar límites de RAM por contenedor
	if resources.RAMPercent > 80 {
		warnings = append(warnings, "RAM usage above safe limit (80%)")
		withinLimits = false
	}

	// Verificar heap size excesivo
	if resources.HeapSizeMB > 1024 {
		warnings = append(warnings, "Heap size above recommended limit (1GB)")
		withinLimits = false
	}

	status["warnings"] = warnings
	status["within_limits"] = withinLimits

	return status
}