package maintenance

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
	"unsafe"
)

// CheckDiskSpace verifica el espacio en disco y toma acciones correctivas
func (s *Service) CheckDiskSpace() error {
	s.logger.Debug("🔍 Checking disk space...")

	// Obtener uso de disco
	diskUsage, err := s.getDiskUsage()
	if err != nil {
		return fmt.Errorf("error getting disk usage: %w", err)
	}

	s.logger.Debug("Current disk usage", "percentage", fmt.Sprintf("%.1f%%", diskUsage))

	// Verificar umbrales y tomar acciones
	if diskUsage > s.diskThresholdCritical {
		s.logger.Error("🚨 CRITICAL: Disk space usage is critically high", "usage", fmt.Sprintf("%.1f%%", diskUsage))
		return s.handleCriticalDiskUsage(diskUsage)
	} else if diskUsage > s.diskThresholdWarning {
		s.logger.Warn("⚠️ WARNING: Disk space usage is high", "usage", fmt.Sprintf("%.1f%%", diskUsage))
		return s.handleHighDiskUsage(diskUsage)
	} else {
		s.logger.Debug("✅ Disk space usage is normal", "usage", fmt.Sprintf("%.1f%%", diskUsage))
	}

	return nil
}

// getDiskUsage obtiene el porcentaje de uso del disco
func (s *Service) getDiskUsage() (float64, error) {
	// En Windows, obtenemos el uso del disco C:
	path := "C:\\"
	
	// Estructura para GetDiskFreeSpaceEx
	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	
	// Llamada a la API de Windows
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	
	r1, _, err := syscall.NewLazyDLL("kernel32.dll").NewProc("GetDiskFreeSpaceExW").Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytesAvailable)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&totalFreeBytes)),
	)
	
	if r1 == 0 {
		return 0, fmt.Errorf("GetDiskFreeSpaceEx failed: %v", err)
	}
	
	usedBytes := totalBytes - totalFreeBytes
	usagePercent := (float64(usedBytes) / float64(totalBytes)) * 100
	
	return usagePercent, nil
}

// handleCriticalDiskUsage maneja uso crítico del disco (>90%)
func (s *Service) handleCriticalDiskUsage(usage float64) error {
	s.logger.Error("🆘 Taking emergency actions for critical disk usage", "usage", fmt.Sprintf("%.1f%%", usage))

	// 1. Limpiar agresivamente archivos temporales
	if err := s.emergencyCleanupTempFiles(); err != nil {
		s.logger.Error("Error in emergency temp cleanup", "error", err)
	}

	// 2. Limpiar logs antiguos inmediatamente
	if err := s.emergencyCleanupLogs(); err != nil {
		s.logger.Error("Error in emergency log cleanup", "error", err)
	}

	// 3. Limpiar Redis agresivamente
	if s.redis != nil {
		if err := s.emergencyCleanupRedis(); err != nil {
			s.logger.Error("Error in emergency Redis cleanup", "error", err)
		}
	}

	// 4. Comprimir y archivar datos antiguos inmediatamente
	if s.db != nil {
		if err := s.emergencyArchiveData(); err != nil {
			s.logger.Error("Error in emergency data archival", "error", err)
		}
	}

	// 5. Verificar si las acciones ayudaron
	newUsage, err := s.getDiskUsage()
	if err == nil {
		freedSpace := usage - newUsage
		s.logger.Info("Emergency cleanup completed", 
			"old_usage", fmt.Sprintf("%.1f%%", usage),
			"new_usage", fmt.Sprintf("%.1f%%", newUsage),
			"freed", fmt.Sprintf("%.1f%%", freedSpace))

		if newUsage > s.diskThresholdCritical {
			s.logger.Error("🚨 STILL CRITICAL: Emergency cleanup was not sufficient")
			// Aquí podrías enviar alertas, detener servicios no críticos, etc.
		}
	}

	return nil
}

// handleHighDiskUsage maneja uso alto del disco (80-90%)
func (s *Service) handleHighDiskUsage(usage float64) error {
	s.logger.Warn("🧹 Taking preventive actions for high disk usage", "usage", fmt.Sprintf("%.1f%%", usage))

	// 1. Limpiar archivos temporales viejos
	// Funcionalidad de limpieza temporalmente deshabilitada
	// TODO: Implementar cleanupOldTempFiles
	
	// 2. Rotar logs prematuramente
	if err := s.forceLogRotation(); err != nil {
		s.logger.Error("Error forcing log rotation", "error", err)
	}

	// 3. Limpiar datos de analytics muy antiguos
	if s.db != nil {
		if err := s.cleanupVeryOldAnalytics(); err != nil {
			s.logger.Error("Error cleaning very old analytics", "error", err)
		}
	}

	return nil
}

