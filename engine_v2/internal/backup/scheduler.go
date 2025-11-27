package backup

import (
	"context"
	"fmt"
	"time"

	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// Scheduler maneja la programación automática de backups
type Scheduler struct {
	service *Service
	logger  *logger.Logger
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewScheduler crea nuevo scheduler de backups
func NewScheduler(service *Service, log *logger.Logger) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		service: service,
		logger:  log,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start inicia el scheduler de backups
func (s *Scheduler) Start() {
	s.logger.Info("🕰️ Starting backup scheduler...")

	// Backup completo diario (02:00 AM)
	go s.runDailyBackups()

	// Backups incrementales cada 6 horas
	go s.runIncrementalBackups()

	// Backups de Redis cada 12 horas
	go s.runRedisBackups()

	// Limpieza de retención cada 24 horas (03:00 AM)
	go s.runRetentionCleanup()

	s.logger.Info("✅ Backup scheduler started")
}

// Stop detiene el scheduler
func (s *Scheduler) Stop() {
	s.logger.Info("🛑 Stopping backup scheduler...")
	s.cancel()
	s.logger.Info("✅ Backup scheduler stopped")
}

// runDailyBackups ejecuta backups diarios a las 02:00 AM con timeout
func (s *Scheduler) runDailyBackups() {
	// Calcular próxima ejecución a las 02:00 AM
	now := time.Now()
	next2AM := s.getNext2AM(now)
	timeUntilNext := next2AM.Sub(now)

	s.logger.Info("Daily backups scheduled", "next_execution", next2AM.Format("2006-01-02 15:04:05"))

	// Timer inicial
	initialTimer := time.NewTimer(timeUntilNext)
	defer initialTimer.Stop()

	select {
	case <-initialTimer.C:
		s.executeDailyBackupsWithTimeout()
	case <-s.ctx.Done():
		return
	}

	// Ticker cada 24 horas
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.executeDailyBackupsWithTimeout()
		case <-s.ctx.Done():
			return
		}
	}
}

// runIncrementalBackups ejecuta backups incrementales cada 6 horas con timeout
func (s *Scheduler) runIncrementalBackups() {
	// Ejecutar inmediatamente al inicio con timeout
	go s.executeIncrementalBackupWithTimeout()

	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()

	s.logger.Info("Incremental backups scheduled every 6 hours")

	for {
		select {
		case <-ticker.C:
			s.executeIncrementalBackupWithTimeout()
		case <-s.ctx.Done():
			return
		}
	}
}

// runRedisBackups ejecuta backups de Redis cada 12 horas
func (s *Scheduler) runRedisBackups() {
	// Ejecutar inmediatamente al inicio con timeout
	go s.executeRedisBackupWithTimeout()

	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()

	s.logger.Info("Redis backups scheduled every 12 hours")

	for {
		select {
		case <-ticker.C:
			s.executeRedisBackupWithTimeout()
		case <-s.ctx.Done():
			return
		}
	}
}

// runRetentionCleanup ejecuta limpieza de retención diaria a las 03:00 AM
func (s *Scheduler) runRetentionCleanup() {
	// Calcular próxima ejecución a las 03:00 AM
	now := time.Now()
	next3AM := s.getNext3AM(now)
	timeUntilNext := next3AM.Sub(now)

	s.logger.Info("Retention cleanup scheduled", "next_execution", next3AM.Format("2006-01-02 15:04:05"))

	// Timer inicial
	initialTimer := time.NewTimer(timeUntilNext)
	defer initialTimer.Stop()

	select {
	case <-initialTimer.C:
		s.executeRetentionCleanup()
	case <-s.ctx.Done():
		return
	}

	// Ticker cada 24 horas
	ticker := time.NewTicker(24 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.executeRetentionCleanup()
		case <-s.ctx.Done():
			return
		}
	}
}

// executeDailyBackupsWithTimeout ejecuta todos los backups diarios con timeout
func (s *Scheduler) executeDailyBackupsWithTimeout() {
	ctx, cancel := context.WithTimeout(s.ctx, 2*time.Hour)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.executeDailyBackups()
	}()

	select {
	case <-done:
		s.logger.Info("Daily backups completed successfully")
	case <-ctx.Done():
		s.logger.Error("Daily backups timed out after 2 hours")
		s.service.sendAlert("BACKUP_TIMEOUT", "critical", "Daily backups exceeded 2 hour timeout")
	}
}

