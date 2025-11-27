package backup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"

	"github.com/tucentropdf/engine-v2/internal/alerts"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// Service servicio principal de backups empresariales
type Service struct {
	db           *gorm.DB
	redis        *redis.Client
	config       *config.Config
	logger       *logger.Logger
	alertService *alerts.Service
	encryptor    *Encryptor
	rclone       *RcloneManager
	scheduler    *Scheduler
	ctx          context.Context
	cancel       context.CancelFunc

	// Configuración de backup
	backupConfig *BackupConfig
}

// BackupConfig configuración del sistema de backups
type BackupConfig struct {
	// Directorios
	BackupDir     string `json:"backup_dir"`
	TempDir       string `json:"temp_dir"`
	ArchiveDir    string `json:"archive_dir"`

	// PostgreSQL
	PGHost        string `json:"pg_host"`
	PGPort        string `json:"pg_port"`
	PGUser        string `json:"pg_user"`
	PGPassword    string `json:"-"`
	PGDatabase    string `json:"pg_database"`

	// Redis
	RedisHost     string `json:"redis_host"`
	RedisPort     string `json:"redis_port"`
	RedisPassword string `json:"-"`

	// Cifrado
	EncryptionKey string `json:"-"`

	// Remoto (Rclone)
	RemoteEnabled bool   `json:"remote_enabled"`
	RemotePath    string `json:"remote_path"`
	RcloneConfig  string `json:"rclone_config"`

	// Retención (días)
	RetentionFull         int `json:"retention_full"`         // 30 días
	RetentionIncremental  int `json:"retention_incremental"`  // 7 días
	RetentionConfig       int `json:"retention_config"`       // 90 días
	RetentionRedis        int `json:"retention_redis"`        // 7 días
	RetentionAnalytics    int `json:"retention_analytics"`    // 365 días (12 meses)

	// Alertas
	MinDiskSpaceGB int `json:"min_disk_space_gb"` // 10GB mínimo
}

// BackupStatus estado del sistema de backups
type BackupStatus struct {
	LastFullBackup        *BackupInfo `json:"last_full_backup"`
	LastIncrementalBackup *BackupInfo `json:"last_incremental_backup"`
	LastConfigBackup      *BackupInfo `json:"last_config_backup"`
	LastRedisBackup       *BackupInfo `json:"last_redis_backup"`
	LastAnalyticsBackup   *BackupInfo `json:"last_analytics_backup"`
	RemoteSync            bool        `json:"remote_sync"`
	RetentionOK           bool        `json:"retention_ok"`
	DiskUsage             string      `json:"disk_usage"`
	DiskFreeGB            float64     `json:"disk_free_gb"`
	Status                string      `json:"status"` // healthy, warning, critical
	LastCheck             time.Time   `json:"last_check"`
}

// BackupInfo información de un backup
type BackupInfo struct {
	Type      string    `json:"type"`      // full, incremental, config, redis, analytics
	Filename  string    `json:"filename"`
	Size      int64     `json:"size"`
	Timestamp time.Time `json:"timestamp"`
	Encrypted bool      `json:"encrypted"`
	Remote    bool      `json:"remote"`
	Checksum  string    `json:"checksum"`
	Success   bool      `json:"success"`
	Error     string    `json:"error,omitempty"`
}

// NewService crea nueva instancia del servicio de backups
func NewService(db *gorm.DB, redisClient *redis.Client, cfg *config.Config, log *logger.Logger, alertService *alerts.Service) *Service {
	ctx, cancel := context.WithCancel(context.Background())

	// Cargar configuración de backup
	backupConfig := loadBackupConfig()

	// Crear servicios auxiliares
	encryptor := NewEncryptor(backupConfig.EncryptionKey, log)
	rclone := NewRcloneManager(backupConfig, log)

	service := &Service{
		db:           db,
		redis:        redisClient,
		config:       cfg,
		logger:       log,
		alertService: alertService,
		encryptor:    encryptor,
		rclone:       rclone,
		ctx:          ctx,
		cancel:       cancel,
		backupConfig: backupConfig,
	}

	// Crear scheduler
	service.scheduler = NewScheduler(service, log)

	return service
}

