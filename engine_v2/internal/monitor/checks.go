package monitor

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/tucentropdf/engine-v2/internal/alerts"
)

// CheckCPU verifica el uso de CPU del sistema
func (s *Service) CheckCPU() {
	cpuUsage, err := s.getCPUUsage()
	if err != nil {
		s.logger.Debug("Error getting CPU usage", "error", err)
		return
	}

	// Actualizar status
	s.mu.Lock()
	if s.systemStatus.Resources == nil {
		s.systemStatus.Resources = &ResourceStatus{}
	}
	s.systemStatus.Resources.CPUPercent = cpuUsage
	s.mu.Unlock()

	s.logger.Debug("CPU usage check", "percentage", fmt.Sprintf("%.1f%%", cpuUsage))

	// Evaluar umbrales
	if cpuUsage > s.thresholds.CPU.Critical {
		s.logger.Error("😨 CPU usage critical!", "percentage", fmt.Sprintf("%.1f%%", cpuUsage))
		s.activateProtectionMode("CPU_CRITICAL", fmt.Sprintf("CPU usage: %.1f%%", cpuUsage))
		s.recordIncident("CPU_HIGH", "critical", fmt.Sprintf("CPU usage reached %.1f%%", cpuUsage), map[string]interface{}{
			"cpu_percent": cpuUsage,
			"threshold":   s.thresholds.CPU.Critical,
		})
	} else if cpuUsage > s.thresholds.CPU.Warning {
		s.logger.Warn("⚠️ CPU usage high", "percentage", fmt.Sprintf("%.1f%%", cpuUsage))
		s.recordIncident("CPU_HIGH", "warning", fmt.Sprintf("CPU usage reached %.1f%%", cpuUsage), map[string]interface{}{
			"cpu_percent": cpuUsage,
			"threshold":   s.thresholds.CPU.Warning,
		})
	}
}

// CheckRAM verifica el uso de memoria RAM
func (s *Service) CheckRAM() {
	ramUsage, ramUsedMB, ramTotalMB, err := s.getRAMUsage()
	if err != nil {
		s.logger.Debug("Error getting RAM usage", "error", err)
		return
	}

	// Actualizar status
	s.mu.Lock()
	if s.systemStatus.Resources == nil {
		s.systemStatus.Resources = &ResourceStatus{}
	}
	s.systemStatus.Resources.RAMPercent = ramUsage
	s.systemStatus.Resources.RAMUsedMB = ramUsedMB
	s.systemStatus.Resources.RAMTotalMB = ramTotalMB
	s.mu.Unlock()

	s.logger.Debug("RAM usage check", "percentage", fmt.Sprintf("%.1f%%", ramUsage), "used_mb", ramUsedMB)

	// Evaluar umbrales críticos
	if ramUsage > s.thresholds.RAM.Emergency {
		s.logger.Error("🆘 RAM usage EMERGENCY!", "percentage", fmt.Sprintf("%.1f%%", ramUsage))
		s.activateProtectionMode("RAM_EMERGENCY", fmt.Sprintf("RAM usage: %.1f%%", ramUsage))
		// Activar protecciones especiales para RAM crítica
		s.activateRAMEmergencyMode()
		s.recordIncident("RAM_EMERGENCY", "critical", fmt.Sprintf("RAM usage reached emergency level: %.1f%%", ramUsage), map[string]interface{}{
			"ram_percent": ramUsage,
			"ram_used_mb": ramUsedMB,
			"threshold":   s.thresholds.RAM.Emergency,
		})
	} else if ramUsage > s.thresholds.RAM.Critical {
		s.logger.Error("😨 RAM usage critical!", "percentage", fmt.Sprintf("%.1f%%", ramUsage))
		s.activateProtectionMode("RAM_CRITICAL", fmt.Sprintf("RAM usage: %.1f%%", ramUsage))
		s.recordIncident("RAM_HIGH", "critical", fmt.Sprintf("RAM usage reached %.1f%%", ramUsage), map[string]interface{}{
			"ram_percent": ramUsage,
			"ram_used_mb": ramUsedMB,
			"threshold":   s.thresholds.RAM.Critical,
		})
	} else if ramUsage > s.thresholds.RAM.Warning {
		s.logger.Warn("⚠️ RAM usage high", "percentage", fmt.Sprintf("%.1f%%", ramUsage))
		s.recordIncident("RAM_HIGH", "warning", fmt.Sprintf("RAM usage reached %.1f%%", ramUsage), map[string]interface{}{
			"ram_percent": ramUsage,
			"ram_used_mb": ramUsedMB,
			"threshold":   s.thresholds.RAM.Warning,
		})
	}
}

