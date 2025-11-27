//go:build !windows
// +build !windows

package utils

import (
	"fmt"
	"syscall"

	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// DiskSpaceChecker verifica espacio disponible en disco
type DiskSpaceChecker struct {
	logger *logger.Logger
}

// NewDiskSpaceChecker crea un nuevo checker de espacio en disco
func NewDiskSpaceChecker(log *logger.Logger) *DiskSpaceChecker {
	return &DiskSpaceChecker{
		logger: log,
	}
}

// CheckSpace verifica que haya suficiente espacio disponible
func (dsc *DiskSpaceChecker) CheckSpace(path string, minGB int) error {
	available, total, err := dsc.GetDiskSpace(path)
	if err != nil {
		return fmt.Errorf("failed to check disk space: %w", err)
	}

	availableGB := available / (1024 * 1024 * 1024)

	if availableGB < uint64(minGB) {
		return fmt.Errorf("insufficient disk space: %dGB available, %dGB required", availableGB, minGB)
	}

	// Log warning si está por debajo del 20% del total
	percentUsed := ((total - available) * 100) / total
	if percentUsed > 80 {
		dsc.logger.Warn("Disk space running low",
			"path", path,
			"available_gb", availableGB,
			"total_gb", total/(1024*1024*1024),
			"percent_used", percentUsed,
		)
	}

	return nil
}

// GetDiskSpace obtiene espacio disponible y total en bytes
func (dsc *DiskSpaceChecker) GetDiskSpace(path string) (available uint64, total uint64, err error) {
	var stat syscall.Statfs_t
	
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, fmt.Errorf("failed to stat filesystem: %w", err)
	}

	// Calcular espacio disponible y total
	available = stat.Bavail * uint64(stat.Bsize)
	total = stat.Blocks * uint64(stat.Bsize)

	return available, total, nil
}

// GetDiskSpacePercent retorna el porcentaje de espacio usado
func (dsc *DiskSpaceChecker) GetDiskSpacePercent(path string) (float64, error) {
	available, total, err := dsc.GetDiskSpace(path)
	if err != nil {
		return 0, err
	}

	if total == 0 {
		return 0, fmt.Errorf("total disk space is 0")
	}

	used := total - available
	percent := (float64(used) / float64(total)) * 100

	return percent, nil
}

// CheckSpaceForFile verifica que haya espacio para un archivo específico
func (dsc *DiskSpaceChecker) CheckSpaceForFile(path string, fileSize int64, marginPercent int) error {
	available, _, err := dsc.GetDiskSpace(path)
	if err != nil {
		return err
	}

	// Agregar margen de seguridad (por defecto 20%)
	if marginPercent == 0 {
		marginPercent = 20
	}
	requiredSpace := fileSize + (fileSize * int64(marginPercent) / 100)

	if uint64(requiredSpace) > available {
		return fmt.Errorf("insufficient space for file: need %dMB, available %dMB",
			requiredSpace/(1024*1024),
			available/(1024*1024),
		)
	}

	return nil
}

// TriggerCleanup determina si se debe ejecutar cleanup basado en espacio
func (dsc *DiskSpaceChecker) TriggerCleanup(path string, thresholdPercent float64) (bool, error) {
	percent, err := dsc.GetDiskSpacePercent(path)
	if err != nil {
		return false, err
	}

	shouldCleanup := percent > thresholdPercent

	if shouldCleanup {
		dsc.logger.Warn("Disk usage threshold exceeded, cleanup recommended",
			"path", path,
			"usage_percent", percent,
			"threshold", thresholdPercent,
		)
	}

	return shouldCleanup, nil
}
