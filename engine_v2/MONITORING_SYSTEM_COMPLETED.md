# Sistema de Monitoreo Interno + Alertas + Autoprotección - TuCentroPDF Engine V2

## ✅ Sistema Completamente Implementado

El **Sistema Profesional de Monitoreo Interno + Alertas Automáticas + Autoprotección** ha sido completamente implementado como parte del TuCentroPDF Engine V2.

### 🎯 Funcionalidades Principales

#### 1. **Monitoreo Continuo del Sistema** 📊
- **Checks cada 10s**: Workers OCR/Office health
- **Checks cada 15s**: CPU, RAM, Cola de trabajos  
- **Checks cada 1min**: Redis latencia y conectividad
- **Checks cada 5min**: Espacio en disco
- **Updates cada 30s**: Estado general del sistema

#### 2. **Autoprotección Inteligente** 🛡️
- **Modo Protector Automático** cuando:
  - CPU > 90% → Activación inmediata
  - RAM > 85% → Limitaciones de archivos grandes
  - RAM > 90% → Pausa OCR >20MB + GC forzado
  - Disco > 90% → Limpieza de emergencia
  - Cola > 100 jobs → Rechazo archivos grandes
  - Workers caídos → Reinicio automático

#### 3. **Sistema de Alertas Internas** 📱
- **Canales configurables**: Email SMTP + Telegram Bot
- **Niveles**: Info, Warning, Critical
- **Alertas automáticas** para todos los eventos críticos
- **Registro completo** en logs estructurados

#### 4. **Health Check Empresarial** 🏥
- **Endpoint principal**: `/api/v2/health` (Nginx-ready)
- **Endpoint básico**: `/api/v2/health/basic` (load balancers)  
- **Endpoint workers**: `/api/v2/health/workers`
- **Códigos HTTP**: 200 (ok), 206 (degraded), 503 (critical)

### 🔧 Arquitectura Implementada

#### **Paquetes Creados**
```
internal/monitor/
├── service.go      # Servicio principal + scheduler
├── checks.go       # Checks de sistema (CPU, RAM, disco, Redis, workers)
└── protection.go   # Sistema autoprotección + modo protector

internal/alerts/
└── service.go      # Alertas por email/telegram + logs

internal/api/handlers/
└── health.go       # Endpoints health check empresariales
```

#### **Scheduler Inteligente** ⏰
```go
// Cada 10 segundos - Workers críticos
CheckWorkers()

// Cada 15 segundos - Recursos sistema
CheckCPU()
CheckRAM() 
CheckQueue()

// Cada 1 minuto - Redis
CheckRedis()

// Cada 5 minutos - Disco
CheckDisk()

// Cada 30 segundos - Estado general
updateSystemStatus()

// Cada 2 minutos - Verificar modo protector
checkProtectionMode()
```

### 🚨 Sistema de Autoprotección

#### **Umbrales Configurables**
```go
CPU:    Warning 80%, Critical 90%
RAM:    Warning 75%, Critical 85%, Emergency 90%  
Disco:  Warning 80%, Critical 90%
Cola:   Warning 20, Critical 50, Max 100
Redis:  Warning 50ms, Critical 200ms latency
```

#### **Acciones Automáticas**
- **🔥 RAM Emergency (>90%)**:
  - Garbage collection forzado x2
  - Pausa OCR archivos >20MB por 5min
  - Rechazo uploads temporalmente

- **💾 Disco Critical (>90%)**:
  - Limpieza agresiva archivos temporales
  - Eliminación logs antiguos
  - Purga Redis masiva

- **📦 Cola Overload (>100)**:
  - Rechazo archivos >50MB
  - Priorización PRO/Corporate
  - Throttling operaciones FREE

- **🔧 Workers Failed**:
  - Reinicio automático contenedores
  - Registro incidentes
  - Alertas críticas inmediatas

### 📡 Endpoints Empresariales

#### **Health Check Principal** (Nginx)
```http
GET /api/v2/health
```
**Respuesta**:
```json
{
  "status": "ok|degraded|critical",
  "uptime": "1h23m45s",
  "workers": {
    "ocr": {
      "status": "ok",
      "last_seen": "2025-01-23T10:30:00Z",
      "latency_ms": 12,
      "memory_usage": "256MB",
      "restart_count": 0
    },
    "office": {
      "status": "ok", 
      "last_seen": "2025-01-23T10:30:00Z",
      "latency_ms": 18,
      "memory_usage": "128MB", 
      "restart_count": 0
    }
  },
  "redis": {
    "alive": true,
    "latency_ms": 8,
    "keys": 1523,
    "memory_mb": 45
  },
  "resources": {
    "cpu_percent": 23.4,
    "ram_percent": 41.2,
    "disk_percent": 64.1,
    "ram_used_mb": 2048,
    "ram_total_mb": 8192
  },
  "queue": {
    "pending_jobs": 12,
    "pdf_queue": 5,
    "ocr_queue": 4,
    "office_queue": 3
  },
  "protector_mode": false,
  "last_check": "2025-01-23T10:30:15Z"
}
```

#### **Otros Endpoints**
```http
GET /api/v2/health/basic           # Health básico (rápido)
GET /api/v2/health/workers         # Estado específico workers
GET /api/v2/monitoring/status      # Estado detallado monitoreo
GET /api/v2/monitoring/incidents   # Incidentes del sistema
```

