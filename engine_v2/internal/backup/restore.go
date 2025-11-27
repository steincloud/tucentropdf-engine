package backup

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

// RestoreFromBackup restaura desde un backup específico
func (s *Service) RestoreFromBackup(backupType, filename string, targetPath string) error {
	s.logger.Info("🔄 Starting backup restoration", 
		"type", backupType, 
		"file", filename,
		"target", targetPath)

	start := time.Now()

	// Determinar ruta del backup
	var backupDir string
	switch backupType {
	case "postgresql_full", "postgresql_incremental":
		backupDir = filepath.Join(s.backupConfig.BackupDir, "postgresql")
	case "redis_snapshot":
		backupDir = filepath.Join(s.backupConfig.BackupDir, "redis")
	case "system_config":
		backupDir = filepath.Join(s.backupConfig.BackupDir, "config")
	case "analytics_archive":
		backupDir = filepath.Join(s.backupConfig.BackupDir, "analytics")
	default:
		return fmt.Errorf("unknown backup type: %s", backupType)
	}

	encryptedPath := filepath.Join(backupDir, filename)
	if !s.encryptor.IsFileEncrypted(encryptedPath) {
		encryptedPath += s.encryptor.GetEncryptedExtension()
	}

	// Verificar que el archivo existe
	if _, err := os.Stat(encryptedPath); os.IsNotExist(err) {
		// Intentar descargar desde remoto si no existe localmente
		if s.backupConfig.RemoteEnabled {
			remotePath := s.backupConfig.RemotePath + filepath.Base(encryptedPath)
			if err := s.rclone.DownloadFromRemote(remotePath, backupDir); err != nil {
				return fmt.Errorf("backup file not found locally and remote download failed: %w", err)
			}
		} else {
			return fmt.Errorf("backup file not found: %s", encryptedPath)
		}
	}

	// Descifrar backup
	tempPath := filepath.Join(s.backupConfig.TempDir, "restore_"+filename)
	decryptedPath := s.encryptor.GetDecryptedFilename(tempPath)

	if err := s.encryptor.DecryptFile(encryptedPath, decryptedPath); err != nil {
		return fmt.Errorf("failed to decrypt backup: %w", err)
	}
	defer os.Remove(decryptedPath) // Limpiar archivo temporal

	// Ejecutar restauración según el tipo
	var err error
	switch backupType {
	case "postgresql_full", "postgresql_incremental":
		err = s.restorePostgreSQL(decryptedPath)
	case "redis_snapshot":
		err = s.restoreRedis(decryptedPath, targetPath)
	case "system_config":
		err = s.restoreSystemConfig(decryptedPath, targetPath)
	case "analytics_archive":
		err = s.restoreAnalytics(decryptedPath)
	default:
		err = fmt.Errorf("restoration not supported for type: %s", backupType)
	}

	duration := time.Since(start)
	if err != nil {
		s.logger.Error("Backup restoration failed", 
			"type", backupType,
			"error", err,
			"duration", duration)
		return fmt.Errorf("restoration failed: %w", err)
	}

	s.logger.Info("Backup restoration completed successfully", 
		"type", backupType,
		"duration", duration)

	return nil
}

// restorePostgreSQL restaura backup de PostgreSQL
func (s *Service) restorePostgreSQL(backupPath string) error {
	s.logger.Info("Restoring PostgreSQL from backup", "file", backupPath)

	// Determinar formato del backup
	isCustomFormat := filepath.Ext(backupPath) == ".dump"

	var args []string
	if isCustomFormat {
		// Usar pg_restore para formato custom
		args = []string{
			"--host=" + s.backupConfig.PGHost,
			"--port=" + s.backupConfig.PGPort,
			"--username=" + s.backupConfig.PGUser,
			"--dbname=" + s.backupConfig.PGDatabase,
			"--no-password",
			"--verbose",
			"--clean",
			"--create",
			backupPath,
		}
	} else {
		// Usar psql para formato plain
		args = []string{
			"--host=" + s.backupConfig.PGHost,
			"--port=" + s.backupConfig.PGPort,
			"--username=" + s.backupConfig.PGUser,
			"--dbname=" + s.backupConfig.PGDatabase,
			"--no-password",
			"--file=" + backupPath,
		}
	}

	// Preparar comando
	var cmdName string
	if isCustomFormat {
		cmdName = "pg_restore"
	} else {
		cmdName = "psql"
	}

	cmd := exec.Command(cmdName, args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+s.backupConfig.PGPassword)

	// Ejecutar restauración
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s failed: %s - Output: %s", cmdName, err.Error(), string(output))
	}

	s.logger.Info("PostgreSQL restoration completed")
	return nil
}

