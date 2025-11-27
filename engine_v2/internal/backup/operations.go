package backup

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// BackupPostgreSQL ejecuta backup completo de PostgreSQL
func (s *Service) FullBackupPostgreSQL() error {
	s.logger.Info("🗃️ Starting PostgreSQL full backup")
	
	start := time.Now()
	timestamp := start.Format("20060102_150405")
	filename := fmt.Sprintf("postgresql_full_%s.sql", timestamp)
	backupPath := filepath.Join(s.backupConfig.BackupDir, "postgresql", filename)
	encryptedPath := backupPath + s.encryptor.GetEncryptedExtension()

	// Registrar inicio del backup
	backupInfo := &BackupInfo{
		Type:      "postgresql_full",
		Filename:  filename,
		Timestamp: start,
		Encrypted: true,
	}

	// Ejecutar pg_dump
	if err := s.executePGDump(backupPath, true); err != nil {
		backupInfo.Success = false
		backupInfo.Error = err.Error()
		s.recordBackup(backupInfo)
		return fmt.Errorf("pg_dump failed: %w", err)
	}

	// Cifrar backup
	if err := s.encryptor.EncryptFile(backupPath, encryptedPath); err != nil {
		backupInfo.Success = false
		backupInfo.Error = fmt.Sprintf("encryption failed: %v", err)
		s.recordBackup(backupInfo)
		os.Remove(backupPath) // Limpiar archivo sin cifrar
		return fmt.Errorf("backup encryption failed: %w", err)
	}

	// Remover archivo sin cifrar
	os.Remove(backupPath)

	// Obtener información del archivo
	if stat, err := os.Stat(encryptedPath); err == nil {
		backupInfo.Size = stat.Size()
	}

	// Calcular checksum
	backupInfo.Checksum = s.calculateChecksum(encryptedPath)

	// Sincronizar con remoto si está habilitado
	if s.backupConfig.RemoteEnabled {
		if _, err := s.rclone.SyncToRemote(filepath.Dir(encryptedPath)); err != nil {
			s.logger.Error("Failed to sync PostgreSQL backup to remote", "error", err)
			// No fallar el backup por esto
		} else {
			backupInfo.Remote = true
		}
	}

	backupInfo.Success = true
	s.recordBackup(backupInfo)

	duration := time.Since(start)
	s.logger.Info("PostgreSQL full backup completed", 
		"file", filename, 
		"size", backupInfo.Size,
		"duration", duration.String())

	return nil
}

// IncrementalBackupPostgreSQL ejecuta backup incremental usando WAL
func (s *Service) IncrementalBackupPostgreSQL() error {
	s.logger.Info("📈 Starting PostgreSQL incremental backup")

	start := time.Now()
	timestamp := start.Format("20060102_150405")
	filename := fmt.Sprintf("postgresql_incremental_%s.sql", timestamp)
	backupPath := filepath.Join(s.backupConfig.BackupDir, "postgresql", filename)
	encryptedPath := backupPath + s.encryptor.GetEncryptedExtension()

	// Registrar inicio del backup
	backupInfo := &BackupInfo{
		Type:      "postgresql_incremental",
		Filename:  filename,
		Timestamp: start,
		Encrypted: true,
	}

	// Para backup incremental, usar pg_dump con filtros por fecha
	if err := s.executePGDump(backupPath, false); err != nil {
		backupInfo.Success = false
		backupInfo.Error = err.Error()
		s.recordBackup(backupInfo)
		return fmt.Errorf("incremental pg_dump failed: %w", err)
	}

	// Cifrar backup
	if err := s.encryptor.EncryptFile(backupPath, encryptedPath); err != nil {
		backupInfo.Success = false
		backupInfo.Error = fmt.Sprintf("encryption failed: %v", err)
		s.recordBackup(backupInfo)
		os.Remove(backupPath)
		return fmt.Errorf("backup encryption failed: %w", err)
	}

	// Remover archivo sin cifrar
	os.Remove(backupPath)

	// Obtener información del archivo
	if stat, err := os.Stat(encryptedPath); err == nil {
		backupInfo.Size = stat.Size()
	}

	// Calcular checksum
	backupInfo.Checksum = s.calculateChecksum(encryptedPath)

	// Sincronizar con remoto
	if s.backupConfig.RemoteEnabled {
		if _, err := s.rclone.SyncToRemote(filepath.Dir(encryptedPath)); err != nil {
			s.logger.Error("Failed to sync incremental backup to remote", "error", err)
		} else {
			backupInfo.Remote = true
		}
	}

	backupInfo.Success = true
	s.recordBackup(backupInfo)

	duration := time.Since(start)
	s.logger.Info("PostgreSQL incremental backup completed", 
		"file", filename,
		"size", backupInfo.Size,
		"duration", duration.String())

	return nil
}

