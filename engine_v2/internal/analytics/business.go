package analytics

import (
	"fmt"
	"math"
	"time"

	"github.com/tucentropdf/engine-v2/internal/analytics/models"
)

// GetUpgradeOpportunities detecta usuarios que deberían hacer upgrade
func (s *Service) GetUpgradeOpportunities() ([]models.UpgradeOpportunity, error) {
	var opportunities []models.UpgradeOpportunity

	// Detectar usuarios FREE que usan mucho el sistema
	freeOpportunities, err := s.detectFreeUpgradeOpportunities()
	if err != nil {
		s.logger.Error("Error detecting free upgrade opportunities", "error", err)
	} else {
		opportunities = append(opportunities, freeOpportunities...)
	}

	// Detectar usuarios PREMIUM que necesitan PRO
	premiumOpportunities, err := s.detectPremiumUpgradeOpportunities()
	if err != nil {
		s.logger.Error("Error detecting premium upgrade opportunities", "error", err)
	} else {
		opportunities = append(opportunities, premiumOpportunities...)
	}

	// Detectar usuarios PRO que necesitan CORPORATE
	corporateOpportunities, err := s.detectCorporateUpgradeOpportunities()
	if err != nil {
		s.logger.Error("Error detecting corporate upgrade opportunities", "error", err)
	} else {
		opportunities = append(opportunities, corporateOpportunities...)
	}

	return opportunities, nil
}

// detectFreeUpgradeOpportunities detecta usuarios FREE que deberían subir a PREMIUM
func (s *Service) detectFreeUpgradeOpportunities() ([]models.UpgradeOpportunity, error) {
	var opportunities []models.UpgradeOpportunity

	// Usuarios FREE que están cerca del límite diario
	query := `
		SELECT 
			user_id,
			COUNT(*) as daily_ops,
			COUNT(DISTINCT DATE(timestamp)) as active_days
		FROM analytics_operations 
		WHERE plan = 'free' 
		  AND timestamp >= NOW() - INTERVAL '30 days'
		GROUP BY user_id
		HAVING COUNT(*) > 150 OR -- Usuarios muy activos
		       (COUNT(*) > 80 AND COUNT(DISTINCT DATE(timestamp)) > 10) -- Usuarios consistentes
		ORDER BY daily_ops DESC
	`

	var candidates []struct {
		UserID     string
		DailyOps   int64
		ActiveDays int64
	}

	err := s.db.Raw(query).Scan(&candidates).Error
	if err != nil {
		return nil, err
	}

	for _, candidate := range candidates {
		confidence := s.calculateUpgradeConfidence(candidate.DailyOps, 200, 0.8)
		revenue := 9.99 // Precio mensual Premium

		reason := fmt.Sprintf("Usuario muy activo: %d operaciones en 30 días, activo %d días", 
			candidate.DailyOps, candidate.ActiveDays)

		opportunities = append(opportunities, models.UpgradeOpportunity{
			UserID:          candidate.UserID,
			CurrentPlan:     "free",
			SuggestedPlan:   "premium",
			Reason:          reason,
			Confidence:      confidence,
			PotentialRevenue: revenue,
			DetectedAt:      time.Now(),
		})
	}

	return opportunities, nil
}

