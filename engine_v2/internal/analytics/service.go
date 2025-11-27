package analytics

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/tucentropdf/engine-v2/internal/analytics/models"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// Service servicio principal de analíticas
type Service struct {
	db     *gorm.DB
	redis  *redis.Client
	config *config.Config
	logger *logger.Logger
	ctx    context.Context
}

// NewService crea nueva instancia del servicio de analytics con connection pooling optimizado
func NewService(db *gorm.DB, redisClient *redis.Client, cfg *config.Config, log *logger.Logger) *Service {
	// Configurar connection pooling para PostgreSQL
	sqlDB, err := db.DB()
	if err == nil {
		// Conexiones máximas: 25 (balance entre concurrencia y recursos)
		sqlDB.SetMaxOpenConns(25)
		
		// Conexiones idle: 5 (reducir overhead de apertura/cierre)
		sqlDB.SetMaxIdleConns(5)
		
		// Lifetime máximo de conexión: 1 hora (prevenir conexiones stale)
		sqlDB.SetConnMaxLifetime(1 * time.Hour)
		
		// Tiempo máximo idle: 10 minutos
		sqlDB.SetConnMaxIdleTime(10 * time.Minute)
		
		log.Info("Analytics connection pool configured",
			"max_open", 25,
			"max_idle", 5,
			"max_lifetime", "1h",
		)
	}
	
	return &Service{
		db:     db,
		redis:  redisClient,
		config: cfg,
		logger: log,
		ctx:    context.Background(),
	}
}

// RegisterOperation registra una operación completada
func (s *Service) RegisterOperation(op *models.AnalyticsOperation) error {
	// Generar ID si no existe
	if op.ID == uuid.Nil {
		op.ID = uuid.New()
	}

	// Establecer timestamp si no existe
	if op.Timestamp.IsZero() {
		op.Timestamp = time.Now()
	}

	// Guardar en PostgreSQL
	if err := s.db.Create(op).Error; err != nil {
		s.logger.Error("Error saving operation to database", "error", err)
		return fmt.Errorf("failed to save operation: %w", err)
	}

	// Actualizar contadores en Redis con timeout
	go func(operation *models.AnalyticsOperation) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.updateRedisCountersWithContext(ctx, operation)
	}(op)

	// Log estructurado para monitoreo externo con timeout
	go func(operation *models.AnalyticsOperation) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		s.logOperationForMonitoringWithContext(ctx, operation)
	}(op)

	return nil
}

// RegisterFailure registra un fallo de operación
func (s *Service) RegisterFailure(userID, plan, tool, reason string, duration int64) error {
	op := &models.AnalyticsOperation{
		ID:         uuid.New(),
		UserID:     userID,
		Plan:       plan,
		Tool:       tool,
		Status:     "failed",
		FailReason: reason,
		Duration:   duration,
		Timestamp:  time.Now(),
	}

	return s.RegisterOperation(op)
}

