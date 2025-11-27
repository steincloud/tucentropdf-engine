package legal_audit

import (
	"fmt"
	"gorm.io/gorm"
	"github.com/tucentropdf/engine-v2/pkg/logger"
)

// RunMigrations ejecuta las migraciones necesarias para el sistema de auditoría legal
func RunMigrations(db *gorm.DB, log *logger.Logger) error {
	log.Info("🔧 Running legal audit system migrations")

	// AutoMigrate para tablas principales
	if err := db.AutoMigrate(&LegalAuditLog{}); err != nil {
		return fmt.Errorf("failed to migrate legal_audit_logs table: %w", err)
	}

	// Crear índices adicionales si no existen
	if err := createCustomIndexes(db, log); err != nil {
		return fmt.Errorf("failed to create custom indexes: %w", err)
	}

	// Crear triggers de inmutabilidad
	if err := createImmutabilityTriggers(db, log); err != nil {
		return fmt.Errorf("failed to create immutability triggers: %w", err)
	}

	// Crear funciones de integridad
	if err := createIntegrityFunctions(db, log); err != nil {
		return fmt.Errorf("failed to create integrity functions: %w", err)
	}

	log.Info("✅ Legal audit migrations completed successfully")
	return nil
}

// createCustomIndexes crea índices optimizados para auditoría legal
func createCustomIndexes(db *gorm.DB, log *logger.Logger) error {
	indexes := []string{
		"CREATE INDEX IF NOT EXISTS idx_legal_audit_logs_timestamp ON legal_audit_logs (timestamp DESC);",
		"CREATE INDEX IF NOT EXISTS idx_legal_audit_logs_user_timestamp ON legal_audit_logs (user_id, timestamp DESC) WHERE user_id IS NOT NULL;",
		"CREATE INDEX IF NOT EXISTS idx_legal_audit_logs_tool_action ON legal_audit_logs (tool, action, timestamp DESC);",
		"CREATE INDEX IF NOT EXISTS idx_legal_audit_logs_integrity ON legal_audit_logs (integrity_hash);",
		"CREATE INDEX IF NOT EXISTS idx_legal_audit_logs_abuse ON legal_audit_logs (abuse, timestamp DESC) WHERE abuse = TRUE;",
		"CREATE INDEX IF NOT EXISTS idx_legal_audit_logs_admin ON legal_audit_logs (admin_id, timestamp DESC) WHERE admin_id IS NOT NULL;",
		"CREATE INDEX IF NOT EXISTS idx_legal_audit_logs_company ON legal_audit_logs (company_id, timestamp DESC) WHERE company_id IS NOT NULL;",
	}

	for _, indexSQL := range indexes {
		if err := db.Exec(indexSQL).Error; err != nil {
			log.Warn("Failed to create index", "sql", indexSQL, "error", err)
			// Continuar con otros índices
		}
	}

	log.Debug("Custom indexes created for legal audit system")
	return nil
}

// createImmutabilityTriggers crea triggers para garantizar inmutabilidad
func createImmutabilityTriggers(db *gorm.DB, log *logger.Logger) error {
	// Función que previene modificación de registros
	functionSQL := `
	CREATE OR REPLACE FUNCTION prevent_legal_audit_modification()
	RETURNS TRIGGER AS $$
	BEGIN
		IF TG_OP = 'UPDATE' THEN
			RAISE EXCEPTION 'LEGAL_AUDIT_VIOLATION: Modification of legal audit records is prohibited. Record ID: %', OLD.id;
		END IF;
		
		IF TG_OP = 'DELETE' THEN
			IF current_setting('app.legal_audit.allow_archival', true) != 'true' THEN
				RAISE EXCEPTION 'LEGAL_AUDIT_VIOLATION: Deletion of legal audit records is prohibited. Record ID: %', OLD.id;
			END IF;
		END IF;
		
		RETURN NULL;
	END;
	$$ LANGUAGE plpgsql;`

	if err := db.Exec(functionSQL).Error; err != nil {
		return fmt.Errorf("failed to create immutability function: %w", err)
	}

	// Crear trigger
	triggerSQL := `
	DROP TRIGGER IF EXISTS trigger_prevent_legal_audit_modification ON legal_audit_logs;
	CREATE TRIGGER trigger_prevent_legal_audit_modification
		BEFORE UPDATE OR DELETE ON legal_audit_logs
		FOR EACH ROW
		EXECUTE FUNCTION prevent_legal_audit_modification();`

	if err := db.Exec(triggerSQL).Error; err != nil {
		return fmt.Errorf("failed to create immutability trigger: %w", err)
	}

	log.Debug("Immutability triggers created for legal audit system")
	return nil
}

// createIntegrityFunctions crea funciones para verificación de integridad
func createIntegrityFunctions(db *gorm.DB, log *logger.Logger) error {
	// Función para generar hash de integridad
	hashFunctionSQL := `
	CREATE OR REPLACE FUNCTION generate_integrity_hash(
		p_user_id BIGINT,
		p_tool VARCHAR,
		p_action VARCHAR,
		p_timestamp TIMESTAMP WITH TIME ZONE,
		p_ip INET
	)
	RETURNS VARCHAR(64) AS $$
	BEGIN
		RETURN encode(
			digest(
				COALESCE(p_user_id::TEXT, 'null') || '|' ||
				p_tool || '|' ||
				p_action || '|' ||
				extract(epoch from p_timestamp)::TEXT || '|' ||
				p_ip::TEXT,
				'sha256'
			),
			'hex'
		);
	END;
	$$ LANGUAGE plpgsql;`

	if err := db.Exec(hashFunctionSQL).Error; err != nil {
		return fmt.Errorf("failed to create hash generation function: %w", err)
	}

	// Función para validar integridad
	validateFunctionSQL := `
	CREATE OR REPLACE FUNCTION validate_record_integrity(
		p_record_id UUID
	)
	RETURNS BOOLEAN AS $$
	DECLARE
		v_record legal_audit_logs%ROWTYPE;
		v_calculated_hash VARCHAR(64);
	BEGIN
		SELECT * INTO v_record 
		FROM legal_audit_logs 
		WHERE id = p_record_id;
		
		IF NOT FOUND THEN
			RETURN FALSE;
		END IF;
		
		v_calculated_hash := generate_integrity_hash(
			v_record.user_id,
			v_record.tool,
			v_record.action,
			v_record.timestamp,
			v_record.ip
		);
		
		RETURN v_record.integrity_hash = v_calculated_hash;
	END;
	$$ LANGUAGE plpgsql;`

	if err := db.Exec(validateFunctionSQL).Error; err != nil {
		return fmt.Errorf("failed to create validation function: %w", err)
	}

	log.Debug("Integrity functions created for legal audit system")
	return nil
}