// CheckDisk verifica el uso del disco
func (s *Service) CheckDisk() {
	diskUsage, err := s.getDiskUsage()
	if err != nil {
		s.logger.Debug("Error getting disk usage", "error", err)
		return
	}

	// Actualizar status
	s.mu.Lock()
	if s.systemStatus.Resources == nil {
		s.systemStatus.Resources = &ResourceStatus{}
	}
	s.systemStatus.Resources.DiskPercent = diskUsage
	s.mu.Unlock()

	s.logger.Debug("Disk usage check", "percentage", fmt.Sprintf("%.1f%%", diskUsage))

	// Evaluar umbrales
	if diskUsage > s.thresholds.Disk.Critical {
		s.logger.Error("💾 Disk usage critical!", "percentage", fmt.Sprintf("%.1f%%", diskUsage))
		s.activateProtectionMode("DISK_CRITICAL", fmt.Sprintf("Disk usage: %.1f%%", diskUsage))
		// Activar limpieza de emergencia
		s.activateDiskEmergencyMode()
		s.recordIncident("DISK_CRITICAL", "critical", fmt.Sprintf("Disk usage reached %.1f%%", diskUsage), map[string]interface{}{
			"disk_percent": diskUsage,
			"threshold":    s.thresholds.Disk.Critical,
		})
	} else if diskUsage > s.thresholds.Disk.Warning {
		s.logger.Warn("⚠️ Disk usage high", "percentage", fmt.Sprintf("%.1f%%", diskUsage))
		s.recordIncident("DISK_HIGH", "warning", fmt.Sprintf("Disk usage reached %.1f%%", diskUsage), map[string]interface{}{
			"disk_percent": diskUsage,
			"threshold":    s.thresholds.Disk.Warning,
		})
		// Activar limpieza preventiva
		go s.runDiskCleanup()
	}
}

// CheckRedis verifica el estado de Redis
func (s *Service) CheckRedis() {
	if s.redis == nil {
		s.mu.Lock()
		s.systemStatus.Redis = &RedisStatus{Alive: false}
		s.mu.Unlock()
		return
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()

	// Ping Redis
	_, err := s.redis.Ping(ctx).Result()
	latency := time.Since(start).Milliseconds()

	alive := err == nil

	// Obtener estadísticas adicionales si está vivo
	var keys int64
	var memoryMB int64
	if alive {
		keys, _ = s.redis.DBSize(ctx).Result()
		memInfo, _ := s.redis.Info(ctx, "memory").Result()
		memoryMB = parseRedisMemory(memInfo)
	}

	// Actualizar status
	s.mu.Lock()
	s.systemStatus.Redis = &RedisStatus{
		Alive:     alive,
		Latency:   latency,
		Keys:      keys,
		MemoryMB:  memoryMB,
	}
	s.mu.Unlock()

	s.logger.Debug("Redis check", "alive", alive, "latency_ms", latency, "keys", keys)

	// Evaluar estado
	if !alive {
		s.logger.Error("📛 Redis is down!", "error", err)
		s.recordIncident("REDIS_DOWN", "critical", "Redis connection failed", map[string]interface{}{
			"error": err.Error(),
		})
	} else if latency > s.thresholds.Redis.LatencyCritical {
		s.logger.Error("🐌 Redis latency critical!", "latency_ms", latency)
		s.recordIncident("REDIS_SLOW", "critical", fmt.Sprintf("Redis latency: %dms", latency), map[string]interface{}{
			"latency_ms": latency,
			"threshold":   s.thresholds.Redis.LatencyCritical,
		})
	} else if latency > s.thresholds.Redis.LatencyWarning {
		s.logger.Warn("⚠️ Redis latency high", "latency_ms", latency)
		s.recordIncident("REDIS_SLOW", "warning", fmt.Sprintf("Redis latency: %dms", latency), map[string]interface{}{
			"latency_ms": latency,
			"threshold":   s.thresholds.Redis.LatencyWarning,
		})
	}
}

// CheckWorkers verifica el estado de todos los workers
func (s *Service) CheckWorkers() {
	s.workerMu.Lock()
	defer s.workerMu.Unlock()

	for name, worker := range s.workerHealth {
		s.checkWorkerHealth(name, worker)
	}
}

// checkWorkerHealth verifica la salud de un worker específico
func (s *Service) checkWorkerHealth(name string, worker *WorkerStatus) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", worker.HealthURL, nil)
	if err != nil {
		s.handleWorkerFailure(name, worker, fmt.Sprintf("Failed to create request: %v", err))
		return
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		s.handleWorkerFailure(name, worker, fmt.Sprintf("Health check failed: %v", err))
		return
	}
	defer resp.Body.Close()

	// Worker está respondiendo
	worker.LastSeen = time.Now()
	worker.Latency = latency

	if resp.StatusCode == 200 {
		// Worker saludable
		if worker.Status == "failed" {
			// Recuperación de worker
			s.logger.Info("❤️‍🩹 Worker recovered!", "worker", name, "latency_ms", latency)
			s.recordIncident("WORKER_RECOVERY", "info", fmt.Sprintf("Worker %s recovered", name), map[string]interface{}{
				"worker":     name,
				"latency_ms": latency,
			})
		}
		worker.Status = "ok"
		s.logger.Debug("Worker health check OK", "worker", name, "latency_ms", latency)
	} else {
		s.handleWorkerFailure(name, worker, fmt.Sprintf("Health check returned status %d", resp.StatusCode))
	}
}

