package maintenance

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"

	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// Service servicio principal de mantenimiento automático
type Service struct {
	db       *gorm.DB
	redis    *redis.Client
	config   *config.Config
	logger   *logger.Logger
	ctx      context.Context
	cancel   context.CancelFunc

	// Configuración de mantenimiento
	diskThresholdWarning  float64 // 80%
	diskThresholdCritical float64 // 90%
	maxTempFileAge        time.Duration // 72 horas
	maxLogAge             time.Duration // 7 días
	maxArchiveAge         time.Duration // 12 meses
	dataRetentionDays     int           // 90 días para datos detallados
}

// NewService crea nueva instancia del servicio de mantenimiento
func NewService(db *gorm.DB, redisClient *redis.Client, cfg *config.Config, log *logger.Logger) *Service {
	ctx, cancel := context.WithCancel(context.Background())

	return &Service{
		db:                    db,
		redis:                 redisClient,
		config:                cfg,
		logger:                log,
		ctx:                   ctx,
		cancel:                cancel,
		diskThresholdWarning:  80.0,
		diskThresholdCritical: 90.0,
		maxTempFileAge:        72 * time.Hour,
		maxLogAge:             7 * 24 * time.Hour,
		maxArchiveAge:         12 * 30 * 24 * time.Hour, // ~12 meses
		dataRetentionDays:     90,
	}
}

// Start inicia el scheduler de mantenimiento automático
func (s *Service) Start() {
	s.logger.Info("🧹 Starting automatic maintenance service...")

	// Scheduler cada 10 minutos
	go s.runPeriodicMaintenance()

	// Scheduler diario
	go s.runDailyMaintenance()

	// Scheduler mensual
	go s.runMonthlyMaintenance()

	s.logger.Info("✅ Automatic maintenance service started")
}

// Stop detiene el servicio de mantenimiento
func (s *Service) Stop() {
	s.logger.Info("🛑 Stopping maintenance service...")
	s.cancel()
	s.logger.Info("✅ Maintenance service stopped")
}

// runPeriodicMaintenance ejecuta tareas cada 10 minutos
func (s *Service) runPeriodicMaintenance() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	// Ejecutar inmediatamente al inicio
	s.runPeriodicTasks()

	for {
		select {
		case <-ticker.C:
			s.runPeriodicTasks()
		case <-s.ctx.Done():
			return
		}
	}
}

// runDailyMaintenance ejecuta tareas diarias
func (s *Service) runDailyMaintenance() {
	// Calcular próxima ejecución a las 2:00 AM
	now := time.Now()
	next2AM := time.Date(now.Year(), now.Month(), now.Day()+1, 2, 0, 0, 0, now.Location())
	timeUntilNext2AM := next2AM.Sub(now)

	// Timer inicial hasta las 2:00 AM
	initialTimer := time.NewTimer(timeUntilNext2AM)
	defer initialTimer.Stop()

	select {
	case <-initialTimer.C:
		s.runDailyTasks()
	case <-s.ctx.Done():
		return
	}

	// Ticker cada 24 horas
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runDailyTasks()
		case <-s.ctx.Done():
			return
		}
	}
}

// runMonthlyMaintenance ejecuta tareas mensuales
func (s *Service) runMonthlyMaintenance() {
	// Calcular próxima ejecución el día 1 del mes a las 3:00 AM
	now := time.Now()
	nextMonth := now.AddDate(0, 1, 0)
	next1st3AM := time.Date(nextMonth.Year(), nextMonth.Month(), 1, 3, 0, 0, 0, now.Location())
	timeUntilNext := next1st3AM.Sub(now)

	// Timer inicial
	initialTimer := time.NewTimer(timeUntilNext)
	defer initialTimer.Stop()

	select {
	case <-initialTimer.C:
		s.runMonthlyTasks()
	case <-s.ctx.Done():
		return
	}

	// Ticker mensual
	ticker := time.NewTicker(30 * 24 * time.Hour) // Aproximado
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runMonthlyTasks()
		case <-s.ctx.Done():
			return
		}
	}
}

