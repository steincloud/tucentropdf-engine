package analytics

import (
	"strings"
	"time"

	"github.com/tucentropdf/engine-v2/internal/analytics/models"
)

// GetMostUsedTools obtiene las herramientas más utilizadas (optimizado con prepared statement)
func (s *Service) GetMostUsedTools(period string, limit int) ([]models.ToolStats, error) {
	var stats []models.ToolStats

	// Query optimizado con índices en (timestamp, tool, status)
	query := `
		SELECT 
			tool,
			COUNT(*) as total_usage,
			ROUND(AVG(CASE WHEN status = 'success' THEN 1.0 ELSE 0.0 END) * 100, 2) as success_rate,
			ROUND(AVG(duration_ms), 2) as avg_duration,
			ROUND(AVG(file_size::float / 1048576), 2) as avg_file_size,
			COUNT(*) FILTER (WHERE status != 'success') as failure_count,
			MAX(timestamp) as last_used
		FROM analytics_operations 
		WHERE timestamp >= ?
		GROUP BY tool 
		ORDER BY total_usage DESC
		LIMIT ?
	`

	startTime := s.getPeriodStartTime(period)
	
	// Usar prepared statement (GORM lo cachea automáticamente)
	err := s.db.Raw(query, startTime, limit).Scan(&stats).Error

	return stats, err
}

// GetLeastUsedTools obtiene las herramientas menos utilizadas
func (s *Service) GetLeastUsedTools(period string, limit int) ([]models.ToolStats, error) {
	var stats []models.ToolStats

	query := `
		SELECT 
			tool,
			COUNT(*) as total_usage,
			ROUND(AVG(CASE WHEN status = 'success' THEN 1.0 ELSE 0.0 END) * 100, 2) as success_rate,
			ROUND(AVG(duration_ms), 2) as avg_duration,
			ROUND(AVG(file_size::float / 1048576), 2) as avg_file_size,
			COUNT(*) FILTER (WHERE status != 'success') as failure_count,
			MAX(timestamp) as last_used
		FROM analytics_operations 
		WHERE timestamp >= ?
		GROUP BY tool 
		ORDER BY total_usage ASC
		LIMIT ?
	`

	startTime := s.getPeriodStartTime(period)
	err := s.db.Raw(query, startTime, limit).Scan(&stats).Error

	return stats, err
}

// GetUserToolBreakdown obtiene el desglose de herramientas por usuario (optimizado)
func (s *Service) GetUserToolBreakdown(userID string) (*models.UserStats, error) {
	var userStats models.UserStats
	userStats.UserID = userID
	userStats.ToolBreakdown = make(map[string]int64)

	// Query optimizado: usar índice compuesto (user_id, timestamp DESC)
	var plan string
	s.db.Raw("SELECT plan FROM analytics_operations WHERE user_id = ? ORDER BY timestamp DESC LIMIT 1", userID).Scan(&plan)
	userStats.Plan = plan

	// Total de operaciones (usa índice user_id)
	s.db.Model(&models.AnalyticsOperation{}).Where("user_id = ?", userID).Count(&userStats.TotalOperations)

	// Operaciones del día (usa índice compuesto user_id, timestamp)
	today := time.Now().Truncate(24 * time.Hour)
	s.db.Model(&models.AnalyticsOperation{}).Where("user_id = ? AND timestamp >= ?", userID, today).Count(&userStats.DailyOperations)

	// Operaciones del mes (usa índice compuesto user_id, timestamp)
	thisMonth := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)
	s.db.Model(&models.AnalyticsOperation{}).Where("user_id = ? AND timestamp >= ?", userID, thisMonth).Count(&userStats.MonthlyOperations)

	// Desglose por herramienta
	var toolBreakdown []struct {
		Tool  string
		Count int64
	}
	s.db.Raw("SELECT tool, COUNT(*) as count FROM analytics_operations WHERE user_id = ? GROUP BY tool", userID).Scan(&toolBreakdown)

	for _, tb := range toolBreakdown {
		userStats.ToolBreakdown[tb.Tool] = tb.Count
	}

	// Total MB procesados
	var totalBytes int64
	s.db.Model(&models.AnalyticsOperation{}).Where("user_id = ?", userID).Select("COALESCE(SUM(file_size), 0)").Scan(&totalBytes)
	userStats.TotalMBProcessed = float64(totalBytes) / 1048576

	// Última actividad
	s.db.Model(&models.AnalyticsOperation{}).Where("user_id = ?", userID).Select("MAX(timestamp)").Scan(&userStats.LastActivity)

	// Usuario activo (actividad en los últimos 30 días)
	last30Days := time.Now().AddDate(0, 0, -30)
	userStats.IsActive = userStats.LastActivity.After(last30Days)

	return &userStats, nil
}

