package legal_audit

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/tucentropdf/engine-v2/internal/alerts"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// Service servicio principal de auditoría legal
type Service struct {
	db               *gorm.DB
	redis            *redis.Client
	config           *config.Config
	logger           *logger.Logger
	alertService     *alerts.Service
	encryptor        *LegalEncryptor
	integrityManager *IntegrityManager
	retentionManager *RetentionManager
	exportManager    *ExportManager
	ctx              context.Context
	cancel           context.CancelFunc

	// Configuración
	auditConfig *AuditConfig
}

// AuditConfig configuración del sistema de auditoría legal
type AuditConfig struct {
	Enabled           bool   `json:"enabled"`
	EncryptSensitive  bool   `json:"encrypt_sensitive"`
	RetentionYears    int    `json:"retention_years"`
	ArchivePath       string `json:"archive_path"`
	SigningKey        string `json:"-"`
	EncryptionKey     string `json:"-"`
	RequireSignature  bool   `json:"require_signature"`
	AutoArchive       bool   `json:"auto_archive"`
	CompressionLevel  int    `json:"compression_level"`
}

// NewService crea nueva instancia del servicio de auditoría legal
func NewService(db *gorm.DB, redisClient *redis.Client, cfg *config.Config, log *logger.Logger, alertService *alerts.Service) *Service {
	ctx, cancel := context.WithCancel(context.Background())

	// Cargar configuración
	auditConfig := loadAuditConfig()

	// Crear componentes especializados
	encryptor := NewLegalEncryptor(auditConfig.EncryptionKey, log)
	integrityManager := NewIntegrityManager(auditConfig.SigningKey, log)
	retentionManager := NewRetentionManager(db, auditConfig, log)
	exportManager := NewExportManager(db, encryptor, integrityManager, log)

	service := &Service{
		db:               db,
		redis:            redisClient,
		config:           cfg,
		logger:           log,
		alertService:     alertService,
		encryptor:        encryptor,
		integrityManager: integrityManager,
		retentionManager: retentionManager,
		exportManager:    exportManager,
		ctx:              ctx,
		cancel:           cancel,
		auditConfig:      auditConfig,
	}

	return service
}

// Start inicia el servicio de auditoría legal
func (s *Service) Start() error {
	s.logger.Info("⚖️ Starting Legal Audit System...")

	// Verificar que está habilitado
	if !s.auditConfig.Enabled {
		s.logger.Info("Legal audit system is disabled")
		return nil
	}

	// Crear tablas si no existen
	if err := s.createTables(); err != nil {
		return fmt.Errorf("failed to create audit tables: %w", err)
	}

	// Validar configuración
	if err := s.validateConfiguration(); err != nil {
		return fmt.Errorf("audit configuration validation failed: %w", err)
	}

	// Iniciar gestores
	if err := s.retentionManager.Start(); err != nil {
		s.logger.Error("Failed to start retention manager", "error", err)
	}

	s.logger.Info("✅ Legal Audit System started successfully")
	return nil
}

// Stop detiene el servicio
func (s *Service) Stop() {
	s.logger.Info("🛑 Stopping Legal Audit System...")
	s.retentionManager.Stop()
	s.cancel()
	s.logger.Info("✅ Legal Audit System stopped")
}

