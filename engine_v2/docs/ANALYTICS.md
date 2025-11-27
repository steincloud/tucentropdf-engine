# 📊 TuCentroPDF Engine V2 - Sistema Completo de Analíticas y Estadísticas

## 🎯 Resumen del Sistema

El sistema de analíticas implementado en TuCentroPDF Engine V2 proporciona una solución completa para medir, analizar y optimizar todos los aspectos de la plataforma PDF.

### ✨ Características Principales

- **Captura Automática**: Middleware que registra cada operación sin modificar el código existente
- **Almacenamiento Multinivel**: Redis para contadores rápidos + PostgreSQL para datos históricos
- **Business Intelligence**: Insights automáticos y oportunidades de negocio
- **API Completa**: 12 endpoints especializados para diferentes tipos de análisis
- **Escalabilidad**: Diseñado para manejar millones de operaciones

---

## 📊 Métricas Capturadas

### 👤 Datos del Usuario
```go
UserID       string    // ID del usuario
Plan         string    // FREE, PREMIUM, PRO, CORPORATE
IsTeamMember bool      // Si pertenece a un equipo
Country      string    // País del usuario (si disponible)
```

### 🔧 Datos de Operación
```go
Tool         string    // pdf_split, pdf_merge, ocr_ai, etc.
Operation    string    // split, merge, convert, etc.
FileSize     int64     // Tamaño del archivo en bytes
ResultSize   int64     // Tamaño del resultado en bytes
Pages        int       // Número de páginas procesadas
Worker       string    // api, ocr-worker, office-worker
Status       string    // success, failed, timeout, canceled
FailReason   string    // Razón del fallo si aplica
```

### ⚡ Datos de Rendimiento
```go
Duration     int64     // Duración total en ms
CPUUsed      float64   // CPU usado por worker (%)
RAMUsed      int64     // RAM usada en bytes
QueueTime    int64     // Tiempo en cola en ms
Retries      int       // Número de reintentos
```

---

## 📦 Arquitectura de Almacenamiento

### 🔥 Redis - Contadores Rápidos

```
tool:{tool_name}:daily_count:{date}
tool:{tool_name}:monthly_count:{month}
user:{user_id}:daily_count:{date}
user:{user_id}:tool_usage:{tool_name}
plan:{plan}:daily_count:{date}
fail_reason:{reason}:{date}
```

**TTL Automático**: Los contadores expiran automáticamente según el período.

### 📊 PostgreSQL - Datos Históricos

**Tabla Principal**: `analytics_operations`
```sql
CREATE TABLE analytics_operations (
    id UUID PRIMARY KEY,
    user_id VARCHAR NOT NULL,
    plan VARCHAR NOT NULL,
    tool VARCHAR NOT NULL,
    file_size BIGINT,
    duration_ms BIGINT,
    status VARCHAR NOT NULL,
    timestamp TIMESTAMP NOT NULL,
    -- ... más campos
);
```

**Índices Optimizados**:
- `idx_analytics_tool_timestamp` - Para consultas por herramienta
- `idx_analytics_user_plan` - Para análisis por usuario/plan
- `idx_analytics_status_tool` - Para tasas de éxito/fallo
- `idx_analytics_failures` - Para análisis de fallos

---

## 🚀 API Endpoints

### 📈 Vista General

| Endpoint | Descripción | Permisos |
|----------|-------------|----------|
| `GET /analytics/overview` | Métricas generales del sistema | Admin/Corporate |
| `GET /analytics/tools` | Estadísticas por herramienta | Admin/Corporate |
| `GET /analytics/tools/most-used` | Top herramientas más usadas | Admin/Corporate |
| `GET /analytics/tools/least-used` | Herramientas menos usadas | Admin/Corporate |
| `GET /analytics/users/{id}` | Análisis de usuario específico | Admin/Corporate/Own |
| `GET /analytics/plans` | Breakdown por plan | Admin/Corporate |
| `GET /analytics/failures` | Análisis de fallos | Admin/Corporate |
| `GET /analytics/workers` | Rendimiento de workers | Admin/Corporate |
| `GET /analytics/performance` | Métricas de rendimiento | Admin/Corporate |
| `GET /analytics/usage/trends` | Tendencias de uso | Admin/Corporate |
| `GET /analytics/upgrade-opportunities` | Oportunidades de upgrade | Admin/Corporate |
| `GET /analytics/business-insights` | Insights de negocio | Admin/Corporate |

### 📌 Parámetros de Consulta

- `period`: `daily`, `weekly`, `monthly`, `yearly`
- `limit`: Número máximo de resultados
- `tool`: Filtrar por herramienta específica

---

## 📊 Analíticas por Tipo de Usuario

### 🆓 Usuarios FREE
- **Herramientas más usadas**: Compresión, división básica
- **Patrones de uso**: Ráfagas cortas, archivos pequeños
- **Oportunidades**: Detectar usuarios que llegan al límite

### 💳 Usuarios PREMIUM
- **Herramientas favoritas**: OCR básico, Office sin marca de agua
- **Comportamiento**: Uso moderado pero consistente
- **Upgrade triggers**: Alto uso de IA OCR

### 🚀 Usuarios PRO
- **Uso intensivo**: Batch processing, herramientas avanzadas
- **Características**: Archivos grandes, equipos pequeños
- **Retención**: Alta satisfacción, uso diario

### 🏢 Usuarios CORPORATE
- **Volumen masivo**: Miles de operaciones por mes
- **Integración API**: Uso programático intensivo
- **Valor crítico**: Flujos de trabajo empresariales

---

## 📈 Business Intelligence

### 📊 Detección de Oportunidades