// BackupRedisSnapshot crea backup del snapshot de Redis
func (s *Service) BackupRedisSnapshot() error {
	s.logger.Info("🐎 Starting Redis snapshot backup")

	start := time.Now()
	timestamp := start.Format("20060102_150405")
	filename := fmt.Sprintf("redis_snapshot_%s.rdb", timestamp)
	backupPath := filepath.Join(s.backupConfig.BackupDir, "redis", filename)
	encryptedPath := backupPath + s.encryptor.GetEncryptedExtension()

	// Registrar inicio del backup
	backupInfo := &BackupInfo{
		Type:      "redis_snapshot",
		Filename:  filename,
		Timestamp: start,
		Encrypted: true,
	}

	// Ejecutar BGSAVE en Redis para crear snapshot
	if err := s.executeRedisBackup(backupPath); err != nil {
		backupInfo.Success = false
		backupInfo.Error = err.Error()
		s.recordBackup(backupInfo)
		return fmt.Errorf("redis backup failed: %w", err)
	}

	// Cifrar backup
	if err := s.encryptor.EncryptFile(backupPath, encryptedPath); err != nil {
		backupInfo.Success = false
		backupInfo.Error = fmt.Sprintf("encryption failed: %v", err)
		s.recordBackup(backupInfo)
		os.Remove(backupPath)
		return fmt.Errorf("redis backup encryption failed: %w", err)
	}

	// Remover archivo sin cifrar
	os.Remove(backupPath)

	// Obtener información del archivo
	if stat, err := os.Stat(encryptedPath); err == nil {
		backupInfo.Size = stat.Size()
	}

	// Calcular checksum
	backupInfo.Checksum = s.calculateChecksum(encryptedPath)

	// Sincronizar con remoto
	if s.backupConfig.RemoteEnabled {
		if _, err := s.rclone.SyncToRemote(filepath.Dir(encryptedPath)); err != nil {
			s.logger.Error("Failed to sync Redis backup to remote", "error", err)
		} else {
			backupInfo.Remote = true
		}
	}

	backupInfo.Success = true
	s.recordBackup(backupInfo)

	duration := time.Since(start)
	s.logger.Info("Redis backup completed", 
		"file", filename,
		"size", backupInfo.Size,
		"duration", duration.String())

	return nil
}

// BackupSystemConfig crea backup de configuración del sistema
func (s *Service) BackupSystemConfig() error {
	s.logger.Info("⚙️ Starting system configuration backup")

	start := time.Now()
	timestamp := start.Format("20060102_150405")
	filename := fmt.Sprintf("system_config_%s.tar.gz", timestamp)
	backupPath := filepath.Join(s.backupConfig.BackupDir, "config", filename)
	encryptedPath := backupPath + s.encryptor.GetEncryptedExtension()

	// Registrar inicio del backup
	backupInfo := &BackupInfo{
		Type:      "system_config",
		Filename:  filename,
		Timestamp: start,
		Encrypted: true,
	}

	// Crear backup de configuración
	if err := s.createConfigBackup(backupPath); err != nil {
		backupInfo.Success = false
		backupInfo.Error = err.Error()
		s.recordBackup(backupInfo)
		return fmt.Errorf("config backup failed: %w", err)
	}

	// Cifrar backup
	if err := s.encryptor.EncryptFile(backupPath, encryptedPath); err != nil {
		backupInfo.Success = false
		backupInfo.Error = fmt.Sprintf("encryption failed: %v", err)
		s.recordBackup(backupInfo)
		os.Remove(backupPath)
		return fmt.Errorf("config backup encryption failed: %w", err)
	}

	// Remover archivo sin cifrar
	os.Remove(backupPath)

	// Obtener información del archivo
	if stat, err := os.Stat(encryptedPath); err == nil {
		backupInfo.Size = stat.Size()
	}

	// Calcular checksum
	backupInfo.Checksum = s.calculateChecksum(encryptedPath)

	// Sincronizar con remoto
	if s.backupConfig.RemoteEnabled {
		if _, err := s.rclone.SyncToRemote(filepath.Dir(encryptedPath)); err != nil {
			s.logger.Error("Failed to sync config backup to remote", "error", err)
		} else {
			backupInfo.Remote = true
		}
	}

	backupInfo.Success = true
	s.recordBackup(backupInfo)

	duration := time.Since(start)
	s.logger.Info("System configuration backup completed", 
		"file", filename,
		"size", backupInfo.Size,
		"duration", duration.String())

	return nil
}