// detectPremiumUpgradeOpportunities detecta usuarios PREMIUM que necesitan PRO
func (s *Service) detectPremiumUpgradeOpportunities() ([]models.UpgradeOpportunity, error) {
	var opportunities []models.UpgradeOpportunity

	// Usuarios PREMIUM que usan mucho IA OCR o tienen equipos
	query := `
		SELECT 
			user_id,
			COUNT(*) as total_ops,
			COUNT(*) FILTER (WHERE tool LIKE '%ai%' OR tool LIKE '%ocr_ai%') as ai_ops,
			COUNT(DISTINCT DATE(timestamp)) as active_days,
			AVG(file_size::float / 1048576) as avg_file_size_mb
		FROM analytics_operations 
		WHERE plan = 'premium' 
		  AND timestamp >= NOW() - INTERVAL '30 days'
		GROUP BY user_id
		HAVING COUNT(*) > 180 OR -- Muy activo
		       COUNT(*) FILTER (WHERE tool LIKE '%ai%') > 50 OR -- Mucho uso de IA
		       AVG(file_size::float / 1048576) > 40 -- Archivos grandes
		ORDER BY total_ops DESC
	`

	var candidates []struct {
		UserID        string
		TotalOps      int64
		AiOps         int64
		ActiveDays    int64
		AvgFileSizeMB float64
	}

	err := s.db.Raw(query).Scan(&candidates).Error
	if err != nil {
		return nil, err
	}

	for _, candidate := range candidates {
		confidence := s.calculateUpgradeConfidence(candidate.TotalOps, 500, 0.7)
		if candidate.AiOps > 50 {
			confidence = math.Min(confidence+0.2, 1.0)
		}

		revenue := 29.99 - 9.99 // Diferencia mensual

		reason := fmt.Sprintf("Alto uso: %d ops, %d ops IA, archivos promedio %.1fMB", 
			candidate.TotalOps, candidate.AiOps, candidate.AvgFileSizeMB)

		opportunities = append(opportunities, models.UpgradeOpportunity{
			UserID:          candidate.UserID,
			CurrentPlan:     "premium",
			SuggestedPlan:   "pro",
			Reason:          reason,
			Confidence:      confidence,
			PotentialRevenue: revenue,
			DetectedAt:      time.Now(),
		})
	}

	return opportunities, nil
}

// detectCorporateUpgradeOpportunities detecta usuarios PRO que necesitan CORPORATE
func (s *Service) detectCorporateUpgradeOpportunities() ([]models.UpgradeOpportunity, error) {
	var opportunities []models.UpgradeOpportunity

	// Usuarios PRO con uso muy intensivo
	query := `
		SELECT 
			user_id,
			COUNT(*) as total_ops,
			COUNT(DISTINCT DATE(timestamp)) as active_days,
			SUM(file_size) / 1073741824.0 as total_gb_processed
		FROM analytics_operations 
		WHERE plan = 'pro' 
		  AND timestamp >= NOW() - INTERVAL '30 days'
		GROUP BY user_id
		HAVING COUNT(*) > 1000 OR -- Muy muy activo
		       SUM(file_size) > 10737418240 OR -- Más de 10GB procesados
		       COUNT(DISTINCT DATE(timestamp)) > 25 -- Uso diario consistente
		ORDER BY total_ops DESC
	`

	var candidates []struct {
		UserID           string
		TotalOps         int64
		ActiveDays       int64
		TotalGBProcessed float64
	}

	err := s.db.Raw(query).Scan(&candidates).Error
	if err != nil {
		return nil, err
	}

	for _, candidate := range candidates {
		confidence := s.calculateUpgradeConfidence(candidate.TotalOps, 2000, 0.6)
		if candidate.TotalGBProcessed > 15 {
			confidence = math.Min(confidence+0.3, 1.0)
		}

		revenue := 199.99 - 29.99 // Diferencia mensual

		reason := fmt.Sprintf("Uso corporativo: %d ops, %.1fGB procesados, %d días activos", 
			candidate.TotalOps, candidate.TotalGBProcessed, candidate.ActiveDays)

		opportunities = append(opportunities, models.UpgradeOpportunity{
			UserID:          candidate.UserID,
			CurrentPlan:     "pro",
			SuggestedPlan:   "corporate",
			Reason:          reason,
			Confidence:      confidence,
			PotentialRevenue: revenue,
			DetectedAt:      time.Now(),
		})
	}

	return opportunities, nil
}