**Upgrade FREE → PREMIUM**
```go
// Criterios automáticos
- Más de 150 operaciones en 30 días
- Uso consistente (>10 días activos)
- Intentos de usar funciones premium
```

**Upgrade PREMIUM → PRO**
```go
// Señales de necesidad
- Más de 50 operaciones IA OCR/mes
- Archivos promedio > 40MB
- Patrones de trabajo en equipo
```

**Upgrade PRO → CORPORATE**
```go
// Indicadores empresariales
- Más de 1000 operaciones/mes
- Más de 10GB procesados
- Uso API intensivo
```

### 🚨 Alertas Automáticas

- **Tasa de fallos > 5%** en cualquier herramienta
- **Tiempo de procesamiento > 2x promedio**
- **Picos de carga inusuales**
- **Usuarios al borde del límite**

---

## 🔧 Herramientas de Optimización

### 📊 Análisis de Rendimiento

**Por Herramienta**:
```json
{
  "pdf_merge": {
    "avg_duration_ms": 1250,
    "avg_file_size_mb": 15.3,
    "success_rate": 98.7,
    "peak_usage_hour": 14
  }
}
```

**Por Worker**:
```json
{
  "ocr-worker": {
    "avg_cpu_percent": 75.2,
    "avg_ram_mb": 512,
    "jobs_per_hour": 120,
    "health_score": 95
  }
}
```

### 🐞 Detección de Problemas

**Patrones de Fallo**:
- **timeout**: Archivos muy grandes o complejos
- **out_of_memory**: Picos de RAM en workers
- **invalid_format**: Problemas de validación
- **quota_exceeded**: Usuarios al límite

---

## 📨 Integración con Handlers

El middleware de analytics se integra automáticamente sin modificar handlers existentes:

### 🔄 Captura Automática
```go
// En cualquier handler, simplemente usar:
analytics.SetAnalyticsData(c, "pages", pageCount)
analytics.SetAnalyticsData(c, "worker", "ocr-worker")
analytics.SetAnalyticsData(c, "file_size", fileSize)
```

### 📄 Logs Estructurados
```json
{
  "event_type": "operation_completed",
  "user_id": "user_123",
  "plan": "premium",
  "tool": "pdf_merge",
  "status": "success",
  "duration_ms": 1250,
  "file_size": 16777216,
  "timestamp": "2025-11-15T10:30:00Z"
}
```

---

## 🎯 Casos de Uso Empresariales

### 📈 Optimización de Producto
1. **Identificar herramientas subutilizadas** → Mejorar UX
2. **Detectar cuellos de botella** → Optimizar rendimiento
3. **Analizar patrones de fallo** → Mejorar estabilidad

### 💰 Crecimiento de Ingresos
1. **Targeting preciso** → Usuarios listos para upgrade
2. **Retención proactiva** → Detectar usuarios en riesgo
3. **Pricing optimization** → Análisis de elasticidad

### 🔍 Insights Operacionales
1. **Capacity planning** → Predicción de carga
2. **SLA monitoring** → Tiempo de respuesta
3. **Cost optimization** → Eficiencia de workers

---

## 🚀 Configuración e Implementación

### 🌍 Variables de Entorno

```bash
# Base de datos PostgreSQL
DB_HOST=localhost
DB_PORT=5432
DB_NAME=tucentropdf_analytics
DB_USER=postgres
DB_PASSWORD=password
DB_SSLMODE=disable

# Redis (ya configurado)
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=
```

### 🔄 Migraciones Automáticas

El sistema ejecuta migraciones automáticamente al iniciar:
- Crea tabla `analytics_operations`
- Crea índices optimizados
- Crea vistas materializadas (opcional)

### 📈 Dashboards Recomendados

**Grafana/Kibana** pueden consumir:
- Logs JSON estructurados
- Métricas de Redis vía exporters
- Datos PostgreSQL directamente

---

## 🛡️ Seguridad y Privacidad

### 🔒 Acceso Controlado
- **Admin**: Acceso completo a todas las analíticas
- **Corporate**: Acceso a datos agregados y trends
- **Usuarios normales**: Solo sus propios datos

### 📜 Retención de Datos
- **Redis**: Expiración automática según TTL
- **PostgreSQL**: Retención configurable (recomendado: 12 meses)
- **Logs**: Rotación según configuración del sistema

### 🚫 Anonimización
- IPs hasheadas en logs
- User IDs pueden ser hasheados en agregaciones
- Datos sensibles nunca almacenados

---

## 📆 Roadmap y Mejoras Futuras

### 🎆 Fase 1 (Completada)
- ✅ Sistema base de captura
- ✅ Endpoints API fundamentales
- ✅ Business intelligence básico

### 🔮 Fase 2 (Siguiente)
- 🔄 Machine learning para predicciones
- 📊 Dashboard web integrado
- 📧 Alertas por email/Slack
- 📈 A/B testing framework

### 🌌 Fase 3 (Futuro)
- 🤖 Recomendaciones IA personalizadas
- 🗺️ Geo-analytics avanzados
- 📱 SDK de analytics para clientes
- 🔄 Real-time analytics

---

## 🎡 Resultado Final

**TuCentroPDF Engine V2** ahora incluye el sistema de analíticas más completo de su categoría:

✅ **Captura todo** - Cada operación, fallo y métrica
✅ **Analiza profundo** - Patterns, trends, opportunities
✅ **Optimiza negocio** - Revenue, retention, growth
✅ **Escala masivo** - Millones de operaciones sin impacto
✅ **Integra fácil** - Cero modificaciones al código existente

**El motor PDF más inteligente del mercado, ahora con inteligencia de negocio incorporada.**