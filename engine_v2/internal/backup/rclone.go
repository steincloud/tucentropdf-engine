package backup

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// RcloneManager maneja sincronización con servicios remotos usando rclone
type RcloneManager struct {
	config   *BackupConfig
	logger   *logger.Logger
	isHealthy bool
}

// SyncResult resultado de sincronización remota
type SyncResult struct {
	Success       bool          `json:"success"`
	FilesUploaded int           `json:"files_uploaded"`
	BytesUploaded int64         `json:"bytes_uploaded"`
	Duration      time.Duration `json:"duration"`
	Error         string        `json:"error,omitempty"`
}

// NewRcloneManager crea nueva instancia del gestor rclone
func NewRcloneManager(config *BackupConfig, log *logger.Logger) *RcloneManager {
	manager := &RcloneManager{
		config:    config,
		logger:    log,
		isHealthy: false,
	}

	// Verificar rclone al inicio
	go manager.healthCheck()

	return manager
}

// ValidateConfiguration valida la configuración de rclone
func (r *RcloneManager) ValidateConfiguration() error {
	// Verificar que rclone esté instalado
	if !r.isRcloneInstalled() {
		return fmt.Errorf("rclone is not installed or not in PATH")
	}

	// Verificar configuración de remoto
	if r.config.RemotePath == "" {
		return fmt.Errorf("RCLONE_REMOTE is not configured")
	}

	// Verificar que el remoto existe en rclone config
	if err := r.validateRemote(); err != nil {
		return fmt.Errorf("rclone remote validation failed: %w", err)
	}

	r.logger.Info("Rclone configuration validated", "remote", r.config.RemotePath)
	return nil
}

// SyncToRemote sincroniza archivos locales al remoto
func (r *RcloneManager) SyncToRemote(localDir string) (*SyncResult, error) {
	if !r.config.RemoteEnabled {
		return &SyncResult{Success: true, FilesUploaded: 0}, nil
	}

	r.logger.Info("🌐 Starting remote sync", "local", localDir, "remote", r.config.RemotePath)
	start := time.Now()

	// Preparar comando rclone sync
	args := []string{
		"sync",
		localDir,
		r.config.RemotePath,
		"--progress",
		"--stats-one-line",
		"--exclude", "*.tmp",
		"--exclude", "*.temp",
		"--retries", "3",
		"--low-level-retries", "10",
		"--timeout", "300s",
	}

	// Agregar config si está especificado
	if r.config.RcloneConfig != "" {
		args = append(args, "--config", r.config.RcloneConfig)
	}

	// Ejecutar rclone
	cmd := exec.Command("rclone", args...)
	output, err := cmd.CombinedOutput()

	duration := time.Since(start)
	result := &SyncResult{
		Duration: duration,
	}

	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("rclone sync failed: %s - Output: %s", err.Error(), string(output))
		r.logger.Error("Remote sync failed", "error", result.Error, "duration", duration)
		return result, err
	}

	// Procesar estadísticas de salida
	r.parseRcloneStats(string(output), result)

	result.Success = true
	r.logger.Info("Remote sync completed", 
		"files", result.FilesUploaded,
		"bytes", result.BytesUploaded,
		"duration", duration)

	return result, nil
}

// DownloadFromRemote descarga archivos del remoto al local
func (r *RcloneManager) DownloadFromRemote(remotePath, localDir string) error {
	if !r.config.RemoteEnabled {
		return fmt.Errorf("remote sync is not enabled")
	}

	r.logger.Info("⬇️ Downloading from remote", "remote", remotePath, "local", localDir)

	// Preparar comando rclone copy
	args := []string{
		"copy",
		remotePath,
		localDir,
		"--progress",
		"--retries", "3",
		"--low-level-retries", "10",
	}

	// Agregar config si está especificado
	if r.config.RcloneConfig != "" {
		args = append(args, "--config", r.config.RcloneConfig)
	}

	// Ejecutar rclone
	cmd := exec.Command("rclone", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("rclone download failed: %s - Output: %s", err.Error(), string(output))
	}

	r.logger.Info("Remote download completed", "remote", remotePath)
	return nil
}

// ListRemoteBackups lista backups disponibles en el remoto
func (r *RcloneManager) ListRemoteBackups() ([]string, error) {
	if !r.config.RemoteEnabled {
		return []string{}, nil
	}

	// Preparar comando rclone lsf (list files)
	args := []string{
		"lsf",
		r.config.RemotePath,
		"--recursive",
	}

	// Agregar config si está especificado
	if r.config.RcloneConfig != "" {
		args = append(args, "--config", r.config.RcloneConfig)
	}

	// Ejecutar rclone
	cmd := exec.Command("rclone", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return nil, fmt.Errorf("failed to list remote backups: %s", err.Error())
	}

	// Procesar lista de archivos
	files := strings.Split(strings.TrimSpace(string(output)), "\n")
	var backups []string

	for _, file := range files {
		if file != "" && (strings.HasSuffix(file, ".sql") || 
			strings.HasSuffix(file, ".sql.enc") ||
			strings.HasSuffix(file, ".rdb") ||
			strings.HasSuffix(file, ".rdb.enc") ||
			strings.HasSuffix(file, ".tar.gz") ||
			strings.HasSuffix(file, ".tar.gz.enc")) {
			backups = append(backups, file)
		}
	}

	return backups, nil
}

