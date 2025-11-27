package maintenance

import (
	"fmt"
	"time"
)

// SummarizeOldData resume datos antiguos para compactación
func (s *Service) SummarizeOldData() error {
	if s.db == nil {
		return nil
	}

	s.logger.Info("📊 Summarizing old analytics data...")

	// Resumir datos más antiguos de 90 días
	cutoffDate := time.Now().AddDate(0, 0, -s.dataRetentionDays)

	// 1. Crear resumen diario
	if err := s.createDailySummary(cutoffDate); err != nil {
		s.logger.Error("Error creating daily summary", "error", err)
	}

	// 2. Crear resumen mensual
	if err := s.createMonthlySummary(cutoffDate); err != nil {
		s.logger.Error("Error creating monthly summary", "error", err)
	}

	// 3. Eliminar datos detallados antiguos
	if err := s.cleanupDetailedData(cutoffDate); err != nil {
		s.logger.Error("Error cleaning detailed data", "error", err)
	}

	s.logger.Info("✅ Data summarization completed")
	return nil
}

// createDailySummary crea resumen diario de datos antiguos
func (s *Service) createDailySummary(cutoffDate time.Time) error {
	// Crear tabla de resumen diario si no existe
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS analytics_daily_summary (
			id SERIAL PRIMARY KEY,
			date DATE NOT NULL UNIQUE,
			total_operations BIGINT NOT NULL DEFAULT 0,
			unique_users BIGINT NOT NULL DEFAULT 0,
			total_file_size BIGINT NOT NULL DEFAULT 0,
			total_result_size BIGINT NOT NULL DEFAULT 0,
			total_pages BIGINT NOT NULL DEFAULT 0,
			success_rate FLOAT NOT NULL DEFAULT 0,
			avg_duration_ms BIGINT NOT NULL DEFAULT 0,
			avg_cpu_used FLOAT NOT NULL DEFAULT 0,
			avg_ram_used BIGINT NOT NULL DEFAULT 0,
			top_tools JSONB,
			plan_breakdown JSONB,
			country_breakdown JSONB,
			failure_reasons JSONB,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW()
		)
	`

	if err := s.db.Exec(createTableSQL).Error; err != nil {
		return fmt.Errorf("error creating daily summary table: %w", err)
	}

	// Resumir datos por día para fechas anteriores al cutoff
	summarizeSQL := `
		INSERT INTO analytics_daily_summary (
			date, total_operations, unique_users, total_file_size, 
			total_result_size, total_pages, success_rate, 
			avg_duration_ms, avg_cpu_used, avg_ram_used,
			top_tools, plan_breakdown, country_breakdown, failure_reasons
		)
		SELECT 
			DATE(timestamp) as date,
			COUNT(*) as total_operations,
			COUNT(DISTINCT user_id) as unique_users,
			SUM(COALESCE(file_size, 0)) as total_file_size,
			SUM(COALESCE(result_size, 0)) as total_result_size,
			SUM(COALESCE(pages, 0)) as total_pages,
			ROUND((COUNT(*) FILTER (WHERE status = 'success')::FLOAT / COUNT(*) * 100), 2) as success_rate,
			ROUND(AVG(COALESCE(duration_ms, 0))) as avg_duration_ms,
			ROUND(AVG(COALESCE(cpu_used, 0)), 3) as avg_cpu_used,
			ROUND(AVG(COALESCE(ram_used, 0))) as avg_ram_used,
			(
				SELECT json_agg(tool_count ORDER BY count DESC)::jsonb 
				FROM (
					SELECT tool, COUNT(*) as count 
					FROM analytics_operations ao2 
					WHERE DATE(ao2.timestamp) = DATE(ao.timestamp) 
					GROUP BY tool 
					LIMIT 10
				) tool_count
			) as top_tools,
			(
				SELECT json_agg(plan_count)::jsonb 
				FROM (
					SELECT plan, COUNT(*) as count 
					FROM analytics_operations ao3 
					WHERE DATE(ao3.timestamp) = DATE(ao.timestamp) 
					GROUP BY plan
				) plan_count
			) as plan_breakdown,
			(
				SELECT json_agg(country_count)::jsonb 
				FROM (
					SELECT COALESCE(country, 'unknown') as country, COUNT(*) as count 
					FROM analytics_operations ao4 
					WHERE DATE(ao4.timestamp) = DATE(ao.timestamp) 
					GROUP BY country
				) country_count
			) as country_breakdown,
			(
				SELECT json_agg(failure_count)::jsonb 
				FROM (
					SELECT fail_reason, COUNT(*) as count 
					FROM analytics_operations ao5 
					WHERE DATE(ao5.timestamp) = DATE(ao.timestamp) 
					  AND status != 'success' 
					  AND fail_reason IS NOT NULL
					GROUP BY fail_reason
				) failure_count
			) as failure_reasons
		FROM analytics_operations ao
		WHERE timestamp < ?
		  AND DATE(timestamp) NOT IN (SELECT date FROM analytics_daily_summary)
		GROUP BY DATE(timestamp)
		ON CONFLICT (date) 
		DO UPDATE SET
			total_operations = EXCLUDED.total_operations,
			unique_users = EXCLUDED.unique_users,
			total_file_size = EXCLUDED.total_file_size,
			total_result_size = EXCLUDED.total_result_size,
			total_pages = EXCLUDED.total_pages,
			success_rate = EXCLUDED.success_rate,
			avg_duration_ms = EXCLUDED.avg_duration_ms,
			avg_cpu_used = EXCLUDED.avg_cpu_used,
			avg_ram_used = EXCLUDED.avg_ram_used,
			top_tools = EXCLUDED.top_tools,
			plan_breakdown = EXCLUDED.plan_breakdown,
			country_breakdown = EXCLUDED.country_breakdown,
			failure_reasons = EXCLUDED.failure_reasons,
			updated_at = NOW()
	`

	result := s.db.Exec(summarizeSQL, cutoffDate)
	if result.Error != nil {
		return fmt.Errorf("error creating daily summaries: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		s.logger.Info("Created daily summaries", "days_summarized", result.RowsAffected)
	}

	return nil
}

// createMonthlySummary crea resumen mensual de datos
func (s *Service) createMonthlySummary(cutoffDate time.Time) error {
	// Crear tabla de resumen mensual si no existe
	createTableSQL := `
		CREATE TABLE IF NOT EXISTS analytics_monthly_summary (
			id SERIAL PRIMARY KEY,
			year_month VARCHAR(7) NOT NULL UNIQUE, -- 'YYYY-MM'
			total_operations BIGINT NOT NULL DEFAULT 0,
			unique_users BIGINT NOT NULL DEFAULT 0,
			total_file_size BIGINT NOT NULL DEFAULT 0,
			total_result_size BIGINT NOT NULL DEFAULT 0,
			total_pages BIGINT NOT NULL DEFAULT 0,
			success_rate FLOAT NOT NULL DEFAULT 0,
			avg_duration_ms BIGINT NOT NULL DEFAULT 0,
			avg_cpu_used FLOAT NOT NULL DEFAULT 0,
			avg_ram_used BIGINT NOT NULL DEFAULT 0,
			peak_daily_operations BIGINT NOT NULL DEFAULT 0,
			peak_date DATE,
			top_tools JSONB,
			plan_breakdown JSONB,
			country_breakdown JSONB,
			daily_trends JSONB,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW()
		)
	`

	if err := s.db.Exec(createTableSQL).Error; err != nil {
		return fmt.Errorf("error creating monthly summary table: %w", err)
	}

	// Resumir por mes usando los resúmenes diarios
	summarizeSQL := `
		INSERT INTO analytics_monthly_summary (
			year_month, total_operations, unique_users, total_file_size,
			total_result_size, total_pages, success_rate,
			avg_duration_ms, avg_cpu_used, avg_ram_used,
			peak_daily_operations, peak_date, top_tools, plan_breakdown,
			country_breakdown, daily_trends
		)
		SELECT 
			TO_CHAR(date, 'YYYY-MM') as year_month,
			SUM(total_operations) as total_operations,
			SUM(unique_users) as unique_users, -- Aproximado
			SUM(total_file_size) as total_file_size,
			SUM(total_result_size) as total_result_size,
			SUM(total_pages) as total_pages,
			ROUND(AVG(success_rate), 2) as success_rate,
			ROUND(AVG(avg_duration_ms)) as avg_duration_ms,
			ROUND(AVG(avg_cpu_used), 3) as avg_cpu_used,
			ROUND(AVG(avg_ram_used)) as avg_ram_used,
			MAX(total_operations) as peak_daily_operations,
			(
				SELECT date 
				FROM analytics_daily_summary ads2 
				WHERE TO_CHAR(ads2.date, 'YYYY-MM') = TO_CHAR(ads.date, 'YYYY-MM')
				ORDER BY total_operations DESC 
				LIMIT 1
			) as peak_date,
			(
				SELECT json_agg(monthly_tools ORDER BY total_count DESC)::jsonb
				FROM (
					SELECT 
						(tool_data->>'tool')::text as tool,
						SUM((tool_data->>'count')::bigint) as total_count
					FROM analytics_daily_summary ads3,
						 jsonb_array_elements(top_tools) as tool_data
					WHERE TO_CHAR(ads3.date, 'YYYY-MM') = TO_CHAR(ads.date, 'YYYY-MM')
					GROUP BY (tool_data->>'tool')::text
					ORDER BY total_count DESC
					LIMIT 10
				) monthly_tools
			) as top_tools,
			(
				SELECT json_agg(monthly_plans)::jsonb
				FROM (
					SELECT 
						(plan_data->>'plan')::text as plan,
						SUM((plan_data->>'count')::bigint) as total_count
					FROM analytics_daily_summary ads4,
						 jsonb_array_elements(plan_breakdown) as plan_data
					WHERE TO_CHAR(ads4.date, 'YYYY-MM') = TO_CHAR(ads.date, 'YYYY-MM')
					GROUP BY (plan_data->>'plan')::text
				) monthly_plans
			) as plan_breakdown,
			(
				SELECT json_agg(monthly_countries)::jsonb
				FROM (
					SELECT 
						(country_data->>'country')::text as country,
						SUM((country_data->>'count')::bigint) as total_count
					FROM analytics_daily_summary ads5,
						 jsonb_array_elements(country_breakdown) as country_data
					WHERE TO_CHAR(ads5.date, 'YYYY-MM') = TO_CHAR(ads.date, 'YYYY-MM')
					GROUP BY (country_data->>'country')::text
					ORDER BY total_count DESC
					LIMIT 20
				) monthly_countries
			) as country_breakdown,
			(
				SELECT json_agg(daily_trend ORDER BY date)::jsonb
				FROM (
					SELECT date, total_operations, success_rate
					FROM analytics_daily_summary ads6
					WHERE TO_CHAR(ads6.date, 'YYYY-MM') = TO_CHAR(ads.date, 'YYYY-MM')
					ORDER BY date
				) daily_trend
			) as daily_trends
		FROM analytics_daily_summary ads
		WHERE date < ?
		  AND TO_CHAR(date, 'YYYY-MM') NOT IN (SELECT year_month FROM analytics_monthly_summary)
		GROUP BY TO_CHAR(date, 'YYYY-MM')
		ON CONFLICT (year_month)
		DO UPDATE SET
			total_operations = EXCLUDED.total_operations,
			unique_users = EXCLUDED.unique_users,
			total_file_size = EXCLUDED.total_file_size,
			total_result_size = EXCLUDED.total_result_size,
			total_pages = EXCLUDED.total_pages,
			success_rate = EXCLUDED.success_rate,
			avg_duration_ms = EXCLUDED.avg_duration_ms,
			avg_cpu_used = EXCLUDED.avg_cpu_used,
			avg_ram_used = EXCLUDED.avg_ram_used,
			peak_daily_operations = EXCLUDED.peak_daily_operations,
			peak_date = EXCLUDED.peak_date,
			top_tools = EXCLUDED.top_tools,
			plan_breakdown = EXCLUDED.plan_breakdown,
			country_breakdown = EXCLUDED.country_breakdown,
			daily_trends = EXCLUDED.daily_trends,
			updated_at = NOW()
	`

	result := s.db.Exec(summarizeSQL, cutoffDate)
	if result.Error != nil {
		return fmt.Errorf("error creating monthly summaries: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		s.logger.Info("Created monthly summaries", "months_summarized", result.RowsAffected)
	}

	return nil
}

// cleanupDetailedData elimina datos detallados después de resumir
func (s *Service) cleanupDetailedData(cutoffDate time.Time) error {
	// Verificar que existen resúmenes antes de eliminar datos detallados
	var summaryCount int64
	countSQL := `
		SELECT COUNT(*) FROM analytics_daily_summary 
		WHERE date < ? 
		  AND date >= ?
	`
	
	// Verificar los últimos 30 días antes del cutoff
	verifyDate := cutoffDate.AddDate(0, 0, -30)
	err := s.db.Raw(countSQL, cutoffDate, verifyDate).Scan(&summaryCount).Error
	if err != nil {
		return fmt.Errorf("error verifying summaries: %w", err)
	}

	if summaryCount == 0 {
		s.logger.Warn("No daily summaries found, skipping detailed data cleanup")
		return nil
	}

	// Eliminar datos detallados anteriores al cutoff
	deleteSQL := `DELETE FROM analytics_operations WHERE timestamp < ?`
	result := s.db.Exec(deleteSQL, cutoffDate)
	if result.Error != nil {
		return fmt.Errorf("error deleting old detailed data: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		s.logger.Info("Cleaned old detailed data", 
			"deleted_operations", result.RowsAffected,
			"cutoff_date", cutoffDate.Format("2006-01-02"))
	}

	return nil
}

// ArchiveOldSummaries archiva resúmenes muy antiguos
func (s *Service) ArchiveOldSummaries() error {
	if s.db == nil {
		return nil
	}

	s.logger.Info("🗃️ Archiving old summary data...")

	// Archivar resúmenes diarios más antiguos de 12 meses
	archiveCutoff := time.Now().AddDate(-1, 0, 0) // 12 meses atrás

	if err := s.archiveDailySummaries(archiveCutoff); err != nil {
		s.logger.Error("Error archiving daily summaries", "error", err)
	}

	// Los resúmenes mensuales se mantienen más tiempo
	// Solo archivar si son más antiguos de 3 años
	monthlyArchiveCutoff := time.Now().AddDate(-3, 0, 0)
	if err := s.archiveMonthlySummaries(monthlyArchiveCutoff); err != nil {
		s.logger.Error("Error archiving monthly summaries", "error", err)
	}

	s.logger.Info("✅ Archive process completed")
	return nil
}

// archiveDailySummaries archiva resúmenes diarios antiguos
func (s *Service) archiveDailySummaries(cutoffDate time.Time) error {
	// Crear tabla de archivo si no existe
	createArchiveSQL := `
		CREATE TABLE IF NOT EXISTS analytics_daily_summary_archive (
			LIKE analytics_daily_summary INCLUDING ALL
		)
	`

	if err := s.db.Exec(createArchiveSQL).Error; err != nil {
		return fmt.Errorf("error creating daily archive table: %w", err)
	}

	// Mover datos antiguos al archivo
	moveToArchiveSQL := `
		INSERT INTO analytics_daily_summary_archive 
		SELECT * FROM analytics_daily_summary 
		WHERE date < ? 
		  AND date NOT IN (SELECT date FROM analytics_daily_summary_archive)
	`

	result := s.db.Exec(moveToArchiveSQL, cutoffDate)
	if result.Error != nil {
		return fmt.Errorf("error moving to daily archive: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		// Eliminar de la tabla principal después de archivar
		deleteSQL := `DELETE FROM analytics_daily_summary WHERE date < ?`
		deleteResult := s.db.Exec(deleteSQL, cutoffDate)
		if deleteResult.Error != nil {
			s.logger.Error("Error deleting from daily summary after archive", "error", deleteResult.Error)
		}

		s.logger.Info("Archived daily summaries", 
			"archived_days", result.RowsAffected,
			"cutoff_date", cutoffDate.Format("2006-01-02"))
	}

	return nil
}

// archiveMonthlySummaries archiva resúmenes mensuales antiguos  
func (s *Service) archiveMonthlySummaries(cutoffDate time.Time) error {
	// Crear tabla de archivo mensual si no existe
	createArchiveSQL := `
		CREATE TABLE IF NOT EXISTS analytics_monthly_summary_archive (
			LIKE analytics_monthly_summary INCLUDING ALL
		)
	`

	if err := s.db.Exec(createArchiveSQL).Error; err != nil {
		return fmt.Errorf("error creating monthly archive table: %w", err)
	}

	// Mover datos antiguos al archivo
	cutoffMonth := cutoffDate.Format("2006-01")
	moveToArchiveSQL := `
		INSERT INTO analytics_monthly_summary_archive 
		SELECT * FROM analytics_monthly_summary 
		WHERE year_month < ? 
		  AND year_month NOT IN (SELECT year_month FROM analytics_monthly_summary_archive)
	`

	result := s.db.Exec(moveToArchiveSQL, cutoffMonth)
	if result.Error != nil {
		return fmt.Errorf("error moving to monthly archive: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		// Eliminar de la tabla principal después de archivar
		deleteSQL := `DELETE FROM analytics_monthly_summary WHERE year_month < ?`
		deleteResult := s.db.Exec(deleteSQL, cutoffMonth)
		if deleteResult.Error != nil {
			s.logger.Error("Error deleting from monthly summary after archive", "error", deleteResult.Error)
		}

		s.logger.Info("Archived monthly summaries", 
			"archived_months", result.RowsAffected,
			"cutoff_month", cutoffMonth)
	}

	return nil
}

// cleanupVeryOldAnalytics limpia analíticas muy antiguas (para uso alto de disco)
func (s *Service) cleanupVeryOldAnalytics() error {
	if s.db == nil {
		return nil
	}

	// En caso de uso alto de disco, ser más agresivo
	// Eliminar datos detallados más antiguos de 30 días (en lugar de 90)
	aggressiveCutoff := time.Now().AddDate(0, 0, -30)

	s.logger.Info("🧹 Aggressive cleanup of old analytics data", "cutoff", aggressiveCutoff.Format("2006-01-02"))

	// Crear resúmenes rápidos antes de eliminar
	if err := s.createDailySummary(aggressiveCutoff); err != nil {
		s.logger.Error("Error creating summaries before aggressive cleanup", "error", err)
	}

	// Eliminar datos detallados
	deleteSQL := `DELETE FROM analytics_operations WHERE timestamp < ?`
	result := s.db.Exec(deleteSQL, aggressiveCutoff)
	if result.Error != nil {
		return fmt.Errorf("error in aggressive analytics cleanup: %w", result.Error)
	}

	if result.RowsAffected > 0 {
		s.logger.Info("Aggressive analytics cleanup completed", 
			"deleted_operations", result.RowsAffected,
			"cutoff_date", aggressiveCutoff.Format("2006-01-02"))
	}

	return nil
}

// emergencyArchiveData archivado de emergencia de datos
func (s *Service) emergencyArchiveData() error {
	if s.db == nil {
		return nil
	}

	s.logger.Info("🆘 Emergency data archival...")

	// En emergencia, archivar datos más antiguos de 7 días
	emergencyCutoff := time.Now().AddDate(0, 0, -7)

	// Crear resúmenes rápidos
	if err := s.createDailySummary(emergencyCutoff); err == nil {
		// Si los resúmenes se crean exitosamente, eliminar datos detallados
		deleteSQL := `DELETE FROM analytics_operations WHERE timestamp < ?`
		result := s.db.Exec(deleteSQL, emergencyCutoff)
		if result.Error == nil && result.RowsAffected > 0 {
			s.logger.Info("Emergency data archive completed",
				"deleted_operations", result.RowsAffected,
				"cutoff_date", emergencyCutoff.Format("2006-01-02"))
		}
	}

	return nil
}