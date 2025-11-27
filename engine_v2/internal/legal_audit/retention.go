package legal_audit

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// RetentionManager gestiona retención y archivado de registros legales
type RetentionManager struct {
	db          *gorm.DB
	config      *AuditConfig
	logger      *logger.Logger
	encryptor   *LegalEncryptor
	running     bool
	stopChan    chan struct{}
}

// NewRetentionManager crea nueva instancia del gestor de retención
func NewRetentionManager(db *gorm.DB, config *AuditConfig, log *logger.Logger) *RetentionManager {
	// Crear encryptor para archivos
	encryptor := NewLegalEncryptor(config.EncryptionKey, log)

	return &RetentionManager{
		db:        db,
		config:    config,
		logger:    log,
		encryptor: encryptor,
		stopChan:  make(chan struct{}),
	}
}

// Start inicia el gestor de retención
func (rm *RetentionManager) Start() error {
	if !rm.config.AutoArchive {
		rm.logger.Info("Auto-archive is disabled")
		return nil
	}

	rm.logger.Info("📚 Starting Legal Audit Retention Manager...")
	rm.running = true

	// Crear directorio de archivos si no existe
	if err := rm.createArchiveDirectories(); err != nil {
		return fmt.Errorf("failed to create archive directories: %w", err)
	}

	// Iniciar rutina de archivado
	go rm.archiveRoutine()

	rm.logger.Info("✅ Legal Audit Retention Manager started")
	return nil
}

// Stop detiene el gestor de retención
func (rm *RetentionManager) Stop() {
	if rm.running {
		rm.logger.Info("🛑 Stopping Legal Audit Retention Manager...")
		rm.running = false
		close(rm.stopChan)
		rm.logger.Info("✅ Legal Audit Retention Manager stopped")
	}
}

// archiveRoutine rutina de archivado automático
func (rm *RetentionManager) archiveRoutine() {
	// Ejecutar inmediatamente al inicio
	rm.executeArchivePass()

	// Ejecutar cada 24 horas
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			rm.executeArchivePass()
		case <-rm.stopChan:
			return
		}
	}
}

// executeArchivePass ejecuta un pase completo de archivado
func (rm *RetentionManager) executeArchivePass() {
	rm.logger.Info("🗃️ Starting legal audit archive pass")
	start := time.Now()

	// Determinar fecha de corte para archivado (mantener solo últimos N meses activos)
	archiveThreshold := time.Now().AddDate(0, -rm.config.RetentionYears*12/4, 0) // 3 meses activos
	
	// Determinar fecha de corte para eliminación completa
	deleteThreshold := time.Now().AddDate(-rm.config.RetentionYears, 0, 0)

	// Fase 1: Archivar registros antiguos
	archivedCount, err := rm.archiveOldRecords(archiveThreshold)
	if err != nil {
		rm.logger.Error("Failed to archive old records", "error", err)
	}

	// Fase 2: Eliminar registros que exceden retención
	deletedCount, err := rm.deleteExpiredRecords(deleteThreshold)
	if err != nil {
		rm.logger.Error("Failed to delete expired records", "error", err)
	}

	// Fase 3: Limpiar archivos antiguos
	cleanedArchives, err := rm.cleanOldArchives(deleteThreshold)
	if err != nil {
		rm.logger.Error("Failed to clean old archives", "error", err)
	}

	duration := time.Since(start)
	rm.logger.Info("Archive pass completed",
		"archived_records", archivedCount,
		"deleted_records", deletedCount,
		"cleaned_archives", cleanedArchives,
		"duration", duration.String())
}

// archiveOldRecords archiva registros antiguos
func (rm *RetentionManager) archiveOldRecords(threshold time.Time) (int, error) {
	rm.logger.Info("📦 Archiving old legal audit records", "threshold", threshold.Format("2006-01-02"))

	// Obtener registros para archivar en lotes
	batchSize := 1000
	totalArchived := 0

	for {
		var records []LegalAuditLog
		err := rm.db.Where("created_at < ? AND id NOT IN (SELECT original_id FROM legal_audit_archives)", threshold).
			Limit(batchSize).
			Order("created_at ASC").
			Find(&records).Error

		if err != nil {
			return totalArchived, fmt.Errorf("failed to fetch records for archiving: %w", err)
		}

		if len(records) == 0 {
			break
		}

		// Agrupar registros por mes para archivado eficiente
		recordsByMonth := rm.groupRecordsByMonth(records)

		for month, monthRecords := range recordsByMonth {
			archivedInMonth, err := rm.archiveMonthRecords(month, monthRecords)
			if err != nil {
				rm.logger.Error("Failed to archive month records", "month", month, "error", err)
				continue
			}
			totalArchived += archivedInMonth
		}
	}

	return totalArchived, nil
}