// executeDailyBackups ejecuta todos los backups diarios
func (s *Scheduler) executeDailyBackups() {
	s.logger.Info("🌅 Starting daily backup routine")

	start := time.Now()
	successCount := 0
	totalTasks := 4

	// 1. Backup completo de PostgreSQL
	if err := s.service.FullBackupPostgreSQL(); err != nil {
		s.logger.Error("Daily PostgreSQL full backup failed", "error", err)
		s.service.sendAlert("BACKUP_PG_FULL_FAILED", "critical", fmt.Sprintf("PostgreSQL full backup failed: %v", err))
	} else {
		successCount++
		s.logger.Info("PostgreSQL full backup completed")
	}

	// 2. Backup de configuración del sistema
	if err := s.service.BackupSystemConfig(); err != nil {
		s.logger.Error("System config backup failed", "error", err)
		s.service.sendAlert("BACKUP_CONFIG_FAILED", "warning", fmt.Sprintf("System config backup failed: %v", err))
	} else {
		successCount++
		s.logger.Info("System config backup completed")
	}

	// 3. Backup de analytics archivadas (mensual)
	if s.shouldRunMonthlyAnalyticsBackup() {
		if err := s.service.BackupAnalyticsArchive(); err != nil {
			s.logger.Error("Analytics archive backup failed", "error", err)
			s.service.sendAlert("BACKUP_ANALYTICS_FAILED", "warning", fmt.Sprintf("Analytics backup failed: %v", err))
		} else {
			successCount++
			s.logger.Info("Analytics archive backup completed")
		}
	} else {
		successCount++ // No era necesario este mes
	}

	// 4. Verificar espacio en disco
	if err := s.service.checkDiskSpace(); err != nil {
		s.logger.Error("Disk space check failed", "error", err)
		s.service.sendAlert("BACKUP_DISK_SPACE_LOW", "critical", fmt.Sprintf("Insufficient disk space: %v", err))
	} else {
		successCount++
	}

	duration := time.Since(start)
	s.logger.Info("Daily backup routine completed", 
		"success_rate", fmt.Sprintf("%d/%d", successCount, totalTasks),
		"duration", duration.String())

	// Enviar resumen si hay fallos
	if successCount < totalTasks {
		s.service.sendAlert("BACKUP_DAILY_PARTIAL_FAILURE", "warning", 
			fmt.Sprintf("Daily backup completed with %d/%d tasks successful", successCount, totalTasks))
	}
}

// executeIncrementalBackupWithTimeout ejecuta backup incremental con timeout
func (s *Scheduler) executeIncrementalBackupWithTimeout() {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Minute)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.executeIncrementalBackup()
	}()

	select {
	case <-done:
		return
	case <-ctx.Done():
		s.logger.Error("Incremental backup timed out after 30 minutes")
		s.service.sendAlert("BACKUP_INCREMENTAL_TIMEOUT", "warning", "Incremental backup exceeded 30 minute timeout")
	}
}

// executeIncrementalBackup ejecuta backup incremental de PostgreSQL
func (s *Scheduler) executeIncrementalBackup() {
	s.logger.Info("📈 Starting incremental PostgreSQL backup")

	if err := s.service.IncrementalBackupPostgreSQL(); err != nil {
		s.logger.Error("Incremental PostgreSQL backup failed", "error", err)
		s.service.sendAlert("BACKUP_PG_INCREMENTAL_FAILED", "warning", fmt.Sprintf("PostgreSQL incremental backup failed: %v", err))
	} else {
		s.logger.Info("Incremental PostgreSQL backup completed")
	}
}

// executeRedisBackupWithTimeout ejecuta backup de Redis con timeout
func (s *Scheduler) executeRedisBackupWithTimeout() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Minute)
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.executeRedisBackup()
	}()

	select {
	case <-done:
		return
	case <-ctx.Done():
		s.logger.Error("Redis backup timed out after 10 minutes")
		s.service.sendAlert("BACKUP_REDIS_TIMEOUT", "warning", "Redis backup exceeded 10 minute timeout")
	}
}

// executeRedisBackup ejecuta backup de Redis
func (s *Scheduler) executeRedisBackup() {
	s.logger.Info("🐎 Starting Redis snapshot backup")

	if err := s.service.BackupRedisSnapshot(); err != nil {
		s.logger.Error("Redis backup failed", "error", err)
		s.service.sendAlert("BACKUP_REDIS_FAILED", "warning", fmt.Sprintf("Redis backup failed: %v", err))
	} else {
		s.logger.Info("Redis backup completed")
	}
}

// executeRetentionCleanup ejecuta limpieza de retención
func (s *Scheduler) executeRetentionCleanup() {
	s.logger.Info("🧹 Starting retention cleanup")

	if err := s.service.CleanOldBackups(); err != nil {
		s.logger.Error("Retention cleanup failed", "error", err)
		s.service.sendAlert("BACKUP_CLEANUP_FAILED", "warning", fmt.Sprintf("Retention cleanup failed: %v", err))
	} else {
		s.logger.Info("Retention cleanup completed")
	}
}

// getNext2AM calcula la próxima ejecución a las 2:00 AM
func (s *Scheduler) getNext2AM(now time.Time) time.Time {
	next2AM := time.Date(now.Year(), now.Month(), now.Day(), 2, 0, 0, 0, now.Location())
	if next2AM.Before(now) {
		next2AM = next2AM.AddDate(0, 0, 1)
	}
	return next2AM
}

// getNext3AM calcula la próxima ejecución a las 3:00 AM
func (s *Scheduler) getNext3AM(now time.Time) time.Time {
	next3AM := time.Date(now.Year(), now.Month(), now.Day(), 3, 0, 0, 0, now.Location())
	if next3AM.Before(now) {
		next3AM = next3AM.AddDate(0, 0, 1)
	}
	return next3AM
}

// shouldRunMonthlyAnalyticsBackup verifica si debe ejecutar backup mensual de analytics
func (s *Scheduler) shouldRunMonthlyAnalyticsBackup() bool {
	// Ejecutar el día 1 de cada mes
	return time.Now().Day() == 1
}