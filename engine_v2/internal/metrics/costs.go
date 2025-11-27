package metrics

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/redis/go-redis/v9"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

const (
	// Costos de OpenAI API (por 1K tokens)
	GPT4VisionCostPer1KInput  = 0.01  // $0.01 por 1K tokens input
	GPT4VisionCostPer1KOutput = 0.03  // $0.03 por 1K tokens output
	
	// Límites de alerta
	DailyBudgetLimit = 100.0 // $100 por día
	HourlyBudgetLimit = 10.0 // $10 por hora
	
	// Keys Redis para tracking
	CostKeyPrefix = "costs:openai:"
)

var (
	// Métricas Prometheus para costos
	openaiTokensConsumed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tucentropdf_openai_tokens_consumed_total",
			Help: "Total de tokens consumidos de OpenAI API",
		},
		[]string{"type", "model", "plan"},
	)
	
	openaiCostEstimated = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tucentropdf_openai_cost_estimated_dollars",
			Help: "Costo estimado de OpenAI API en dólares",
		},
		[]string{"model", "plan"},
	)
	
	openaiRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "tucentropdf_openai_requests_total",
			Help: "Total de requests a OpenAI API",
		},
		[]string{"model", "status", "plan"},
	)
	
	openaiRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "tucentropdf_openai_request_duration_seconds",
			Help:    "Duración de requests a OpenAI API",
			Buckets: []float64{1, 2, 5, 10, 20, 30, 60, 120},
		},
		[]string{"model", "plan"},
	)
)

// CostTracker rastrea costos de OpenAI API
type CostTracker struct {
	redis  *redis.Client
	logger *logger.Logger
	mu     sync.RWMutex
}

// UsageRecord registro de uso de API
type UsageRecord struct {
	RequestID     string    `json:"request_id"`
	UserID        string    `json:"user_id"`
	Plan          string    `json:"plan"`
	Model         string    `json:"model"`
	InputTokens   int       `json:"input_tokens"`
	OutputTokens  int       `json:"output_tokens"`
	TotalTokens   int       `json:"total_tokens"`
	CostUSD       float64   `json:"cost_usd"`
	Duration      float64   `json:"duration_seconds"`
	Success       bool      `json:"success"`
	Timestamp     time.Time `json:"timestamp"`
}

// CostSummary resumen de costos
type CostSummary struct {
	Period        string  `json:"period"` // hour, day, month
	TotalRequests int64   `json:"total_requests"`
	TotalTokens   int64   `json:"total_tokens"`
	TotalCostUSD  float64 `json:"total_cost_usd"`
	AvgCostPerRequest float64 `json:"avg_cost_per_request"`
	ByPlan        map[string]float64 `json:"by_plan"`
}

// NewCostTracker crea una nueva instancia
func NewCostTracker(redisClient *redis.Client, log *logger.Logger) *CostTracker {
	return &CostTracker{
		redis:  redisClient,
		logger: log,
	}
}

// RecordUsage registra el uso de OpenAI API
func (ct *CostTracker) RecordUsage(ctx context.Context, record *UsageRecord) error {
	// Calcular costo
	cost := ct.calculateCost(record.Model, record.InputTokens, record.OutputTokens)
	record.CostUSD = cost
	record.TotalTokens = record.InputTokens + record.OutputTokens
	record.Timestamp = time.Now()
	
	// Registrar en Prometheus
	openaiTokensConsumed.WithLabelValues("input", record.Model, record.Plan).Add(float64(record.InputTokens))
	openaiTokensConsumed.WithLabelValues("output", record.Model, record.Plan).Add(float64(record.OutputTokens))
	openaiCostEstimated.WithLabelValues(record.Model, record.Plan).Add(cost)
	
	status := "success"
	if !record.Success {
		status = "failed"
	}
	openaiRequestsTotal.WithLabelValues(record.Model, status, record.Plan).Inc()
	openaiRequestDuration.WithLabelValues(record.Model, record.Plan).Observe(record.Duration)
	
	// Almacenar en Redis para tracking histórico
	if err := ct.storeInRedis(ctx, record); err != nil {
		ct.logger.Error("Failed to store usage in Redis", "error", err)
		// No fallar si Redis falla
	}
	
	// Verificar límites
	if err := ct.checkBudgetLimits(ctx, cost); err != nil {
		ct.logger.Warn("Budget limit warning", "error", err)
		// Enviar alerta (TODO: integrar con sistema de alertas)
	}
	
	ct.logger.Info("OpenAI usage recorded",
		"request_id", record.RequestID,
		"user_id", record.UserID,
		"plan", record.Plan,
		"input_tokens", record.InputTokens,
		"output_tokens", record.OutputTokens,
		"cost_usd", fmt.Sprintf("$%.4f", cost),
	)
	
	return nil
}

