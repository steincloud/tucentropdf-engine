package queue

import (
	"fmt"
	"time"

	"github.com/hibiken/asynq"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

const (
	// Prioridades base por plan
	PriorityFree    = 1
	PriorityPremium = 5
	PriorityPro     = 8
	
	// Boost por tiempo de espera (cada 5 minutos)
	WaitTimeBoostInterval = 5 * time.Minute
	WaitTimeBoostAmount   = 1
	MaxWaitTimeBoost      = 5 // Máximo +5 de boost
	
	// Penalty por alto uso
	HighUsagePenalty = 2
	HighUsageThreshold = 100 // >100 jobs en 1 hora
)

// PriorityScorer calcula prioridad dinámica para jobs
type PriorityScorer struct {
	logger *logger.Logger
}

// NewPriorityScorer crea una nueva instancia
func NewPriorityScorer(log *logger.Logger) *PriorityScorer {
	return &PriorityScorer{
		logger: log,
	}
}

// ComputePriority calcula la prioridad dinámica de un job
func (ps *PriorityScorer) ComputePriority(plan string, waitTime time.Duration, userJobCount int) int {
	// 1. Prioridad base según plan
	basePriority := ps.getBasePriority(plan)
	
	// 2. Boost por tiempo de espera
	waitBoost := ps.computeWaitTimeBoost(waitTime)
	
	// 3. Penalty por alto uso
	usagePenalty := ps.computeUsagePenalty(userJobCount)
	
	// Calcular prioridad final
	finalPriority := basePriority + waitBoost - usagePenalty
	
	// Clamp entre 1 y 10
	if finalPriority < 1 {
		finalPriority = 1
	}
	if finalPriority > 10 {
		finalPriority = 10
	}
	
	ps.logger.Debug("Priority computed",
		"plan", plan,
		"base", basePriority,
		"wait_boost", waitBoost,
		"usage_penalty", usagePenalty,
		"final", finalPriority,
		"wait_time", waitTime,
		"job_count", userJobCount,
	)
	
	return finalPriority
}

// getBasePriority obtiene prioridad base según plan
func (ps *PriorityScorer) getBasePriority(plan string) int {
	switch plan {
	case "pro":
		return PriorityPro
	case "premium":
		return PriorityPremium
	default:
		return PriorityFree
	}
}

// computeWaitTimeBoost calcula boost por tiempo de espera
func (ps *PriorityScorer) computeWaitTimeBoost(waitTime time.Duration) int {
	if waitTime <= 0 {
		return 0
	}
	
	// +1 por cada WaitTimeBoostInterval (5 minutos)
	intervals := int(waitTime / WaitTimeBoostInterval)
	boost := intervals * WaitTimeBoostAmount
	
	// Limitar boost máximo
	if boost > MaxWaitTimeBoost {
		boost = MaxWaitTimeBoost
	}
	
	return boost
}

// computeUsagePenalty calcula penalty por alto uso
func (ps *PriorityScorer) computeUsagePenalty(userJobCount int) int {
	if userJobCount > HighUsageThreshold {
		return HighUsagePenalty
	}
	return 0
}

// GetAsynqPriority convierte prioridad numérica a nivel de Asynq
func (ps *PriorityScorer) GetAsynqPriority(priority int) asynq.Option {
	// Mapear 1-10 a niveles de Asynq (P0-P9)
	// P0 = más baja, P9 = más alta
	level := priority - 1
	if level < 0 {
		level = 0
	}
	if level > 9 {
		level = 9
	}
	
	// Asynq usa niveles en orden inverso
	// P0 = prioridad más baja, P9 = prioridad más alta
	switch level {
	case 0:
		return asynq.Queue("default")
	case 1, 2, 3:
		return asynq.Queue("low")
	case 4, 5, 6:
		return asynq.Queue("normal")
	case 7, 8:
		return asynq.Queue("high")
	case 9:
		return asynq.Queue("critical")
	default:
		return asynq.Queue("normal")
	}
}

// GetQueueName obtiene nombre de cola según prioridad
func (ps *PriorityScorer) GetQueueName(priority int) string {
	switch {
	case priority >= 9:
		return "critical"
	case priority >= 7:
		return "high"
	case priority >= 4:
		return "normal"
	case priority >= 2:
		return "low"
	default:
		return "default"
	}
}

// RecomputePendingPriority re-calcula prioridad de jobs pendientes
// Se ejecuta periódicamente para ajustar prioridades según tiempo de espera
func (ps *PriorityScorer) RecomputePendingPriority(jobID string, plan string, enqueuedAt time.Time, userJobCount int) (int, error) {
	waitTime := time.Since(enqueuedAt)
	
	newPriority := ps.ComputePriority(plan, waitTime, userJobCount)
	
	ps.logger.Info("Priority recomputed",
		"job_id", jobID,
		"plan", plan,
		"wait_time", waitTime,
		"new_priority", newPriority,
	)
	
	return newPriority, nil
}

// EstimateQueueTime estima tiempo de espera en cola
func (ps *PriorityScorer) EstimateQueueTime(priority int, queueLength int) time.Duration {
	// Estimación simplificada basada en prioridad y longitud de cola
	
	// Jobs de alta prioridad procesan más rápido
	baseTime := 30 * time.Second // Tiempo base por job
	
	// Ajustar según prioridad (prioridad alta = menos tiempo)
	priorityFactor := float64(11-priority) / 10.0 // 1.0 para P10, 0.1 para P1
	
	// Ajustar según longitud de cola
	queueFactor := float64(queueLength) / 10.0
	if queueFactor < 1.0 {
		queueFactor = 1.0
	}
	
	estimatedTime := time.Duration(float64(baseTime) * priorityFactor * queueFactor)
	
	ps.logger.Debug("Queue time estimated",
		"priority", priority,
		"queue_length", queueLength,
		"estimated_time", estimatedTime,
	)
	
	return estimatedTime
}

// GetPriorityExplanation retorna explicación de la prioridad calculada
func (ps *PriorityScorer) GetPriorityExplanation(plan string, priority int, waitTime time.Duration, userJobCount int) string {
	basePriority := ps.getBasePriority(plan)
	waitBoost := ps.computeWaitTimeBoost(waitTime)
	usagePenalty := ps.computeUsagePenalty(userJobCount)
	
	explanation := fmt.Sprintf("Prioridad: %d | Plan: %s (base: %d)", priority, plan, basePriority)
	
	if waitBoost > 0 {
		explanation += fmt.Sprintf(" | Boost por espera: +%d (%s)", waitBoost, waitTime.Round(time.Minute))
	}
	
	if usagePenalty > 0 {
		explanation += fmt.Sprintf(" | Penalty por uso: -%d (%d jobs)", usagePenalty, userJobCount)
	}
	
	return explanation
}

// ShouldPromoteJob determina si un job debe ser promovido a mayor prioridad
func (ps *PriorityScorer) ShouldPromoteJob(currentPriority int, waitTime time.Duration) bool {
	// Promover si ha esperado más de 15 minutos y prioridad es baja
	if waitTime > 15*time.Minute && currentPriority < 5 {
		return true
	}
	
	// Promover si ha esperado más de 30 minutos sin importar prioridad
	if waitTime > 30*time.Minute {
		return true
	}
	
	return false
}

// ComputeRetryPriority calcula prioridad para reintentos
func (ps *PriorityScorer) ComputeRetryPriority(originalPriority int, retryCount int) int {
	// Aumentar prioridad en reintentos (el job ya falló antes)
	retryBonus := retryCount * 2
	
	newPriority := originalPriority + retryBonus
	
	// Clamp
	if newPriority > 10 {
		newPriority = 10
	}
	
	ps.logger.Debug("Retry priority computed",
		"original", originalPriority,
		"retry_count", retryCount,
		"new_priority", newPriority,
	)
	
	return newPriority
}