// RecordEvent registra un evento de auditoría legal
func (s *Service) RecordEvent(event *AuditEvent) error {
	if !s.auditConfig.Enabled {
		return nil // Sistema deshabilitado
	}

	s.logger.Debug("Recording legal audit event", 
		"tool", event.Tool,
		"action", event.Action,
		"status", event.Status,
		"user_id", event.UserID)

	// Crear registro de auditoría
	record := &LegalAuditLog{
		ID:        uuid.New(),
		UserID:    event.UserID,
		Tool:      event.Tool,
		Action:    event.Action,
		Plan:      event.Plan,
		FileSize:  event.FileSize,
		IP:        event.IP,
		UserAgent: event.UserAgent,
		Status:    event.Status,
		Reason:    event.Reason,
		Timestamp: time.Now(),
		Metadata:  event.Metadata,
		Abuse:     event.Abuse,
		CompanyID: event.CompanyID,
		APIKeyID:  event.APIKeyID,
		AdminID:   event.AdminID,
		CreatedAt: time.Now(),
	}

	// Cifrar metadatos sensibles si está habilitado
	if s.auditConfig.EncryptSensitive && record.Metadata != nil {
		encryptedMetadata, err := s.encryptor.EncryptMetadata(record.Metadata)
		if err != nil {
			s.logger.Error("Failed to encrypt audit metadata", "error", err)
			// No fallar por esto, continuar sin cifrado
		} else {
			record.Metadata = encryptedMetadata
		}
	}

	// Generar hash de integridad
	integrityHash, err := s.integrityManager.GenerateIntegrityHash(record)
	if err != nil {
		return fmt.Errorf("failed to generate integrity hash: %w", err)
	}
	record.IntegrityHash = integrityHash

	// Generar firma digital si está habilitada
	if s.auditConfig.RequireSignature {
		signature, err := s.integrityManager.GenerateSignature(record)
		if err != nil {
			return fmt.Errorf("failed to generate signature: %w", err)
		}
		record.Signature = signature
	} else {
		record.Signature = "disabled"
	}

	// Guardar en base de datos
	if err := s.db.Create(record).Error; err != nil {
		return fmt.Errorf("failed to save audit record: %w", err)
	}

	// Actualizar estadísticas en Redis
	go s.updateStatistics(event)

	s.logger.Debug("Legal audit event recorded", "id", record.ID.String())
	return nil
}