// calculateCost calcula el costo basado en tokens
func (ct *CostTracker) calculateCost(model string, inputTokens, outputTokens int) float64 {
	// Convertir a miles de tokens
	inputK := float64(inputTokens) / 1000.0
	outputK := float64(outputTokens) / 1000.0
	
	// Calcular costo según modelo
	switch model {
	case "gpt-4-vision-preview", "gpt-4.1-mini":
		return (inputK * GPT4VisionCostPer1KInput) + (outputK * GPT4VisionCostPer1KOutput)
	default:
		// Modelo desconocido, usar costos de GPT-4 Vision
		return (inputK * GPT4VisionCostPer1KInput) + (outputK * GPT4VisionCostPer1KOutput)
	}
}

// storeInRedis almacena el registro en Redis
func (ct *CostTracker) storeInRedis(ctx context.Context, record *UsageRecord) error {
	// Incrementar contadores por período
	now := time.Now()
	
	// Key format: costs:openai:{period}:{date}
	hourKey := fmt.Sprintf("%shour:%s", CostKeyPrefix, now.Format("2006-01-02-15"))
	dayKey := fmt.Sprintf("%sday:%s", CostKeyPrefix, now.Format("2006-01-02"))
	monthKey := fmt.Sprintf("%smonth:%s", CostKeyPrefix, now.Format("2006-01"))
	
	pipe := ct.redis.Pipeline()
	
	// Incrementar costos por período
	pipe.IncrByFloat(ctx, hourKey, record.CostUSD)
	pipe.Expire(ctx, hourKey, 48*time.Hour) // Retener 48 horas
	
	pipe.IncrByFloat(ctx, dayKey, record.CostUSD)
	pipe.Expire(ctx, dayKey, 31*24*time.Hour) // Retener 31 días
	
	pipe.IncrByFloat(ctx, monthKey, record.CostUSD)
	pipe.Expire(ctx, monthKey, 13*30*24*time.Hour) // Retener 13 meses
	
	// Incrementar contadores por plan
	planKey := fmt.Sprintf("%splan:%s:%s", CostKeyPrefix, record.Plan, now.Format("2006-01-02"))
	pipe.IncrByFloat(ctx, planKey, record.CostUSD)
	pipe.Expire(ctx, planKey, 31*24*time.Hour)
	
	_, err := pipe.Exec(ctx)
	return err
}

// checkBudgetLimits verifica límites de presupuesto
func (ct *CostTracker) checkBudgetLimits(ctx context.Context, newCost float64) error {
	now := time.Now()
	
	// Verificar límite por hora
	hourKey := fmt.Sprintf("%shour:%s", CostKeyPrefix, now.Format("2006-01-02-15"))
	hourlyCost, err := ct.redis.Get(ctx, hourKey).Float64()
	if err != nil && err != redis.Nil {
		return err
	}
	
	if hourlyCost+newCost > HourlyBudgetLimit {
		return fmt.Errorf("hourly budget limit exceeded: $%.2f + $%.2f > $%.2f",
			hourlyCost, newCost, HourlyBudgetLimit)
	}
	
	// Verificar límite diario
	dayKey := fmt.Sprintf("%sday:%s", CostKeyPrefix, now.Format("2006-01-02"))
	dailyCost, err := ct.redis.Get(ctx, dayKey).Float64()
	if err != nil && err != redis.Nil {
		return err
	}
	
	if dailyCost+newCost > DailyBudgetLimit {
		return fmt.Errorf("daily budget limit exceeded: $%.2f + $%.2f > $%.2f",
			dailyCost, newCost, DailyBudgetLimit)
	}
	
	return nil
}

// GetHourlyCost obtiene el costo de la hora actual
func (ct *CostTracker) GetHourlyCost(ctx context.Context) (float64, error) {
	hourKey := fmt.Sprintf("%shour:%s", CostKeyPrefix, time.Now().Format("2006-01-02-15"))
	cost, err := ct.redis.Get(ctx, hourKey).Float64()
	if err == redis.Nil {
		return 0.0, nil
	}
	return cost, err
}

// GetDailyCost obtiene el costo del día actual
func (ct *CostTracker) GetDailyCost(ctx context.Context) (float64, error) {
	dayKey := fmt.Sprintf("%sday:%s", CostKeyPrefix, time.Now().Format("2006-01-02"))
	cost, err := ct.redis.Get(ctx, dayKey).Float64()
	if err == redis.Nil {
		return 0.0, nil
	}
	return cost, err
}

