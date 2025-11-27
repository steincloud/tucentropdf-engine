package backup

import (
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
)

// Handler maneja las rutas HTTP para el sistema de backup
type Handler struct {
	service *Service
}

// NewHandler crea nueva instancia del handler de backup
func NewHandler(service *Service) *Handler {
	return &Handler{
		service: service,
	}
}

// RegisterRoutes registra las rutas del sistema de backup
func (h *Handler) RegisterRoutes(api fiber.Router) {
	backup := api.Group("/backup")

	// Estado del sistema
	backup.Get("/status", h.GetStatus)
	backup.Get("/health", h.GetHealth)
	
	// Operaciones de backup
	backup.Post("/run/full", h.RunFullBackup)
	backup.Post("/run/incremental", h.RunIncrementalBackup)
	backup.Post("/run/redis", h.RunRedisBackup)
	backup.Post("/run/config", h.RunConfigBackup)
	backup.Post("/run/analytics", h.RunAnalyticsBackup)
	
	// Restauración
	backup.Post("/restore/:type/:filename", h.RestoreBackup)
	backup.Get("/list", h.ListBackups)
	backup.Post("/verify/:type/:filename", h.VerifyBackup)
	
	// Gestión de retención
	backup.Post("/cleanup", h.RunCleanup)
	backup.Get("/retention", h.GetRetentionReport)
	
	// Remoto
	backup.Post("/sync", h.SyncToRemote)
	backup.Get("/remote/list", h.ListRemoteBackups)
	backup.Get("/remote/quota", h.GetRemoteQuota)
}

// GetStatus obtiene el estado del sistema de backup
func (h *Handler) GetStatus(c *fiber.Ctx) error {
	status, err := h.service.GetStatus()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to get backup status",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Backup status retrieved",
		"data":    status,
	})
}

// GetHealth verifica la salud del sistema de backup
func (h *Handler) GetHealth(c *fiber.Ctx) error {
	health := map[string]interface{}{
		"status":        "healthy",
		"timestamp":     time.Now(),
		"rclone_health": h.service.rclone.IsHealthy(),
		"encryption":    "enabled",
	}

	// Verificar espacio en disco
	if err := h.service.checkDiskSpace(); err != nil {
		health["status"] = "warning"
		health["disk_warning"] = err.Error()
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Backup service health check",
		"data":    health,
	})
}

// RunFullBackup ejecuta backup completo de PostgreSQL
func (h *Handler) RunFullBackup(c *fiber.Ctx) error {
	go func() {
		if err := h.service.FullBackupPostgreSQL(); err != nil {
			h.service.logger.Error("Manual full backup failed", "error", err)
		}
	}()

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Full backup started",
		"data": map[string]interface{}{
			"message": "PostgreSQL full backup started in background",
			"started": time.Now(),
		},
	})
}

// RunIncrementalBackup ejecuta backup incremental de PostgreSQL
func (h *Handler) RunIncrementalBackup(c *fiber.Ctx) error {
	go func() {
		if err := h.service.IncrementalBackupPostgreSQL(); err != nil {
			h.service.logger.Error("Manual incremental backup failed", "error", err)
		}
	}()

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Incremental backup started",
		"data": map[string]interface{}{
			"message": "PostgreSQL incremental backup started in background",
			"started": time.Now(),
		},
	})
}

// RunRedisBackup ejecuta backup de Redis
func (h *Handler) RunRedisBackup(c *fiber.Ctx) error {
	go func() {
		if err := h.service.BackupRedisSnapshot(); err != nil {
			h.service.logger.Error("Manual Redis backup failed", "error", err)
		}
	}()

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Redis backup started",
		"data": map[string]interface{}{
			"message": "Redis snapshot backup started in background",
			"started": time.Now(),
		},
	})
}

// RunConfigBackup ejecuta backup de configuración
func (h *Handler) RunConfigBackup(c *fiber.Ctx) error {
	go func() {
		if err := h.service.BackupSystemConfig(); err != nil {
			h.service.logger.Error("Manual config backup failed", "error", err)
		}
	}()

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Config backup started",
		"data": map[string]interface{}{
			"message": "System configuration backup started in background",
			"started": time.Now(),
		},
	})
}

// RunAnalyticsBackup ejecuta backup de analytics
func (h *Handler) RunAnalyticsBackup(c *fiber.Ctx) error {
	go func() {
		if err := h.service.BackupAnalyticsArchive(); err != nil {
			h.service.logger.Error("Manual analytics backup failed", "error", err)
		}
	}()

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Analytics backup started",
		"data": map[string]interface{}{
			"message": "Analytics archive backup started in background",
			"started": time.Now(),
		},
	})
}