// groupRecordsByMonth agrupa registros por mes
func (rm *RetentionManager) groupRecordsByMonth(records []LegalAuditLog) map[string][]LegalAuditLog {
	result := make(map[string][]LegalAuditLog)

	for _, record := range records {
		month := record.CreatedAt.Format("2006-01")
		result[month] = append(result[month], record)
	}

	return result
}

// archiveMonthRecords archiva registros de un mes específico
func (rm *RetentionManager) archiveMonthRecords(month string, records []LegalAuditLog) (int, error) {
	if len(records) == 0 {
		return 0, nil
	}

	rm.logger.Debug("Archiving month records", "month", month, "count", len(records))

	// Crear archivo tar.gz
	archivePath, err := rm.createMonthArchive(month, records)
	if err != nil {
		return 0, fmt.Errorf("failed to create archive for month %s: %w", month, err)
	}

	// Obtener información del archivo
	archiveInfo, err := os.Stat(archivePath)
	if err != nil {
		return 0, fmt.Errorf("failed to get archive info: %w", err)
	}

	// Cifrar archivo si está habilitado
	var finalArchivePath string
	if rm.config.EncryptSensitive {
		encryptedPath, err := rm.encryptor.EncryptExportFile(archivePath)
		if err != nil {
			rm.logger.Error("Failed to encrypt archive", "error", err)
			// Continuar sin cifrado
			finalArchivePath = archivePath
		} else {
			// Eliminar archivo sin cifrar y usar cifrado
			os.Remove(archivePath)
			finalArchivePath = encryptedPath
		}
	} else {
		finalArchivePath = archivePath
	}

	// Registrar archivo en base de datos
	archivedCount := 0
	for _, record := range records {
		archiveRecord := &ArchiveRecord{
			OriginalID:    record.ID,
			ArchivePath:   finalArchivePath,
			CompressedSize: archiveInfo.Size(),
			OriginalSize:   int64(len(fmt.Sprintf("%+v", record))), // Aproximación
			Encrypted:     rm.config.EncryptSensitive,
			ArchiveDate:   time.Now(),
			IntegrityHash: record.IntegrityHash, // Mantener hash original
		}

		if err := rm.db.Create(archiveRecord).Error; err != nil {
			rm.logger.Error("Failed to create archive record", "original_id", record.ID, "error", err)
			continue
		}

		// Eliminar registro original de la tabla principal
		if err := rm.db.Delete(&record).Error; err != nil {
			rm.logger.Error("Failed to delete archived record", "id", record.ID, "error", err)
			continue
		}

		archivedCount++
	}

	rm.logger.Info("Month archive created",
		"month", month,
		"records_archived", archivedCount,
		"archive_path", finalArchivePath,
		"file_size", archiveInfo.Size())

	return archivedCount, nil
}

// createMonthArchive crea archivo comprimido para registros de un mes
func (rm *RetentionManager) createMonthArchive(month string, records []LegalAuditLog) (string, error) {
	// Crear directorio del año si no existe
	year := strings.Split(month, "-")[0]
	yearDir := filepath.Join(rm.config.ArchivePath, year)
	if err := os.MkdirAll(yearDir, 0750); err != nil {
		return "", fmt.Errorf("failed to create year directory: %w", err)
	}

	// Crear archivo de archivo
	archivePath := filepath.Join(yearDir, fmt.Sprintf("legal_audit_%s.tar.gz", month))
	archiveFile, err := os.Create(archivePath)
	if err != nil {
		return "", fmt.Errorf("failed to create archive file: %w", err)
	}
	defer archiveFile.Close()

	// Crear compressor gzip
	gzipWriter, err := gzip.NewWriterLevel(archiveFile, rm.config.CompressionLevel)
	if err != nil {
		return "", fmt.Errorf("failed to create gzip writer: %w", err)
	}
	defer gzipWriter.Close()

	// Crear archivador tar
	tarWriter := tar.NewWriter(gzipWriter)
	defer tarWriter.Close()

	// Agregar registros al archivo
	for _, record := range records {
		if err := rm.addRecordToArchive(tarWriter, &record); err != nil {
			rm.logger.Error("Failed to add record to archive", "id", record.ID, "error", err)
			continue
		}
	}

	return archivePath, nil
}

