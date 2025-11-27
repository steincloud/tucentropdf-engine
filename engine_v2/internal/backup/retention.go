package backup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CleanOldBackups ejecuta limpieza según políticas de retención
func (s *Service) CleanOldBackups() error {
	s.logger.Info("🧹 Starting retention cleanup")

	start := time.Now()
	totalDeleted := 0
	totalFreed := int64(0)

	// Limpiar cada tipo de backup según su política
	deleted, freed, err := s.cleanBackupType("postgresql_full", s.backupConfig.RetentionFull)
	if err != nil {
		s.logger.Error("Failed to clean PostgreSQL full backups", "error", err)
	} else {
		totalDeleted += deleted
		totalFreed += freed
	}

	deleted, freed, err = s.cleanBackupType("postgresql_incremental", s.backupConfig.RetentionIncremental)
	if err != nil {
		s.logger.Error("Failed to clean PostgreSQL incremental backups", "error", err)
	} else {
		totalDeleted += deleted
		totalFreed += freed
	}

	deleted, freed, err = s.cleanBackupType("redis_snapshot", s.backupConfig.RetentionRedis)
	if err != nil {
		s.logger.Error("Failed to clean Redis backups", "error", err)
	} else {
		totalDeleted += deleted
		totalFreed += freed
	}

	deleted, freed, err = s.cleanBackupType("system_config", s.backupConfig.RetentionConfig)
	if err != nil {
		s.logger.Error("Failed to clean config backups", "error", err)
	} else {
		totalDeleted += deleted
		totalFreed += freed
	}

	deleted, freed, err = s.cleanBackupType("analytics_archive", s.backupConfig.RetentionAnalytics)
	if err != nil {
		s.logger.Error("Failed to clean analytics backups", "error", err)
	} else {
		totalDeleted += deleted
		totalFreed += freed
	}

	// Limpiar archivos temporales
	s.cleanTempFiles()

	// Limpiar registros antiguos de la base de datos
	s.cleanOldBackupRecords()

	// Limpiar backups remotos si está habilitado
	if s.backupConfig.RemoteEnabled {
		s.cleanRemoteBackups()
	}

	duration := time.Since(start)
	s.logger.Info("Retention cleanup completed",
		"files_deleted", totalDeleted,
		"space_freed_mb", totalFreed/(1024*1024),
		"duration", duration.String())

	return nil
}

// cleanBackupType limpia backups de un tipo específico según días de retención
func (s *Service) cleanBackupType(backupType string, retentionDays int) (deleted int, freed int64, err error) {
	s.logger.Debug("Cleaning backup type", "type", backupType, "retention_days", retentionDays)

	// Determinar directorio según tipo
	var dir string
	switch {
	case strings.Contains(backupType, "postgresql"):
		dir = filepath.Join(s.backupConfig.BackupDir, "postgresql")
	case backupType == "redis_snapshot":
		dir = filepath.Join(s.backupConfig.BackupDir, "redis")
	case backupType == "system_config":
		dir = filepath.Join(s.backupConfig.BackupDir, "config")
	case backupType == "analytics_archive":
		dir = filepath.Join(s.backupConfig.BackupDir, "analytics")
	default:
		return 0, 0, fmt.Errorf("unknown backup type: %s", backupType)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return 0, 0, nil // Directorio no existe, no hay nada que limpiar
	}

	// Calcular fecha de corte
	cutoffTime := time.Now().AddDate(0, 0, -retentionDays)

	// Leer archivos del directorio
	files, err := os.ReadDir(dir)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	var filesToDelete []string
	
	// Identificar archivos para eliminar
	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filePath := filepath.Join(dir, file.Name())
		info, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		// Verificar si el archivo es más antiguo que la política de retención
		if info.ModTime().Before(cutoffTime) {
			// Verificar que coincida con el patrón del tipo de backup
			if s.matchesBackupType(file.Name(), backupType) {
				filesToDelete = append(filesToDelete, filePath)
			}
		}
	}

	// Eliminar archivos identificados
	for _, filePath := range filesToDelete {
		// Obtener tamaño antes de eliminar
		if info, err := os.Stat(filePath); err == nil {
			freed += info.Size()
		}

		// Eliminar archivo local
		if err := os.Remove(filePath); err != nil {
			s.logger.Error("Failed to delete backup file", "file", filePath, "error", err)
			continue
		}

		deleted++
		filename := filepath.Base(filePath)
		s.logger.Debug("Deleted old backup", "file", filename, "type", backupType)

		// Marcar como eliminado en la base de datos
		s.markBackupDeleted(filename, backupType)

		// Eliminar del remoto si está habilitado
		if s.backupConfig.RemoteEnabled {
			remotePath := s.backupConfig.RemotePath + filename
			if err := s.rclone.DeleteRemoteFile(remotePath); err != nil {
				s.logger.Error("Failed to delete remote backup file", "file", filename, "error", err)
			}
		}
	}

	return deleted, freed, nil
}

