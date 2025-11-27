-- ============================================================================
-- MIGRACIONES DE AUDITORÍA LEGAL - TuCentroPDF Engine V2
-- ============================================================================
-- Este archivo contiene todas las migraciones necesarias para el sistema
-- de auditoría legal con evidencia verificable criptográficamente
-- ============================================================================

-- Habilitar extensión para UUID
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

-- Habilitar extensión para cifrado
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ============================================================================
-- 1. TABLA PRINCIPAL DE AUDITORÍA LEGAL
-- ============================================================================

CREATE TABLE IF NOT EXISTS legal_audit_logs (
    -- Identificadores únicos
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    
    -- Datos del usuario
    user_id BIGINT,
    company_id BIGINT,
    admin_id BIGINT,
    api_key_id VARCHAR(255),
    
    -- Información de la operación
    tool VARCHAR(100) NOT NULL,
    action VARCHAR(100) NOT NULL,
    plan VARCHAR(50) NOT NULL,
    status VARCHAR(50) NOT NULL,
    reason TEXT,
    
    -- Detalles del archivo
    file_size BIGINT,
    
    -- Información de conexión
    ip INET NOT NULL,
    user_agent TEXT,
    
    -- Timestamp inmutable
    timestamp TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    
    -- Metadatos adicionales (JSONB para flexibilidad)
    metadata JSONB,
    
    -- Detección de abuso
    abuse BOOLEAN DEFAULT FALSE,
    
    -- Integridad y seguridad
    integrity_hash VARCHAR(64) NOT NULL,
    signature VARCHAR(128) NOT NULL,
    
    -- Campos de auditoría (inmutables)
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) DEFAULT 'system',
    
    -- Índices para búsquedas eficientes
    CONSTRAINT chk_tool_valid CHECK (tool IN (
        'pdf-merge', 'pdf-split', 'pdf-compress', 'pdf-extract',
        'pdf-convert', 'pdf-encrypt', 'pdf-decrypt', 'pdf-rotate',
        'pdf-watermark', 'pdf-stamp', 'pdf-optimize', 'office-convert',
        'ocr-text', 'ocr-image', 'image-convert', 'image-compress'
    )),
    
    CONSTRAINT chk_action_valid CHECK (action IN (
        'process', 'upload', 'download', 'convert', 'merge', 'split',
        'compress', 'extract', 'encrypt', 'decrypt', 'rotate', 'watermark',
        'stamp', 'optimize', 'ocr', 'preview', 'validate'
    )),
    
    CONSTRAINT chk_plan_valid CHECK (plan IN (
        'free', 'basic', 'professional', 'enterprise', 'api'
    )),
    
    CONSTRAINT chk_status_valid CHECK (status IN (
        'success', 'error', 'failed', 'timeout', 'cancelled', 'processing'
    ))
);

-- ============================================================================
-- 2. ÍNDICES PARA PERFORMANCE OPTIMIZADA
-- ============================================================================

-- Índice principal por timestamp para consultas por rango de fechas
CREATE INDEX IF NOT EXISTS idx_legal_audit_logs_timestamp 
    ON legal_audit_logs (timestamp DESC);

-- Índice compuesto para consultas por usuario
CREATE INDEX IF NOT EXISTS idx_legal_audit_logs_user_timestamp 
    ON legal_audit_logs (user_id, timestamp DESC) 
    WHERE user_id IS NOT NULL;

-- Índice para herramientas y acciones
CREATE INDEX IF NOT EXISTS idx_legal_audit_logs_tool_action 
    ON legal_audit_logs (tool, action, timestamp DESC);

-- Índice para consultas de integridad
CREATE INDEX IF NOT EXISTS idx_legal_audit_logs_integrity 
    ON legal_audit_logs (integrity_hash);

-- Índice para detección de abuso
CREATE INDEX IF NOT EXISTS idx_legal_audit_logs_abuse 
    ON legal_audit_logs (abuse, timestamp DESC) 
    WHERE abuse = TRUE;

-- Índice para administradores
CREATE INDEX IF NOT EXISTS idx_legal_audit_logs_admin 
    ON legal_audit_logs (admin_id, timestamp DESC) 
    WHERE admin_id IS NOT NULL;

-- Índice para exportaciones por empresa
CREATE INDEX IF NOT EXISTS idx_legal_audit_logs_company 
    ON legal_audit_logs (company_id, timestamp DESC) 
    WHERE company_id IS NOT NULL;

