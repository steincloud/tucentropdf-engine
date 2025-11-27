package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/go-redis/redis/v8"
	"github.com/tucentropdf/engine-v2/internal/config"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// UsageTracker servicio para rastrear el uso de usuarios
type UsageTracker struct {
	redis  *redis.Client
	config *config.Config
	logger *logger.Logger
}

// NewUsageTracker crea un nuevo tracker de uso
func NewUsageTracker(redisClient *redis.Client, cfg *config.Config, log *logger.Logger) *UsageTracker {
	return &UsageTracker{
		redis:  redisClient,
		config: cfg,
		logger: log,
	}
}

// OperationType representa el tipo de operación
type OperationType string

const (
	OpTypePDF    OperationType = "pdf"
	OpTypeOCR    OperationType = "ocr"
	OpTypeAIOCR  OperationType = "ai_ocr"
	OpTypeOffice OperationType = "office"
	OpTypeUpload OperationType = "upload"
)

// UsageOperation representa una operación a rastrear
type UsageOperation struct {
	UserID        string        `json:"user_id"`
	OperationType OperationType `json:"operation_type"`
	FileSize      int64         `json:"file_size"`
	Pages         int           `json:"pages"`
	ProcessingTime int64        `json:"processing_time_ms"`
	Success       bool          `json:"success"`
	Timestamp     time.Time     `json:"timestamp"`
}

// GetUserUsage obtiene las estadísticas de uso de un usuario
func (ut *UsageTracker) GetUserUsage(ctx context.Context, userID string) (*config.UserUsageStats, error) {
	pipe := ut.redis.Pipeline()
	
	// Obtener todos los contadores de una vez
	dailyOpsCmd := pipe.Get(ctx, ut.keyDailyOperations(userID))
	monthlyOpsCmd := pipe.Get(ctx, ut.keyMonthlyOperations(userID))
	dailyFilesCmd := pipe.Get(ctx, ut.keyDailyFiles(userID))
	monthlyFilesCmd := pipe.Get(ctx, ut.keyMonthlyFiles(userID))
	dailyBytesCmd := pipe.Get(ctx, ut.keyDailyBytes(userID))
	monthlyBytesCmd := pipe.Get(ctx, ut.keyMonthlyBytes(userID))
	dailyPagesCmd := pipe.Get(ctx, ut.keyDailyPages(userID))
	monthlyPagesCmd := pipe.Get(ctx, ut.keyMonthlyPages(userID))
	dailyOCRCmd := pipe.Get(ctx, ut.keyDailyOCRPages(userID))
	monthlyOCRCmd := pipe.Get(ctx, ut.keyMonthlyOCRPages(userID))
	dailyAIOCRCmd := pipe.Get(ctx, ut.keyDailyAIOCRPages(userID))
	monthlyAIOCRCmd := pipe.Get(ctx, ut.keyMonthlyAIOCRPages(userID))
	dailyOfficeCmd := pipe.Get(ctx, ut.keyDailyOfficePages(userID))
	monthlyOfficeCmd := pipe.Get(ctx, ut.keyMonthlyOfficePages(userID))
	planCmd := pipe.Get(ctx, ut.keyUserPlan(userID))
	lastDailyResetCmd := pipe.Get(ctx, ut.keyLastDailyReset(userID))
	lastMonthlyResetCmd := pipe.Get(ctx, ut.keyLastMonthlyReset(userID))
	
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, fmt.Errorf("failed to get user usage: %w", err)
	}
	
	// Obtener plan del usuario
	planStr, _ := planCmd.Result()
	if planStr == "" {
		planStr = string(config.PlanFree)
	}
	
	// Construir estadísticas
	usage := &config.UserUsageStats{
		UserID: userID,
		Plan:   config.Plan(planStr),
		DailyStats: config.DailyUsageStats{
			Operations:     ut.getIntValue(dailyOpsCmd),
			FilesProcessed: ut.getIntValue(dailyFilesCmd),
			PagesProcessed: ut.getIntValue(dailyPagesCmd),
			BytesProcessed: ut.getInt64Value(dailyBytesCmd),
			OCRPages:       ut.getIntValue(dailyOCRCmd),
			AIOCRPages:     ut.getIntValue(dailyAIOCRCmd),
			OfficePages:    ut.getIntValue(dailyOfficeCmd),
		},
		MonthlyStats: config.MonthlyUsageStats{
			Operations:     ut.getIntValue(monthlyOpsCmd),
			FilesProcessed: ut.getIntValue(monthlyFilesCmd),
			PagesProcessed: ut.getIntValue(monthlyPagesCmd),
			BytesProcessed: ut.getInt64Value(monthlyBytesCmd),
			OCRPages:       ut.getIntValue(monthlyOCRCmd),
			AIOCRPages:     ut.getIntValue(monthlyAIOCRCmd),
			OfficePages:    ut.getIntValue(monthlyOfficeCmd),
		},
		LastUpdated:      time.Now(),
		LastDailyReset:   ut.getTimeValue(lastDailyResetCmd),
		LastMonthlyReset: ut.getTimeValue(lastMonthlyResetCmd),
	}
	
	return usage, nil
}