// Start inicia el servicio de backups
func (s *Service) Start() {
	s.logger.Info("💾 Starting enterprise backup service...")

	// Crear directorios necesarios
	if err := s.createDirectories(); err != nil {
		s.logger.Error("Error creating backup directories", "error", err)
		return
	}

	// Validar configuración
	if err := s.validateConfiguration(); err != nil {
		s.logger.Error("Backup configuration validation failed", "error", err)
		s.sendAlert("BACKUP_CONFIG_ERROR", "critical", fmt.Sprintf("Backup configuration invalid: %v", err))
		return
	}

	// Crear tabla de backups si no existe
	if s.db != nil {
		if err := s.createBackupsTable(); err != nil {
			s.logger.Error("Error creating backups table", "error", err)
		}
	}

	// Iniciar scheduler
	s.scheduler.Start()

	s.logger.Info("✅ Enterprise backup service started")
}

// Stop detiene el servicio de backups
func (s *Service) Stop() {
	s.logger.Info("🛑 Stopping backup service...")
	if s.scheduler != nil {
		s.scheduler.Stop()
	}
	s.cancel()
	s.logger.Info("✅ Backup service stopped")
}

// loadBackupConfig carga configuración desde variables de entorno
func loadBackupConfig() *BackupConfig {
	return &BackupConfig{
		// Directorios
		BackupDir:  getEnvOrDefault("BACKUP_DIR", "./backups"),
		TempDir:    getEnvOrDefault("BACKUP_TEMP_DIR", "./backups/temp"),
		ArchiveDir: getEnvOrDefault("BACKUP_ARCHIVE_DIR", "./backups/archive"),

		// PostgreSQL
		PGHost:     getEnvOrDefault("DB_HOST", "localhost"),
		PGPort:     getEnvOrDefault("DB_PORT", "5432"),
		PGUser:     getEnvOrDefault("DB_USER", "postgres"),
		PGPassword: getEnvOrDefault("DB_PASSWORD", ""),
		PGDatabase: getEnvOrDefault("DB_NAME", "tucentropdf"),

		// Redis
		RedisHost:     getEnvOrDefault("REDIS_HOST", "localhost"),
		RedisPort:     getEnvOrDefault("REDIS_PORT", "6379"),
		RedisPassword: getEnvOrDefault("REDIS_PASSWORD", ""),

		// Cifrado
		EncryptionKey: getEnvOrDefault("BACKUP_ENCRYPTION_KEY", ""),

		// Remoto
		RemoteEnabled: getEnvBoolOrDefault("BACKUP_REMOTE_ENABLED", false),
		RemotePath:    getEnvOrDefault("RCLONE_REMOTE", "drive:/tucentropdf_backups/"),
		RcloneConfig:  getEnvOrDefault("RCLONE_CONFIG", ""),

		// Retención
		RetentionFull:        getEnvIntOrDefault("BACKUP_RETENTION_FULL_DAYS", 30),
		RetentionIncremental: getEnvIntOrDefault("BACKUP_RETENTION_INCREMENTAL_DAYS", 7),
		RetentionConfig:      getEnvIntOrDefault("BACKUP_RETENTION_CONFIG_DAYS", 90),
		RetentionRedis:       getEnvIntOrDefault("BACKUP_RETENTION_REDIS_DAYS", 7),
		RetentionAnalytics:   getEnvIntOrDefault("BACKUP_RETENTION_ANALYTICS_DAYS", 365),

		// Alertas
		MinDiskSpaceGB: getEnvIntOrDefault("BACKUP_MIN_DISK_SPACE_GB", 10),
	}
}

// createDirectories crea los directorios necesarios
func (s *Service) createDirectories() error {
	dirs := []string{
		s.backupConfig.BackupDir,
		s.backupConfig.TempDir,
		s.backupConfig.ArchiveDir,
		filepath.Join(s.backupConfig.BackupDir, "postgresql"),
		filepath.Join(s.backupConfig.BackupDir, "redis"),
		filepath.Join(s.backupConfig.BackupDir, "config"),
		filepath.Join(s.backupConfig.BackupDir, "analytics"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0750); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}

	s.logger.Info("Backup directories created", "backup_dir", s.backupConfig.BackupDir)
	return nil
}

