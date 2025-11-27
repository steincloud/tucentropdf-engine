package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// FileTracker rastrea archivos en uso para prevenir eliminación prematura
type FileTracker struct {
	inUse         sync.Map         // map[string]int32 - contador atómico de referencias
	logger        *logger.Logger
	mu            sync.RWMutex
	activeCleanup atomic.Bool      // flag para prevenir múltiples cleanups simultáneos
}

// NewFileTracker crea un nuevo tracker de archivos
func NewFileTracker(log *logger.Logger) *FileTracker {
	return &FileTracker{
		logger: log,
	}
}

// MarkInUse marca un archivo como en uso (incrementa contador)
func (ft *FileTracker) MarkInUse(filePath string) {
	// Cargar o crear contador atómico
	actual, _ := ft.inUse.LoadOrStore(filePath, new(int32))
	counter := actual.(*int32)
	
	newCount := atomic.AddInt32(counter, 1)
	
	ft.logger.Debug("File marked in use",
		"path", filePath,
		"ref_count", newCount,
	)
}

// MarkAvailable marca un archivo como disponible (decrementa contador)
func (ft *FileTracker) MarkAvailable(filePath string) {
	actual, exists := ft.inUse.Load(filePath)
	if !exists {
		ft.logger.Warn("Attempted to release file not in tracker", "path", filePath)
		return
	}
	
	counter := actual.(*int32)
	newCount := atomic.AddInt32(counter, -1)
	
	// Si el contador llega a 0, eliminar del mapa
	if newCount <= 0 {
		ft.inUse.Delete(filePath)
		ft.logger.Debug("File released from tracker", "path", filePath)
	} else {
		ft.logger.Debug("File still in use",
			"path", filePath,
			"ref_count", newCount,
		)
	}
}

// IsInUse verifica si un archivo está en uso
func (ft *FileTracker) IsInUse(filePath string) bool {
	actual, exists := ft.inUse.Load(filePath)
	if !exists {
		return false
	}
	
	counter := actual.(*int32)
	return atomic.LoadInt32(counter) > 0
}

// GetRefCount obtiene el número de referencias a un archivo
func (ft *FileTracker) GetRefCount(filePath string) int32 {
	actual, exists := ft.inUse.Load(filePath)
	if !exists {
		return 0
	}
	
	counter := actual.(*int32)
	return atomic.LoadInt32(counter)
}

// AtomicCleanup ejecuta limpieza de archivos antiguos de forma segura
type AtomicCleanup struct {
	tracker   *FileTracker
	logger    *logger.Logger
	tempDir   string
	maxAge    time.Duration
	isRunning atomic.Bool
}

// NewAtomicCleanup crea un nuevo servicio de cleanup atómico
func NewAtomicCleanup(tracker *FileTracker, log *logger.Logger, tempDir string, maxAge time.Duration) *AtomicCleanup {
	return &AtomicCleanup{
		tracker: tracker,
		logger:  log,
		tempDir: tempDir,
		maxAge:  maxAge,
	}
}

// CleanupOldFiles ejecuta cleanup de archivos antiguos (thread-safe)
func (ac *AtomicCleanup) CleanupOldFiles(ctx context.Context) (int, error) {
	// Prevenir múltiples cleanups simultáneos
	if !ac.isRunning.CompareAndSwap(false, true) {
		ac.logger.Warn("Cleanup already in progress, skipping")
		return 0, fmt.Errorf("cleanup already running")
	}
	defer ac.isRunning.Store(false)

	ac.logger.Info("🧹 Starting atomic cleanup", "temp_dir", ac.tempDir, "max_age", ac.maxAge)

	start := time.Now()
	deletedCount := 0
	skippedCount := 0
	errorCount := 0

	// Listar archivos en directorio temporal
	files, err := os.ReadDir(ac.tempDir)
	if err != nil {
		return 0, fmt.Errorf("failed to read temp directory: %w", err)
	}

	for _, entry := range files {
		// Verificar si el context fue cancelado
		select {
		case <-ctx.Done():
			ac.logger.Warn("Cleanup cancelled", "deleted", deletedCount, "skipped", skippedCount)
			return deletedCount, ctx.Err()
		default:
		}

		if entry.IsDir() {
			continue
		}

		filePath := filepath.Join(ac.tempDir, entry.Name())

		// Verificar si está en uso
		if ac.tracker.IsInUse(filePath) {
			skippedCount++
			ac.logger.Debug("Skipping file in use",
				"path", filePath,
				"ref_count", ac.tracker.GetRefCount(filePath),
			)
			continue
		}

		// Obtener info del archivo
		info, err := entry.Info()
		if err != nil {
			ac.logger.Warn("Failed to get file info", "path", filePath, "error", err)
			errorCount++
			continue
		}

		// Verificar antigüedad
		age := time.Since(info.ModTime())
		if age < ac.maxAge {
			continue
		}

		// Intentar eliminar (con retry simple)
		if err := ac.deleteWithRetry(filePath, 3); err != nil {
			ac.logger.Error("Failed to delete old file", "path", filePath, "error", err)
			errorCount++
		} else {
			deletedCount++
			ac.logger.Debug("Deleted old file", "path", filePath, "age", age)
		}
	}

	duration := time.Since(start)
	ac.logger.Info("Cleanup completed",
		"deleted", deletedCount,
		"skipped_in_use", skippedCount,
		"errors", errorCount,
		"duration", duration,
	)

	return deletedCount, nil
}

// deleteWithRetry intenta eliminar un archivo con reintentos
func (ac *AtomicCleanup) deleteWithRetry(filePath string, maxRetries int) error {
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		// Verificar nuevamente que no esté en uso antes de eliminar
		if ac.tracker.IsInUse(filePath) {
			return fmt.Errorf("file marked as in-use during delete attempt")
		}

		err := os.Remove(filePath)
		if err == nil {
			return nil
		}

		lastErr = err

		// Si el archivo no existe, considerarlo éxito
		if os.IsNotExist(err) {
			return nil
		}

		// Esperar antes de reintentar
		if i < maxRetries-1 {
			time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
		}
	}

	return fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}

// StartPeriodicCleanup inicia cleanup periódico en background
func (ac *AtomicCleanup) StartPeriodicCleanup(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	ac.logger.Info("Starting periodic cleanup", "interval", interval)

	for {
		select {
		case <-ticker.C:
			cleanupCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
			_, err := ac.CleanupOldFiles(cleanupCtx)
			if err != nil {
				ac.logger.Error("Periodic cleanup failed", "error", err)
			}
			cancel()

		case <-ctx.Done():
			ac.logger.Info("Stopping periodic cleanup")
			return
		}
	}
}

// ForceCleanup fuerza limpieza de un archivo específico (ignora inUse)
func (ac *AtomicCleanup) ForceCleanup(filePath string) error {
	ac.logger.Warn("Force cleanup requested", "path", filePath)

	// Eliminar del tracker
	ac.tracker.inUse.Delete(filePath)

	// Intentar eliminar
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("force cleanup failed: %w", err)
	}

	ac.logger.Info("Force cleanup completed", "path", filePath)
	return nil
}

// GetStats obtiene estadísticas del cleanup
func (ac *AtomicCleanup) GetStats() map[string]interface{} {
	filesInUse := 0
	ac.tracker.inUse.Range(func(key, value interface{}) bool {
		filesInUse++
		return true
	})

	return map[string]interface{}{
		"files_in_use":  filesInUse,
		"is_running":    ac.isRunning.Load(),
		"temp_dir":      ac.tempDir,
		"max_age":       ac.maxAge.String(),
	}
}