// TrackUsage registra una operación de uso
func (ut *UsageTracker) TrackUsage(ctx context.Context, operation *UsageOperation) error {
	pipe := ut.redis.Pipeline()
	
	// Asegurar que se reseteen los contadores si es necesario
	if err := ut.ensureCountersReset(ctx, operation.UserID); err != nil {
		ut.logger.Warn("Failed to reset counters", "user_id", operation.UserID, "error", err)
	}
	
	// TTL para contadores (se autolimpian después de 35 días)
	ttl := 35 * 24 * time.Hour
	
	// Incrementar contadores generales
	pipe.Incr(ctx, ut.keyDailyOperations(operation.UserID))
	pipe.Expire(ctx, ut.keyDailyOperations(operation.UserID), ttl)
	
	pipe.Incr(ctx, ut.keyMonthlyOperations(operation.UserID))
	pipe.Expire(ctx, ut.keyMonthlyOperations(operation.UserID), ttl)
	
	pipe.Incr(ctx, ut.keyDailyFiles(operation.UserID))
	pipe.Expire(ctx, ut.keyDailyFiles(operation.UserID), ttl)
	
	pipe.Incr(ctx, ut.keyMonthlyFiles(operation.UserID))
	pipe.Expire(ctx, ut.keyMonthlyFiles(operation.UserID), ttl)
	
	// Incrementar bytes procesados
	pipe.IncrBy(ctx, ut.keyDailyBytes(operation.UserID), operation.FileSize)
	pipe.Expire(ctx, ut.keyDailyBytes(operation.UserID), ttl)
	
	pipe.IncrBy(ctx, ut.keyMonthlyBytes(operation.UserID), operation.FileSize)
	pipe.Expire(ctx, ut.keyMonthlyBytes(operation.UserID), ttl)
	
	// Incrementar páginas procesadas
	if operation.Pages > 0 {
		pipe.IncrBy(ctx, ut.keyDailyPages(operation.UserID), int64(operation.Pages))
		pipe.Expire(ctx, ut.keyDailyPages(operation.UserID), ttl)
		
		pipe.IncrBy(ctx, ut.keyMonthlyPages(operation.UserID), int64(operation.Pages))
		pipe.Expire(ctx, ut.keyMonthlyPages(operation.UserID), ttl)
	}
	
	// Incrementar contadores específicos por tipo de operación
	switch operation.OperationType {
	case OpTypeOCR:
		pipe.IncrBy(ctx, ut.keyDailyOCRPages(operation.UserID), int64(operation.Pages))
		pipe.Expire(ctx, ut.keyDailyOCRPages(operation.UserID), ttl)
		
		pipe.IncrBy(ctx, ut.keyMonthlyOCRPages(operation.UserID), int64(operation.Pages))
		pipe.Expire(ctx, ut.keyMonthlyOCRPages(operation.UserID), ttl)
		
	case OpTypeAIOCR:
		pipe.IncrBy(ctx, ut.keyDailyAIOCRPages(operation.UserID), int64(operation.Pages))
		pipe.Expire(ctx, ut.keyDailyAIOCRPages(operation.UserID), ttl)
		
		pipe.IncrBy(ctx, ut.keyMonthlyAIOCRPages(operation.UserID), int64(operation.Pages))
		pipe.Expire(ctx, ut.keyMonthlyAIOCRPages(operation.UserID), ttl)
		
	case OpTypeOffice:
		pipe.IncrBy(ctx, ut.keyDailyOfficePages(operation.UserID), int64(operation.Pages))
		pipe.Expire(ctx, ut.keyDailyOfficePages(operation.UserID), ttl)
		
		pipe.IncrBy(ctx, ut.keyMonthlyOfficePages(operation.UserID), int64(operation.Pages))
		pipe.Expire(ctx, ut.keyMonthlyOfficePages(operation.UserID), ttl)
	}
	
	// Guardar operación en historial (solo últimas 1000)
	operationData, _ := json.Marshal(operation)
	pipe.LPush(ctx, ut.keyOperationHistory(operation.UserID), operationData)
	pipe.LTrim(ctx, ut.keyOperationHistory(operation.UserID), 0, 999) // Mantener solo 1000
	pipe.Expire(ctx, ut.keyOperationHistory(operation.UserID), ttl)
	
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to track usage: %w", err)
	}
	
	ut.logger.Debug("Usage tracked",
		"user_id", operation.UserID,
		"operation_type", operation.OperationType,
		"file_size", operation.FileSize,
		"pages", operation.Pages,
		"success", operation.Success,
	)
	
	return nil
}

