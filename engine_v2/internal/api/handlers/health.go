package handlers

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/tucentropdf/engine-v2/internal/monitor"
)

// HealthHandlers maneja las rutas de health check
type HealthHandlers struct {
	monitorService *monitor.Service
	startTime      time.Time
}

// NewHealthHandlers crea nuevo handler de health
func NewHealthHandlers(monitorService *monitor.Service) *HealthHandlers {
	return &HealthHandlers{
		monitorService: monitorService,
		startTime:      time.Now(),
	}
}

// GetHealthCheck endpoint completo de health check para Nginx
func (h *HealthHandlers) GetHealthCheck(c *fiber.Ctx) error {
	// Obtener estado del sistema
	status := h.monitorService.GetSystemStatus()

	// Formatear respuesta
	healthResponse := map[string]interface{}{
		"status":         status.Status,
		"uptime":         status.Uptime.String(),
		"workers":        status.Workers,
		"redis":          status.Redis,
		"resources":      status.Resources,
		"queue":          status.Queue,
		"protector_mode": status.ProtectorMode,
		"last_check":     status.LastCheck,
		"timestamp":      time.Now(),
	}

	// Determinar código HTTP basado en el estado
	var httpStatus int
	switch status.Status {
	case "ok":
		httpStatus = fiber.StatusOK
	case "degraded":
		httpStatus = fiber.StatusPartialContent // 206
	case "critical":
		httpStatus = fiber.StatusServiceUnavailable // 503
	default:
		httpStatus = fiber.StatusInternalServerError // 500
	}

	return c.Status(httpStatus).JSON(healthResponse)
}

// GetBasicHealth endpoint básico de health (más rápido)
func (h *HealthHandlers) GetBasicHealth(c *fiber.Ctx) error {
	// Health check simplificado para load balancers rápidos
	isProtected := h.monitorService.IsInProtectionMode()

	response := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now(),
		"uptime":    time.Since(h.startTime).String(),
	}

	if isProtected {
		response["status"] = "degraded"
		return c.Status(fiber.StatusPartialContent).JSON(response)
	}

	return c.JSON(response)
}

// GetMonitoringStatus endpoint detallado de estado de monitoreo
func (h *HealthHandlers) GetMonitoringStatus(c *fiber.Ctx) error {
	status := h.monitorService.GetSystemStatus()
	protectionStatus := h.monitorService.GetProtectionStatus()

	detailedStatus := map[string]interface{}{
		"system":           status,
		"protection":       protectionStatus,
		"monitor_uptime":   time.Since(h.startTime).String(),
		"last_updated":     time.Now(),
	}

	return c.JSON(fiber.Map{"success": true, "message": "Monitoring status retrieved", "data": detailedStatus})
}

// GetSystemIncidents obtiene incidentes recientes del sistema
func (h *HealthHandlers) GetSystemIncidents(c *fiber.Ctx) error {
	// Parámetro para limitar resultados
	limit := c.QueryInt("limit", 50)
	if limit > 200 {
		limit = 200
	}

	// Parámetro para filtrar por severidad
	severity := c.Query("severity", "")

	// Parámetro para filtrar por tipo
	incidentType := c.Query("type", "")

	// Obtener incidentes (implementar en monitor service)
	incidents := h.getRecentIncidents(limit, severity, incidentType)

	return c.JSON(fiber.Map{
		"success": true,
		"message": "System incidents retrieved",
		"data": map[string]interface{}{
			"incidents": incidents,
			"count":     len(incidents),
			"filters": map[string]interface{}{
				"limit":    limit,
				"severity": severity,
				"type":     incidentType,
			},
		},
	})
}

// getRecentIncidents obtiene incidentes recientes (mock por ahora)
func (h *HealthHandlers) getRecentIncidents(limit int, severity, incidentType string) []map[string]interface{} {
	// Placeholder - implementar consulta real a la base de datos
	return []map[string]interface{}{
		{
			"id":        1,
			"type":      "WORKER_RECOVERY",
			"severity":  "info",
			"message":   "Worker ocr recovered",
			"timestamp": time.Now().Add(-5 * time.Minute),
			"resolved":  true,
		},
		{
			"id":        2,
			"type":      "CPU_HIGH",
			"severity":  "warning",
			"message":   "CPU usage reached 82.5%",
			"timestamp": time.Now().Add(-10 * time.Minute),
			"resolved":  true,
		},
	}
}

// GetWorkerHealth endpoint específico para estado de workers
func (h *HealthHandlers) GetWorkerHealth(c *fiber.Ctx) error {
	status := h.monitorService.GetSystemStatus()

	// Formatear información de workers
	workersInfo := make(map[string]interface{})
	for name, worker := range status.Workers {
		workersInfo[name] = map[string]interface{}{
			"status":        worker.Status,
			"last_seen":     worker.LastSeen,
			"latency_ms":    worker.Latency,
			"memory_usage":  worker.MemoryUsage,
			"restart_count": worker.RestartCount,
			"health_url":    worker.HealthURL,
		}
	}

	// Estado general de workers
	allHealthy := true
	for _, worker := range status.Workers {
		if worker.Status != "ok" {
			allHealthy = false
			break
		}
	}

	response := map[string]interface{}{
		"overall_status": "ok",
		"all_healthy":    allHealthy,
		"workers":        workersInfo,
		"total_workers":  len(status.Workers),
		"last_check":     status.LastCheck,
	}

	if !allHealthy {
		response["overall_status"] = "degraded"
	}

	return c.JSON(fiber.Map{"success": true, "message": "Worker health status retrieved", "data": response})
}