// addRecordToArchive agrega un registro al archivo tar
func (rm *RetentionManager) addRecordToArchive(tarWriter *tar.Writer, record *LegalAuditLog) error {
	// Serializar registro a JSON
	recordJSON, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("failed to marshal record: %w", err)
	}

	// Crear header para el archivo
	header := &tar.Header{
		Name: fmt.Sprintf("%s.json", record.ID.String()),
		Size: int64(len(recordJSON)),
		Mode: 0640,
		ModTime: record.CreatedAt,
	}

	// Escribir header
	if err := tarWriter.WriteHeader(header); err != nil {
		return fmt.Errorf("failed to write tar header: %w", err)
	}

	// Escribir contenido
	if _, err := tarWriter.Write(recordJSON); err != nil {
		return fmt.Errorf("failed to write record data: %w", err)
	}

	return nil
}

// deleteExpiredRecords elimina registros que exceden el período de retención
func (rm *RetentionManager) deleteExpiredRecords(threshold time.Time) (int, error) {
	rm.logger.Info("🗑️ Deleting expired legal audit records", "threshold", threshold.Format("2006-01-02"))

	// Eliminar registros principales expirados
	result := rm.db.Where("created_at < ?", threshold).Delete(&LegalAuditLog{})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to delete expired records: %w", result.Error)
	}

	deletedCount := int(result.RowsAffected)
	
	rm.logger.Info("Expired records deleted", "count", deletedCount)
	return deletedCount, nil
}

// cleanOldArchives limpia archivos de archivo antiguos
func (rm *RetentionManager) cleanOldArchives(threshold time.Time) (int, error) {
	rm.logger.Info("🧹 Cleaning old archive files", "threshold", threshold.Format("2006-01-02"))

	// Obtener registros de archivo expirados
	var expiredArchives []ArchiveRecord
	if err := rm.db.Where("archive_date < ?", threshold).Find(&expiredArchives).Error; err != nil {
		return 0, fmt.Errorf("failed to find expired archives: %w", err)
	}

	cleanedCount := 0
	for _, archive := range expiredArchives {
		// Eliminar archivo físico
		if err := os.Remove(archive.ArchivePath); err != nil && !os.IsNotExist(err) {
			rm.logger.Error("Failed to delete archive file", "path", archive.ArchivePath, "error", err)
			continue
		}

		// Eliminar registro de archivo
		if err := rm.db.Delete(&archive).Error; err != nil {
			rm.logger.Error("Failed to delete archive record", "id", archive.ID, "error", err)
			continue
		}

		cleanedCount++
	}

	rm.logger.Info("Old archives cleaned", "count", cleanedCount)
	return cleanedCount, nil
}

// createArchiveDirectories crea estructura de directorios para archivos
func (rm *RetentionManager) createArchiveDirectories() error {
	// Crear directorio base
	if err := os.MkdirAll(rm.config.ArchivePath, 0750); err != nil {
		return fmt.Errorf("failed to create archive base directory: %w", err)
	}

	// Crear directorios para años actuales y futuros
	currentYear := time.Now().Year()
	for year := currentYear - 1; year <= currentYear + 1; year++ {
		yearDir := filepath.Join(rm.config.ArchivePath, fmt.Sprintf("%d", year))
		if err := os.MkdirAll(yearDir, 0750); err != nil {
			rm.logger.Warn("Failed to create year directory", "year", year, "error", err)
		}
	}

	return nil
}

// RestoreFromArchive restaura registros desde archivo
func (rm *RetentionManager) RestoreFromArchive(archiveID uuid.UUID) ([]LegalAuditLog, error) {
	rm.logger.Info("🔄 Restoring records from archive", "archive_id", archiveID)

	// Obtener información del archivo
	var archiveRecord ArchiveRecord
	if err := rm.db.Where("id = ?", archiveID).First(&archiveRecord).Error; err != nil {
		return nil, fmt.Errorf("archive record not found: %w", err)
	}

	// Verificar que el archivo existe
	if _, err := os.Stat(archiveRecord.ArchivePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("archive file not found: %s", archiveRecord.ArchivePath)
	}

	// Descifrar archivo si está cifrado
	var readPath string
	if archiveRecord.Encrypted {
		tempDir := filepath.Join(rm.config.ArchivePath, "temp")
		os.MkdirAll(tempDir, 0750)
		
		tempPath := filepath.Join(tempDir, fmt.Sprintf("restore_%s.tar.gz", archiveID.String()))
		if err := rm.encryptor.DecryptExportFile(archiveRecord.ArchivePath, tempPath); err != nil {
			return nil, fmt.Errorf("failed to decrypt archive: %w", err)
		}
		defer os.Remove(tempPath)
		readPath = tempPath
	} else {
		readPath = archiveRecord.ArchivePath
	}

	// Leer archivo comprimido
	records, err := rm.readArchiveFile(readPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read archive file: %w", err)
	}

	rm.logger.Info("Records restored from archive", "count", len(records))
	return records, nil
}