// CheckLimits verifica si un usuario puede realizar una operación
func (ut *UsageTracker) CheckLimits(ctx context.Context, userID string, operation *UsageOperation, planLimits config.PlanLimits) error {
	usage, err := ut.GetUserUsage(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user usage: %w", err)
	}
	
	// Verificar límites diarios
	if operation.FileSize > 0 {
		if usage.DailyStats.BytesProcessed+operation.FileSize > planLimits.MaxBytesPerDay {
			return fmt.Errorf("daily bytes limit exceeded")
		}
	}
	
	if operation.Pages > 0 {
		if usage.DailyStats.PagesProcessed+operation.Pages > planLimits.MaxPages {
			return fmt.Errorf("daily pages limit exceeded")
		}
	}
	
	if usage.DailyStats.Operations+1 > planLimits.DailyOperations {
		return fmt.Errorf("daily operations limit exceeded")
	}
	
	if usage.DailyStats.FilesProcessed+1 > planLimits.MaxFilesPerDay {
		return fmt.Errorf("daily files limit exceeded")
	}
	
	// Verificar límites específicos por tipo de operación
	switch operation.OperationType {
	case OpTypeOCR:
		if usage.DailyStats.OCRPages+operation.Pages > planLimits.OCRPagesPerDay {
			return fmt.Errorf("daily OCR pages limit exceeded")
		}
		
	case OpTypeAIOCR:
		if usage.DailyStats.AIOCRPages+operation.Pages > planLimits.AIOCRPagesPerDay {
			return fmt.Errorf("daily AI OCR pages limit exceeded")
		}
		
	case OpTypeOffice:
		if usage.DailyStats.OfficePages+operation.Pages > planLimits.OfficePagesPerDay {
			return fmt.Errorf("daily Office pages limit exceeded")
		}
	}
	
	// Verificar límites mensuales
	if usage.MonthlyStats.BytesProcessed+operation.FileSize > planLimits.MaxBytesPerMonth {
		return fmt.Errorf("monthly bytes limit exceeded")
	}
	
	if usage.MonthlyStats.Operations+1 > planLimits.MonthlyOperations {
		return fmt.Errorf("monthly operations limit exceeded")
	}
	
	if usage.MonthlyStats.FilesProcessed+1 > planLimits.MaxFilesPerMonth {
		return fmt.Errorf("monthly files limit exceeded")
	}
	
	// Verificar límites mensuales específicos por tipo
	switch operation.OperationType {
	case OpTypeOCR:
		if usage.MonthlyStats.OCRPages+operation.Pages > planLimits.OCRPagesPerMonth {
			return fmt.Errorf("monthly OCR pages limit exceeded")
		}
		
	case OpTypeAIOCR:
		if usage.MonthlyStats.AIOCRPages+operation.Pages > planLimits.AIOCRPagesPerMonth {
			return fmt.Errorf("monthly AI OCR pages limit exceeded")
		}
		
	case OpTypeOffice:
		if usage.MonthlyStats.OfficePages+operation.Pages > planLimits.OfficePagesPerMonth {
			return fmt.Errorf("monthly Office pages limit exceeded")
		}
	}
	
	return nil
}