// updateRedisCountersWithContext actualiza contadores rápidos en Redis con context
func (s *Service) updateRedisCountersWithContext(ctx context.Context, op *models.AnalyticsOperation) {
	today := time.Now().Format("2006-01-02")
	thisMonth := time.Now().Format("2006-01")

	pipe := s.redis.Pipeline()

	// Contadores globales por herramienta
	pipe.Incr(ctx, fmt.Sprintf("tool:%s:daily_count:%s", op.Tool, today))
	pipe.Incr(ctx, fmt.Sprintf("tool:%s:monthly_count:%s", op.Tool, thisMonth))
	pipe.Expire(ctx, fmt.Sprintf("tool:%s:daily_count:%s", op.Tool, today), 25*time.Hour)
	pipe.Expire(ctx, fmt.Sprintf("tool:%s:monthly_count:%s", op.Tool, thisMonth), 32*24*time.Hour)

	// Contadores por usuario
	pipe.Incr(ctx, fmt.Sprintf("user:%s:daily_count:%s", op.UserID, today))
	pipe.Incr(ctx, fmt.Sprintf("user:%s:monthly_count:%s", op.UserID, thisMonth))
	pipe.Incr(ctx, fmt.Sprintf("user:%s:tool_usage:%s", op.UserID, op.Tool))
	pipe.Expire(ctx, fmt.Sprintf("user:%s:daily_count:%s", op.UserID, today), 25*time.Hour)
	pipe.Expire(ctx, fmt.Sprintf("user:%s:monthly_count:%s", op.UserID, thisMonth), 32*24*time.Hour)

	// Contadores por plan
	pipe.Incr(ctx, fmt.Sprintf("plan:%s:daily_count:%s", op.Plan, today))
	pipe.Incr(ctx, fmt.Sprintf("plan:%s:monthly_count:%s", op.Plan, thisMonth))
	pipe.Expire(ctx, fmt.Sprintf("plan:%s:daily_count:%s", op.Plan, today), 25*time.Hour)
	pipe.Expire(ctx, fmt.Sprintf("plan:%s:monthly_count:%s", op.Plan, thisMonth), 32*24*time.Hour)

	// Contadores de estado
	if op.Status == "success" {
		pipe.Incr(ctx, fmt.Sprintf("tool:%s:success_count:%s", op.Tool, today))
		pipe.Expire(ctx, fmt.Sprintf("tool:%s:success_count:%s", op.Tool, today), 25*time.Hour)
	} else {
		pipe.Incr(ctx, fmt.Sprintf("tool:%s:fail_count:%s", op.Tool, today))
		pipe.Expire(ctx, fmt.Sprintf("tool:%s:fail_count:%s", op.Tool, today), 25*time.Hour)
		if op.FailReason != "" {
			pipe.Incr(ctx, fmt.Sprintf("fail_reason:%s:%s", op.FailReason, today))
			pipe.Expire(ctx, fmt.Sprintf("fail_reason:%s:%s", op.FailReason, today), 25*time.Hour)
		}
	}

	// Estadísticas de rendimiento (promedio móvil)
	if op.Duration > 0 {
		pipe.LPush(ctx, fmt.Sprintf("tool:%s:durations", op.Tool), op.Duration)
		pipe.LTrim(ctx, fmt.Sprintf("tool:%s:durations", op.Tool), 0, 999) // últimas 1000 operaciones
	}

	if op.FileSize > 0 {
		pipe.LPush(ctx, fmt.Sprintf("tool:%s:filesizes", op.Tool), op.FileSize)
		pipe.LTrim(ctx, fmt.Sprintf("tool:%s:filesizes", op.Tool), 0, 999) // últimas 1000 operaciones
	}

	// Ejecutar pipeline con context
	if _, err := pipe.Exec(ctx); err != nil {
		s.logger.Error("Error updating Redis counters", "error", err)
	}
}

// logOperationForMonitoringWithContext registra operación en formato JSON estructurado con context
func (s *Service) logOperationForMonitoringWithContext(ctx context.Context, op *models.AnalyticsOperation) {
	// Verificar si el context ya fue cancelado
	select {
	case <-ctx.Done():
		s.logger.Warn("Log operation cancelled due to context timeout")
		return
	default:
	}
	logData := map[string]interface{}{
		"event_type":    "operation_completed",
		"user_id":       op.UserID,
		"plan":          op.Plan,
		"tool":          op.Tool,
		"status":        op.Status,
		"duration_ms":   op.Duration,
		"file_size":     op.FileSize,
		"result_size":   op.ResultSize,
		"pages":         op.Pages,
		"worker":        op.Worker,
		"cpu_used":      op.CPUUsed,
		"ram_used":      op.RAMUsed,
		"queue_time":    op.QueueTime,
		"retries":       op.Retries,
		"timestamp":     op.Timestamp,
		"is_team_member": op.IsTeamMember,
		"country":       op.Country,
	}

	if op.Status != "success" {
		logData["fail_reason"] = op.FailReason
	}

	logJSON, _ := json.Marshal(logData)
	s.logger.Info("Analytics", "data", string(logJSON))
}