-- Índice para metadatos JSONB
CREATE INDEX IF NOT EXISTS idx_legal_audit_logs_metadata 
    ON legal_audit_logs USING GIN (metadata);

-- ============================================================================
-- 3. TRIGGERS PARA GARANTIZAR INMUTABILIDAD LEGAL
-- ============================================================================

-- Función que previene cualquier modificación de registros de auditoría
CREATE OR REPLACE FUNCTION prevent_legal_audit_modification()
RETURNS TRIGGER AS $$
BEGIN
    -- Prevenir UPDATE de cualquier registro
    IF TG_OP = 'UPDATE' THEN
        RAISE EXCEPTION 'LEGAL_AUDIT_VIOLATION: Modification of legal audit records is prohibited. Record ID: %', OLD.id;
    END IF;
    
    -- Prevenir DELETE de registros (solo permitir archivado)
    IF TG_OP = 'DELETE' THEN
        -- Solo permitir DELETE si es parte del proceso de archivado
        -- (verificar si se está ejecutando desde proceso autorizado)
        IF current_setting('app.legal_audit.allow_archival', true) != 'true' THEN
            RAISE EXCEPTION 'LEGAL_AUDIT_VIOLATION: Deletion of legal audit records is prohibited. Record ID: %', OLD.id;
        END IF;
    END IF;
    
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

-- Aplicar trigger de inmutabilidad
DROP TRIGGER IF EXISTS trigger_prevent_legal_audit_modification ON legal_audit_logs;
CREATE TRIGGER trigger_prevent_legal_audit_modification
    BEFORE UPDATE OR DELETE ON legal_audit_logs
    FOR EACH ROW
    EXECUTE FUNCTION prevent_legal_audit_modification();

-- ============================================================================
-- 4. FUNCIÓN PARA GENERAR HASH DE INTEGRIDAD
-- ============================================================================

CREATE OR REPLACE FUNCTION generate_integrity_hash(
    p_user_id BIGINT,
    p_tool VARCHAR,
    p_action VARCHAR,
    p_timestamp TIMESTAMP WITH TIME ZONE,
    p_ip INET
)
RETURNS VARCHAR(64) AS $$
BEGIN
    -- Generar hash SHA-256 de campos críticos
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
$$ LANGUAGE plpgsql;

-- ============================================================================
-- 5. FUNCIÓN PARA VALIDAR INTEGRIDAD DE REGISTROS
-- ============================================================================

CREATE OR REPLACE FUNCTION validate_record_integrity(
    p_record_id UUID
)
RETURNS BOOLEAN AS $$
DECLARE
    v_record legal_audit_logs%ROWTYPE;
    v_calculated_hash VARCHAR(64);
BEGIN
    -- Obtener el registro
    SELECT * INTO v_record 
    FROM legal_audit_logs 
    WHERE id = p_record_id;
    
    IF NOT FOUND THEN
        RETURN FALSE;
    END IF;
    
    -- Calcular hash esperado
    v_calculated_hash := generate_integrity_hash(
        v_record.user_id,
        v_record.tool,
        v_record.action,
        v_record.timestamp,
        v_record.ip
    );
    
    -- Comparar con hash almacenado
    RETURN v_record.integrity_hash = v_calculated_hash;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- 6. TABLA DE EXPORTACIONES DE EVIDENCIA LEGAL
-- ============================================================================

CREATE TABLE IF NOT EXISTS legal_export_requests (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    admin_id BIGINT NOT NULL,
    export_format VARCHAR(10) NOT NULL CHECK (export_format IN ('json', 'csv', 'xml')),
    encrypted BOOLEAN DEFAULT FALSE,
    record_count INTEGER NOT NULL,
    file_path TEXT NOT NULL,
    file_size BIGINT NOT NULL,
    integrity_score DECIMAL(5,2) NOT NULL,
    download_token VARCHAR(128) UNIQUE NOT NULL,
    filter_criteria JSONB,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    downloaded_at TIMESTAMP WITH TIME ZONE,
    download_count INTEGER DEFAULT 0,
    
    -- Índices
    CONSTRAINT chk_integrity_score CHECK (integrity_score >= 0 AND integrity_score <= 100)
);