// matchesBackupType verifica si un archivo coincide con el patrón del tipo de backup
func (s *Service) matchesBackupType(filename, backupType string) bool {
	filename = strings.ToLower(filename)
	
	switch backupType {
	case "postgresql_full":
		return strings.Contains(filename, "postgresql_full") || strings.Contains(filename, "pg_full")
	case "postgresql_incremental":
		return strings.Contains(filename, "postgresql_incremental") || strings.Contains(filename, "pg_incremental")
	case "redis_snapshot":
		return strings.Contains(filename, "redis_snapshot") || strings.Contains(filename, "redis_")
	case "system_config":
		return strings.Contains(filename, "system_config") || strings.Contains(filename, "config_")
	case "analytics_archive":
		return strings.Contains(filename, "analytics_archive") || strings.Contains(filename, "analytics_")
	default:
		return false
	}
}

// cleanTempFiles limpia archivos temporales
func (s *Service) cleanTempFiles() {
	s.logger.Debug("Cleaning temporary files")

	tempDir := s.backupConfig.TempDir
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		return
	}

	files, err := os.ReadDir(tempDir)
	if err != nil {
		s.logger.Error("Failed to read temp directory", "error", err)
		return
	}

	deleted := 0
	cutoffTime := time.Now().Add(-24 * time.Hour) // Archivos temporales de más de 24 horas

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filePath := filepath.Join(tempDir, file.Name())
		info, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		// Eliminar archivos temporales antiguos
		if info.ModTime().Before(cutoffTime) {
			if err := os.Remove(filePath); err != nil {
				s.logger.Error("Failed to delete temp file", "file", filePath, "error", err)
			} else {
				deleted++
			}
		}
	}

	if deleted > 0 {
		s.logger.Debug("Cleaned temporary files", "count", deleted)
	}
}

// cleanOldBackupRecords limpia registros antiguos de la tabla de backups
func (s *Service) cleanOldBackupRecords() {
	if s.db == nil {
		return
	}

	s.logger.Debug("Cleaning old backup records")

	// Mantener registros por máximo 1 año
	cutoffTime := time.Now().AddDate(-1, 0, 0)

	result := s.db.Exec("DELETE FROM system_backups WHERE created_at < ?", cutoffTime)
	if result.Error != nil {
		s.logger.Error("Failed to clean old backup records", "error", result.Error)
		return
	}

	if result.RowsAffected > 0 {
		s.logger.Debug("Cleaned old backup records", "count", result.RowsAffected)
	}
}

// cleanRemoteBackups limpia backups remotos según políticas
func (s *Service) cleanRemoteBackups() {
	s.logger.Debug("Cleaning remote backups")

	// Obtener lista de backups remotos
	remoteFiles, err := s.rclone.ListRemoteBackups()
	if err != nil {
		s.logger.Error("Failed to list remote backups for cleanup", "error", err)
		return
	}

	// Analizar cada archivo y aplicar políticas de retención
	for _, filename := range remoteFiles {
		shouldDelete, backupType := s.shouldDeleteRemoteBackup(filename)
		if shouldDelete {
			remotePath := s.backupConfig.RemotePath + filename
			if err := s.rclone.DeleteRemoteFile(remotePath); err != nil {
				s.logger.Error("Failed to delete remote backup", "file", filename, "error", err)
			} else {
				s.logger.Debug("Deleted remote backup", "file", filename, "type", backupType)
			}
		}
	}
}