// BackupAnalyticsArchive crea backup mensual de analytics archivadas
func (s *Service) BackupAnalyticsArchive() error {
	s.logger.Info("📊 Starting analytics archive backup")

	start := time.Now()
	timestamp := start.Format("200601") // YYYYMM para backup mensual
	filename := fmt.Sprintf("analytics_archive_%s.sql", timestamp)
	backupPath := filepath.Join(s.backupConfig.BackupDir, "analytics", filename)
	encryptedPath := backupPath + s.encryptor.GetEncryptedExtension()

	// Registrar inicio del backup
	backupInfo := &BackupInfo{
		Type:      "analytics_archive",
		Filename:  filename,
		Timestamp: start,
		Encrypted: true,
	}

	// Crear backup de analytics
	if err := s.createAnalyticsBackup(backupPath); err != nil {
		backupInfo.Success = false
		backupInfo.Error = err.Error()
		s.recordBackup(backupInfo)
		return fmt.Errorf("analytics backup failed: %w", err)
	}

	// Cifrar backup
	if err := s.encryptor.EncryptFile(backupPath, encryptedPath); err != nil {
		backupInfo.Success = false
		backupInfo.Error = fmt.Sprintf("encryption failed: %v", err)
		s.recordBackup(backupInfo)
		os.Remove(backupPath)
		return fmt.Errorf("analytics backup encryption failed: %w", err)
	}

	// Remover archivo sin cifrar
	os.Remove(backupPath)

	// Obtener información del archivo
	if stat, err := os.Stat(encryptedPath); err == nil {
		backupInfo.Size = stat.Size()
	}

	// Calcular checksum
	backupInfo.Checksum = s.calculateChecksum(encryptedPath)

	// Sincronizar con remoto
	if s.backupConfig.RemoteEnabled {
		if _, err := s.rclone.SyncToRemote(filepath.Dir(encryptedPath)); err != nil {
			s.logger.Error("Failed to sync analytics backup to remote", "error", err)
		} else {
			backupInfo.Remote = true
		}
	}

	backupInfo.Success = true
	s.recordBackup(backupInfo)

	duration := time.Since(start)
	s.logger.Info("Analytics archive backup completed", 
		"file", filename,
		"size", backupInfo.Size,
		"duration", duration.String())

	return nil
}

// executePGDump ejecuta pg_dump para crear backup de PostgreSQL
func (s *Service) executePGDump(outputPath string, fullBackup bool) error {
	// Preparar comando pg_dump
	args := []string{
		"--host=" + s.backupConfig.PGHost,
		"--port=" + s.backupConfig.PGPort,
		"--username=" + s.backupConfig.PGUser,
		"--dbname=" + s.backupConfig.PGDatabase,
		"--no-password",
		"--verbose",
		"--clean",
		"--create",
		"--file=" + outputPath,
	}

	// Configuraciones específicas según el tipo de backup
	if fullBackup {
		args = append(args, "--format=custom")
		// Cambiar extensión para formato custom
		outputPath = strings.Replace(outputPath, ".sql", ".dump", 1)
		args[len(args)-1] = "--file=" + outputPath
	} else {
		// Para backup incremental, filtrar por fecha
		args = append(args, "--format=plain")
		// Se podría agregar filtros WHERE para tablas específicas
	}

	// Configurar variable de entorno para password
	cmd := exec.Command("pg_dump", args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+s.backupConfig.PGPassword)

	// Ejecutar comando
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pg_dump error: %s - Output: %s", err.Error(), string(output))
	}

	s.logger.Debug("pg_dump completed", "output_file", outputPath)
	return nil
}