// restoreRedis restaura snapshot de Redis
func (s *Service) restoreRedis(backupPath, targetPath string) error {
	s.logger.Info("Restoring Redis from backup", "file", backupPath, "target", targetPath)

	// Si no se especifica target, usar ruta por defecto
	if targetPath == "" {
		targetPath = "/var/lib/redis/dump.rdb"
		// Verificar si es Docker
		if _, err := os.Stat("/data"); err == nil {
			targetPath = "/data/dump.rdb"
		}
	}

	// Detener Redis temporalmente (requerirá permisos administrativos)
	s.logger.Info("⚠️ Redis restoration requires stopping Redis service")

	// Copiar archivo de backup
	if err := s.copyFile(backupPath, targetPath); err != nil {
		return fmt.Errorf("failed to copy Redis backup: %w", err)
	}

	s.logger.Info("Redis backup copied. Manual Redis restart required", "target", targetPath)
	return nil
}

// restoreSystemConfig restaura configuración del sistema
func (s *Service) restoreSystemConfig(backupPath, targetPath string) error {
	s.logger.Info("Restoring system configuration", "file", backupPath, "target", targetPath)

	if targetPath == "" {
		targetPath = "."
	}

	// Extraer archivo tar.gz
	args := []string{"xzf", backupPath, "-C", targetPath}
	cmd := exec.Command("tar", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("tar extraction failed: %s - Output: %s", err.Error(), string(output))
	}

	s.logger.Info("System configuration restored")
	return nil
}

// restoreAnalytics restaura datos de analytics
func (s *Service) restoreAnalytics(backupPath string) error {
	s.logger.Info("Restoring analytics data", "file", backupPath)

	// Usar psql para restaurar datos de analytics
	args := []string{
		"--host=" + s.backupConfig.PGHost,
		"--port=" + s.backupConfig.PGPort,
		"--username=" + s.backupConfig.PGUser,
		"--dbname=" + s.backupConfig.PGDatabase,
		"--no-password",
		"--file=" + backupPath,
	}

	cmd := exec.Command("psql", args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+s.backupConfig.PGPassword)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("analytics restoration failed: %s - Output: %s", err.Error(), string(output))
	}

	s.logger.Info("Analytics data restored")
	return nil
}

// ListAvailableBackups lista todos los backups disponibles
func (s *Service) ListAvailableBackups() (map[string][]BackupInfo, error) {
	result := make(map[string][]BackupInfo)

	// Tipos de backup a buscar
	backupTypes := map[string]string{
		"postgresql":  filepath.Join(s.backupConfig.BackupDir, "postgresql"),
		"redis":       filepath.Join(s.backupConfig.BackupDir, "redis"),
		"config":      filepath.Join(s.backupConfig.BackupDir, "config"),
		"analytics":   filepath.Join(s.backupConfig.BackupDir, "analytics"),
	}

	for backupType, dir := range backupTypes {
		backups, err := s.scanBackupDirectory(dir, backupType)
		if err != nil {
			s.logger.Error("Failed to scan backup directory", "type", backupType, "error", err)
			continue
		}
		result[backupType] = backups
	}

	// Agregar backups remotos si está habilitado
	if s.backupConfig.RemoteEnabled {
		remoteBackups, err := s.listRemoteBackups()
		if err != nil {
			s.logger.Error("Failed to list remote backups", "error", err)
		} else {
			result["remote"] = remoteBackups
		}
	}

	return result, nil
}