// runPeriodicTasks ejecuta tareas de mantenimiento periódicas
func (s *Service) runPeriodicTasks() {
	s.logger.Info("🔄 Running periodic maintenance tasks...")

	// 1. Verificar espacio en disco
	if err := s.CheckDiskSpace(); err != nil {
		s.logger.Error("Error checking disk space", "error", err)
	}

	// 2. Limpiar Redis
	if s.redis != nil {
		if err := s.CleanupRedis(); err != nil {
			s.logger.Error("Error cleaning Redis", "error", err)
		}
	}

	// 3. Limpiar carpeta temporal
	if err := s.CheckTempFolder(); err != nil {
		s.logger.Error("Error cleaning temp folder", "error", err)
	}

	// 4. Rotar logs
	if err := s.RotateLogs(); err != nil {
		s.logger.Error("Error rotating logs", "error", err)
	}

	s.logger.Info("✅ Periodic maintenance completed")
}

// runDailyTasks ejecuta tareas de mantenimiento diarias
func (s *Service) runDailyTasks() {
	s.logger.Info("🌅 Running daily maintenance tasks...")

	// 1. Resumir datos antiguos (compactación)
	if s.db != nil {
		if err := s.SummarizeOldData(); err != nil {
			s.logger.Error("Error summarizing old data", "error", err)
		}
	}

	// 2. Limpieza profunda de Redis
	if s.redis != nil {
		if err := s.DeepCleanupRedis(); err != nil {
			s.logger.Error("Error in deep Redis cleanup", "error", err)
		}
	}

	s.logger.Info("✅ Daily maintenance completed")
}

// runMonthlyTasks ejecuta tareas de mantenimiento mensuales
func (s *Service) runMonthlyTasks() {
	s.logger.Info("📅 Running monthly maintenance tasks...")

	// 1. Crear partición mensual
	if s.db != nil {
		if err := s.CreateMonthlyPartition(); err != nil {
			s.logger.Error("Error creating monthly partition", "error", err)
		}
	}

	// 2. Archivar datos antiguos
	if s.db != nil {
		if err := s.ArchiveOldSummaries(); err != nil {
			s.logger.Error("Error archiving old summaries", "error", err)
		}
	}

	s.logger.Info("✅ Monthly maintenance completed")
}

// GetSystemStatus obtiene el estado actual del sistema
func (s *Service) GetSystemStatus() (*SystemStatus, error) {
	status := &SystemStatus{
		Timestamp: time.Now(),
		Status:    "healthy",
	}

	// Verificar espacio en disco
	diskUsage, err := s.getDiskUsage()
	if err != nil {
		status.DiskUsage = "unknown"
		status.Status = "warning"
	} else {
		status.DiskUsage = fmt.Sprintf("%.1f%%", diskUsage)
		if diskUsage > s.diskThresholdCritical {
			status.Status = "critical"
		} else if diskUsage > s.diskThresholdWarning {
			status.Status = "warning"
		}
	}

	// Verificar Redis
	if s.redis != nil {
		redisInfo, err := s.getRedisInfo()
		if err != nil {
			status.RedisKeys = 0
			status.RedisMemory = "unknown"
		} else {
			status.RedisKeys = redisInfo.Keys
			status.RedisMemory = redisInfo.Memory
		}
	}

	// Verificar particiones de base de datos
	if s.db != nil {
		partitions, err := s.getActivePartitions()
		if err != nil {
			status.Partitions = []string{}
		} else {
			status.Partitions = partitions
		}
	}

	// Verificar carpeta temporal
	tempSize, err := s.getTempFolderSize()
	if err != nil {
		status.TempFolderSize = "unknown"
	} else {
		status.TempFolderSize = formatBytes(tempSize)
	}

	return status, nil
}

// SystemStatus estructura del estado del sistema
type SystemStatus struct {
	Timestamp      time.Time `json:"timestamp"`
	DiskUsage      string    `json:"disk_usage"`
	RedisKeys      int64     `json:"redis_keys"`
	RedisMemory    string    `json:"redis_memory"`
	Partitions     []string  `json:"partitions"`
	TempFolderSize string    `json:"temp_folder_size"`
	Status         string    `json:"status"`
}

// RedisInfo información de Redis
type RedisInfo struct {
	Keys   int64
	Memory string
}

// formatBytes formatea bytes en formato legible
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}