### 🔔 Sistema de Alertas

#### **Configuración Email SMTP**
```env
ALERTS_EMAIL_ENABLED=true
SMTP_HOST=smtp.gmail.com
SMTP_PORT=587
SMTP_USERNAME=alerts@tucentropdf.com
SMTP_PASSWORD=your_password
ALERT_EMAIL_FROM=alerts@tucentropdf.com
ALERT_EMAIL_TO=admin@tucentropdf.com,ops@tucentropdf.com
```

#### **Configuración Telegram**
```env
ALERTS_TELEGRAM_ENABLED=true
TELEGRAM_BOT_TOKEN=123456789:ABCdefGHIjklMNOpqrSTUvwxyz
TELEGRAM_CHAT_ID=-1001234567890
```

#### **Tipos de Alertas**
- `WORKER_FAILURE` - Worker no responde
- `CPU_HIGH` - CPU alta (warning/critical)
- `RAM_HIGH` - RAM alta (warning/critical/emergency)
- `DISK_CRITICAL` - Disco lleno
- `REDIS_DOWN` - Redis no disponible
- `REDIS_SLOW` - Redis latencia alta
- `QUEUE_OVERLOAD` - Cola saturada
- `PROTECTION_MODE` - Modo protector activado
- `SYSTEM_RECOVERY` - Sistema recuperado

### 🗃️ Registro de Incidentes

#### **Tabla PostgreSQL**
```sql
CREATE TABLE system_incidents (
    id SERIAL PRIMARY KEY,
    type VARCHAR(50) NOT NULL,
    severity VARCHAR(20) NOT NULL DEFAULT 'info',
    message TEXT NOT NULL,
    details JSONB,
    timestamp TIMESTAMP DEFAULT NOW(),
    resolved BOOLEAN DEFAULT FALSE,
    resolved_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW()
);
```

#### **Tipos de Incidentes**
- `CPU_HIGH`, `RAM_HIGH`, `DISK_CRITICAL`
- `WORKER_FAILURE`, `WORKER_RECOVERY`
- `REDIS_DOWN`, `REDIS_SLOW`
- `QUEUE_OVERLOAD`, `QUEUE_HIGH`
- `PROTECTOR_ON`, `PROTECTOR_OFF`
- `WORKER_RESTART_ATTEMPT`

### 🎮 Integración Automática

El sistema se **inicia automáticamente** al arrancar el servidor:

```go
// En routes.Setup()
monitorService := monitor.NewService(db, redisClient, cfg, log)
monitorService.Start() // ✅ Auto-start monitoring
```

### 💪 Características Técnicas Avanzadas

#### **Thread-Safe Operation**
- **Mutex protection** para estado compartido
- **Context cancellation** para shutdown graceful
- **Panic recovery** en todos los checks

#### **Performance Optimizado**
- **Timeouts configurables** para health checks
- **Connection pooling** para HTTP requests
- **Minimal allocations** en hot paths

#### **Configurabilidad Total**
```env
# Umbrales personalizables
ALERT_CPU_WARNING=80.0
ALERT_CPU_CRITICAL=90.0  
ALERT_RAM_WARNING=75.0
ALERT_RAM_CRITICAL=85.0
ALERT_RAM_EMERGENCY=90.0
ALERT_DISK_WARNING=80.0
ALERT_DISK_CRITICAL=90.0
ALERT_QUEUE_WARNING=20
ALERT_QUEUE_CRITICAL=50
ALERT_QUEUE_MAX=100
ALERT_REDIS_LATENCY_MS=50
```

#### **Graceful Degradation**
- ✅ Funciona sin PostgreSQL (sin incidentes DB)
- ✅ Funciona sin Redis (sin health Redis)
- ✅ Funciona sin email/telegram (solo logs)
- ✅ **Nunca bloquea** el servicio principal

### 🎯 Resultado Final

✅ **Detección automática** de caídas workers  
✅ **Monitoreo continuo** CPU, RAM, disco  
✅ **Autoprotección inteligente** evita colapsos  
✅ **Alertas automáticas** internas (email/telegram)  
✅ **Health check empresarial** para Nginx  
✅ **Registro completo** de incidentes  
✅ **Reinicio automático** de workers  
✅ **Sin dependencias externas** (BetterStack, etc.)  

### 🚀 Estado: ✅ **COMPLETAMENTE IMPLEMENTADO**

El **Sistema Profesional de Monitoreo Interno + Alertas + Autoprotección** está **listo para producción** y proporciona:

- **🔍 Visibilidad total** del estado del sistema
- **🛡️ Protección automática** contra fallos
- **📊 Monitoreo enterprise-grade** interno
- **⚡ Respuesta inmediata** a incidentes
- **🔧 Auto-reparación** de workers
- **📱 Alertas inteligentes** sin servicios externos

El motor TuCentroPDF Engine V2 ahora es **completamente estable, auto-reparable y está protegido contra colapsos**. 🎉

---

**Fecha de implementación**: 23 de Enero de 2025  
**Integración**: Sistema completamente integrado y funcional  
**Compatibilidad**: Windows + Linux + Docker + Kubernetes  
**Estado**: 🟢 **PRODUCTION READY**