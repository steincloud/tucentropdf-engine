# Sistema de Mantenimiento Automático - TuCentroPDF Engine V2

## ✅ Sistema Completamente Implementado

El **Sistema de Mantenimiento Automático** ha sido completamente implementado como parte del TuCentroPDF Engine V2. Este sistema proporciona:

### 🎯 Funcionalidades Principales

#### 1. **Monitoreo Automático de Disco** 📊
- **Verificación cada 10 minutos** del espacio disponible
- **Umbrales configurables**: 80% advertencia, 90% crítico  
- **Acciones automáticas** en base a niveles de uso
- **Soporte Windows** con API nativa GetDiskFreeSpaceEx

#### 2. **Limpieza Automática de Redis** 🗃️
- **Eliminación de claves expiradas** manualmente
- **Corrección automática de TTLs** faltantes  
- **Limpieza de contadores** diarios/mensuales antiguos
- **Optimización de memoria** Redis con MEMORY PURGE

#### 3. **Particionamiento PostgreSQL** 🗂️
- **Particiones mensuales automáticas** para `analytics_operations`
- **Creación anticipada** del próximo mes
- **Índices optimizados** por partición
- **Migración automática** de tabla existente

#### 4. **Rotación y Archivado de Logs** 📄
- **Rotación automática** cada 7 días
- **Compresión** de logs archivados
- **Limpieza de emergencia** cuando disco está lleno
- **Detección inteligente** de archivos de log

#### 5. **Summarización de Datos** 📈
- **Compactación diaria** de datos analytics antiguos (>90 días)
- **Resúmenes mensuales** con tendencias y estadísticas
- **Archivado a largo plazo** de resúmenes antiguos
- **Preservación de información** crítica de negocio

### 🕰️ Cronogramas de Ejecución

#### **Cada 10 Minutos** ⏰
```
✅ Verificar espacio en disco
✅ Limpiar Redis (claves expiradas)
✅ Limpiar archivos temporales
✅ Rotar logs básicos
```

#### **Diario (2:00 AM)** 🌙
```
✅ Summarizar datos antiguos (compactación)
✅ Limpieza profunda de Redis
✅ Resetear contadores obsoletos
```

#### **Mensual (Día 1, 3:00 AM)** 📅
```
✅ Crear particiones PostgreSQL
✅ Archivar resúmenes antiguos
✅ Limpieza de logs rotados muy antiguos
```

### 🚨 Protecciones de Emergencia

#### **Uso Crítico de Disco (>90%)**
- **Limpieza agresiva** de archivos temporales
- **Eliminación inmediata** de logs antiguos  
- **Purga masiva** de Redis temporal
- **Archivado de emergencia** de datos (>7 días)

#### **Uso Alto de Disco (80-90%)**
- **Limpieza preventiva** de archivos temporales
- **Rotación prematura** de logs
- **Limpieza de analytics** muy antiguos

### 📡 Endpoints de API

#### **Estado del Sistema**
```http
GET /api/v2/maintenance/status
```
Respuesta:
```json
{
  "timestamp": "2025-01-23T10:30:00Z",
  "disk_usage": "75.2%",
  "redis_keys": 14523,
  "redis_memory": "156MB", 
  "partitions": ["2025_01", "2025_02"],
  "temp_folder_size": "2.1GB",
  "status": "healthy"
}
```

#### **Configuración de Mantenimiento**
```http
GET /api/v2/maintenance/config
```

#### **Ejecución Manual**
```http
POST /api/v2/maintenance/trigger?type=all
POST /api/v2/maintenance/trigger?type=disk
POST /api/v2/maintenance/trigger?type=redis
POST /api/v2/maintenance/trigger?type=logs
POST /api/v2/maintenance/trigger?type=data
```

### 🛠️ Archivos Implementados

```
internal/maintenance/
├── service.go              # Servicio principal y scheduler
├── redis_cleanup.go        # Limpieza automática Redis
├── database_partitions.go  # Particionamiento PostgreSQL
├── disk_monitor.go         # Monitoreo y protección disco
├── log_rotation.go         # Rotación y compresión logs
└── data_archival.go        # Summarización y archivado

internal/api/handlers/
└── maintenance.go          # API endpoints mantenimiento

internal/api/routes/
└── routes.go               # (Actualizado con rutas mantenimiento)
```

### 🎮 Uso del Sistema

#### **Inicio Automático**
El sistema se **inicia automáticamente** cuando arranca el servidor:

```go
// En routes.Setup()
maintenanceService := maintenance.NewService(db, redisClient, cfg, log)
maintenanceService.Start() // ✅ Inicia automáticamente
```

#### **Configuración**
```go
Service{
    diskThresholdWarning:  80.0,  // %
    diskThresholdCritical: 90.0,  // %
    maxTempFileAge:        72 * time.Hour,
    maxLogAge:             7 * 24 * time.Hour,
    dataRetentionDays:     90,    // días
}
```

### 🔧 Características Técnicas

#### **Graceful Degradation**
- Funciona sin Redis ✅
- Funciona sin PostgreSQL ✅ 
- **Logs detallados** de todas las operaciones
- **Manejo de errores** sin fallos críticos

#### **Optimizaciones**
- **Context-aware**: Respeta cancelaciones
- **Pool de conexiones** eficiente
- **Índices específicos** por partición
- **Compresión** de archivos archivados

#### **Monitoreo**
- **Logs estructurados** con niveles apropiados
- **Métricas de rendimiento** (archivos eliminados, espacio liberado)
- **Alertas automáticas** en situaciones críticas

### 🎯 Beneficios

✅ **Prevención proactiva** de problemas de espacio  
✅ **Optimización automática** de rendimiento  
✅ **Escalabilidad** a largo plazo con particionamiento  
✅ **Visibilidad completa** del estado del sistema  
✅ **Recuperación automática** en situaciones críticas  
✅ **Configuración flexible** y extensible  

### 🚀 Estado: ✅ **COMPLETAMENTE IMPLEMENTADO**

El sistema está **listo para producción** y proporcionará mantenimiento automático profesional para garantizar la estabilidad y escalabilidad del TuCentroPDF Engine V2.

---

**Fecha de implementación**: 23 de Enero de 2025  
**Integración**: Sistema completamente integrado con architecture existente  
**Compatibilidad**: Windows + Linux + PostgreSQL + Redis  
**Estado**: 🟢 **PRODUCTION READY**