// ResetDailyCounters resetea los contadores diarios de un usuario
func (ut *UsageTracker) ResetDailyCounters(ctx context.Context, userID string) error {
	pipe := ut.redis.Pipeline()
	
	// Eliminar contadores diarios
	pipe.Del(ctx, ut.keyDailyOperations(userID))
	pipe.Del(ctx, ut.keyDailyFiles(userID))
	pipe.Del(ctx, ut.keyDailyBytes(userID))
	pipe.Del(ctx, ut.keyDailyPages(userID))
	pipe.Del(ctx, ut.keyDailyOCRPages(userID))
	pipe.Del(ctx, ut.keyDailyAIOCRPages(userID))
	pipe.Del(ctx, ut.keyDailyOfficePages(userID))
	
	// Actualizar timestamp de último reset
	pipe.Set(ctx, ut.keyLastDailyReset(userID), time.Now().Unix(), 35*24*time.Hour)
	
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to reset daily counters: %w", err)
	}
	
	ut.logger.Info("Daily counters reset", "user_id", userID)
	return nil
}

// ResetMonthlyCounters resetea los contadores mensuales de un usuario
func (ut *UsageTracker) ResetMonthlyCounters(ctx context.Context, userID string) error {
	pipe := ut.redis.Pipeline()
	
	// Eliminar contadores mensuales
	pipe.Del(ctx, ut.keyMonthlyOperations(userID))
	pipe.Del(ctx, ut.keyMonthlyFiles(userID))
	pipe.Del(ctx, ut.keyMonthlyBytes(userID))
	pipe.Del(ctx, ut.keyMonthlyPages(userID))
	pipe.Del(ctx, ut.keyMonthlyOCRPages(userID))
	pipe.Del(ctx, ut.keyMonthlyAIOCRPages(userID))
	pipe.Del(ctx, ut.keyMonthlyOfficePages(userID))
	
	// Actualizar timestamp de último reset
	pipe.Set(ctx, ut.keyLastMonthlyReset(userID), time.Now().Unix(), 35*24*time.Hour)
	
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to reset monthly counters: %w", err)
	}
	
	ut.logger.Info("Monthly counters reset", "user_id", userID)
	return nil
}

// ensureCountersReset verifica y resetea contadores si es necesario
func (ut *UsageTracker) ensureCountersReset(ctx context.Context, userID string) error {
	now := time.Now()
	
	// Verificar reset diario
	lastDailyResetCmd := ut.redis.Get(ctx, ut.keyLastDailyReset(userID))
	lastDailyReset := ut.getTimeValue(lastDailyResetCmd)
	
	if lastDailyReset.IsZero() || !ut.isSameDay(lastDailyReset, now) {
		if err := ut.ResetDailyCounters(ctx, userID); err != nil {
			return err
		}
	}
	
	// Verificar reset mensual
	lastMonthlyResetCmd := ut.redis.Get(ctx, ut.keyLastMonthlyReset(userID))
	lastMonthlyReset := ut.getTimeValue(lastMonthlyResetCmd)
	
	if lastMonthlyReset.IsZero() || !ut.isSameMonth(lastMonthlyReset, now) {
		if err := ut.ResetMonthlyCounters(ctx, userID); err != nil {
			return err
		}
	}
	
	return nil
}