// shouldDeleteRemoteBackup determina si un backup remoto debe ser eliminado
func (s *Service) shouldDeleteRemoteBackup(filename string) (bool, string) {
	// Intentar extraer fecha del nombre del archivo
	timestamp, backupType := s.parseBackupFilename(filename)
	if timestamp.IsZero() {
		return false, ""
	}

	// Determinar política de retención según el tipo
	var retentionDays int
	switch {
	case strings.Contains(backupType, "full"):
		retentionDays = s.backupConfig.RetentionFull
	case strings.Contains(backupType, "incremental"):
		retentionDays = s.backupConfig.RetentionIncremental
	case strings.Contains(backupType, "redis"):
		retentionDays = s.backupConfig.RetentionRedis
	case strings.Contains(backupType, "config"):
		retentionDays = s.backupConfig.RetentionConfig
	case strings.Contains(backupType, "analytics"):
		retentionDays = s.backupConfig.RetentionAnalytics
	default:
		return false, ""
	}

	cutoffTime := time.Now().AddDate(0, 0, -retentionDays)
	return timestamp.Before(cutoffTime), backupType
}

// parseBackupFilename extrae timestamp y tipo de un nombre de archivo de backup
func (s *Service) parseBackupFilename(filename string) (time.Time, string) {
	// Patrones comunes de nombres de backup:
	// postgresql_full_20250115_143022.sql.enc
	// redis_snapshot_20250115_143022.rdb.enc
	// system_config_20250115_143022.tar.gz.enc

	parts := strings.Split(filename, "_")
	if len(parts) < 3 {
		return time.Time{}, ""
	}

	// Intentar parsear diferentes formatos de fecha
	var timestamp time.Time
	var err error
	
	// Formato: YYYYMMDD_HHMMSS
	if len(parts) >= 3 {
		dateStr := parts[len(parts)-2] + "_" + strings.Split(parts[len(parts)-1], ".")[0]
		timestamp, err = time.Parse("20060102_150405", dateStr)
		if err != nil {
			// Intentar otros formatos
			timestamp, err = time.Parse("20060102", parts[len(parts)-2])
		}
	}

	if err != nil {
		return time.Time{}, ""
	}

	// Determinar tipo de backup
	backupType := strings.Join(parts[:len(parts)-2], "_")
	
	return timestamp, backupType
}

// markBackupDeleted marca un backup como eliminado en la base de datos
func (s *Service) markBackupDeleted(filename, backupType string) {
	if s.db == nil {
		return
	}

	// Agregar nota de eliminación al registro
	result := s.db.Exec(`
		UPDATE system_backups 
		SET error_message = COALESCE(error_message || '; ', '') || 'Deleted by retention policy at ' || NOW()::text
		WHERE filename = ? AND type = ?`,
		filename, backupType)

	if result.Error != nil {
		s.logger.Error("Failed to mark backup as deleted", "file", filename, "error", result.Error)
	}
}