// CheckQueue verifica el estado de las colas de trabajo
func (s *Service) CheckQueue() {
	if s.redis == nil {
		return
	}

	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()

	// Obtener longitudes de colas
	pdfQueue, _ := s.redis.LLen(ctx, "queue:pdf").Result()
	ocrQueue, _ := s.redis.LLen(ctx, "queue:ocr").Result()
	officeQueue, _ := s.redis.LLen(ctx, "queue:office").Result()
	totalJobs := int(pdfQueue + ocrQueue + officeQueue)

	// Actualizar status
	s.mu.Lock()
	s.systemStatus.Queue = &QueueStatus{
		PendingJobs: totalJobs,
		PDFQueue:    int(pdfQueue),
		OCRQueue:    int(ocrQueue),
		OfficeQueue: int(officeQueue),
	}
	s.mu.Unlock()

	s.logger.Debug("Queue check", "total_jobs", totalJobs, "pdf", pdfQueue, "ocr", ocrQueue, "office", officeQueue)

	// Evaluar umbrales
	if totalJobs > s.thresholds.Queue.Max {
		s.logger.Error("📦 Queue overloaded!", "total_jobs", totalJobs)
		s.activateProtectionMode("QUEUE_OVERLOAD", fmt.Sprintf("Queue has %d jobs", totalJobs))
		s.activateQueueProtection()
		s.recordIncident("QUEUE_OVERLOAD", "critical", fmt.Sprintf("Queue overloaded with %d jobs", totalJobs), map[string]interface{}{
			"total_jobs": totalJobs,
			"pdf_queue": pdfQueue,
			"ocr_queue": ocrQueue,
			"office_queue": officeQueue,
			"threshold":    s.thresholds.Queue.Max,
		})
	} else if totalJobs > s.thresholds.Queue.Critical {
		s.logger.Warn("⚠️ Queue critical", "total_jobs", totalJobs)
		s.activateQueueThrottling()
		s.recordIncident("QUEUE_HIGH", "warning", fmt.Sprintf("Queue has %d jobs", totalJobs), map[string]interface{}{
			"total_jobs": totalJobs,
			"threshold":  s.thresholds.Queue.Critical,
		})
	} else if totalJobs > s.thresholds.Queue.Warning {
		s.logger.Info("📊 Queue elevated", "total_jobs", totalJobs)
	}
}

