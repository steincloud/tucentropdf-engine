package backup

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// Manager gestiona backups automatizados
type Manager struct {
	logger       *logger.Logger
	db           *sql.DB
	config       *Config
}

// Config configuración de backups
type Config struct {
	Enabled         bool
	BackupDir       string
	S3Bucket        string
	S3Region        string
	RetentionDays   int
	Schedule        string // Cron expression
	DatabaseURL     string
	EncryptionKey   string
}

// NewManager crea un nuevo manager de backups
func NewManager(log *logger.Logger, db *sql.DB, config *Config) *Manager {
	return &Manager{
		logger: log,
		db:     db,
		config: config,
	}
}

// BackupDatabase crea backup de PostgreSQL
func (bm *Manager) BackupDatabase(ctx context.Context) (string, error) {
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("tucentropdf_backup_%s.sql.gz", timestamp)
	filepath := filepath.Join(bm.config.BackupDir, filename)

	bm.logger.Info("Starting database backup", "file", filename)

	// pg_dump con compresión
	cmd := exec.CommandContext(ctx,
		"pg_dump",
		bm.config.DatabaseURL,
		"--format=custom",
		"--compress=9",
		"--file="+filepath,
	)

	if output, err := cmd.CombinedOutput(); err != nil {
		bm.logger.Error("Database backup failed", "error", err, "output", string(output))
		return "", fmt.Errorf("backup failed: %w", err)
	}

	// Verificar tamaño
	info, _ := os.Stat(filepath)
	bm.logger.Info("Database backup completed",
		"file", filename,
		"size_mb", float64(info.Size())/1024/1024,
	)

	// Subir a S3 si está configurado
	if bm.config.S3Bucket != "" {
		if err := bm.uploadToS3(ctx, filepath); err != nil {
			bm.logger.Warn("S3 upload failed", "error", err)
		}
	}

	return filepath, nil
}

// RestoreDatabase restaura desde backup
func (bm *Manager) RestoreDatabase(ctx context.Context, backupFile string) error {
	bm.logger.Info("Starting database restore", "file", backupFile)

	cmd := exec.CommandContext(ctx,
		"pg_restore",
		"--clean",
		"--if-exists",
		"--dbname="+bm.config.DatabaseURL,
		backupFile,
	)

	if output, err := cmd.CombinedOutput(); err != nil {
		bm.logger.Error("Database restore failed", "error", err, "output", string(output))
		return fmt.Errorf("restore failed: %w", err)
	}

	bm.logger.Info("Database restore completed", "file", backupFile)
	return nil
}

// uploadToS3 sube backup a S3
func (bm *Manager) uploadToS3(ctx context.Context, filepath string) error {
	// TODO: Implementar upload real a S3
	// aws s3 cp filepath s3://bucket/backups/
	bm.logger.Info("Uploading backup to S3", "file", filepath)
	return nil
}

// CleanupOldBackups elimina backups antiguos
func (bm *Manager) CleanupOldBackups(ctx context.Context) error {
	cutoff := time.Now().AddDate(0, 0, -bm.config.RetentionDays)

	files, err := os.ReadDir(bm.config.BackupDir)
	if err != nil {
		return err
	}

	deleted := 0
	for _, file := range files {
		info, _ := file.Info()
		if info.ModTime().Before(cutoff) {
			path := filepath.Join(bm.config.BackupDir, file.Name())
			if err := os.Remove(path); err == nil {
				deleted++
			}
		}
	}

	bm.logger.Info("Cleaned up old backups", "deleted", deleted)
	return nil
}

// ScheduleBackups ejecuta backups periódicamente
func (bm *Manager) ScheduleBackups(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := bm.BackupDatabase(ctx); err != nil {
				bm.logger.Error("Scheduled backup failed", "error", err)
			}
			bm.CleanupOldBackups(ctx)
		}
	}
}

// ListBackups lista backups disponibles
func (bm *Manager) ListBackups() ([]BackupInfo, error) {
	files, err := os.ReadDir(bm.config.BackupDir)
	if err != nil {
		return nil, err
	}

	var backups []BackupInfo
	for _, file := range files {
		info, _ := file.Info()
		backups = append(backups, BackupInfo{
			Filename:  file.Name(),
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
		})
	}

	return backups, nil
}

// BackupInfo is defined in service.go; avoid duplicate declarations here.