// RestoreBackup restaura desde un backup específico
func (h *Handler) RestoreBackup(c *fiber.Ctx) error {
	backupType := c.Params("type")
	filename := c.Params("filename")
	
	// Obtener parámetros opcionales
	targetPath := c.Query("target", "")
	
	if backupType == "" || filename == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Type and filename are required",
		})
	}

	// Verificar que el tipo de backup sea válido
	validTypes := []string{"postgresql", "redis", "config", "analytics"}
	validType := false
	for _, vt := range validTypes {
		if backupType == vt {
			validType = true
			break
		}
	}
	
	if !validType {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Invalid backup type",
		})
	}

	// Ejecutar restauración en background
	go func() {
		if err := h.service.RestoreFromBackup(backupType, filename, targetPath); err != nil {
			h.service.logger.Error("Manual backup restoration failed", 
				"type", backupType, "file", filename, "error", err)
		} else {
			h.service.logger.Info("Manual backup restoration completed", 
				"type", backupType, "file", filename)
		}
	}()

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Backup restoration started",
		"data": map[string]interface{}{
			"message": fmt.Sprintf("Restoration of %s backup '%s' started in background", backupType, filename),
			"type":    backupType,
			"file":    filename,
			"started": time.Now(),
		},
	})
}

// ListBackups lista todos los backups disponibles
func (h *Handler) ListBackups(c *fiber.Ctx) error {
	backups, err := h.service.ListAvailableBackups()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to list backups",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Backups listed successfully",
		"data":    backups,
	})
}

// VerifyBackup verifica la integridad de un backup
func (h *Handler) VerifyBackup(c *fiber.Ctx) error {
	backupType := c.Params("type")
	filename := c.Params("filename")
	
	if backupType == "" || filename == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Type and filename are required",
		})
	}

	// Verificar integridad
	isValid, err := h.service.VerifyBackupIntegrity(backupType, filename)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Verification failed",
			"details": err.Error(),
		})
	}

	status := "invalid"
	if isValid {
		status = "valid"
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Backup verification completed",
		"data": map[string]interface{}{
			"file":   filename,
			"type":   backupType,
			"status": status,
			"valid":  isValid,
		},
	})
}

// RunCleanup ejecuta limpieza de retención
func (h *Handler) RunCleanup(c *fiber.Ctx) error {
	go func() {
		if err := h.service.CleanOldBackups(); err != nil {
			h.service.logger.Error("Manual retention cleanup failed", "error", err)
		}
	}()

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Retention cleanup started",
		"data": map[string]interface{}{
			"message": "Retention cleanup started in background",
			"started": time.Now(),
		},
	})
}

// GetRetentionReport obtiene reporte de retención
func (h *Handler) GetRetentionReport(c *fiber.Ctx) error {
	report, err := h.service.GetRetentionReport()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to generate retention report",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Retention report generated",
		"data":    report,
	})
}

// SyncToRemote sincroniza backups al remoto
func (h *Handler) SyncToRemote(c *fiber.Ctx) error {
	if !h.service.backupConfig.RemoteEnabled {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Remote sync is not enabled",
		})
	}

	// Obtener directorio a sincronizar (opcional)
	directory := c.Query("directory", h.service.backupConfig.BackupDir)

	go func() {
		result, err := h.service.rclone.SyncToRemote(directory)
		if err != nil {
			h.service.logger.Error("Manual remote sync failed", "error", err)
		} else {
			h.service.logger.Info("Manual remote sync completed", 
				"files_uploaded", result.FilesUploaded,
				"bytes_uploaded", result.BytesUploaded)
		}
	}()

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Remote sync started",
		"data": map[string]interface{}{
			"message":   "Remote synchronization started in background",
			"directory": directory,
			"started":   time.Now(),
		},
	})
}

// ListRemoteBackups lista backups disponibles en el remoto
func (h *Handler) ListRemoteBackups(c *fiber.Ctx) error {
	if !h.service.backupConfig.RemoteEnabled {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Remote sync is not enabled",
		})
	}

	files, err := h.service.rclone.ListRemoteBackups()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to list remote backups",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Remote backups listed",
		"data": map[string]interface{}{
			"files": files,
			"count": len(files),
		},
	})
}

// GetRemoteQuota obtiene información de cuota del remoto
func (h *Handler) GetRemoteQuota(c *fiber.Ctx) error {
	if !h.service.backupConfig.RemoteEnabled {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"success": false,
			"error":   "Remote sync is not enabled",
		})
	}

	quota, err := h.service.rclone.GetRemoteQuota()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"success": false,
			"error":   "Failed to get remote quota",
			"details": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Remote quota information",
		"data":    quota,
	})
}