// handleWorkerFailure maneja fallos de workers
func (s *Service) handleWorkerFailure(name string, worker *WorkerStatus, reason string) {
	if worker.Status != "failed" {
		s.logger.Error("😱 Worker failed!", "worker", name, "reason", reason)
		s.recordIncident("WORKER_FAILURE", "critical", fmt.Sprintf("Worker %s failed: %s", name, reason), map[string]interface{}{
			"worker": name,
			"reason": reason,
			"restart_count": worker.RestartCount,
		})

		// Enviar alerta
		go s.alertService.SendAlert(&alerts.Alert{
			Type:     "WORKER_FAILURE",
			Severity: "critical",
			Message:  fmt.Sprintf("Worker %s is not responding", name),
			Details: map[string]interface{}{
				"worker": name,
				"reason": reason,
			},
		})
	}

	worker.Status = "failed"
	
	// Intentar restart automático si no se ha intentado recientemente
	if time.Since(worker.LastSeen) > 2*time.Minute {
		go s.attemptWorkerRestart(name, worker)
	}
}

// getCPUUsage obtiene el uso de CPU usando runtime
func (s *Service) getCPUUsage() (float64, error) {
	// Usando runtime de Go para obtener estadísticas básicas
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	// Para Windows, aproximar CPU basado en goroutines activas
	numGoroutines := runtime.NumGoroutine()
	numCPU := runtime.NumCPU()
	
	// Aproximación simple: más goroutines = más uso de CPU
	cpuUsage := float64(numGoroutines) / float64(numCPU) * 10.0
	if cpuUsage > 100 {
		cpuUsage = 100
	}
	
	return cpuUsage, nil
}

// getRAMUsage obtiene el uso de memoria RAM
func (s *Service) getRAMUsage() (float64, int64, int64, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	
	// Usar la memoria del proceso Go como proxy
	usedMB := int64(m.Sys / 1024 / 1024)
	
	// Intentar obtener memoria total del sistema en Windows
	totalMB, err := s.getTotalSystemRAM()
	if err != nil {
		// Fallback: usar memoria del heap como base
		totalMB = int64(m.HeapSys/1024/1024) * 4 // Aproximación
	}
	
	usagePercent := float64(usedMB) / float64(totalMB) * 100
	
	return usagePercent, usedMB, totalMB, nil
}

// getTotalSystemRAM obtiene la RAM total del sistema en Windows
func (s *Service) getTotalSystemRAM() (int64, error) {
	// Para Windows, usar GlobalMemoryStatusEx
	type memoryStatusEx struct {
		dwLength                uint32
		dwMemoryLoad            uint32
		ullTotalPhys            uint64
		ullAvailPhys            uint64
		ullTotalPageFile        uint64
		ullAvailPageFile        uint64
		ullTotalVirtual         uint64
		ullAvailVirtual         uint64
		ullAvailExtendedVirtual uint64
	}
	
	var memStatus memoryStatusEx
	memStatus.dwLength = uint32(unsafe.Sizeof(memStatus))
	
	k32, err := syscall.LoadLibrary("kernel32.dll")
	if err != nil {
		return 0, err
	}
	defer syscall.FreeLibrary(k32)
	
	globalMemoryStatusEx, err := syscall.GetProcAddress(k32, "GlobalMemoryStatusEx")
	if err != nil {
		return 0, err
	}
	
	r1, _, _ := syscall.Syscall(globalMemoryStatusEx, 1, uintptr(unsafe.Pointer(&memStatus)), 0, 0)
	if r1 == 0 {
		return 0, fmt.Errorf("GlobalMemoryStatusEx failed")
	}
	
	return int64(memStatus.ullTotalPhys / 1024 / 1024), nil
}

// getDiskUsage obtiene el uso del disco usando GetDiskFreeSpaceEx
func (s *Service) getDiskUsage() (float64, error) {
	// Usar la misma función que en maintenance para consistencia
	path := "C:\\"
	
	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	
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

// parseRedisMemory parsea la información de memoria de Redis
func parseRedisMemory(memInfo string) int64 {
	lines := strings.Split(memInfo, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "used_memory:") {
			memStr := strings.TrimPrefix(line, "used_memory:")
			memStr = strings.TrimSpace(memStr)
			if memBytes, err := strconv.ParseInt(memStr, 10, 64); err == nil {
				return memBytes / 1024 / 1024 // Convertir a MB
			}
		}
	}
	return 0
}