// Helper methods para generar keys de Redis
func (ut *UsageTracker) keyDailyOperations(userID string) string {
	return fmt.Sprintf("user:%s:daily:operations", userID)
}

func (ut *UsageTracker) keyMonthlyOperations(userID string) string {
	return fmt.Sprintf("user:%s:monthly:operations", userID)
}

func (ut *UsageTracker) keyDailyFiles(userID string) string {
	return fmt.Sprintf("user:%s:daily:files", userID)
}

func (ut *UsageTracker) keyMonthlyFiles(userID string) string {
	return fmt.Sprintf("user:%s:monthly:files", userID)
}

func (ut *UsageTracker) keyDailyBytes(userID string) string {
	return fmt.Sprintf("user:%s:daily:bytes", userID)
}

func (ut *UsageTracker) keyMonthlyBytes(userID string) string {
	return fmt.Sprintf("user:%s:monthly:bytes", userID)
}

func (ut *UsageTracker) keyDailyPages(userID string) string {
	return fmt.Sprintf("user:%s:daily:pages", userID)
}

func (ut *UsageTracker) keyMonthlyPages(userID string) string {
	return fmt.Sprintf("user:%s:monthly:pages", userID)
}

func (ut *UsageTracker) keyDailyOCRPages(userID string) string {
	return fmt.Sprintf("user:%s:daily:ocr_pages", userID)
}

func (ut *UsageTracker) keyMonthlyOCRPages(userID string) string {
	return fmt.Sprintf("user:%s:monthly:ocr_pages", userID)
}

func (ut *UsageTracker) keyDailyAIOCRPages(userID string) string {
	return fmt.Sprintf("user:%s:daily:ai_ocr_pages", userID)
}

func (ut *UsageTracker) keyMonthlyAIOCRPages(userID string) string {
	return fmt.Sprintf("user:%s:monthly:ai_ocr_pages", userID)
}

func (ut *UsageTracker) keyDailyOfficePages(userID string) string {
	return fmt.Sprintf("user:%s:daily:office_pages", userID)
}

func (ut *UsageTracker) keyMonthlyOfficePages(userID string) string {
	return fmt.Sprintf("user:%s:monthly:office_pages", userID)
}

func (ut *UsageTracker) keyUserPlan(userID string) string {
	return fmt.Sprintf("user:%s:plan", userID)
}

func (ut *UsageTracker) keyLastDailyReset(userID string) string {
	return fmt.Sprintf("user:%s:last_daily_reset", userID)
}

func (ut *UsageTracker) keyLastMonthlyReset(userID string) string {
	return fmt.Sprintf("user:%s:last_monthly_reset", userID)
}

func (ut *UsageTracker) keyOperationHistory(userID string) string {
	return fmt.Sprintf("user:%s:operations_history", userID)
}

// Helper methods para parsear valores de Redis
func (ut *UsageTracker) getIntValue(cmd *redis.StringCmd) int {
	val, err := cmd.Result()
	if err != nil {
		return 0
	}
	intVal, err := strconv.Atoi(val)
	if err != nil {
		return 0
	}
	return intVal
}

func (ut *UsageTracker) getInt64Value(cmd *redis.StringCmd) int64 {
	val, err := cmd.Result()
	if err != nil {
		return 0
	}
	int64Val, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0
	}
	return int64Val
}

func (ut *UsageTracker) getTimeValue(cmd *redis.StringCmd) time.Time {
	val, err := cmd.Result()
	if err != nil {
		return time.Time{}
	}
	timestamp, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(timestamp, 0)
}

func (ut *UsageTracker) isSameDay(t1, t2 time.Time) bool {
	y1, m1, d1 := t1.Date()
	y2, m2, d2 := t2.Date()
	return y1 == y2 && m1 == m2 && d1 == d2
}

func (ut *UsageTracker) isSameMonth(t1, t2 time.Time) bool {
	y1, m1, _ := t1.Date()
	y2, m2, _ := t2.Date()
	return y1 == y2 && m1 == m2
}