// GetMonthlyCost obtiene el costo del mes actual
func (ct *CostTracker) GetMonthlyCost(ctx context.Context) (float64, error) {
	monthKey := fmt.Sprintf("%smonth:%s", CostKeyPrefix, time.Now().Format("2006-01"))
	cost, err := ct.redis.Get(ctx, monthKey).Float64()
	if err == redis.Nil {
		return 0.0, nil
	}
	return cost, err
}

// GetCostSummary obtiene resumen de costos por período
func (ct *CostTracker) GetCostSummary(ctx context.Context, period string) (*CostSummary, error) {
	summary := &CostSummary{
		Period: period,
		ByPlan: make(map[string]float64),
	}
	
	var pattern string
	switch period {
	case "hour":
		pattern = fmt.Sprintf("%shour:%s", CostKeyPrefix, time.Now().Format("2006-01-02-15"))
	case "day":
		pattern = fmt.Sprintf("%sday:%s", CostKeyPrefix, time.Now().Format("2006-01-02"))
	case "month":
		pattern = fmt.Sprintf("%smonth:%s", CostKeyPrefix, time.Now().Format("2006-01"))
	default:
		return nil, fmt.Errorf("invalid period: %s", period)
	}
	
	// Obtener costo total del período
	cost, err := ct.redis.Get(ctx, pattern).Float64()
	if err != nil && err != redis.Nil {
		return nil, err
	}
	summary.TotalCostUSD = cost
	
	// TODO: Agregar conteo de requests y tokens desde Prometheus
	
	return summary, nil
}

// GetCostByPlan obtiene costos desglosados por plan
func (ct *CostTracker) GetCostByPlan(ctx context.Context, date string) (map[string]float64, error) {
	costs := make(map[string]float64)
	
	plans := []string{"free", "premium", "pro"}
	for _, plan := range plans {
		key := fmt.Sprintf("%splan:%s:%s", CostKeyPrefix, plan, date)
		cost, err := ct.redis.Get(ctx, key).Float64()
		if err == redis.Nil {
			costs[plan] = 0.0
			continue
		}
		if err != nil {
			return nil, err
		}
		costs[plan] = cost
	}
	
	return costs, nil
}

// EstimateMonthlyCost estima el costo mensual basado en uso actual
func (ct *CostTracker) EstimateMonthlyCost(ctx context.Context) (float64, error) {
	// Obtener costo del día actual
	dailyCost, err := ct.GetDailyCost(ctx)
	if err != nil {
		return 0.0, err
	}
	
	// Estimar costo mensual (día actual * 30)
	now := time.Now()
	hour := now.Hour()
	
	// Ajustar por hora del día (extrapolación)
	var estimatedDailyCost float64
	if hour > 0 {
		estimatedDailyCost = dailyCost * (24.0 / float64(hour))
	} else {
		estimatedDailyCost = dailyCost
	}
	
	estimatedMonthlyCost := estimatedDailyCost * 30.0
	
	ct.logger.Info("Monthly cost estimated",
		"daily_cost", fmt.Sprintf("$%.2f", dailyCost),
		"estimated_daily", fmt.Sprintf("$%.2f", estimatedDailyCost),
		"estimated_monthly", fmt.Sprintf("$%.2f", estimatedMonthlyCost),
	)
	
	return estimatedMonthlyCost, nil
}

// ResetDailyCost resetea el contador diario (admin only, para testing)
func (ct *CostTracker) ResetDailyCost(ctx context.Context) error {
	dayKey := fmt.Sprintf("%sday:%s", CostKeyPrefix, time.Now().Format("2006-01-02"))
	return ct.redis.Del(ctx, dayKey).Err()
}

// GetBudgetStatus obtiene el estado actual del presupuesto
func (ct *CostTracker) GetBudgetStatus(ctx context.Context) (map[string]interface{}, error) {
	hourlyCost, _ := ct.GetHourlyCost(ctx)
	dailyCost, _ := ct.GetDailyCost(ctx)
	monthlyCost, _ := ct.GetMonthlyCost(ctx)
	estimatedMonthly, _ := ct.EstimateMonthlyCost(ctx)
	
	return map[string]interface{}{
		"hourly": map[string]interface{}{
			"cost":  hourlyCost,
			"limit": HourlyBudgetLimit,
			"usage_pct": (hourlyCost / HourlyBudgetLimit) * 100,
		},
		"daily": map[string]interface{}{
			"cost":  dailyCost,
			"limit": DailyBudgetLimit,
			"usage_pct": (dailyCost / DailyBudgetLimit) * 100,
		},
		"monthly": map[string]interface{}{
			"cost":      monthlyCost,
			"estimated": estimatedMonthly,
		},
	}, nil
}
