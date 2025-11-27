package handlers

import (
	"github.com/gofiber/fiber/v2"
	"github.com/tucentropdf/engine-v2/internal/maintenance"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// MaintenanceHandlers maneja las rutas de mantenimiento
type MaintenanceHandlers struct {
	maintenanceService *maintenance.Service
	logger            *logger.Logger
}

// NewMaintenanceHandlers crea nuevo handler de mantenimiento
func NewMaintenanceHandlers(maintenanceService *maintenance.Service, log *logger.Logger) *MaintenanceHandlers {
	return &MaintenanceHandlers{
		maintenanceService: maintenanceService,
		logger:            log,
	}
}

// GetSystemStatus obtiene el estado actual del sistema
func (h *MaintenanceHandlers) GetSystemStatus(c *fiber.Ctx) error {
	status, err := h.maintenanceService.GetSystemStatus()
	if err != nil {
		h.logger.Error("Error getting system status", "error", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "Error getting system status", "details": err.Error()})
	}

	return c.JSON(fiber.Map{"success": true, "message": "System status retrieved successfully", "data": status})
}

// TriggerMaintenance ejecuta mantenimiento manual
func (h *MaintenanceHandlers) TriggerMaintenance(c *fiber.Ctx) error {
	maintenanceType := c.Query("type", "all")

	h.logger.Info("Manual maintenance triggered", "type", maintenanceType, "user_id", c.Locals("user_id"))

	switch maintenanceType {
	case "disk":
		if err := h.maintenanceService.CheckDiskSpace(); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "Error checking disk space", "details": err.Error()})
		}
	case "redis":
		if err := h.maintenanceService.CleanupRedis(); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "Error cleaning Redis", "details": err.Error()})
		}
	case "logs":
		if err := h.maintenanceService.RotateLogs(); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "Error rotating logs", "details": err.Error()})
		}
	case "data":
		if err := h.maintenanceService.SummarizeOldData(); err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"success": false, "error": "Error summarizing data", "details": err.Error()})
		}
	case "all":
		// Ejecutar todas las tareas de mantenimiento
		go func() {
			h.maintenanceService.CheckDiskSpace()
			h.maintenanceService.CleanupRedis()
			h.maintenanceService.RotateLogs()
			h.maintenanceService.CheckTempFolder()
		}()
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"success": false, "error": "Invalid maintenance type", "details": "Valid types: disk, redis, logs, data, all"})
	}

	return c.JSON(fiber.Map{"success": true, "message": "Maintenance task triggered successfully", "data": map[string]string{
		"type": maintenanceType,
		"status": "started",
	}})
}

// GetMaintenanceConfig obtiene la configuración de mantenimiento
func (h *MaintenanceHandlers) GetMaintenanceConfig(c *fiber.Ctx) error {
	config := map[string]interface{}{
		"disk_threshold_warning":  80.0,
		"disk_threshold_critical": 90.0,
		"max_temp_file_age_hours": 72,
		"max_log_age_days":       7,
		"data_retention_days":    90,
		"schedules": map[string]string{
			"periodic_check": "every 10 minutes",
			"daily_tasks":    "daily at 2:00 AM",
			"monthly_tasks":  "1st day of month at 3:00 AM",
		},
	}

	return c.JSON(fiber.Map{"success": true, "message": "Maintenance configuration retrieved", "data": config})
}