CREATE INDEX IF NOT EXISTS idx_legal_export_admin 
    ON legal_export_requests (admin_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_legal_export_token 
    ON legal_export_requests (download_token) 
    WHERE download_token IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_legal_export_expires 
    ON legal_export_requests (expires_at) 
    WHERE expires_at > NOW();

-- ============================================================================
-- 7. TABLA DE RETENCIÓN Y ARCHIVADO
-- ============================================================================

CREATE TABLE IF NOT EXISTS legal_audit_archives (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    archive_date DATE NOT NULL,
    archive_file_path TEXT NOT NULL,
    record_count INTEGER NOT NULL,
    compressed_size BIGINT NOT NULL,
    original_size BIGINT NOT NULL,
    checksum VARCHAR(64) NOT NULL,
    encryption_enabled BOOLEAN DEFAULT TRUE,
    retention_until DATE NOT NULL,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL,
    
    -- Prevenir archivos duplicados para la misma fecha
    UNIQUE(archive_date)
);

CREATE INDEX IF NOT EXISTS idx_legal_archives_date 
    ON legal_audit_archives (archive_date DESC);

CREATE INDEX IF NOT EXISTS idx_legal_archives_retention 
    ON legal_audit_archives (retention_until);

-- ============================================================================
-- 8. TABLA DE ESTADÍSTICAS RÁPIDAS (PARA PERFORMANCE)
-- ============================================================================

CREATE TABLE IF NOT EXISTS legal_audit_stats (
    stat_date DATE PRIMARY KEY,
    total_events INTEGER NOT NULL DEFAULT 0,
    abuse_events INTEGER NOT NULL DEFAULT 0,
    tool_usage JSONB,
    status_counts JSONB,
    plan_distribution JSONB,
    integrity_score DECIMAL(5,2),
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- ============================================================================
-- 9. FUNCIÓN PARA LIMPIAR EXPORTACIONES EXPIRADAS
-- ============================================================================

CREATE OR REPLACE FUNCTION cleanup_expired_exports()
RETURNS INTEGER AS $$
DECLARE
    v_deleted_count INTEGER;
BEGIN
    -- Eliminar exportaciones expiradas
    WITH deleted AS (
        DELETE FROM legal_export_requests 
        WHERE expires_at < NOW()
        RETURNING id
    )
    SELECT count(*) INTO v_deleted_count FROM deleted;
    
    RETURN v_deleted_count;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- 10. FUNCIÓN PARA ESTADÍSTICAS DIARIAS AUTOMATIZADAS
-- ============================================================================

CREATE OR REPLACE FUNCTION update_daily_legal_stats(p_date DATE DEFAULT CURRENT_DATE)
RETURNS VOID AS $$
DECLARE
    v_total_events INTEGER;
    v_abuse_events INTEGER;
    v_tool_usage JSONB;
    v_status_counts JSONB;
    v_plan_distribution JSONB;
BEGIN
    -- Calcular estadísticas del día
    SELECT 
        COUNT(*),
        COUNT(*) FILTER (WHERE abuse = TRUE)
    INTO v_total_events, v_abuse_events
    FROM legal_audit_logs 
    WHERE timestamp::date = p_date;
    
    -- Distribución de herramientas
    SELECT jsonb_object_agg(tool, cnt)
    INTO v_tool_usage
    FROM (
        SELECT tool, COUNT(*) as cnt
        FROM legal_audit_logs 
        WHERE timestamp::date = p_date
        GROUP BY tool
    ) t;
    
    -- Conteo de estados
    SELECT jsonb_object_agg(status, cnt)
    INTO v_status_counts
    FROM (
        SELECT status, COUNT(*) as cnt
        FROM legal_audit_logs 
        WHERE timestamp::date = p_date
        GROUP BY status
    ) s;
    
    -- Distribución de planes
    SELECT jsonb_object_agg(plan, cnt)
    INTO v_plan_distribution
    FROM (
        SELECT plan, COUNT(*) as cnt
        FROM legal_audit_logs 
        WHERE timestamp::date = p_date
        GROUP BY plan
    ) p;
    
    -- Insertar o actualizar estadísticas
    INSERT INTO legal_audit_stats (
        stat_date, total_events, abuse_events,
        tool_usage, status_counts, plan_distribution,
        updated_at
    ) VALUES (
        p_date, v_total_events, v_abuse_events,
        v_tool_usage, v_status_counts, v_plan_distribution,
        NOW()
    )
    ON CONFLICT (stat_date) DO UPDATE SET
        total_events = EXCLUDED.total_events,
        abuse_events = EXCLUDED.abuse_events,
        tool_usage = EXCLUDED.tool_usage,
        status_counts = EXCLUDED.status_counts,
        plan_distribution = EXCLUDED.plan_distribution,
        updated_at = NOW();
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- 11. CONFIGURAR ROLES Y PERMISOS
-- ============================================================================

-- Crear rol para aplicación con permisos limitados
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'tucentropdf_audit') THEN
        CREATE ROLE tucentropdf_audit WITH LOGIN PASSWORD 'secure_audit_password_2024';
    END IF;
END
$$;

-- Otorgar permisos específicos
GRANT SELECT, INSERT ON legal_audit_logs TO tucentropdf_audit;
GRANT SELECT, INSERT, UPDATE ON legal_export_requests TO tucentropdf_audit;
GRANT SELECT, INSERT ON legal_audit_archives TO tucentropdf_audit;
GRANT SELECT, INSERT, UPDATE ON legal_audit_stats TO tucentropdf_audit;

-- Permisos para funciones
GRANT EXECUTE ON FUNCTION generate_integrity_hash TO tucentropdf_audit;
GRANT EXECUTE ON FUNCTION validate_record_integrity TO tucentropdf_audit;
GRANT EXECUTE ON FUNCTION cleanup_expired_exports TO tucentropdf_audit;
GRANT EXECUTE ON FUNCTION update_daily_legal_stats TO tucentropdf_audit;

-- ============================================================================
-- 12. CREAR VISTA PARA CONSULTAS OPTIMIZADAS
-- ============================================================================

CREATE OR REPLACE VIEW legal_audit_summary AS
SELECT 
    id,
    user_id,
    tool,
    action,
    plan,
    status,
    file_size,
    ip,
    abuse,
    timestamp,
    created_at,
    -- Campos calculados
    CASE 
        WHEN validate_record_integrity(id) THEN 'VERIFIED'
        ELSE 'COMPROMISED'
    END as integrity_status,
    
    -- Metadatos útiles
    (metadata->>'duration_ms')::INTEGER as duration_ms,
    metadata->>'worker_id' as worker_id,
    metadata->>'domain' as domain
    
FROM legal_audit_logs;

-- Otorgar acceso a la vista
GRANT SELECT ON legal_audit_summary TO tucentropdf_audit;

-- ============================================================================
-- 13. COMENTARIOS PARA DOCUMENTACIÓN
-- ============================================================================

COMMENT ON TABLE legal_audit_logs IS 'Registro inmutable de auditoría legal con evidencia criptográfica verificable';
COMMENT ON COLUMN legal_audit_logs.integrity_hash IS 'Hash SHA-256 para verificación de integridad del registro';
COMMENT ON COLUMN legal_audit_logs.signature IS 'Firma digital HMAC-SHA256 para autenticidad';
COMMENT ON COLUMN legal_audit_logs.metadata IS 'Metadatos adicionales en formato JSONB para flexibilidad';

COMMENT ON FUNCTION prevent_legal_audit_modification() IS 'Garantiza inmutabilidad legal de registros de auditoría';
COMMENT ON FUNCTION validate_record_integrity(UUID) IS 'Valida integridad criptográfica de un registro específico';

-- ============================================================================
-- MIGRACIÓN COMPLETADA EXITOSAMENTE
-- ============================================================================

-- Log de migración
INSERT INTO legal_audit_logs (
    user_id, tool, action, plan, status, ip, user_agent,
    integrity_hash, signature, metadata
) VALUES (
    NULL, 'system', 'migration', 'system', 'success', '127.0.0.1', 'TuCentroPDF-Migration/1.0',
    generate_integrity_hash(NULL, 'system', 'migration', NOW(), '127.0.0.1'),
    'migration_signature_placeholder',
    '{"type": "database_migration", "version": "1.0.0", "tables_created": 4, "functions_created": 5}'
);

SELECT 
    'MIGRACIÓN DE AUDITORÍA LEGAL COMPLETADA' as status,
    'Tablas: legal_audit_logs, legal_export_requests, legal_audit_archives, legal_audit_stats' as tables,
    'Triggers: Inmutabilidad garantizada' as security,
    'Funciones: Integridad criptográfica' as features,
    NOW() as completed_at;