// readArchiveFile lee registros desde archivo tar.gz
func (rm *RetentionManager) readArchiveFile(archivePath string) ([]LegalAuditLog, error) {
	// Abrir archivo
	file, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open archive file: %w", err)
	}
	defer file.Close()

	// Crear descompressor gzip
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzipReader.Close()

	// Crear lector tar
	tarReader := tar.NewReader(gzipReader)

	var records []LegalAuditLog

	// Leer cada archivo en el tar
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tar header: %w", err)
		}

		// Leer contenido del archivo
		content := make([]byte, header.Size)
		if _, err := io.ReadFull(tarReader, content); err != nil {
			rm.logger.Error("Failed to read record from archive", "file", header.Name, "error", err)
			continue
		}

		// Deserializar registro
		var record LegalAuditLog
		if err := json.Unmarshal(content, &record); err != nil {
			rm.logger.Error("Failed to unmarshal record", "file", header.Name, "error", err)
			continue
		}

		records = append(records, record)
	}

	return records, nil
}

// GetArchiveInfo obtiene información de archivos disponibles
func (rm *RetentionManager) GetArchiveInfo() ([]ArchiveInfo, error) {
	var archives []ArchiveRecord
	if err := rm.db.Order("archive_date DESC").Find(&archives).Error; err != nil {
		return nil, fmt.Errorf("failed to get archive records: %w", err)
	}

	result := make([]ArchiveInfo, len(archives))
	for i, archive := range archives {
		result[i] = ArchiveInfo{
			ID:             archive.ID,
			ArchivePath:    archive.ArchivePath,
			CompressedSize: archive.CompressedSize,
			OriginalSize:   archive.OriginalSize,
			Encrypted:      archive.Encrypted,
			ArchiveDate:    archive.ArchiveDate,
			RecordCount:    rm.countRecordsInArchive(archive.ArchivePath),
		}
	}

	return result, nil
}

// ArchiveInfo información de un archivo
type ArchiveInfo struct {
	ID             uuid.UUID `json:"id"`
	ArchivePath    string    `json:"archive_path"`
	CompressedSize int64     `json:"compressed_size"`
	OriginalSize   int64     `json:"original_size"`
	Encrypted      bool      `json:"encrypted"`
	ArchiveDate    time.Time `json:"archive_date"`
	RecordCount    int       `json:"record_count"`
}

// countRecordsInArchive cuenta registros en un archivo (estimación)
func (rm *RetentionManager) countRecordsInArchive(archivePath string) int {
	// Implementación básica - en producción se podría cachear esta información
	return 0 // Placeholder
}

// GetRetentionStatus obtiene estado actual de retención
func (rm *RetentionManager) GetRetentionStatus() (*RetentionStatus, error) {
	status := &RetentionStatus{
		Enabled:         rm.config.AutoArchive,
		RetentionYears:  rm.config.RetentionYears,
		ArchivePath:     rm.config.ArchivePath,
		LastArchiveRun:  time.Time{}, // TODO: Implementar tracking
	}

	// Obtener estadísticas de registros activos
	if err := rm.db.Model(&LegalAuditLog{}).Count(&status.ActiveRecords).Error; err != nil {
		rm.logger.Error("Failed to count active records", "error", err)
	}

	// Obtener estadísticas de registros archivados
	if err := rm.db.Model(&ArchiveRecord{}).Count(&status.ArchivedRecords).Error; err != nil {
		rm.logger.Error("Failed to count archived records", "error", err)
	}

	// Calcular tamaño de archivos
	var totalCompressedSize int64
	if err := rm.db.Model(&ArchiveRecord{}).Select("COALESCE(SUM(compressed_size), 0)").Scan(&totalCompressedSize).Error; err != nil {
		rm.logger.Error("Failed to calculate archive size", "error", err)
	}
	status.TotalArchiveSize = totalCompressedSize

	// Calcular registros que necesitan archivado
	archiveThreshold := time.Now().AddDate(0, -rm.config.RetentionYears*12/4, 0)
	if err := rm.db.Model(&LegalAuditLog{}).Where("created_at < ?", archiveThreshold).Count(&status.PendingArchive).Error; err != nil {
		rm.logger.Error("Failed to count pending archive records", "error", err)
	}

	return status, nil
}

// RetentionStatus estado del sistema de retención
type RetentionStatus struct {
	Enabled          bool      `json:"enabled"`
	RetentionYears   int       `json:"retention_years"`
	ArchivePath      string    `json:"archive_path"`
	ActiveRecords    int64     `json:"active_records"`
	ArchivedRecords  int64     `json:"archived_records"`
	PendingArchive   int64     `json:"pending_archive"`
	TotalArchiveSize int64     `json:"total_archive_size_bytes"`
	LastArchiveRun   time.Time `json:"last_archive_run"`
}