// GetHeavyUsers detecta usuarios que usan mucho el sistema
func (s *Service) GetHeavyUsers() ([]models.UserStats, error) {
	var heavyUsers []models.UserStats

	query := `
		SELECT 
			user_id,
			plan,
			COUNT(*) as total_operations,
			COUNT(*) FILTER (WHERE timestamp >= NOW() - INTERVAL '1 day') as daily_operations,
			COUNT(*) FILTER (WHERE timestamp >= NOW() - INTERVAL '30 days') as monthly_operations,
			SUM(file_size) / 1048576.0 as total_mb_processed,
			MAX(timestamp) as last_activity
		FROM analytics_operations 
		WHERE timestamp >= NOW() - INTERVAL '30 days'
		GROUP BY user_id, plan
		HAVING COUNT(*) > 100 -- Más de 100 operaciones en 30 días
		ORDER BY total_operations DESC
		LIMIT 50
	`

	err := s.db.Raw(query).Scan(&heavyUsers).Error
	if err != nil {
		return nil, err
	}

	// Marcar usuarios como activos
	last30Days := time.Now().AddDate(0, 0, -30)
	for i := range heavyUsers {
		heavyUsers[i].IsActive = heavyUsers[i].LastActivity.After(last30Days)
	}

	return heavyUsers, nil
}

// GetUsagePeaks detecta picos de uso del sistema
func (s *Service) GetUsagePeaks() ([]models.TrendData, error) {
	var peaks []models.TrendData

	// Obtener uso por hora en los últimos 7 días
	query := `
		SELECT 
			DATE_TRUNC('hour', timestamp) as date,
			COUNT(*) as value
		FROM analytics_operations 
		WHERE timestamp >= NOW() - INTERVAL '7 days'
		GROUP BY DATE_TRUNC('hour', timestamp)
		ORDER BY date
	`

	err := s.db.Raw(query).Scan(&peaks).Error
	if err != nil {
		return nil, err
	}

	// Agregar etiquetas para picos identificados
	if len(peaks) > 0 {
		avgValue := s.calculateAverage(peaks)
		stdDev := s.calculateStdDev(peaks, avgValue)
		threshold := avgValue + (2 * stdDev) // Picos son 2 desviaciones estándar por encima

		for i := range peaks {
			if peaks[i].Value > threshold {
				peaks[i].Label = "Peak"
			}
		}
	}

	return peaks, nil
}

// GetToolGrowthTrend obtiene tendencia de crecimiento por herramienta
func (s *Service) GetToolGrowthTrend(tool string) ([]models.TrendData, error) {
	var trends []models.TrendData

	query := `
		SELECT 
			DATE_TRUNC('day', timestamp) as date,
			COUNT(*) as value
		FROM analytics_operations 
		WHERE tool = ? 
		  AND timestamp >= NOW() - INTERVAL '30 days'
		GROUP BY DATE_TRUNC('day', timestamp)
		ORDER BY date
	`

	err := s.db.Raw(query, tool).Scan(&trends).Error
	if err != nil {
		return nil, err
	}

	// Calcular tendencia (crecimiento/decrecimiento)
	for i := range trends {
		if i > 0 {
			prevValue := trends[i-1].Value
			currentValue := trends[i].Value
			if prevValue > 0 {
				growthRate := ((currentValue - prevValue) / prevValue) * 100
				if growthRate > 10 {
					trends[i].Label = "Growth"
				} else if growthRate < -10 {
					trends[i].Label = "Decline"
				}
			}
		}
	}

	return trends, nil
}

// GenerateBusinessInsights genera insights automáticos de negocio
func (s *Service) GenerateBusinessInsights() ([]models.BusinessInsight, error) {
	var insights []models.BusinessInsight

	// Insight 1: Herramientas con alta tasa de fallo
	failInsights, err := s.generateFailureInsights()
	if err == nil {
		insights = append(insights, failInsights...)
	}

	// Insight 2: Herramientas poco utilizadas
	lowUsageInsights, err := s.generateLowUsageInsights()
	if err == nil {
		insights = append(insights, lowUsageInsights...)
	}

	// Insight 3: Oportunidades de ingresos
	revenueInsights, err := s.generateRevenueInsights()
	if err == nil {
		insights = append(insights, revenueInsights...)
	}

	// Insight 4: Rendimiento del sistema
	performanceInsights, err := s.generatePerformanceInsights()
	if err == nil {
		insights = append(insights, performanceInsights...)
	}

	return insights, nil
}