// GetPlanUsageBreakdown obtiene el desglose de uso por plan
func (s *Service) GetPlanUsageBreakdown(plan string) (*models.PlanStats, error) {
	var planStats models.PlanStats
	planStats.Plan = plan
	planStats.MostUsedTools = make(map[string]int64)

	// Total de usuarios únicos
	s.db.Model(&models.AnalyticsOperation{}).Where("plan = ?", plan).Distinct("user_id").Count(&planStats.TotalUsers)

	// Usuarios activos (últimos 30 días)
	last30Days := time.Now().AddDate(0, 0, -30)
	s.db.Model(&models.AnalyticsOperation{}).Where("plan = ? AND timestamp >= ?", plan, last30Days).Distinct("user_id").Count(&planStats.ActiveUsers)

	// Total operaciones
	s.db.Model(&models.AnalyticsOperation{}).Where("plan = ?", plan).Count(&planStats.TotalOperations)

	// Promedio operaciones por usuario
	if planStats.TotalUsers > 0 {
		planStats.AvgOperationsPerUser = float64(planStats.TotalOperations) / float64(planStats.TotalUsers)
	}

	// Herramientas más usadas por este plan
	var toolUsage []struct {
		Tool  string
		Count int64
	}
	s.db.Raw("SELECT tool, COUNT(*) as count FROM analytics_operations WHERE plan = ? GROUP BY tool ORDER BY count DESC LIMIT 10", plan).Scan(&toolUsage)

	for _, tu := range toolUsage {
		planStats.MostUsedTools[tu.Tool] = tu.Count
	}

	// Tasa de retención (usuarios activos vs total)
	if planStats.TotalUsers > 0 {
		planStats.RetentionRate = float64(planStats.ActiveUsers) / float64(planStats.TotalUsers) * 100
	}

	return &planStats, nil
}

// GetGlobalUsage obtiene el uso global del sistema
func (s *Service) GetGlobalUsage() (*models.SystemOverview, error) {
	var overview models.SystemOverview
	overview.TopFailureReasons = make(map[string]int64)
	overview.PlanDistribution = make(map[string]int64)

	// Total operaciones
	s.db.Model(&models.AnalyticsOperation{}).Count(&overview.TotalOperations)

	// Operaciones hoy
	today := time.Now().Truncate(24 * time.Hour)
	s.db.Model(&models.AnalyticsOperation{}).Where("timestamp >= ?", today).Count(&overview.OperationsToday)

	// Operaciones este mes
	thisMonth := time.Date(time.Now().Year(), time.Now().Month(), 1, 0, 0, 0, 0, time.UTC)
	s.db.Model(&models.AnalyticsOperation{}).Where("timestamp >= ?", thisMonth).Count(&overview.OperationsThisMonth)

	// Usuarios únicos totales
	s.db.Model(&models.AnalyticsOperation{}).Distinct("user_id").Count(&overview.TotalUsers)

	// Usuarios activos (últimos 30 días)
	last30Days := time.Now().AddDate(0, 0, -30)
	s.db.Model(&models.AnalyticsOperation{}).Where("timestamp >= ?", last30Days).Distinct("user_id").Count(&overview.ActiveUsers)

	// Herramienta más popular
	var mostPopular struct {
		Tool  string
		Count int64
	}
	s.db.Raw("SELECT tool, COUNT(*) as count FROM analytics_operations GROUP BY tool ORDER BY count DESC LIMIT 1").Scan(&mostPopular)
	overview.MostPopularTool = mostPopular.Tool

	// Tasa de éxito general
	var successCount int64
	s.db.Model(&models.AnalyticsOperation{}).Where("status = 'success'").Count(&successCount)
	if overview.TotalOperations > 0 {
		overview.OverallSuccessRate = float64(successCount) / float64(overview.TotalOperations) * 100
	}

	// Tiempo promedio de procesamiento
	var avgDuration float64
	s.db.Model(&models.AnalyticsOperation{}).Select("AVG(duration_ms)").Scan(&avgDuration)
	overview.AvgProcessingTime = avgDuration

	// Top razones de fallo
	var failReasons []struct {
		Reason string
		Count  int64
	}
	s.db.Raw("SELECT fail_reason as reason, COUNT(*) as count FROM analytics_operations WHERE status != 'success' AND fail_reason IS NOT NULL AND fail_reason != '' GROUP BY fail_reason ORDER BY count DESC LIMIT 5").Scan(&failReasons)

	for _, fr := range failReasons {
		overview.TopFailureReasons[fr.Reason] = fr.Count
	}

	// Distribución por plan
	var planDist []struct {
		Plan  string
		Count int64
	}
	s.db.Raw("SELECT plan, COUNT(DISTINCT user_id) as count FROM analytics_operations GROUP BY plan").Scan(&planDist)

	for _, pd := range planDist {
		overview.PlanDistribution[pd.Plan] = pd.Count
	}

	// Determinar salud del sistema
	overview.SystemHealth = s.calculateSystemHealth(overview.OverallSuccessRate, overview.OperationsToday)

	return &overview, nil
}

// GetFailureRate obtiene la tasa de fallos por herramienta
func (s *Service) GetFailureRate(tool string) (float64, error) {
	var total, failures int64

	s.db.Model(&models.AnalyticsOperation{}).Where("tool = ?", tool).Count(&total)
	s.db.Model(&models.AnalyticsOperation{}).Where("tool = ? AND status != 'success'", tool).Count(&failures)

	if total == 0 {
		return 0, nil
	}

	return float64(failures) / float64(total) * 100, nil
}

