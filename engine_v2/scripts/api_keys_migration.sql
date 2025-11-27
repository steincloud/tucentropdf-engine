-- Migration: API Keys System
-- Description: Sistema de autenticación con API Keys para TuCentroPDF Engine V2
-- Version: 1.0.0
-- Date: 2025-11-18

-- ============================================
-- Tabla: api_keys
-- ============================================
CREATE TABLE IF NOT EXISTS api_keys (
    -- Identificación
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    
    -- Usuario y relación
    user_id VARCHAR(255) NOT NULL,
    company_id VARCHAR(255),
    
    -- API Key (almacenada como hash SHA-256)
    key_hash VARCHAR(64) NOT NULL UNIQUE,
    key_prefix VARCHAR(16) NOT NULL, -- Primeros 8 caracteres para identificación
    
    -- Plan y permisos
    plan VARCHAR(50) NOT NULL DEFAULT 'free',
    -- Valores permitidos: 'free', 'premium', 'pro', 'corporate'
    
    -- Estado
    active BOOLEAN NOT NULL DEFAULT true,
    revoked BOOLEAN NOT NULL DEFAULT false,
    revoked_at TIMESTAMP,
    revoked_reason TEXT,
    
    -- Metadata
    name VARCHAR(255), -- Nombre descriptivo de la key
    description TEXT,
    
    -- Fechas
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP,
    last_used_at TIMESTAMP,
    
    -- Uso
    total_requests BIGINT NOT NULL DEFAULT 0,
    total_bytes BIGINT NOT NULL DEFAULT 0,
    
    -- Restricciones de seguridad
    allowed_ips TEXT[], -- Array de IPs permitidas (null = todas)
    allowed_origins TEXT[], -- Array de orígenes permitidos
    rate_limit_override INTEGER, -- Override de rate limit específico
    
    -- Auditoría
    created_by VARCHAR(255),
    updated_by VARCHAR(255),
    
    -- Constraints
    CONSTRAINT chk_plan CHECK (plan IN ('free', 'premium', 'pro', 'corporate')),
    CONSTRAINT chk_key_prefix_length CHECK (LENGTH(key_prefix) = 8),
    CONSTRAINT chk_key_hash_length CHECK (LENGTH(key_hash) = 64)
);

-- ============================================
-- Índices para optimización
-- ============================================

-- Índice principal para búsqueda por hash
CREATE INDEX IF NOT EXISTS idx_api_keys_key_hash ON api_keys(key_hash) WHERE active = true;

-- Índice para búsqueda por usuario
CREATE INDEX IF NOT EXISTS idx_api_keys_user_id ON api_keys(user_id) WHERE active = true;

-- Índice para búsqueda por plan
CREATE INDEX IF NOT EXISTS idx_api_keys_plan ON api_keys(plan) WHERE active = true;

-- Índice para búsqueda por empresa
CREATE INDEX IF NOT EXISTS idx_api_keys_company_id ON api_keys(company_id) WHERE company_id IS NOT NULL;

-- Índice para búsqueda por expiración
CREATE INDEX IF NOT EXISTS idx_api_keys_expires_at ON api_keys(expires_at) WHERE expires_at IS NOT NULL;

-- Índice para claves activas y no revocadas
CREATE INDEX IF NOT EXISTS idx_api_keys_active_not_revoked ON api_keys(active, revoked) WHERE active = true AND revoked = false;

-- ============================================
-- Trigger para updated_at automático
-- ============================================
CREATE OR REPLACE FUNCTION update_api_keys_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_api_keys_updated_at
    BEFORE UPDATE ON api_keys
    FOR EACH ROW
    EXECUTE FUNCTION update_api_keys_updated_at();

-- ============================================
-- Función: Generar API Key segura
-- ============================================
CREATE OR REPLACE FUNCTION generate_api_key_prefix()
RETURNS TEXT AS $$
DECLARE
    chars TEXT := 'ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789';
    result TEXT := 'tc_';
    i INTEGER;
BEGIN
    FOR i IN 1..5 LOOP
        result := result || substr(chars, floor(random() * length(chars) + 1)::int, 1);
    END LOOP;
    RETURN result;
END;
$$ LANGUAGE plpgsql;

-- ============================================
-- Datos iniciales para testing
-- ============================================

-- Admin key (tc_ADMIN_...)
-- Key en texto plano: tc_ADMIN_TEST1234567890ABCDEFGHIJKLMNOP (solo para desarrollo)
-- Hash SHA-256: calculado en la aplicación
-- Nota: En producción, generar keys reales con el endpoint

INSERT INTO api_keys (
    user_id,
    key_hash,
    key_prefix,
    plan,
    name,
    description,
    active,
    expires_at
) VALUES (
    'admin',
    'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855', -- Placeholder hash
    'tc_ADMIN',
    'corporate',
    'Admin Development Key',
    'Key de desarrollo para administrador - CAMBIAR EN PRODUCCIÓN',
    true,
    NOW() + INTERVAL '1 year'
) ON CONFLICT (key_hash) DO NOTHING;