// DeleteRemoteFile elimina un archivo del remoto
func (r *RcloneManager) DeleteRemoteFile(remotePath string) error {
	if !r.config.RemoteEnabled {
		return nil
	}

	r.logger.Debug("Deleting remote file", "path", remotePath)

	// Preparar comando rclone delete
	args := []string{
		"deletefile",
		remotePath,
	}

	// Agregar config si está especificado
	if r.config.RcloneConfig != "" {
		args = append(args, "--config", r.config.RcloneConfig)
	}

	// Ejecutar rclone
	cmd := exec.Command("rclone", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("failed to delete remote file %s: %s - Output: %s", 
			remotePath, err.Error(), string(output))
	}

	return nil
}

// IsHealthy verifica si rclone está funcionando correctamente
func (r *RcloneManager) IsHealthy() bool {
	return r.isHealthy
}

// healthCheck verifica periódicamente la salud de rclone
func (r *RcloneManager) healthCheck() {
	// Verificación inicial
	r.checkHealth()

	// Verificar cada 30 minutos
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		r.checkHealth()
	}
}

// checkHealth verifica el estado de rclone
func (r *RcloneManager) checkHealth() {
	// Verificar instalación
	if !r.isRcloneInstalled() {
		r.isHealthy = false
		return
	}

	// Si no está habilitado el remoto, marcar como saludable
	if !r.config.RemoteEnabled {
		r.isHealthy = true
		return
	}

	// Verificar conectividad con un comando simple
	if err := r.testConnectivity(); err != nil {
		r.logger.Error("Rclone connectivity test failed", "error", err)
		r.isHealthy = false
		return
	}

	r.isHealthy = true
}

// isRcloneInstalled verifica si rclone está instalado
func (r *RcloneManager) isRcloneInstalled() bool {
	_, err := exec.LookPath("rclone")
	return err == nil
}

// validateRemote valida que el remoto esté configurado
func (r *RcloneManager) validateRemote() error {
	// Extraer nombre del remoto del path
	remoteName := r.getRemoteName()
	if remoteName == "" {
		return fmt.Errorf("invalid remote path format")
	}

	// Listar remotes configurados
	cmd := exec.Command("rclone", "listremotes")
	if r.config.RcloneConfig != "" {
		cmd.Args = append(cmd.Args, "--config", r.config.RcloneConfig)
	}

	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list remotes: %w", err)
	}

	remotes := strings.Split(string(output), "\n")
	for _, remote := range remotes {
		if strings.TrimSpace(remote) == remoteName+":" {
			return nil
		}
	}

	return fmt.Errorf("remote '%s' not found in rclone config", remoteName)
}

// testConnectivity prueba conectividad con el remoto
func (r *RcloneManager) testConnectivity() error {
	// Usar 'rclone about' para verificar conectividad
	remoteName := r.getRemoteName()
	if remoteName == "" {
		return fmt.Errorf("invalid remote configuration")
	}

	args := []string{"about", remoteName + ":"}
	if r.config.RcloneConfig != "" {
		args = append(args, "--config", r.config.RcloneConfig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "rclone", args...)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return fmt.Errorf("connectivity test failed: %s - Output: %s", err.Error(), string(output))
	}

	return nil
}

// getRemoteName extrae el nombre del remoto del path
func (r *RcloneManager) getRemoteName() string {
	parts := strings.Split(r.config.RemotePath, ":")
	if len(parts) >= 2 {
		return parts[0]
	}
	return ""
}

// parseRcloneStats procesa las estadísticas de rclone
func (r *RcloneManager) parseRcloneStats(output string, result *SyncResult) {
	lines := strings.Split(output, "\n")
	
	for _, line := range lines {
		// Buscar líneas con estadísticas
		if strings.Contains(line, "Transferred:") {
			// Parsear bytes transferidos
			if idx := strings.Index(line, "Bytes"); idx > 0 {
				// Implementación básica - se puede mejorar con regex
				result.BytesUploaded = 0 // Placeholder
			}
		}
		
		if strings.Contains(line, "Checks:") && strings.Contains(line, "Transfers:") {
			// Parsear número de archivos
			// Implementación básica - se puede mejorar con regex
			result.FilesUploaded = 1 // Placeholder
		}
	}
}

// GetRemoteQuota obtiene información de cuota del remoto (si está disponible)
func (r *RcloneManager) GetRemoteQuota() (map[string]interface{}, error) {
	if !r.config.RemoteEnabled {
		return nil, fmt.Errorf("remote sync is not enabled")
	}

	remoteName := r.getRemoteName()
	args := []string{"about", remoteName + ":", "--json"}
	
	if r.config.RcloneConfig != "" {
		args = append(args, "--config", r.config.RcloneConfig)
	}

	cmd := exec.Command("rclone", args...)
	output, err := cmd.Output()

	if err != nil {
		return nil, fmt.Errorf("failed to get remote quota: %w", err)
	}

	// Retornar como mapa básico (se puede parsear JSON si se necesita)
	return map[string]interface{}{
		"raw_output": string(output),
	}, nil
}