//go:build windows
// +build windows

package utils

import (
    "fmt"
    "github.com/tucentropdf/engine-v2/pkg/logger"
)

// DiskSpaceChecker (stub) implementación para Windows en tests locales.
type DiskSpaceChecker struct {
    logger *logger.Logger
}

// NewDiskSpaceChecker crea un checker (Windows stub)
func NewDiskSpaceChecker(log *logger.Logger) *DiskSpaceChecker {
    return &DiskSpaceChecker{logger: log}
}

// CheckSpace verifica espacio mínimo (stub: asume espacio suficiente)
func (dsc *DiskSpaceChecker) CheckSpace(path string, minGB int) error {
    available, _, err := dsc.GetDiskSpace(path)
    if err != nil {
        return err
    }
    availableGB := available / (1024 * 1024 * 1024)
    if availableGB < uint64(minGB) {
        return fmt.Errorf("insufficient disk space: %dGB available, %dGB required", availableGB, minGB)
    }
    return nil
}

// GetDiskSpace retorna valores simulados para Windows
func (dsc *DiskSpaceChecker) GetDiskSpace(path string) (uint64, uint64, error) {
    // Devuelve 100GB disponibles, 200GB total como stub para CI/Windows
    available := uint64(100) * 1024 * 1024 * 1024
    total := uint64(200) * 1024 * 1024 * 1024
    return available, total, nil
}

// GetDiskSpacePercent calcula porcentaje basado en stub
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

// CheckSpaceForFile verifica espacio para un archivo
func (dsc *DiskSpaceChecker) CheckSpaceForFile(path string, fileSize int64, marginPercent int) error {
    available, _, err := dsc.GetDiskSpace(path)
    if err != nil {
        return err
    }
    if marginPercent == 0 {
        marginPercent = 20
    }
    required := uint64(fileSize) + uint64(fileSize)*uint64(marginPercent)/100
    if required > available {
        return fmt.Errorf("insufficient space for file: need %dMB, available %dMB", required/(1024*1024), available/(1024*1024))
    }
    return nil
}

// TriggerCleanup stub
func (dsc *DiskSpaceChecker) TriggerCleanup(path string, thresholdPercent float64) (bool, error) {
    percent, err := dsc.GetDiskSpacePercent(path)
    if err != nil {
        return false, err
    }
    should := percent > thresholdPercent
    if should {
        dsc.logger.Warn("Disk usage threshold exceeded (windows stub)", "path", path, "usage_percent", percent)
    }
    return should, nil
}
