package maintenance

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RotateLogs rota los archivos de log
func (s *Service) RotateLogs() error {
	s.logger.Debug("🔄 Rotating logs...")

	logPaths := []string{
		"./logs",
		"./log",
		"/var/log/tucentropdf",
		"C:/logs/tucentropdf",
	}

	for _, logPath := range logPaths {
		if err := s.rotateLogsInPath(logPath); err != nil {
			s.logger.Debug("Could not rotate logs in path", "path", logPath, "error", err)
		}
	}

	return nil
}

// forceLogRotation fuerza la rotación de logs cuando el disco está lleno
func (s *Service) forceLogRotation() error {
	s.logger.Info("🔄 Forcing log rotation due to disk space...")

	logPaths := []string{
		"./logs",
		"./log",
		"/var/log/tucentropdf",
		"C:/logs/tucentropdf",
	}

	totalDeleted := 0
	var totalFreed int64

	for _, logPath := range logPaths {
		deleted, freed, err := s.aggressiveLogCleanup(logPath)
		if err == nil {
			totalDeleted += deleted
			totalFreed += freed
		}
	}

	if totalDeleted > 0 {
		s.logger.Info("Aggressive log cleanup completed",
			"deleted_files", totalDeleted,
			"freed_space", formatBytes(totalFreed))
	}

	return nil
}

// emergencyCleanupLogs limpieza de emergencia de logs
func (s *Service) emergencyCleanupLogs() error {
	s.logger.Info("🆘 Emergency log cleanup...")

	logPaths := []string{
		"./logs",
		"./log", 
		"/var/log/tucentropdf",
		"C:/logs/tucentropdf",
		os.TempDir(), // También buscar logs en temp
	}

	totalDeleted := 0
	var totalFreed int64

	for _, logPath := range logPaths {
		deleted, freed, err := s.emergencyLogCleanupInPath(logPath)
		if err == nil {
			totalDeleted += deleted
			totalFreed += freed
		}
	}

	if totalDeleted > 0 {
		s.logger.Info("Emergency log cleanup completed",
			"deleted_files", totalDeleted,
			"freed_space", formatBytes(totalFreed))
	}

	return nil
}

// rotateLogsInPath rota logs en una ruta específica
func (s *Service) rotateLogsInPath(logPath string) error {
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return nil
	}

	rotatedCount := 0

	err := filepath.Walk(logPath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		// Verificar si es un archivo de log
		if s.isLogFile(filePath) {
			// Si el archivo es más antiguo que maxLogAge
			if time.Since(info.ModTime()) > s.maxLogAge {
				if err := s.rotateLogFile(filePath, info); err == nil {
					rotatedCount++
				}
			}
		}

		return nil
	})

	if rotatedCount > 0 {
		s.logger.Debug("Rotated logs", "path", logPath, "count", rotatedCount)
	}

	return err
}

// aggressiveLogCleanup limpieza agresiva de logs
func (s *Service) aggressiveLogCleanup(logPath string) (int, int64, error) {
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return 0, 0, nil
	}

	deletedFiles := 0
	var deletedSize int64

	// En limpieza agresiva, eliminar logs más antiguos de 24 horas
	aggressiveMaxAge := 24 * time.Hour

	err := filepath.Walk(logPath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		if s.isLogFile(filePath) && time.Since(info.ModTime()) > aggressiveMaxAge {
			size := info.Size()
			if err := s.compressAndDeleteLog(filePath, info); err == nil {
				deletedFiles++
				deletedSize += size
			}
		}

		return nil
	})

	return deletedFiles, deletedSize, err
}

// emergencyLogCleanupInPath limpieza de emergencia en una ruta
func (s *Service) emergencyLogCleanupInPath(logPath string) (int, int64, error) {
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return 0, 0, nil
	}

	deletedFiles := 0
	var deletedSize int64

	// En emergencia, eliminar todos los logs excepto el actual
	err := filepath.Walk(logPath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		// En emergencia, eliminar archivos de log más antiguos de 2 horas
		if s.isLogFile(filePath) && time.Since(info.ModTime()) > 2*time.Hour {
			size := info.Size()
			if err := os.Remove(filePath); err == nil {
				deletedFiles++
				deletedSize += size
				s.logger.Debug("Emergency deleted log", "file", filePath)
			}
		}

		return nil
	})

	return deletedFiles, deletedSize, err
}