// executeRedisBackup crea backup de Redis
func (s *Service) executeRedisBackup(outputPath string) error {
	// Usar redis-cli para ejecutar BGSAVE y luego copiar el dump.rdb
	args := []string{
		"-h", s.backupConfig.RedisHost,
		"-p", s.backupConfig.RedisPort,
		"BGSAVE",
	}

	// Agregar password si está configurado
	if s.backupConfig.RedisPassword != "" {
		args = append([]string{"-a", s.backupConfig.RedisPassword}, args...)
	}

	// Ejecutar BGSAVE
	cmd := exec.Command("redis-cli", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("redis BGSAVE failed: %s - Output: %s", err.Error(), string(output))
	}

	// Esperar a que termine el BGSAVE
	time.Sleep(5 * time.Second)

	// Encontrar y copiar el archivo dump.rdb
	// Nota: En producción se debería obtener la ruta desde Redis CONFIG GET dir
	redisDataDir := "/var/lib/redis" // Ruta típica en Linux
	if _, err := os.Stat("/data/dump.rdb"); err == nil {
		redisDataDir = "/data" // Docker Redis
	}

	dumpPath := filepath.Join(redisDataDir, "dump.rdb")
	if _, err := os.Stat(dumpPath); os.IsNotExist(err) {
		return fmt.Errorf("Redis dump file not found at %s", dumpPath)
	}

	// Copiar archivo
	if err := s.copyFile(dumpPath, outputPath); err != nil {
		return fmt.Errorf("failed to copy Redis dump: %w", err)
	}

	return nil
}

// createConfigBackup crea backup de configuración del sistema
func (s *Service) createConfigBackup(outputPath string) error {
	// Directorios y archivos de configuración a incluir
	configPaths := []string{
		"./config",
		"./docker-compose.yml",
		"./docker-compose.prod.yml",
		"./Dockerfile",
		"./go.mod",
		"./go.sum",
		"./Makefile",
		"./.env*",
	}

	// Crear archivo tar.gz con configuraciones
	args := []string{"czf", outputPath}
	
	// Agregar rutas que existen
	for _, path := range configPaths {
		if _, err := os.Stat(path); err == nil {
			args = append(args, path)
		}
	}

	cmd := exec.Command("tar", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("tar command failed: %s - Output: %s", err.Error(), string(output))
	}

	return nil
}

// createAnalyticsBackup crea backup de tablas de analytics
func (s *Service) createAnalyticsBackup(outputPath string) error {
	// Backup específico de tablas de analytics
	args := []string{
		"--host=" + s.backupConfig.PGHost,
		"--port=" + s.backupConfig.PGPort,
		"--username=" + s.backupConfig.PGUser,
		"--dbname=" + s.backupConfig.PGDatabase,
		"--no-password",
		"--verbose",
		"--data-only", // Solo datos, no estructura
		"--file=" + outputPath,
	}

	// Incluir solo tablas de analytics
	analyticsTablePrefixes := []string{
		"analytics_",
		"stats_",
		"requests_",
		"performance_",
	}

	for _, prefix := range analyticsTablePrefixes {
		args = append(args, "--table="+prefix+"*")
	}

	// Configurar variable de entorno para password
	cmd := exec.Command("pg_dump", args...)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+s.backupConfig.PGPassword)

	// Ejecutar comando
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("analytics pg_dump error: %s - Output: %s", err.Error(), string(output))
	}

	return nil
}

// copyFile copia un archivo de src a dst
func (s *Service) copyFile(src, dst string) error {
	source, err := os.Open(src)
	if err != nil {
		return err
	}
	defer source.Close()

	destination, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destination.Close()

	// Copiar contenido
	buf := make([]byte, 64*1024) // Buffer de 64KB
	for {
		n, err := source.Read(buf)
		if err != nil && err.Error() != "EOF" {
			return err
		}
		if n == 0 {
			break
		}

		if _, err := destination.Write(buf[:n]); err != nil {
			return err
		}
	}

	return nil
}

// recordBackup registra información del backup en la base de datos
func (s *Service) recordBackup(info *BackupInfo) {
	if s.db == nil {
		return
	}

	endTime := time.Now()
	duration := int(endTime.Sub(info.Timestamp).Seconds())

	insertSQL := `
		INSERT INTO system_backups 
		(type, filename, size_bytes, encrypted, remote_uploaded, checksum, 
		 success, error_message, start_time, end_time, duration_seconds)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	if err := s.db.Exec(insertSQL, 
		info.Type, info.Filename, info.Size, info.Encrypted, 
		info.Remote, info.Checksum, info.Success, info.Error,
		info.Timestamp, endTime, duration).Error; err != nil {
		s.logger.Error("Failed to record backup info", "error", err)
	}
}