// CheckTempFolder verifica y limpia la carpeta temporal
func (s *Service) CheckTempFolder() error {
	tempPaths := []string{
		os.TempDir(),
		"./temp",
		"./tmp",
		"./uploads",
	}

	for _, tempPath := range tempPaths {
		if err := s.cleanupTempPath(tempPath); err != nil {
			s.logger.Error("Error cleaning temp path", "path", tempPath, "error", err)
		}
	}

	return nil
}

// getTempFolderSize obtiene el tamaño de las carpetas temporales
func (s *Service) getTempFolderSize() (int64, error) {
	tempPaths := []string{
		os.TempDir(),
		"./temp",
		"./tmp",
		"./uploads",
	}

	var totalSize int64

	for _, tempPath := range tempPaths {
		size, err := s.getFolderSize(tempPath)
		if err == nil {
			totalSize += size
		}
	}

	return totalSize, nil
}

// cleanupTempPath limpia una carpeta temporal específica
func (s *Service) cleanupTempPath(path string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	deletedFiles := 0
	var deletedSize int64

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continuar con otros archivos
		}

		// No procesar directorios
		if info.IsDir() {
			return nil
		}

		// Verificar si el archivo es antiguo
		if time.Since(info.ModTime()) > s.maxTempFileAge {
			size := info.Size()
			if err := os.Remove(filePath); err == nil {
				deletedFiles++
				deletedSize += size
				s.logger.Debug("Deleted old temp file", "file", filePath, "age", time.Since(info.ModTime()))
			}
		}

		return nil
	})

	if deletedFiles > 0 {
		s.logger.Info("Cleaned temp folder", 
			"path", path, 
			"deleted_files", deletedFiles, 
			"freed_space", formatBytes(deletedSize))
	}

	return err
}

// emergencyCleanupTempFiles limpieza de emergencia de archivos temporales
func (s *Service) emergencyCleanupTempFiles() error {
	s.logger.Info("🆘 Emergency temp files cleanup...")

	tempPaths := []string{
		os.TempDir(),
		"./temp",
		"./tmp", 
		"./uploads",
		"./cache",
	}

	totalDeleted := 0
	var totalFreed int64

	for _, tempPath := range tempPaths {
		deleted, freed, err := s.emergencyCleanupPath(tempPath)
		if err == nil {
			totalDeleted += deleted
			totalFreed += freed
		}
	}

	if totalDeleted > 0 {
		s.logger.Info("Emergency temp cleanup completed",
			"deleted_files", totalDeleted,
			"freed_space", formatBytes(totalFreed))
	}

	return nil
}

// emergencyCleanupPath limpieza de emergencia de una ruta específica
func (s *Service) emergencyCleanupPath(path string) (int, int64, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return 0, 0, nil
	}

	deletedFiles := 0
	var deletedSize int64

	// En emergencia, eliminar todos los archivos temporales independientemente de la edad
	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		// En emergencia, eliminar archivos con ciertas extensiones
		ext := filepath.Ext(filePath)
		emergencyExtensions := []string{".tmp", ".temp", ".cache", ".log", ".bak"}
		
		for _, emergencyExt := range emergencyExtensions {
			if ext == emergencyExt {
				size := info.Size()
				if err := os.Remove(filePath); err == nil {
					deletedFiles++
					deletedSize += size
				}
				break
			}
		}

		return nil
	})

	return deletedFiles, deletedSize, err
}

// getFolderSize calcula el tamaño de una carpeta
func (s *Service) getFolderSize(path string) (int64, error) {
	var size int64

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	return size, err
}

// emergencyCleanupRedis limpieza de emergencia de Redis
func (s *Service) emergencyCleanupRedis() error {
	if s.redis == nil {
		return nil
	}

	s.logger.Info("🆘 Emergency Redis cleanup...")

	// Eliminar todas las claves temporales sin importar TTL
	emergencyPatterns := []string{
		"temp:*",
		"cache:*", 
		"session:*",
		"queue:*",
		"lock:*",
		"job:*",
		"*:daily_count:*",
	}

	totalDeleted := 0
	for _, pattern := range emergencyPatterns {
		keys, err := s.redis.Keys(s.ctx, pattern).Result()
		if err != nil {
			continue
		}

		if len(keys) > 0 {
			deleted, err := s.redis.Del(s.ctx, keys...).Result()
			if err == nil {
				totalDeleted += int(deleted)
			}
		}
	}

	if totalDeleted > 0 {
		s.logger.Info("Emergency Redis cleanup completed", "deleted_keys", totalDeleted)
	}

	// Liberar memoria
	s.redis.Do(s.ctx, "MEMORY", "PURGE")

	return nil
}