// generateFailureInsights genera insights sobre fallos
func (s *Service) generateFailureInsights() ([]models.BusinessInsight, error) {
	var insights []models.BusinessInsight

	// Herramientas con tasa de fallo > 5%
	query := `
		SELECT 
			tool,
			COUNT(*) as total,
			COUNT(*) FILTER (WHERE status != 'success') as failures,
			ROUND(COUNT(*) FILTER (WHERE status != 'success')::float / COUNT(*) * 100, 2) as fail_rate
		FROM analytics_operations 
		WHERE timestamp >= NOW() - INTERVAL '7 days'
		GROUP BY tool
		HAVING COUNT(*) > 10 AND COUNT(*) FILTER (WHERE status != 'success')::float / COUNT(*) > 0.05
		ORDER BY fail_rate DESC
	`

	var failingTools []struct {
		Tool     string
		Total    int64
		Failures int64
		FailRate float64
	}

	err := s.db.Raw(query).Scan(&failingTools).Error
	if err != nil {
		return insights, err
	}

	for _, tool := range failingTools {
		severity := "medium"
		if tool.FailRate > 15 {
			severity = "high"
		} else if tool.FailRate > 25 {
			severity = "critical"
		}

		insight := models.BusinessInsight{
			Type:        "technical",
			Title:       fmt.Sprintf("Alta tasa de fallos en %s", tool.Tool),
			Description: fmt.Sprintf("La herramienta %s tiene una tasa de fallos del %.1f%% (%d fallos de %d operaciones)", tool.Tool, tool.FailRate, tool.Failures, tool.Total),
			Severity:    severity,
			Data: map[string]interface{}{
				"tool":      tool.Tool,
				"fail_rate": tool.FailRate,
				"failures":  tool.Failures,
				"total":     tool.Total,
			},
			ActionItems: []string{
				"Investigar causas de los fallos",
				"Optimizar el procesamiento",
				"Mejorar manejo de errores",
				"Considerar aumentar timeouts",
			},
			GeneratedAt: time.Now(),
		}

		insights = append(insights, insight)
	}

	return insights, nil
}

// Helper functions

func (s *Service) calculateUpgradeConfidence(usage int64, threshold int64, baseConfidence float64) float64 {
	if usage <= threshold {
		return baseConfidence * (float64(usage) / float64(threshold))
	}
	return math.Min(baseConfidence+(float64(usage-threshold)/float64(threshold))*0.3, 1.0)
}

func (s *Service) calculateAverage(data []models.TrendData) float64 {
	if len(data) == 0 {
		return 0
	}

	sum := 0.0
	for _, d := range data {
		sum += d.Value
	}
	return sum / float64(len(data))
}

func (s *Service) calculateStdDev(data []models.TrendData, avg float64) float64 {
	if len(data) == 0 {
		return 0
	}

	sum := 0.0
	for _, d := range data {
		sum += math.Pow(d.Value-avg, 2)
	}
	return math.Sqrt(sum / float64(len(data)))
}

// generateLowUsageInsights insights sobre herramientas poco usadas
func (s *Service) generateLowUsageInsights() ([]models.BusinessInsight, error) {
	// Implementation for low usage insights
	return []models.BusinessInsight{}, nil
}

// generateRevenueInsights insights sobre oportunidades de ingresos
func (s *Service) generateRevenueInsights() ([]models.BusinessInsight, error) {
	// Implementation for revenue insights
	return []models.BusinessInsight{}, nil
}

// generatePerformanceInsights insights sobre rendimiento
func (s *Service) generatePerformanceInsights() ([]models.BusinessInsight, error) {
	// Implementation for performance insights
	return []models.BusinessInsight{}, nil
}