package monitor

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
	"gorm.io/gorm"

	"github.com/tucentropdf/engine-v2/internal/alerts"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// Service servicio principal de monitoreo del sistema
type Service struct {
	db           *gorm.DB
	redis        *redis.Client
	config       *config.Config
	logger       *logger.Logger
	alertService *alerts.Service
	ctx          context.Context
	cancel       context.CancelFunc

	// Estado del sistema
	systemStatus     *SystemStatus
	protectionMode   atomic.Bool  // Thread-safe usando atomic
	protectionStart  time.Time
	mu               sync.RWMutex

	// Configuración de umbrales
	thresholds *Thresholds

	// Workers health
	workerHealth map[string]*WorkerStatus
	workerMu     sync.RWMutex

	// Métricas de rendimiento
	metrics *PerformanceMetrics
}

// SystemStatus estado completo del sistema
type SystemStatus struct {
	Uptime       time.Duration              `json:"uptime"`
	Status       string                     `json:"status"` // ok, degraded, critical
	Workers      map[string]*WorkerStatus   `json:"workers"`
	Redis        *RedisStatus              `json:"redis"`
	Resources    *ResourceStatus           `json:"resources"`
	Queue        *QueueStatus              `json:"queue"`
	ProtectorMode bool                     `json:"protector_mode"`
	LastCheck    time.Time                 `json:"last_check"`
}

// WorkerStatus estado de un worker
type WorkerStatus struct {
	Name         string    `json:"name"`
	Status       string    `json:"status"` // ok, warning, failed
	LastSeen     time.Time `json:"last_seen"`
	Latency      int64     `json:"latency_ms"`
	MemoryUsage  string    `json:"memory_usage"`
	HealthURL    string    `json:"health_url"`
	RestartCount int       `json:"restart_count"`
}

// RedisStatus estado de Redis
type RedisStatus struct {
	Alive     bool  `json:"alive"`
	Latency   int64 `json:"latency_ms"`
	Keys      int64 `json:"keys"`
	MemoryMB  int64 `json:"memory_mb"`
}

// ResourceStatus estado de recursos del sistema
type ResourceStatus struct {
	CPUPercent  float64 `json:"cpu_percent"`
	RAMPercent  float64 `json:"ram_percent"`
	DiskPercent float64 `json:"disk_percent"`
	RAMUsedMB   int64   `json:"ram_used_mb"`
	RAMTotalMB  int64   `json:"ram_total_mb"`
}

// QueueStatus estado de colas de trabajo
type QueueStatus struct {
	PendingJobs int `json:"pending_jobs"`
	PDFQueue    int `json:"pdf_queue"`
	OCRQueue    int `json:"ocr_queue"`
	OfficeQueue int `json:"office_queue"`
}

// Thresholds umbrales de alerta del sistema
type Thresholds struct {
	CPU struct {
		Warning  float64 `json:"warning"`  // 80%
		Critical float64 `json:"critical"` // 90%
	}
	RAM struct {
		Warning    float64 `json:"warning"`    // 75%
		Critical   float64 `json:"critical"`   // 85%
		Emergency  float64 `json:"emergency"`  // 90%
	}
	Disk struct {
		Warning  float64 `json:"warning"`  // 80%
		Critical float64 `json:"critical"` // 90%
	}
	Queue struct {
		Warning  int `json:"warning"`  // 20
		Critical int `json:"critical"` // 50
		Max      int `json:"max"`      // 100
	}
	Redis struct {
		LatencyWarning  int64 `json:"latency_warning"`  // 50ms
		LatencyCritical int64 `json:"latency_critical"` // 200ms
	}
}

// PerformanceMetrics métricas de rendimiento
type PerformanceMetrics struct {
	AvgResponseTime time.Duration
	TotalRequests   int64
	FailedRequests  int64
	LastUpdated     time.Time
	mu              sync.RWMutex
}

// NewService crea nueva instancia del servicio de monitoreo
func NewService(db *gorm.DB, redisClient *redis.Client, cfg *config.Config, log *logger.Logger) *Service {
	ctx, cancel := context.WithCancel(context.Background())

	// Crear servicio de alertas
	alertService := alerts.NewService(cfg, log)

	return &Service{
		db:           db,
		redis:        redisClient,
		config:       cfg,
		logger:       log,
		alertService: alertService,
		ctx:          ctx,
		cancel:       cancel,
		systemStatus: &SystemStatus{
			Uptime:        0,
			Status:        "starting",
			Workers:       make(map[string]*WorkerStatus),
			ProtectorMode: false,
		},
		thresholds:     getDefaultThresholds(),
		workerHealth:   make(map[string]*WorkerStatus),
		metrics: &PerformanceMetrics{
			LastUpdated: time.Now(),
		},
	}
}