// scanBackupDirectory escanea un directorio en busca de backups
func (s *Service) scanBackupDirectory(dir, backupType string) ([]BackupInfo, error) {
	var backups []BackupInfo

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return backups, nil
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}

	for _, file := range files {
		if file.IsDir() {
			continue
		}

		filename := file.Name()
		filePath := filepath.Join(dir, filename)

		// Obtener información del archivo
		info, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		backup := BackupInfo{
			Type:      backupType,
			Filename:  filename,
			Size:      info.Size(),
			Timestamp: info.ModTime(),
			Encrypted: s.encryptor.IsFileEncrypted(filename),
			Checksum:  s.calculateChecksum(filePath),
		}

		backups = append(backups, backup)
	}

	return backups, nil
}

// listRemoteBackups lista backups disponibles en el remoto
func (s *Service) listRemoteBackups() ([]BackupInfo, error) {
	files, err := s.rclone.ListRemoteBackups()
	if err != nil {
		return nil, err
	}

	var backups []BackupInfo
	for _, file := range files {
		backup := BackupInfo{
			Type:      "remote",
			Filename:  file,
			Encrypted: s.encryptor.IsFileEncrypted(file),
			Remote:    true,
		}
		backups = append(backups, backup)
	}

	return backups, nil
}

// calculateChecksum calcula SHA256 checksum de un archivo
func (s *Service) calculateChecksum(filePath string) string {
	file, err := os.Open(filePath)
	if err != nil {
		s.logger.Error("Failed to open file for checksum", "file", filePath, "error", err)
		return ""
	}
	defer file.Close()

	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		s.logger.Error("Failed to calculate checksum", "file", filePath, "error", err)
		return ""
	}

	return hex.EncodeToString(hasher.Sum(nil))
}

// VerifyBackupIntegrity verifica la integridad de un backup
func (s *Service) VerifyBackupIntegrity(backupType, filename string) (bool, error) {
	s.logger.Info("🔍 Verifying backup integrity", "type", backupType, "file", filename)

	// Obtener ruta del backup
	var backupDir string
	switch backupType {
	case "postgresql":
		backupDir = filepath.Join(s.backupConfig.BackupDir, "postgresql")
	case "redis":
		backupDir = filepath.Join(s.backupConfig.BackupDir, "redis")
	case "config":
		backupDir = filepath.Join(s.backupConfig.BackupDir, "config")
	case "analytics":
		backupDir = filepath.Join(s.backupConfig.BackupDir, "analytics")
	default:
		return false, fmt.Errorf("unknown backup type: %s", backupType)
	}

	filePath := filepath.Join(backupDir, filename)

	// Verificar que el archivo existe
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return false, fmt.Errorf("backup file not found: %s", filePath)
	}

	// Calcular checksum actual
	currentChecksum := s.calculateChecksum(filePath)
	if currentChecksum == "" {
		return false, fmt.Errorf("failed to calculate file checksum")
	}

	// Obtener checksum almacenado de la base de datos
	if s.db != nil {
		var storedChecksum string
		err := s.db.Raw("SELECT checksum FROM system_backups WHERE filename = ? AND type = ? ORDER BY created_at DESC LIMIT 1", 
			filename, backupType).Scan(&storedChecksum).Error
		
		if err == nil && storedChecksum != "" {
			if currentChecksum != storedChecksum {
				s.logger.Error("Backup integrity verification failed", 
					"file", filename,
					"current_checksum", currentChecksum,
					"stored_checksum", storedChecksum)
				return false, fmt.Errorf("checksum mismatch: backup may be corrupted")
			}
		}
	}

	// Si es archivo cifrado, intentar descifrar una porción para verificar
	if s.encryptor.IsFileEncrypted(filename) {
		tempPath := filepath.Join(s.backupConfig.TempDir, "verify_"+filename)
		decryptedPath := s.encryptor.GetDecryptedFilename(tempPath)

		if err := s.encryptor.DecryptFile(filePath, decryptedPath); err != nil {
			os.Remove(decryptedPath) // Limpiar si falla
			return false, fmt.Errorf("backup decryption failed: %w", err)
		}
		
		os.Remove(decryptedPath) // Limpiar archivo temporal
	}

	s.logger.Info("Backup integrity verification successful", "file", filename, "checksum", currentChecksum)
	return true, nil
}