// GetAuditLogs obtiene registros de auditoría con filtros
func (s *Service) GetAuditLogs(filter *AuditFilter) ([]LegalAuditLog, error) {
	query := s.db.Model(&LegalAuditLog{})

	// Aplicar filtros
	if filter.UserID != nil {
		query = query.Where("user_id = ?", *filter.UserID)
	}
	if filter.CompanyID != nil {
		query = query.Where("company_id = ?", *filter.CompanyID)
	}
	if filter.AdminID != nil {
		query = query.Where("admin_id = ?", *filter.AdminID)
	}
	if filter.Tool != "" {
		query = query.Where("tool = ?", filter.Tool)
	}
	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if filter.Plan != "" {
		query = query.Where("plan = ?", filter.Plan)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.APIKeyID != nil {
		query = query.Where("api_key_id = ?", *filter.APIKeyID)
	}
	if filter.FromDate != nil {
		query = query.Where("timestamp >= ?", *filter.FromDate)
	}
	if filter.ToDate != nil {
		query = query.Where("timestamp <= ?", *filter.ToDate)
	}
	if filter.AbuseOnly {
		query = query.Where("abuse = true")
	}

	// Aplicar límites
	if filter.Limit > 0 {
		query = query.Limit(filter.Limit)
	} else {
		query = query.Limit(1000) // Límite por defecto
	}
	if filter.Offset > 0 {
		query = query.Offset(filter.Offset)
	}

	// Ordenar por fecha descendente
	query = query.Order("timestamp DESC")

	var records []LegalAuditLog
	if err := query.Find(&records).Error; err != nil {
		return nil, fmt.Errorf("failed to query audit logs: %w", err)
	}

	// Descifrar metadatos si es necesario
	for i := range records {
		if s.auditConfig.EncryptSensitive && records[i].Metadata != nil {
			decryptedMetadata, err := s.encryptor.DecryptMetadata(records[i].Metadata)
			if err != nil {
				s.logger.Error("Failed to decrypt audit metadata", "id", records[i].ID, "error", err)
				continue
			}
			records[i].Metadata = decryptedMetadata
		}
	}

	return records, nil
}

// GetAuditRecord obtiene un registro específico por ID
func (s *Service) GetAuditRecord(id uuid.UUID) (*LegalAuditLog, error) {
	var record LegalAuditLog
	if err := s.db.Where("id = ?", id).First(&record).Error; err != nil {
		return nil, fmt.Errorf("audit record not found: %w", err)
	}

	// Descifrar metadatos si es necesario
	if s.auditConfig.EncryptSensitive && record.Metadata != nil {
		decryptedMetadata, err := s.encryptor.DecryptMetadata(record.Metadata)
		if err != nil {
			s.logger.Error("Failed to decrypt audit metadata", "id", id, "error", err)
		} else {
			record.Metadata = decryptedMetadata
		}
	}

	return &record, nil
}

// VerifyRecord verifica la integridad de un registro específico
func (s *Service) VerifyRecord(id uuid.UUID) (bool, error) {
	record, err := s.GetAuditRecord(id)
	if err != nil {
		return false, err
	}

	return s.integrityManager.VerifyRecord(record)
}

// VerifyBatchIntegrity verifica integridad de múltiples registros
func (s *Service) VerifyBatchIntegrity(filter *AuditFilter) (*IntegrityReport, error) {
	records, err := s.GetAuditLogs(filter)
	if err != nil {
		return nil, err
	}

	return s.integrityManager.VerifyBatchIntegrity(records), nil
}

// ExportAuditData exporta datos de auditoría para evidencia legal
func (s *Service) ExportAuditData(request *ExportRequest) (*ExportResult, error) {
	// Registrar la solicitud de exportación
	exportEvent := &AuditEvent{
		AdminID:   &request.AdminID,
		Tool:      ToolAdmin,
		Action:    ActionExport,
		Status:    StatusPending,
		IP:        request.RequestIP,
		UserAgent: "legal-export-system",
		Metadata: &JSONBMetadata{
			AdminAction: "legal_audit_export",
			ExportType:  request.Format,
			Extra: map[string]interface{}{
				"reason": request.Reason,
				"encrypted": request.Encrypted,
			},
		},
	}

	// Crear filtro para exportación
	filter := &AuditFilter{
		UserID:    request.UserID,
		CompanyID: request.CompanyID,
		FromDate:  &request.FromDate,
		ToDate:    &request.ToDate,
		Tool:      request.Tool,
		Action:    request.Action,
		Status:    request.Status,
		AbuseOnly: request.IncludeAbuse,
	}

	// Ejecutar exportación
	result, err := s.exportManager.ExportToFile(filter, request)
	if err != nil {
		exportEvent.Status = StatusFail
		errStr := err.Error()
		exportEvent.Reason = &errStr
		s.RecordEvent(exportEvent) // Registrar fallo
		return nil, fmt.Errorf("export failed: %w", err)
	}

	// Registrar exportación exitosa
	exportEvent.Status = StatusSuccess
	s.RecordEvent(exportEvent)

	return result, nil
}

// GetAuditStats obtiene estadísticas de auditoría
func (s *Service) GetAuditStats(filter *AuditFilter) (*AuditStats, error) {
	// Obtener estadísticas básicas
	stats := &AuditStats{
		RecordsByTool:   make(map[string]int64),
		RecordsByAction: make(map[string]int64),
		RecordsByStatus: make(map[string]int64),
		RecordsByPlan:   make(map[string]int64),
	}

	// Query base
	baseQuery := s.db.Model(&LegalAuditLog{})
	if filter.FromDate != nil {
		baseQuery = baseQuery.Where("timestamp >= ?", *filter.FromDate)
	}
	if filter.ToDate != nil {
		baseQuery = baseQuery.Where("timestamp <= ?", *filter.ToDate)
	}

	// Total de registros
	baseQuery.Count(&stats.TotalRecords)

	// Registros de abuso
	baseQuery.Where("abuse = true").Count(&stats.AbuseRecords)

	// Registros fallidos
	baseQuery.Where("status IN ?", []string{StatusFail, StatusRejected, StatusProtectorBlocked}).Count(&stats.FailedRecords)

	// Estadísticas por herramienta
	var toolStats []struct {
		Tool  string `json:"tool"`
		Count int64  `json:"count"`
	}
	s.db.Model(&LegalAuditLog{}).Select("tool, COUNT(*) as count").Group("tool").Scan(&toolStats)
	for _, stat := range toolStats {
		stats.RecordsByTool[stat.Tool] = stat.Count
	}

	// Estadísticas por acción
	var actionStats []struct {
		Action string `json:"action"`
		Count  int64  `json:"count"`
	}
	s.db.Model(&LegalAuditLog{}).Select("action, COUNT(*) as count").Group("action").Scan(&actionStats)
	for _, stat := range actionStats {
		stats.RecordsByAction[stat.Action] = stat.Count
	}

	// Estadísticas por estado
	var statusStats []struct {
		Status string `json:"status"`
		Count  int64  `json:"count"`
	}
	s.db.Model(&LegalAuditLog{}).Select("status, COUNT(*) as count").Group("status").Scan(&statusStats)
	for _, stat := range statusStats {
		stats.RecordsByStatus[stat.Status] = stat.Count
	}

	// Estadísticas por plan
	var planStats []struct {
		Plan  string `json:"plan"`
		Count int64  `json:"count"`
	}
	s.db.Model(&LegalAuditLog{}).Select("plan, COUNT(*) as count").Group("plan").Scan(&planStats)
	for _, stat := range planStats {
		stats.RecordsByPlan[stat.Plan] = stat.Count
	}

	// Top usuarios
	var topUsers []UserAuditSummary
	s.db.Model(&LegalAuditLog{}).
		Select("user_id, COUNT(*) as record_count, SUM(CASE WHEN abuse = true THEN 1 ELSE 0 END) as abuse_count, SUM(CASE WHEN status IN ('fail', 'rejected') THEN 1 ELSE 0 END) as failed_count, MAX(timestamp) as last_activity").
		Where("user_id IS NOT NULL").
		Group("user_id").
		Order("record_count DESC").
		Limit(10).
		Scan(&topUsers)
	stats.TopUsers = topUsers

	// Top empresas
	var topCompanies []CompanyAuditSummary
	s.db.Model(&LegalAuditLog{}).
		Select("company_id, COUNT(*) as record_count, SUM(CASE WHEN tool = 'api' THEN 1 ELSE 0 END) as api_usage, SUM(CASE WHEN abuse = true THEN 1 ELSE 0 END) as abuse_count, MAX(timestamp) as last_activity").
		Where("company_id IS NOT NULL").
		Group("company_id").
		Order("record_count DESC").
		Limit(10).
		Scan(&topCompanies)
	stats.TopCompanies = topCompanies

	// Establecer rango de fechas
	if filter.FromDate != nil && filter.ToDate != nil {
		stats.DateRange = DateRange{
			From: *filter.FromDate,
			To:   *filter.ToDate,
		}
	}

	return stats, nil
}

// loadAuditConfig carga configuración desde variables de entorno
func loadAuditConfig() *AuditConfig {
	return &AuditConfig{
		Enabled:          getEnvBoolOrDefault("LEGAL_AUDIT_ENABLED", true),
		EncryptSensitive: getEnvBoolOrDefault("LEGAL_AUDIT_ENCRYPT_SENSITIVE", true),
		RetentionYears:   getEnvIntOrDefault("LEGAL_AUDIT_RETENTION_YEARS", 3),
		ArchivePath:      getEnvOrDefault("LEGAL_AUDIT_ARCHIVE_PATH", "/var/tucentropdf/archive/legal"),
		SigningKey:       getEnvOrDefault("LEGAL_AUDIT_SIGNING_KEY", ""),
		EncryptionKey:    getEnvOrDefault("BACKUP_ENCRYPTION_KEY", ""), // Reutilizar clave de backup
		RequireSignature: getEnvBoolOrDefault("LEGAL_AUDIT_REQUIRE_SIGNATURE", true),
		AutoArchive:      getEnvBoolOrDefault("LEGAL_AUDIT_AUTO_ARCHIVE", true),
		CompressionLevel: getEnvIntOrDefault("LEGAL_AUDIT_COMPRESSION_LEVEL", 6),
	}
}

// createTables crea las tablas necesarias
func (s *Service) createTables() error {
	// Auto-migrar modelo principal
	if err := s.db.AutoMigrate(&LegalAuditLog{}); err != nil {
		return fmt.Errorf("failed to migrate legal_audit_logs: %w", err)
	}

	// Auto-migrar modelo de archivo
	if err := s.db.AutoMigrate(&ArchiveRecord{}); err != nil {
		return fmt.Errorf("failed to migrate legal_audit_archives: %w", err)
	}

	// Crear restricciones adicionales
	if err := s.createConstraints(); err != nil {
		return fmt.Errorf("failed to create constraints: %w", err)
	}

	return nil
}

// createConstraints crea restricciones adicionales para inmutabilidad
func (s *Service) createConstraints() error {
	// Prevenir actualizaciones en la tabla legal_audit_logs
	triggerSQL := `
		CREATE OR REPLACE FUNCTION prevent_legal_audit_updates()
		RETURNS TRIGGER AS $$
		BEGIN
			RAISE EXCEPTION 'Legal audit logs are immutable. Updates not allowed.';
			RETURN NULL;
		END;
		$$ LANGUAGE plpgsql;

		DROP TRIGGER IF EXISTS prevent_updates_legal_audit_logs ON legal_audit_logs;
		CREATE TRIGGER prevent_updates_legal_audit_logs
			BEFORE UPDATE ON legal_audit_logs
			FOR EACH ROW
			EXECUTE FUNCTION prevent_legal_audit_updates();
	`

	if err := s.db.Exec(triggerSQL).Error; err != nil {
		s.logger.Warn("Failed to create immutability trigger (may already exist)", "error", err)
		// No fallar por esto, puede que ya exista
	}

	return nil
}

// IsHealthy verifica si el servicio está saludable
func (s *Service) IsHealthy() bool {
	return s.db != nil && s.redis != nil
}

// validateConfiguration valida la configuración del servicio
func (s *Service) validateConfiguration() error {
	if s.auditConfig.SigningKey == "" {
		return fmt.Errorf("LEGAL_AUDIT_SIGNING_KEY is required")
	}

	if s.auditConfig.EncryptionKey == "" {
		return fmt.Errorf("encryption key is required for legal audit")
	}

	if s.auditConfig.RetentionYears < 1 || s.auditConfig.RetentionYears > 10 {
		return fmt.Errorf("retention years must be between 1 and 10")
	}

	// Validar cifrado
	if err := s.encryptor.ValidateEncryption(); err != nil {
		return fmt.Errorf("encryption validation failed: %w", err)
	}

	// Crear directorio de archivos si no existe
	if err := os.MkdirAll(s.auditConfig.ArchivePath, 0750); err != nil {
		return fmt.Errorf("failed to create archive directory: %w", err)
	}

	return nil
}

// updateStatistics actualiza estadísticas en Redis
func (s *Service) updateStatistics(event *AuditEvent) {
	if s.redis == nil {
		return
	}

	// Incrementar contadores diarios
	today := time.Now().Format("2006-01-02")
	key := fmt.Sprintf("legal_audit:stats:%s", today)

	// Incrementar contadores por herramienta y acción
	s.redis.HIncrBy(s.ctx, key+":tools", event.Tool, 1)
	s.redis.HIncrBy(s.ctx, key+":actions", event.Action, 1)
	s.redis.HIncrBy(s.ctx, key+":status", event.Status, 1)

	if event.Abuse {
		s.redis.HIncrBy(s.ctx, key+":abuse", "total", 1)
	}

	// Expirar estadísticas después de 90 días
	s.redis.Expire(s.ctx, key+":tools", 90*24*time.Hour)
	s.redis.Expire(s.ctx, key+":actions", 90*24*time.Hour)
	s.redis.Expire(s.ctx, key+":status", 90*24*time.Hour)
	s.redis.Expire(s.ctx, key+":abuse", 90*24*time.Hour)
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