// getDefaultThresholds obtiene umbrales por defecto
func getDefaultThresholds() *Thresholds {
	return &Thresholds{
		CPU: struct {
			Warning  float64 `json:"warning"`
			Critical float64 `json:"critical"`
		}{Warning: 80.0, Critical: 90.0},
		RAM: struct {
			Warning    float64 `json:"warning"`
			Critical   float64 `json:"critical"`
			Emergency  float64 `json:"emergency"`
		}{Warning: 75.0, Critical: 85.0, Emergency: 90.0},
		Disk: struct {
			Warning  float64 `json:"warning"`
			Critical float64 `json:"critical"`
		}{Warning: 80.0, Critical: 90.0},
		Queue: struct {
			Warning  int `json:"warning"`
			Critical int `json:"critical"`
			Max      int `json:"max"`
		}{Warning: 20, Critical: 50, Max: 100},
		Redis: struct {
			LatencyWarning  int64 `json:"latency_warning"`
			LatencyCritical int64 `json:"latency_critical"`
		}{LatencyWarning: 50, LatencyCritical: 200},
	}
}

// Start inicia el servicio de monitoreo con scheduler inteligente
func (s *Service) Start() {
	s.logger.Info("📊 Starting internal monitoring service...")

	// Configurar workers conocidos
	s.setupKnownWorkers()

	// Crear tabla de incidentes si no existe
	if s.db != nil {
		if err := s.createIncidentsTable(); err != nil {
			s.logger.Error("Error creating incidents table", "error", err)
		}
	}

	// Iniciar scheduler de checks
	go s.startScheduler()

	s.logger.Info("✅ Internal monitoring service started")
}

// Stop detiene el servicio de monitoreo
func (s *Service) Stop() {
	s.logger.Info("🛑 Stopping monitoring service...")
	s.cancel()
	s.logger.Info("✅ Monitoring service stopped")
}

// setupKnownWorkers configura workers conocidos del sistema
func (s *Service) setupKnownWorkers() {
	s.workerMu.Lock()
	defer s.workerMu.Unlock()

	// Workers conocidos (URLs deben ser configurables)
	knownWorkers := map[string]string{
		"ocr":    "http://localhost:8081/internal/health",
		"office": "http://localhost:8082/internal/health", 
	}

	for name, healthURL := range knownWorkers {
		s.workerHealth[name] = &WorkerStatus{
			Name:      name,
			Status:    "unknown",
			LastSeen:  time.Time{},
			HealthURL: healthURL,
		}
	}
}

// startScheduler inicia los schedulers de checks con diferentes frecuencias
func (s *Service) startScheduler() {
	startTime := time.Now()

	// Cada 10 segundos - checks críticos
	go s.runPeriodicChecks(10*time.Second, func() {
		s.CheckWorkers()
	}, "workers")

	// Cada 15 segundos - recursos
	go s.runPeriodicChecks(15*time.Second, func() {
		s.CheckCPU()
		s.CheckRAM()
		s.CheckQueue()
	}, "resources")

	// Cada 1 minuto - Redis
	go s.runPeriodicChecks(1*time.Minute, func() {
		s.CheckRedis()
	}, "redis")

	// Cada 5 minutos - disco
	go s.runPeriodicChecks(5*time.Minute, func() {
		s.CheckDisk()
	}, "disk")

	// Cada 30 segundos - update status general
	go s.runPeriodicChecks(30*time.Second, func() {
		s.updateSystemStatus(startTime)
	}, "status")

	// Cada 2 minutos - verificar modo protector
	go s.runPeriodicChecks(2*time.Minute, func() {
		s.checkProtectionMode()
	}, "protection")
}

// runPeriodicChecks ejecuta checks periódicos con timeout
func (s *Service) runPeriodicChecks(interval time.Duration, checkFunc func(), checkName string) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			// Ejecutar check con timeout para prevenir hangs
			ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
			done := make(chan struct{})

			go func() {
				defer func() {
					if r := recover(); r != nil {
						s.logger.Error("Panic in monitoring check", "check", checkName, "error", r)
					}
					close(done)
				}()
				checkFunc()
			}()

			// Esperar completitud o timeout
			select {
			case <-done:
				cancel()
			case <-ctx.Done():
				s.logger.Warn("Monitoring check timed out", "check", checkName)
				cancel()
			}

		case <-s.ctx.Done():
			return
		}
	}
}