-- Free tier test key
INSERT INTO api_keys (
    user_id,
    key_hash,
    key_prefix,
    plan,
    name,
    description,
    active,
    expires_at
) VALUES (
    'test_user_free',
    'f7fbba6e0636f890e56fbbf3283e524c6fa3204ae298382d624741d0dc6638326e',
    'tc_FREE0',
    'free',
    'Free Tier Test Key',
    'Key de prueba para plan gratuito',
    true,
    NOW() + INTERVAL '1 year'
) ON CONFLICT (key_hash) DO NOTHING;

-- ============================================
-- Vista: Active API Keys (para consultas rápidas)
-- ============================================
CREATE OR REPLACE VIEW active_api_keys AS
SELECT 
    id,
    user_id,
    company_id,
    key_prefix,
    plan,
    name,
    created_at,
    expires_at,
    last_used_at,
    total_requests,
    total_bytes
FROM api_keys
WHERE active = true 
  AND revoked = false 
  AND (expires_at IS NULL OR expires_at > NOW());

-- ============================================
-- Función: Validar API Key
-- ============================================
CREATE OR REPLACE FUNCTION validate_api_key(
    p_key_hash VARCHAR(64)
)
RETURNS TABLE (
    is_valid BOOLEAN,
    user_id VARCHAR(255),
    plan VARCHAR(50),
    expires_at TIMESTAMP,
    rate_limit_override INTEGER
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        true AS is_valid,
        k.user_id,
        k.plan,
        k.expires_at,
        k.rate_limit_override
    FROM api_keys k
    WHERE k.key_hash = p_key_hash
      AND k.active = true
      AND k.revoked = false
      AND (k.expires_at IS NULL OR k.expires_at > NOW())
    LIMIT 1;
    
    IF NOT FOUND THEN
        RETURN QUERY SELECT false, NULL::VARCHAR, NULL::VARCHAR, NULL::TIMESTAMP, NULL::INTEGER;
    END IF;
END;
$$ LANGUAGE plpgsql;

-- ============================================
-- Función: Registrar uso de API Key
-- ============================================
CREATE OR REPLACE FUNCTION track_api_key_usage(
    p_key_hash VARCHAR(64),
    p_bytes BIGINT DEFAULT 0
)
RETURNS VOID AS $$
BEGIN
    UPDATE api_keys
    SET 
        total_requests = total_requests + 1,
        total_bytes = total_bytes + p_bytes,
        last_used_at = NOW()
    WHERE key_hash = p_key_hash;
END;
$$ LANGUAGE plpgsql;

-- ============================================
-- Función: Revocar API Key
-- ============================================
CREATE OR REPLACE FUNCTION revoke_api_key(
    p_key_hash VARCHAR(64),
    p_reason TEXT DEFAULT NULL,
    p_revoked_by VARCHAR(255) DEFAULT NULL
)
RETURNS BOOLEAN AS $$
DECLARE
    v_found BOOLEAN;
BEGIN
    UPDATE api_keys
    SET 
        revoked = true,
        revoked_at = NOW(),
        revoked_reason = p_reason,
        updated_by = p_revoked_by
    WHERE key_hash = p_key_hash
      AND active = true
      AND revoked = false
    RETURNING true INTO v_found;
    
    RETURN COALESCE(v_found, false);
END;
$$ LANGUAGE plpgsql;

-- ============================================
-- Comentarios de documentación
-- ============================================
COMMENT ON TABLE api_keys IS 'Sistema de autenticación con API Keys para TuCentroPDF Engine V2';
COMMENT ON COLUMN api_keys.key_hash IS 'Hash SHA-256 de la API key (64 caracteres hex)';
COMMENT ON COLUMN api_keys.key_prefix IS 'Primeros 8 caracteres de la key para identificación (tc_XXXXX)';
COMMENT ON COLUMN api_keys.plan IS 'Plan del usuario: free, premium, pro, corporate';
COMMENT ON COLUMN api_keys.allowed_ips IS 'IPs permitidas para usar esta key (NULL = todas)';
COMMENT ON COLUMN api_keys.rate_limit_override IS 'Override de rate limit específico para esta key';

-- ============================================
-- Grants de permisos (ajustar según usuario DB)
-- ============================================
-- GRANT SELECT, INSERT, UPDATE ON api_keys TO tucentropdf_app;
-- GRANT SELECT ON active_api_keys TO tucentropdf_app;
-- GRANT EXECUTE ON FUNCTION validate_api_key TO tucentropdf_app;
-- GRANT EXECUTE ON FUNCTION track_api_key_usage TO tucentropdf_app;
-- GRANT EXECUTE ON FUNCTION revoke_api_key TO tucentropdf_app;

-- ============================================
-- Verificación final
-- ============================================
DO $$
BEGIN
    RAISE NOTICE '✅ Migration completed successfully';
    RAISE NOTICE '📊 API Keys table created';
    RAISE NOTICE '🔍 Indexes created';
    RAISE NOTICE '⚡ Triggers configured';
    RAISE NOTICE '🎯 Helper functions created';
    RAISE NOTICE '';
    RAISE NOTICE '🔐 Test keys inserted:';
    RAISE NOTICE '   - Admin key: tc_ADMIN (corporate plan)';
    RAISE NOTICE '   - Free key: tc_FREE0 (free plan)';
    RAISE NOTICE '';
    RAISE NOTICE '⚠️  IMPORTANTE: Cambiar las keys de prueba en producción';
END $$;