// isLogFile verifica si un archivo es un archivo de log
func (s *Service) isLogFile(filePath string) bool {
	fileName := strings.ToLower(filepath.Base(filePath))
	ext := strings.ToLower(filepath.Ext(filePath))
	
	// Extensiones de log conocidas
	logExtensions := []string{".log", ".out", ".err", ".txt"}
	for _, logExt := range logExtensions {
		if ext == logExt {
			return true
		}
	}

	// Patrones de nombres de archivo de log
	logPatterns := []string{
		"access",
		"error", 
		"debug",
		"info",
		"warn",
		"audit",
		"application",
		"server",
		"worker",
		"gin",
		"fiber",
		"tucentropdf",
	}

	for _, pattern := range logPatterns {
		if strings.Contains(fileName, pattern) {
			return true
		}
	}

	return false
}

// rotateLogFile rota un archivo de log específico
func (s *Service) rotateLogFile(filePath string, info os.FileInfo) error {
	// Crear nombre rotado con timestamp
	dir := filepath.Dir(filePath)
	base := filepath.Base(filePath)
	ext := filepath.Ext(filePath)
	nameWithoutExt := strings.TrimSuffix(base, ext)
	
	timestamp := info.ModTime().Format("2006-01-02_15-04-05")
	rotatedName := fmt.Sprintf("%s_%s%s.gz", nameWithoutExt, timestamp, ext)
	rotatedPath := filepath.Join(dir, "archived", rotatedName)

	// Crear directorio archived si no existe
	archivedDir := filepath.Join(dir, "archived")
	if err := os.MkdirAll(archivedDir, 0755); err != nil {
		return err
	}

	// Comprimir y mover el archivo
	if err := s.compressFile(filePath, rotatedPath); err != nil {
		return err
	}

	// Eliminar el archivo original
	if err := os.Remove(filePath); err != nil {
		// Si no se puede eliminar, al menos se comprimió
		s.logger.Debug("Could not remove original log file after compression", "file", filePath, "error", err)
	}

	s.logger.Debug("Rotated log file", "original", filePath, "rotated", rotatedPath)
	return nil
}

// compressAndDeleteLog comprime y elimina un log (para limpieza agresiva)
func (s *Service) compressAndDeleteLog(filePath string, info os.FileInfo) error {
	// En limpieza agresiva, comprimir a carpeta temp y luego eliminar
	tempDir := os.TempDir()
	base := filepath.Base(filePath)
	compressedName := fmt.Sprintf("%s_%s.gz", base, info.ModTime().Format("2006-01-02"))
	compressedPath := filepath.Join(tempDir, "logs_compressed", compressedName)

	// Crear directorio si no existe
	if err := os.MkdirAll(filepath.Dir(compressedPath), 0755); err != nil {
		return err
	}

	// Comprimir
	if err := s.compressFile(filePath, compressedPath); err != nil {
		return err
	}

	// Eliminar original
	if err := os.Remove(filePath); err != nil {
		return err
	}

	s.logger.Debug("Compressed and deleted log", "original", filePath, "compressed", compressedPath)
	return nil
}

// compressFile comprime un archivo usando gzip
func (s *Service) compressFile(srcPath, destPath string) error {
	// Implementación simplificada - en un entorno real usarías compress/gzip
	// Por ahora, solo copiar el archivo
	
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	destFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer destFile.Close()

	// En una implementación real, aquí comprimirías el contenido
	// Por simplicidad, solo copiamos (podrías usar compress/gzip)
	_, err = destFile.ReadFrom(srcFile)
	return err
}

// cleanupRotatedLogs limpia logs rotados muy antiguos
func (s *Service) cleanupRotatedLogs() error {
	logPaths := []string{
		"./logs/archived",
		"./log/archived",
		"/var/log/tucentropdf/archived",
		"C:/logs/tucentropdf/archived",
	}

	for _, logPath := range logPaths {
		if err := s.cleanupOldRotatedLogsInPath(logPath); err != nil {
			s.logger.Debug("Could not cleanup rotated logs", "path", logPath, "error", err)
		}
	}

	return nil
}

// cleanupOldRotatedLogsInPath limpia logs rotados antiguos en una ruta
func (s *Service) cleanupOldRotatedLogsInPath(logPath string) error {
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		return nil
	}

	deletedCount := 0
	var deletedSize int64

	// Eliminar logs rotados más antiguos de 30 días
	maxRotatedAge := 30 * 24 * time.Hour

	err := filepath.Walk(logPath, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		if time.Since(info.ModTime()) > maxRotatedAge {
			size := info.Size()
			if err := os.Remove(filePath); err == nil {
				deletedCount++
				deletedSize += size
				s.logger.Debug("Deleted old rotated log", "file", filePath)
			}
		}

		return nil
	})

	if deletedCount > 0 {
		s.logger.Info("Cleaned old rotated logs", 
			"path", logPath, 
			"deleted_count", deletedCount,
			"freed_space", formatBytes(deletedSize))
	}

	return err
}