// updateSystemStatus actualiza el estado general del sistema
func (s *Service) updateSystemStatus(startTime time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.systemStatus.Uptime = time.Since(startTime)
	s.systemStatus.LastCheck = time.Now()
	s.systemStatus.ProtectorMode = s.protectionMode.Load()

	// Copiar worker status
	s.workerMu.RLock()
	s.systemStatus.Workers = make(map[string]*WorkerStatus)
	for k, v := range s.workerHealth {
		s.systemStatus.Workers[k] = v
	}
	s.workerMu.RUnlock()

	// Determinar status general
	s.systemStatus.Status = s.calculateOverallStatus()
}

// calculateOverallStatus calcula el estado general del sistema
func (s *Service) calculateOverallStatus() string {
	if s.protectionMode.Load() {
		return "critical"
	}

	// Verificar workers
	for _, worker := range s.systemStatus.Workers {
		if worker.Status == "failed" {
			return "degraded"
		}
	}

	// Verificar recursos si están disponibles
	if s.systemStatus.Resources != nil {
		if s.systemStatus.Resources.CPUPercent > s.thresholds.CPU.Critical ||
			s.systemStatus.Resources.RAMPercent > s.thresholds.RAM.Critical ||
			s.systemStatus.Resources.DiskPercent > s.thresholds.Disk.Critical {
			return "critical"
		}
		if s.systemStatus.Resources.CPUPercent > s.thresholds.CPU.Warning ||
			s.systemStatus.Resources.RAMPercent > s.thresholds.RAM.Warning ||
			s.systemStatus.Resources.DiskPercent > s.thresholds.Disk.Warning {
			return "degraded"
		}
	}

	// Verificar cola
	if s.systemStatus.Queue != nil {
		if s.systemStatus.Queue.PendingJobs > s.thresholds.Queue.Critical {
			return "degraded"
		}
	}

	return "ok"
}

// GetSystemStatus obtiene el estado actual del sistema (thread-safe)
func (s *Service) GetSystemStatus() *SystemStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Crear copia para evitar race conditions
	status := &SystemStatus{
		Uptime:        s.systemStatus.Uptime,
		Status:        s.systemStatus.Status,
		Workers:       make(map[string]*WorkerStatus),
		Redis:         s.systemStatus.Redis,
		Resources:     s.systemStatus.Resources,
		Queue:         s.systemStatus.Queue,
		ProtectorMode: s.systemStatus.ProtectorMode,
		LastCheck:     s.systemStatus.LastCheck,
	}

	// Copiar workers
	for k, v := range s.systemStatus.Workers {
		workerCopy := *v
		status.Workers[k] = &workerCopy
	}

	return status
}

// createIncidentsTable crea tabla de incidentes del sistema
func (s *Service) createIncidentsTable() error {
	createSql := `
		CREATE TABLE IF NOT EXISTS system_incidents (
			id SERIAL PRIMARY KEY,
			type VARCHAR(50) NOT NULL,
			severity VARCHAR(20) NOT NULL DEFAULT 'info',
			message TEXT NOT NULL,
			details JSONB,
			timestamp TIMESTAMP DEFAULT NOW(),
			resolved BOOLEAN DEFAULT FALSE,
			resolved_at TIMESTAMP,
			created_at TIMESTAMP DEFAULT NOW()
		);
		CREATE INDEX IF NOT EXISTS idx_incidents_type ON system_incidents(type);
		CREATE INDEX IF NOT EXISTS idx_incidents_timestamp ON system_incidents(timestamp);
		CREATE INDEX IF NOT EXISTS idx_incidents_resolved ON system_incidents(resolved);
	`

	return s.db.Exec(createSql).Error
}

// recordIncident registra un incidente en la base de datos con timeout
func (s *Service) recordIncident(incidentType, severity, message string, details map[string]interface{}) {
	if s.db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Insertar incidente con context
	result := s.db.WithContext(ctx).Exec(
		"INSERT INTO system_incidents (type, severity, message, details) VALUES (?, ?, ?, ?)",
		incidentType, severity, message, details,
	)

	if result.Error != nil {
		s.logger.Error("Error recording incident", "error", result.Error)
	}
}

// recordIncidentLegacy es el método legacy mantenido para compatibilidad
func (s *Service) recordIncidentLegacy(incidentType, severity, message string, details map[string]interface{}) {
	if s.db == nil {
		return
	}

	var detailsJSON interface{}
	if details != nil {
		detailsJSON = details
	}

	insertSql := `
		INSERT INTO system_incidents (type, severity, message, details) 
		VALUES (?, ?, ?, ?)
	`

	if err := s.db.Exec(insertSql, incidentType, severity, message, detailsJSON).Error; err != nil {
		s.logger.Error("Error recording incident", "error", err, "type", incidentType)
	}
}

// IsInProtectionMode verifica si el sistema está en modo protector
func (s *Service) IsInProtectionMode() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.protectionMode.Load()
}