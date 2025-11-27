package analytics

import (
	"gorm.io/gorm"
	"github.com/tucentropdf/engine-v2/internal/analytics/models"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// RunMigrations ejecuta las migraciones de la base de datos para analytics
func RunMigrations(db *gorm.DB, log *logger.Logger) error {
	log.Info("🔄 Running analytics database migrations...")

	// Auto-migrar las tablas de analytics
	err := db.AutoMigrate(
		&models.AnalyticsOperation{},
	)
	if err != nil {
		log.Error("Error running analytics migrations", "error", err)
		return err
	}

	// Crear índices adicionales para optimizar consultas
	if err := createCustomIndexes(db, log); err != nil {
		log.Error("Error creating custom indexes", "error", err)
		return err
	}

	log.Info("✅ Analytics migrations completed successfully")
	return nil
}

// createCustomIndexes crea índices personalizados para optimizar las consultas
func createCustomIndexes(db *gorm.DB, log *logger.Logger) error {
	// Índice compuesto para consultas de herramientas por período
	db.Exec("CREATE INDEX IF NOT EXISTS idx_analytics_tool_timestamp ON analytics_operations(tool, timestamp)")
	
	// Índice compuesto para consultas de usuario por plan
	db.Exec("CREATE INDEX IF NOT EXISTS idx_analytics_user_plan ON analytics_operations(user_id, plan)")
	
	// Índice compuesto para consultas de estado por herramienta
	db.Exec("CREATE INDEX IF NOT EXISTS idx_analytics_status_tool ON analytics_operations(status, tool, timestamp)")
	
	// Índice para consultas de planes por timestamp
	db.Exec("CREATE INDEX IF NOT EXISTS idx_analytics_plan_timestamp ON analytics_operations(plan, timestamp)")
	
	// Índice para consultas de workers
	db.Exec("CREATE INDEX IF NOT EXISTS idx_analytics_worker_timestamp ON analytics_operations(worker, timestamp)")
	
	// Índice para consultas de fallos
	db.Exec("CREATE INDEX IF NOT EXISTS idx_analytics_failures ON analytics_operations(status, fail_reason, timestamp) WHERE status != 'success'")
	
	// Índice para búsquedas de duración
	db.Exec("CREATE INDEX IF NOT EXISTS idx_analytics_duration ON analytics_operations(duration_ms) WHERE duration_ms > 0")
	
	// Índice para búsquedas de tamaño de archivo
	db.Exec("CREATE INDEX IF NOT EXISTS idx_analytics_file_size ON analytics_operations(file_size) WHERE file_size > 0")

	log.Info("✅ Custom indexes created successfully")
	return nil
}

// CreateAnalyticsViews crea vistas materializadas para consultas rápidas (opcional)
func CreateAnalyticsViews(db *gorm.DB, log *logger.Logger) error {
	log.Info("🔄 Creating analytics views...")

	// Vista para estadísticas diarias por herramienta
	dailyStatsView := `
		CREATE OR REPLACE VIEW daily_tool_stats AS
		SELECT 
			DATE(timestamp) as date,
			tool,
			COUNT(*) as total_operations,
			COUNT(*) FILTER (WHERE status = 'success') as successful_operations,
			ROUND(AVG(duration_ms), 2) as avg_duration_ms,
			ROUND(AVG(file_size::float), 2) as avg_file_size_bytes,
			ROUND(COUNT(*) FILTER (WHERE status = 'success')::float / COUNT(*) * 100, 2) as success_rate_percent
		FROM analytics_operations 
		WHERE timestamp >= CURRENT_DATE - INTERVAL '30 days'
		GROUP BY DATE(timestamp), tool
		ORDER BY date DESC, tool;
	`

	// Vista para estadísticas de usuarios activos
	activeUsersView := `
		CREATE OR REPLACE VIEW active_users_stats AS
		SELECT 
			user_id,
			plan,
			COUNT(*) as total_operations,
			COUNT(DISTINCT tool) as tools_used,
			MAX(timestamp) as last_activity,
			ROUND(SUM(file_size)::float / 1048576, 2) as total_mb_processed
		FROM analytics_operations 
		WHERE timestamp >= CURRENT_DATE - INTERVAL '30 days'
		GROUP BY user_id, plan
		HAVING COUNT(*) > 5
		ORDER BY total_operations DESC;
	`

	// Vista para análisis de fallos
	failureAnalysisView := `
		CREATE OR REPLACE VIEW failure_analysis AS
		SELECT 
			tool,
			fail_reason,
			COUNT(*) as failure_count,
			ROUND(AVG(duration_ms), 2) as avg_duration_before_failure,
			ROUND(AVG(file_size::float), 2) as avg_file_size_on_failure
		FROM analytics_operations 
		WHERE status != 'success' 
		  AND timestamp >= CURRENT_DATE - INTERVAL '7 days'
		  AND fail_reason IS NOT NULL
		GROUP BY tool, fail_reason
		ORDER BY failure_count DESC;
	`

	// Ejecutar creación de vistas
	if err := db.Exec(dailyStatsView).Error; err != nil {
		log.Error("Error creating daily_tool_stats view", "error", err)
		return err
	}

	if err := db.Exec(activeUsersView).Error; err != nil {
		log.Error("Error creating active_users_stats view", "error", err)
		return err
	}

	if err := db.Exec(failureAnalysisView).Error; err != nil {
		log.Error("Error creating failure_analysis view", "error", err)
		return err
	}

	log.Info("✅ Analytics views created successfully")
	return nil
}

// DropAnalyticsViews elimina las vistas de analytics (para rollback)
func DropAnalyticsViews(db *gorm.DB, log *logger.Logger) error {
	views := []string{
		"daily_tool_stats",
		"active_users_stats",
		"failure_analysis",
	}

	for _, view := range views {
		if err := db.Exec("DROP VIEW IF EXISTS " + view).Error; err != nil {
			log.Error("Error dropping view", "view", view, "error", err)
		}
	}

	log.Info("✅ Analytics views dropped successfully")
	return nil
}