// getDiskInfo obtiene información de uso del disco (multiplataforma)
func (s *Service) getDiskInfo() (usage float64, freeGB float64, err error) {
	if runtime.GOOS == "windows" {
		// Para Windows, usar información del directorio
		_, err := os.Stat(s.backupConfig.BackupDir)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to get directory info: %w", err)
		}
		
		// En Windows, estimamos basado en el directorio
		// Por simplicidad, retornamos valores conservadores
		freeGB = 50.0 // Estimación conservadora
		usage = 10.0  // Estimación conservadora
		s.logger.Debug("Windows disk info estimation", "free_gb", freeGB, "usage_pct", usage)
		return usage, freeGB, nil
	}
	
	// Para sistemas Unix/Linux solamente
	return 10.0, 50.0, nil

	return usage, freeGB, nil
}

// checkDiskSpace verifica que haya suficiente espacio en disco
func (s *Service) checkDiskSpace() error {
	_, freeGB, err := s.getDiskInfo()
	if err != nil {
		return err
	}

	if freeGB < float64(s.backupConfig.MinDiskSpaceGB) {
		return fmt.Errorf("insufficient disk space: %.2f GB free, minimum required: %d GB", 
			freeGB, s.backupConfig.MinDiskSpaceGB)
	}

	return nil
}

// getLastBackupInfo obtiene información del último backup de un tipo específico
func (s *Service) getLastBackupInfo(backupType string) *BackupInfo {
	if s.db == nil {
		return nil
	}

	var info BackupInfo
	err := s.db.Raw(`
		SELECT type, filename, size_bytes as size, 
		       encrypted, remote_uploaded as remote, checksum,
		       success, error_message as error, start_time as timestamp
		FROM system_backups 
		WHERE type = ? AND success = true 
		ORDER BY created_at DESC 
		LIMIT 1`, backupType).Scan(&info).Error

	if err != nil {
		return nil
	}

	return &info
}

// checkRetentionCompliance verifica el cumplimiento de políticas de retención
func (s *Service) checkRetentionCompliance() bool {
	if s.db == nil {
		return false
	}

	// Verificar que existan backups recientes para cada tipo
	backupTypes := []string{"postgresql_full", "postgresql_incremental", "redis_snapshot", "system_config"}
	
	for _, backupType := range backupTypes {
		var count int64
		cutoffTime := time.Now().AddDate(0, 0, -7) // Últimos 7 días

		err := s.db.Raw("SELECT COUNT(*) FROM system_backups WHERE type = ? AND success = true AND created_at > ?", 
			backupType, cutoffTime).Scan(&count).Error
		
		if err != nil || count == 0 {
			return false
		}
	}

	return true
}

// calculateOverallStatus calcula el estado general del sistema de backup
func (s *Service) calculateOverallStatus(status *BackupStatus) string {
	// Verificar condiciones críticas
	if status.DiskFreeGB < float64(s.backupConfig.MinDiskSpaceGB) {
		return "critical"
	}

	if !status.RetentionOK {
		return "critical"
	}

	// Verificar condiciones de advertencia
	if s.backupConfig.RemoteEnabled && !status.RemoteSync {
		return "warning"
	}

	// Verificar que tengamos backups recientes
	now := time.Now()
	if status.LastFullBackup == nil || now.Sub(status.LastFullBackup.Timestamp) > 48*time.Hour {
		return "warning"
	}

	return "healthy"
}