// validateConfiguration valida la configuración del servicio
func (s *Service) validateConfiguration() error {
	// Validar clave de cifrado
	if s.backupConfig.EncryptionKey == "" {
		return fmt.Errorf("BACKUP_ENCRYPTION_KEY is required")
	}
	if len(s.backupConfig.EncryptionKey) < 32 {
		return fmt.Errorf("BACKUP_ENCRYPTION_KEY must be at least 32 characters")
	}

	// Validar configuración PostgreSQL
	if s.backupConfig.PGHost == "" || s.backupConfig.PGDatabase == "" {
		return fmt.Errorf("PostgreSQL configuration incomplete")
	}

	// Validar espacio en disco
	if err := s.checkDiskSpace(); err != nil {
		return fmt.Errorf("insufficient disk space: %w", err)
	}

	// Validar rclone si está habilitado
	if s.backupConfig.RemoteEnabled {
		if err := s.rclone.ValidateConfiguration(); err != nil {
			return fmt.Errorf("rclone configuration error: %w", err)
		}
	}

	return nil
}

// GetStatus obtiene el estado actual del sistema de backups
func (s *Service) GetStatus() (*BackupStatus, error) {
	status := &BackupStatus{
		LastCheck: time.Now(),
		Status:    "healthy",
	}

	// Obtener últimos backups
	status.LastFullBackup = s.getLastBackupInfo("postgresql_full")
	status.LastIncrementalBackup = s.getLastBackupInfo("postgresql_incremental")
	status.LastConfigBackup = s.getLastBackupInfo("system_config")
	status.LastRedisBackup = s.getLastBackupInfo("redis_snapshot")
	status.LastAnalyticsBackup = s.getLastBackupInfo("analytics_archive")

	// Verificar sincronización remota
	status.RemoteSync = s.backupConfig.RemoteEnabled && s.rclone.IsHealthy()

	// Verificar retención
	status.RetentionOK = s.checkRetentionCompliance()

	// Verificar espacio en disco
	diskUsage, freeGB, err := s.getDiskInfo()
	if err != nil {
		status.DiskUsage = "unknown"
		status.DiskFreeGB = 0
	} else {
		status.DiskUsage = fmt.Sprintf("%.1f%%", diskUsage)
		status.DiskFreeGB = freeGB
	}

	// Determinar estado general
	status.Status = s.calculateOverallStatus(status)

	return status, nil
}

// createBackupsTable crea la tabla de registro de backups
func (s *Service) createBackupsTable() error {
	createSQL := `
		CREATE TABLE IF NOT EXISTS system_backups (
			id SERIAL PRIMARY KEY,
			type VARCHAR(50) NOT NULL,
			filename VARCHAR(255) NOT NULL,
			size_bytes BIGINT NOT NULL DEFAULT 0,
			encrypted BOOLEAN DEFAULT FALSE,
			remote_uploaded BOOLEAN DEFAULT FALSE,
			checksum VARCHAR(64),
			success BOOLEAN DEFAULT FALSE,
			error_message TEXT,
			start_time TIMESTAMP DEFAULT NOW(),
			end_time TIMESTAMP,
			duration_seconds INTEGER,
			created_at TIMESTAMP DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_backups_type_created ON system_backups(type, created_at);
		CREATE INDEX IF NOT EXISTS idx_backups_success ON system_backups(success, created_at);
	`

	return s.db.Exec(createSQL).Error
}

// sendAlert envía una alerta del sistema de backup
func (s *Service) sendAlert(alertType, severity, message string) {
	if s.alertService != nil {
		go s.alertService.SendAlert(&alerts.Alert{
			Type:     alertType,
			Severity: severity,
			Message:  message,
			Details: map[string]interface{}{
				"component": "backup_service",
				"timestamp": time.Now(),
			},
		})
	}
}

// Helpers para variables de entorno
func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvIntOrDefault(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

func getEnvBoolOrDefault(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if boolValue, err := strconv.ParseBool(value); err == nil {
			return boolValue
		}
	}
	return defaultValue
}

