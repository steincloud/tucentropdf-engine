package monitor

import (
	"fmt"
	"runtime"
	"time"

	"github.com/tucentropdf/engine-v2/internal/alerts"
)

// activateProtectionMode activa el modo protector del sistema
func (s *Service) activateProtectionMode(reason, details string) {
	// Usar CompareAndSwap atómico para evitar race conditions
	if s.protectionMode.CompareAndSwap(false, true) {
		s.mu.Lock()
		s.protectionStart = time.Now()
		s.mu.Unlock()
		s.logger.Error("🛡️ PROTECTION MODE ACTIVATED!", "reason", reason, "details", details)
		
		s.recordIncident("PROTECTOR_ON", "critical", fmt.Sprintf("Protection mode activated: %s", reason), map[string]interface{}{
			"reason": reason,
			"details": details,
		})

		// Enviar alerta crítica
		go s.alertService.SendAlert(&alerts.Alert{
			Type:     "PROTECTION_MODE",
			Severity: "critical",
			Message:  fmt.Sprintf("System protection activated: %s", reason),
			Details: map[string]interface{}{
				"reason":    reason,
				"details":   details,
				"timestamp": time.Now(),
			},
		})
	}
}

// checkProtectionMode verifica si el modo protector debe desactivarse
func (s *Service) checkProtectionMode() {
	if !s.protectionMode.Load() {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Verificar si han pasado al menos 2 minutos en modo protector
	if time.Since(s.protectionStart) < 2*time.Minute {
		return
	}

	// Verificar si las condiciones han mejorado
	if s.shouldDeactivateProtection() {
		s.deactivateProtectionMode()
	}
}

// shouldDeactivateProtection verifica si se deben desactivar las protecciones
func (s *Service) shouldDeactivateProtection() bool {
	// Verificar recursos si están disponibles
	if s.systemStatus.Resources != nil {
		if s.systemStatus.Resources.CPUPercent > s.thresholds.CPU.Warning ||
			s.systemStatus.Resources.RAMPercent > s.thresholds.RAM.Warning ||
			s.systemStatus.Resources.DiskPercent > s.thresholds.Disk.Warning {
			return false
		}
	}

	// Verificar cola
	if s.systemStatus.Queue != nil {
		if s.systemStatus.Queue.PendingJobs > s.thresholds.Queue.Warning {
			return false
		}
	}

	// Verificar workers críticos
	for _, worker := range s.systemStatus.Workers {
		if worker.Status == "failed" {
			return false
		}
	}

	return true
}

// deactivateProtectionMode desactiva el modo protector
func (s *Service) deactivateProtectionMode() {
	s.protectionMode.Store(false)
	duration := time.Since(s.protectionStart)
	s.logger.Info("✅ PROTECTION MODE DEACTIVATED - System recovered!", "duration", duration)

	s.recordIncident("PROTECTOR_OFF", "info", "Protection mode deactivated - system recovered", map[string]interface{}{
		"duration_minutes": duration.Minutes(),
		"recovery_time":    time.Now(),
	})

	// Enviar alerta de recuperación
	go s.alertService.SendAlert(&alerts.Alert{
		Type:     "SYSTEM_RECOVERY",
		Severity: "info",
		Message:  fmt.Sprintf("System recovered after %.1f minutes", duration.Minutes()),
		Details: map[string]interface{}{
			"duration_minutes": duration.Minutes(),
			"timestamp":        time.Now(),
		},
	})
}

// activateRAMEmergencyMode activa protecciones especiales para RAM crítica
func (s *Service) activateRAMEmergencyMode() {
	s.logger.Error("🆘 ACTIVATING RAM EMERGENCY PROTOCOLS!")

	// Aquí puedes implementar medidas drásticas:
	// - Pausar OCR de archivos > 20MB por 5 minutos
	// - Forzar garbage collection
	// - Rechazar uploads temporalmente

	// Forzar garbage collection inmediato
	go func() {
		s.logger.Info("🗑️ Force garbage collection due to RAM emergency")
		runtime.GC()
		runtime.GC() // Doble GC para mayor efecto
	}()

	// Implementar lógica adicional según necesidades
	// Esto podría incluir:
	// - Configurar flags globales para rechazar archivos grandes
	// - Pausar workers temporalmente
	// - Limpiar caches en memoria
}

// activateDiskEmergencyMode activa protecciones para disco crítico
func (s *Service) activateDiskEmergencyMode() {
	s.logger.Error("💾 ACTIVATING DISK EMERGENCY PROTOCOLS!")

	// Activar limpieza de emergencia
	go func() {
		// Integración con el servicio de mantenimiento si está disponible
		// Esto podría activar limpieza agresiva de temporales
		s.logger.Info("🧹 Emergency disk cleanup triggered")
		// Aquí se podría llamar al servicio de mantenimiento
	}()
}

// activateQueueProtection activa protecciones para cola sobrecargada
func (s *Service) activateQueueProtection() {
	s.logger.Error("📦 ACTIVATING QUEUE OVERLOAD PROTECTION!")

	// Implementar:
	// - Rechazar archivos > 50MB temporalmente
	// - Priorizar usuarios PRO/Corporate
	// - Pausar operaciones FREE largas

	go s.alertService.SendAlert(&alerts.Alert{
		Type:     "QUEUE_OVERLOAD",
		Severity: "critical",
		Message:  "Queue overloaded - implementing protective measures",
		Details: map[string]interface{}{
			"timestamp": time.Now(),
		},
	})
}

// activateQueueThrottling activa throttling de cola
func (s *Service) activateQueueThrottling() {
	s.logger.Warn("🐌 Activating queue throttling")

	// Implementar throttling más suave:
	// - Limitar nuevos jobs grandes
	// - Aumentar delays entre operaciones
	// - Priorizar usuarios premium
}

// attemptWorkerRestart intenta reiniciar un worker
func (s *Service) attemptWorkerRestart(name string, worker *WorkerStatus) {
	worker.RestartCount++
	s.logger.Info("🔄 Attempting worker restart", "worker", name, "attempt", worker.RestartCount)

	// Implementar lógica de reinicio según el tipo de worker
	// Esto podría incluir:
	// - Llamadas a Docker API para reiniciar contenedores
	// - Señales de reinicio a procesos
	// - Comandos del sistema

	// Por ahora, registrar el intento
	s.recordIncident("WORKER_RESTART_ATTEMPT", "warning", fmt.Sprintf("Attempting to restart worker %s", name), map[string]interface{}{
		"worker":        name,
		"restart_count": worker.RestartCount,
	})

	// Esperar antes del siguiente intento
	time.Sleep(30 * time.Second)
}

// runDiskCleanup ejecuta limpieza preventiva de disco
func (s *Service) runDiskCleanup() {
	s.logger.Info("🧹 Running preventive disk cleanup")

	// Implementar limpieza preventiva:
	// - Limpiar archivos temporales antiguos
	// - Rotar logs prematuramente
	// - Comprimir archivos grandes

	s.recordIncident("DISK_CLEANUP", "info", "Preventive disk cleanup triggered", map[string]interface{}{
		"timestamp": time.Now(),
	})
}

// GetProtectionStatus obtiene el estado actual del modo protector
func (s *Service) GetProtectionStatus() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	status := map[string]interface{}{
		"active":   s.protectionMode.Load(),
		"duration": 0,
	}

	if s.protectionMode.Load() {
		status["duration"] = time.Since(s.protectionStart).Seconds()
		status["started_at"] = s.protectionStart
	}

	return status
}