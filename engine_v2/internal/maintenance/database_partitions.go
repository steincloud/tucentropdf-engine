package maintenance

import (
	"fmt"
	"time"
)

// CreateMonthlyPartition crea partición mensual para analytics_operations
func (s *Service) CreateMonthlyPartition() error {
	if s.db == nil {
		return nil
	}

	s.logger.Info("📅 Creating monthly partition...")

	// Crear partición para el mes actual
	if err := s.createPartitionForMonth(time.Now()); err != nil {
		s.logger.Error("Error creating current month partition", "error", err)
	}

	// Crear partición para el próximo mes
	nextMonth := time.Now().AddDate(0, 1, 0)
	if err := s.createPartitionForMonth(nextMonth); err != nil {
		s.logger.Error("Error creating next month partition", "error", err)
	}

	s.logger.Info("✅ Monthly partitions created")
	return nil
}

// createPartitionForMonth crea una partición para un mes específico
func (s *Service) createPartitionForMonth(date time.Time) error {
	year := date.Year()
	month := int(date.Month())
	partitionName := fmt.Sprintf("analytics_operations_%d_%02d", year, month)

	// Verificar si la partición ya existe
	var exists bool
	query := `
		SELECT EXISTS (
			SELECT 1 FROM pg_tables 
			WHERE tablename = ? 
			AND schemaname = 'public'
		)
	`
	err := s.db.Raw(query, partitionName).Scan(&exists).Error
	if err != nil {
		return fmt.Errorf("error checking partition existence: %w", err)
	}

	if exists {
		s.logger.Debug("Partition already exists", "partition", partitionName)
		return nil
	}

	// Calcular rango de fechas para la partición
	startDate := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC)
	endDate := startDate.AddDate(0, 1, 0) // Primer día del siguiente mes

	// Crear la partición
	createPartitionSQL := fmt.Sprintf(`
		CREATE TABLE %s PARTITION OF analytics_operations 
		FOR VALUES FROM ('%s') TO ('%s')
	`, partitionName, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	err = s.db.Exec(createPartitionSQL).Error
	if err != nil {
		return fmt.Errorf("error creating partition %s: %w", partitionName, err)
	}

	// Crear índices específicos para la partición
	if err := s.createPartitionIndexes(partitionName); err != nil {
		s.logger.Error("Error creating partition indexes", "partition", partitionName, "error", err)
	}

	s.logger.Info("Created partition", "partition", partitionName, "start", startDate.Format("2006-01"), "end", endDate.Format("2006-01"))
	return nil
}

// createPartitionIndexes crea índices para una partición específica
func (s *Service) createPartitionIndexes(partitionName string) error {
	indexes := []string{
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_user_timestamp ON %s(user_id, timestamp)", partitionName, partitionName),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_tool_timestamp ON %s(tool, timestamp)", partitionName, partitionName),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_plan_timestamp ON %s(plan, timestamp)", partitionName, partitionName),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_status ON %s(status) WHERE status != 'success'", partitionName, partitionName),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_worker ON %s(worker, timestamp)", partitionName, partitionName),
	}

	for _, indexSQL := range indexes {
		if err := s.db.Exec(indexSQL).Error; err != nil {
			s.logger.Error("Error creating index", "sql", indexSQL, "error", err)
		}
	}

	return nil
}

// setupPartitionedTable configura la tabla principal como particionada
func (s *Service) SetupPartitionedTable() error {
	if s.db == nil {
		return nil
	}

	s.logger.Info("🗂️ Setting up partitioned analytics table...")

	// Verificar si la tabla ya está particionada
	var isPartitioned bool
	query := `
		SELECT EXISTS (
			SELECT 1 FROM pg_partitioned_table 
			WHERE schemaname = 'public' AND tablename = 'analytics_operations'
		)
	`
	err := s.db.Raw(query).Scan(&isPartitioned).Error
	if err != nil {
		return fmt.Errorf("error checking if table is partitioned: %w", err)
	}

	if isPartitioned {
		s.logger.Info("Table is already partitioned")
		return nil
	}

	// Si la tabla existe pero no está particionada, necesitamos migrarla
	var tableExists bool
	existsQuery := `
		SELECT EXISTS (
			SELECT 1 FROM pg_tables 
			WHERE tablename = 'analytics_operations' 
			AND schemaname = 'public'
		)
	`
	err = s.db.Raw(existsQuery).Scan(&tableExists).Error
	if err != nil {
		return fmt.Errorf("error checking table existence: %w", err)
	}

	if tableExists {
		// Migrar tabla existente a particionada
		return s.migrateToPartitionedTable()
	} else {
		// Crear nueva tabla particionada
		return s.createPartitionedTable()
	}
}

// createPartitionedTable crea una nueva tabla particionada
func (s *Service) createPartitionedTable() error {
	createSQL := `
		CREATE TABLE analytics_operations (
			id UUID DEFAULT gen_random_uuid(),
			user_id VARCHAR NOT NULL,
			plan VARCHAR NOT NULL,
			is_team_member BOOLEAN DEFAULT FALSE,
			country VARCHAR,
			tool VARCHAR NOT NULL,
			operation VARCHAR,
			file_size BIGINT,
			result_size BIGINT,
			pages INTEGER,
			worker VARCHAR,
			status VARCHAR NOT NULL,
			fail_reason VARCHAR,
			duration_ms BIGINT,
			cpu_used FLOAT,
			ram_used BIGINT,
			queue_time_ms BIGINT,
			retries INTEGER DEFAULT 0,
			timestamp TIMESTAMP NOT NULL,
			created_at TIMESTAMP DEFAULT NOW(),
			updated_at TIMESTAMP DEFAULT NOW(),
			PRIMARY KEY (id, timestamp)
		) PARTITION BY RANGE (timestamp)
	`

	err := s.db.Exec(createSQL).Error
	if err != nil {
		return fmt.Errorf("error creating partitioned table: %w", err)
	}

	s.logger.Info("✅ Created partitioned analytics_operations table")
	return nil
}

// migrateToPartitionedTable migra tabla existente a particionada
func (s *Service) migrateToPartitionedTable() error {
	s.logger.Info("🔄 Migrating existing table to partitioned...")

	// Este es un proceso complejo que requiere:
	// 1. Renombrar tabla existente
	// 2. Crear nueva tabla particionada
	// 3. Migrar datos
	// 4. Limpiar tabla antigua

	// Por ahora, loggear que se requiere migración manual
	s.logger.Warn("⚠️ Manual migration required: existing analytics_operations table needs to be partitioned")
	s.logger.Info("Recommendation: Use pg_partman or manual partitioning for existing data")

	return nil
}

// getActivePartitions obtiene lista de particiones activas
func (s *Service) getActivePartitions() ([]string, error) {
	if s.db == nil {
		return []string{}, nil
	}

	var partitions []string
	query := `
		SELECT tablename 
		FROM pg_tables 
		WHERE tablename LIKE 'analytics_operations_%' 
		  AND schemaname = 'public'
		ORDER BY tablename
	`

	err := s.db.Raw(query).Scan(&partitions).Error
	if err != nil {
		return nil, err
	}

	// Formatear nombres para mostrar solo año_mes
	var formatted []string
	for _, partition := range partitions {
		// analytics_operations_2025_01 -> 2025_01
		if len(partition) > len("analytics_operations_") {
			suffix := partition[len("analytics_operations_"):]
			formatted = append(formatted, suffix)
		}
	}

	return formatted, nil
}