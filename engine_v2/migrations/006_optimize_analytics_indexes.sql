-- SQL Migration: Optimización de índices para analytics
-- Archivo: migrations/006_optimize_analytics_indexes.sql
-- Fecha: 2025-11-20

-- Descripción: Agrega índices optimizados para mejorar performance de queries analytics

BEGIN;

-- 1. Índice compuesto para queries filtrados por user_id y timestamp
-- Mejora: GetUserToolBreakdown, GetUserUsageHistory
CREATE INDEX IF NOT EXISTS idx_analytics_user_timestamp 
ON analytics_operations(user_id, timestamp DESC);

-- 2. Índice compuesto para queries filtrados por timestamp y tool
-- Mejora: GetMostUsedTools, GetLeastUsedTools
CREATE INDEX IF NOT EXISTS idx_analytics_timestamp_tool 
ON analytics_operations(timestamp DESC, tool);

-- 3. Índice para queries filtrados por operation type
-- Mejora: GetOperationBreakdown
CREATE INDEX IF NOT EXISTS idx_analytics_operation 
ON analytics_operations(operation);

-- 4. Índice compuesto para queries de plan usage
-- Mejora: GetPlanUsageBreakdown
CREATE INDEX IF NOT EXISTS idx_analytics_plan_timestamp 
ON analytics_operations(plan, timestamp DESC);

-- 5. Índice para queries de status (success/failed)
-- Mejora: Success rate calculations
CREATE INDEX IF NOT EXISTS idx_analytics_status 
ON analytics_operations(status);

-- 6. Índice parcial para errores (solo registros failed)
-- Mejora: Error analysis queries
CREATE INDEX IF NOT EXISTS idx_analytics_failures 
ON analytics_operations(tool, fail_reason, timestamp DESC)
WHERE status = 'failed';

-- 7. Índice para búsqueda por ID (UUID)
-- Ya existe por PRIMARY KEY, pero agregamos explícitamente
-- CREATE INDEX IF NOT EXISTS idx_analytics_id ON analytics_operations(id);

COMMIT;

-- Verificar índices creados
SELECT 
    tablename,
    indexname,
    indexdef
FROM pg_indexes
WHERE tablename = 'analytics_operations'
ORDER BY indexname;

-- Analizar tabla para actualizar estadísticas del query planner
ANALYZE analytics_operations;

-- Notas de optimización:
-- 1. Índice user_id + timestamp: Soporta queries de historial de usuario ordenados por fecha
-- 2. Índice timestamp + tool: Soporta queries de herramientas más usadas en período
-- 3. Índice plan + timestamp: Soporta análisis por plan de suscripción
-- 4. Índice parcial de failures: Reduce tamaño de índice solo para errores
-- 5. ANALYZE actualiza estadísticas para mejor plan de ejecución

-- Impacto esperado:
-- - Query time reducción: 60-80% en queries con filtros
-- - Index size: ~20-30% del tamaño de la tabla
-- - Write performance: Impacto mínimo (<5% overhead)
