package backup

import (
	"time"

	"github.com/tucentropdf/engine-v2/internal/alerts"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"
)

// BackupModule módulo principal del sistema de backup empresarial
type BackupModule struct {
	service *Service
	handler *Handler
	logger  *logger.Logger
}

// NewBackupModule crea nueva instancia del módulo de backup
func NewBackupModule(db *gorm.DB, redisClient *redis.Client, cfg *config.Config, log *logger.Logger, alertService *alerts.Service) *BackupModule {
	// Crear servicio principal
	service := NewService(db, redisClient, cfg, log, alertService)
	
	// Crear handler HTTP
	handler := NewHandler(service)

	return &BackupModule{
		service: service,
		handler: handler,
		logger:  log,
	}
}

// Start inicia el módulo de backup
func (m *BackupModule) Start() error {
	m.logger.Info("🚀 Starting Enterprise Backup Module...")

	// Iniciar servicio de backup
	m.service.Start()

	m.logger.Info("✅ Enterprise Backup Module started successfully")
	return nil
}

// Stop detiene el módulo de backup
func (m *BackupModule) Stop() error {
	m.logger.Info("🛑 Stopping Enterprise Backup Module...")

	// Detener servicio
	m.service.Stop()

	m.logger.Info("✅ Enterprise Backup Module stopped")
	return nil
}

// GetService retorna el servicio de backup
func (m *BackupModule) GetService() *Service {
	return m.service
}

// GetHandler retorna el handler HTTP
func (m *BackupModule) GetHandler() *Handler {
	return m.handler
}

// GetStatus retorna el estado del módulo
func (m *BackupModule) GetStatus() (*ModuleStatus, error) {
	backupStatus, err := m.service.GetStatus()
	if err != nil {
		return nil, err
	}

	return &ModuleStatus{
		ModuleName:    "enterprise_backup",
		Status:        "active",
		LastUpdate:    time.Now(),
		BackupStatus:  backupStatus,
		Configuration: m.getConfigSummary(),
		Features: []string{
			"AES256 Encryption",
			"Automated Scheduling", 
			"Remote Sync (Rclone)",
			"Retention Policies",
			"PostgreSQL Full/Incremental Backups",
			"Redis Snapshots",
			"System Configuration Backups",
			"Analytics Archive",
			"Backup Verification",
			"Disaster Recovery",
		},
	}, nil
}

// ModuleStatus estado del módulo de backup
type ModuleStatus struct {
	ModuleName    string                 `json:"module_name"`
	Status        string                 `json:"status"`
	LastUpdate    time.Time              `json:"last_update"`
	BackupStatus  *BackupStatus          `json:"backup_status"`
	Configuration map[string]interface{} `json:"configuration"`
	Features      []string               `json:"features"`
}

// getConfigSummary retorna resumen de configuración
func (m *BackupModule) getConfigSummary() map[string]interface{} {
	config := m.service.backupConfig
	
	return map[string]interface{}{
		"backup_directory":        config.BackupDir,
		"encryption_enabled":      config.EncryptionKey != "",
		"remote_sync_enabled":     config.RemoteEnabled,
		"retention_full_days":     config.RetentionFull,
		"retention_incremental":   config.RetentionIncremental,
		"retention_redis_days":    config.RetentionRedis,
		"retention_config_days":   config.RetentionConfig,
		"retention_analytics_days": config.RetentionAnalytics,
		"min_disk_space_gb":       config.MinDiskSpaceGB,
		"postgresql_configured":   config.PGHost != "" && config.PGDatabase != "",
		"redis_configured":        config.RedisHost != "",
		"rclone_healthy":          m.service.rclone.IsHealthy(),
	}
}

// Métodos de conveniencia para operaciones comunes

// TriggerFullBackup dispara backup completo manual
func (m *BackupModule) TriggerFullBackup() error {
	return m.service.FullBackupPostgreSQL()
}

// TriggerIncrementalBackup dispara backup incremental manual
func (m *BackupModule) TriggerIncrementalBackup() error {
	return m.service.IncrementalBackupPostgreSQL()
}

// TriggerRedisBackup dispara backup de Redis manual
func (m *BackupModule) TriggerRedisBackup() error {
	return m.service.BackupRedisSnapshot()
}

// TriggerConfigBackup dispara backup de configuración manual
func (m *BackupModule) TriggerConfigBackup() error {
	return m.service.BackupSystemConfig()
}

// TriggerAnalyticsBackup dispara backup de analytics manual
func (m *BackupModule) TriggerAnalyticsBackup() error {
	return m.service.BackupAnalyticsArchive()
}

// TriggerCleanup dispara limpieza de retención manual
func (m *BackupModule) TriggerCleanup() error {
	return m.service.CleanOldBackups()
}

// RestoreBackup restaura desde backup específico
func (m *BackupModule) RestoreBackup(backupType, filename, targetPath string) error {
	return m.service.RestoreFromBackup(backupType, filename, targetPath)
}

// VerifyBackup verifica integridad de backup
func (m *BackupModule) VerifyBackup(backupType, filename string) (bool, error) {
	return m.service.VerifyBackupIntegrity(backupType, filename)
}

// ListBackups lista backups disponibles
func (m *BackupModule) ListBackups() (map[string][]BackupInfo, error) {
	return m.service.ListAvailableBackups()
}

// GetRetentionReport obtiene reporte de retención
func (m *BackupModule) GetRetentionReport() (*RetentionReport, error) {
	return m.service.GetRetentionReport()
}

// SyncToRemote sincroniza al remoto
func (m *BackupModule) SyncToRemote(directory string) (*SyncResult, error) {
	if directory == "" {
		directory = m.service.backupConfig.BackupDir
	}
	return m.service.rclone.SyncToRemote(directory)
}

// Health Check para monitoreo
func (m *BackupModule) IsHealthy() bool {
	status, err := m.service.GetStatus()
	if err != nil {
		return false
	}
	return status.Status == "healthy"
}

// ValidateConfiguration valida configuración actual
func (m *BackupModule) ValidateConfiguration() error {
	return m.service.validateConfiguration()
}

// GetBackupConfig retorna configuración actual
func (m *BackupModule) GetBackupConfig() *BackupConfig {
	return m.service.backupConfig
}