// GetTopFailReasons obtiene las principales razones de fallo
func (s *Service) GetTopFailReasons(period string) (map[string]int64, error) {
	startTime := s.getPeriodStartTime(period)
	results := make(map[string]int64)

	var failReasons []struct {
		Reason string
		Count  int64
	}

	query := `
		SELECT fail_reason as reason, COUNT(*) as count 
		FROM analytics_operations 
		WHERE timestamp >= ? 
		  AND status != 'success' 
		  AND fail_reason IS NOT NULL 
		  AND fail_reason != ''
		GROUP BY fail_reason 
		ORDER BY count DESC 
		LIMIT 10
	`

	err := s.db.Raw(query, startTime).Scan(&failReasons).Error
	if err != nil {
		return nil, err
	}

	for _, fr := range failReasons {
		results[fr.Reason] = fr.Count
	}

	return results, nil
}

// GetWorkerPerformance obtiene estadísticas de rendimiento por worker
func (s *Service) GetWorkerPerformance(worker string) (*models.WorkerStats, error) {
	var stats models.WorkerStats
	stats.Worker = worker

	// Total trabajos
	s.db.Model(&models.AnalyticsOperation{}).Where("worker = ?", worker).Count(&stats.TotalJobs)

	// Tasa de éxito
	var successCount int64
	s.db.Model(&models.AnalyticsOperation{}).Where("worker = ? AND status = 'success'", worker).Count(&successCount)
	if stats.TotalJobs > 0 {
		stats.SuccessRate = float64(successCount) / float64(stats.TotalJobs) * 100
	}

	// Duración promedio
	s.db.Model(&models.AnalyticsOperation{}).Where("worker = ?", worker).Select("AVG(duration_ms)").Scan(&stats.AvgDuration)

	// CPU promedio
	s.db.Model(&models.AnalyticsOperation{}).Where("worker = ? AND cpu_used > 0", worker).Select("AVG(cpu_used)").Scan(&stats.AvgCPU)

	// RAM promedio (convertir a MB)
	var avgRAMBytes float64
	s.db.Model(&models.AnalyticsOperation{}).Where("worker = ? AND ram_used > 0", worker).Select("AVG(ram_used)").Scan(&avgRAMBytes)
	stats.AvgRAM = avgRAMBytes / 1048576

	// Determinar si está saludable
	stats.IsHealthy = stats.SuccessRate >= 95.0 && stats.AvgCPU < 80.0

	return &stats, nil
}

// getPeriodStartTime obtiene el tiempo de inicio basado en el período
func (s *Service) getPeriodStartTime(period string) time.Time {
	now := time.Now()

	switch strings.ToLower(period) {
	case "daily", "day", "today":
		return now.Truncate(24 * time.Hour)
	case "weekly", "week":
		return now.AddDate(0, 0, -7)
	case "monthly", "month":
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	case "yearly", "year":
		return time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
	default:
		return now.AddDate(0, 0, -7) // default a weekly
	}
}

// calculateSystemHealth determina la salud del sistema
func (s *Service) calculateSystemHealth(successRate float64, operationsToday int64) string {
	if successRate >= 98.0 && operationsToday > 0 {
		return "healthy"
	} else if successRate >= 95.0 {
		return "warning"
	} else {
		return "critical"
	}
}

// GetProcessingTimesByTool obtiene tiempos de procesamiento por herramienta
func (s *Service) GetProcessingTimesByTool() (map[string]float64, error) {
	results := make(map[string]float64)

	var toolTimes []struct {
		Tool        string
		AvgDuration float64
	}

	query := `
		SELECT tool, AVG(duration_ms) as avg_duration
		FROM analytics_operations 
		WHERE duration_ms > 0
		GROUP BY tool
		ORDER BY avg_duration DESC
	`

	err := s.db.Raw(query).Scan(&toolTimes).Error
	if err != nil {
		return nil, err
	}

	for _, tt := range toolTimes {
		results[tt.Tool] = tt.AvgDuration
	}

	return results, nil
}

// GetFileSizeDistribution obtiene distribución de tamaños de archivo
func (s *Service) GetFileSizeDistribution() (map[string]int64, error) {
	results := make(map[string]int64)

	query := `
		SELECT 
			CASE 
				WHEN file_size < 1048576 THEN '< 1MB'
				WHEN file_size < 10485760 THEN '1-10MB'
				WHEN file_size < 52428800 THEN '10-50MB'
				WHEN file_size < 104857600 THEN '50-100MB'
				WHEN file_size < 209715200 THEN '100-200MB'
				ELSE '> 200MB'
			END as size_range,
			COUNT(*) as count
		FROM analytics_operations 
		WHERE file_size > 0
		GROUP BY size_range
		ORDER BY MIN(file_size)
	`

	var sizeRanges []struct {
		SizeRange string
		Count     int64
	}

	err := s.db.Raw(query).Scan(&sizeRanges).Error
	if err != nil {
		return nil, err
	}

	for _, sr := range sizeRanges {
		results[sr.SizeRange] = sr.Count
	}

	return results, nil
}