// GetRetentionReport genera reporte de estado de retención
func (s *Service) GetRetentionReport() (*RetentionReport, error) {
	report := &RetentionReport{
		Generated: time.Now(),
		Policies:  make(map[string]RetentionPolicy),
		Status:    make(map[string]BackupTypeStatus),
	}

	// Definir políticas configuradas
	report.Policies["postgresql_full"] = RetentionPolicy{
		Type:           "postgresql_full",
		RetentionDays:  s.backupConfig.RetentionFull,
		Directory:      filepath.Join(s.backupConfig.BackupDir, "postgresql"),
	}
	report.Policies["postgresql_incremental"] = RetentionPolicy{
		Type:           "postgresql_incremental", 
		RetentionDays:  s.backupConfig.RetentionIncremental,
		Directory:      filepath.Join(s.backupConfig.BackupDir, "postgresql"),
	}
	report.Policies["redis_snapshot"] = RetentionPolicy{
		Type:           "redis_snapshot",
		RetentionDays:  s.backupConfig.RetentionRedis,
		Directory:      filepath.Join(s.backupConfig.BackupDir, "redis"),
	}
	report.Policies["system_config"] = RetentionPolicy{
		Type:           "system_config",
		RetentionDays:  s.backupConfig.RetentionConfig,
		Directory:      filepath.Join(s.backupConfig.BackupDir, "config"),
	}
	report.Policies["analytics_archive"] = RetentionPolicy{
		Type:           "analytics_archive",
		RetentionDays:  s.backupConfig.RetentionAnalytics,
		Directory:      filepath.Join(s.backupConfig.BackupDir, "analytics"),
	}

	// Analizar estado para cada tipo
	for backupType, policy := range report.Policies {
		status, err := s.analyzeBackupTypeStatus(policy)
		if err != nil {
			s.logger.Error("Failed to analyze backup type", "type", backupType, "error", err)
			continue
		}
		report.Status[backupType] = status
	}

	return report, nil
}

// analyzeBackupTypeStatus analiza el estado de un tipo de backup
func (s *Service) analyzeBackupTypeStatus(policy RetentionPolicy) (BackupTypeStatus, error) {
	status := BackupTypeStatus{
		Type:              policy.Type,
		TotalBackups:      0,
		ValidBackups:      0,
		ExpiredBackups:    0,
		TotalSize:         0,
		OldestBackup:      time.Now(),
		NewestBackup:      time.Time{},
		ExpiredFiles:      []string{},
	}

	// Verificar si el directorio existe
	if _, err := os.Stat(policy.Directory); os.IsNotExist(err) {
		return status, nil
	}

	// Leer archivos del directorio
	files, err := os.ReadDir(policy.Directory)
	if err != nil {
		return status, fmt.Errorf("failed to read directory: %w", err)
	}

	cutoffTime := time.Now().AddDate(0, 0, -policy.RetentionDays)

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		if !s.matchesBackupType(file.Name(), policy.Type) {
			continue
		}

		filePath := filepath.Join(policy.Directory, file.Name())
		info, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		status.TotalBackups++
		status.TotalSize += info.Size()

		// Actualizar fechas de backup más antiguo y más nuevo
		if info.ModTime().Before(status.OldestBackup) {
			status.OldestBackup = info.ModTime()
		}
		if info.ModTime().After(status.NewestBackup) {
			status.NewestBackup = info.ModTime()
		}

		// Verificar si el backup ha expirado
		if info.ModTime().Before(cutoffTime) {
			status.ExpiredBackups++
			status.ExpiredFiles = append(status.ExpiredFiles, file.Name())
		} else {
			status.ValidBackups++
		}
	}

	return status, nil
}

// RetentionReport estructura del reporte de retención
type RetentionReport struct {
	Generated time.Time                      `json:"generated"`
	Policies  map[string]RetentionPolicy     `json:"policies"`
	Status    map[string]BackupTypeStatus    `json:"status"`
}

// RetentionPolicy define una política de retención
type RetentionPolicy struct {
	Type          string `json:"type"`
	RetentionDays int    `json:"retention_days"`
	Directory     string `json:"directory"`
}

// BackupTypeStatus estado de backups para un tipo específico
type BackupTypeStatus struct {
	Type           string    `json:"type"`
	TotalBackups   int       `json:"total_backups"`
	ValidBackups   int       `json:"valid_backups"`
	ExpiredBackups int       `json:"expired_backups"`
	TotalSize      int64     `json:"total_size_bytes"`
	OldestBackup   time.Time `json:"oldest_backup"`
	NewestBackup   time.Time `json:"newest_backup"`
	ExpiredFiles   []string  `json:"expired_files"`
}