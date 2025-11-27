# Enterprise Backup System for TuCentroPDF Engine V2

## Descripción General

Sistema de backup empresarial completo con cifrado AES256, sincronización remota, retención automática y recuperación ante desastres para TuCentroPDF Engine V2.

## Características Principales

### 🔒 Seguridad
- **Cifrado AES256-GCM**: Todos los backups están cifrados con AES256 usando PBKDF2
- **Autenticación de integridad**: Verificación automática de checksums SHA256
- **Claves derivadas**: Uso de PBKDF2 para derivar claves seguras

### 📅 Programación Automática
- **Backups completos**: Diarios a las 02:00 AM
- **Backups incrementales**: Cada 6 horas
- **Snapshots de Redis**: Cada 12 horas  
- **Limpieza de retención**: Diaria a las 03:00 AM

### 🌐 Sincronización Remota
- **Rclone**: Soporte para múltiples proveedores cloud (Google Drive, AWS S3, Azure, etc.)
- **Sincronización automática**: Upload automático después de cada backup
- **Verificación de conectividad**: Monitoreo continuo de la salud del remoto

### 🗂️ Tipos de Backup

#### PostgreSQL
- **Full backups**: Backup completo de la base de datos
- **Incremental backups**: Backups incrementales eficientes
- **Formatos**: Soporte para custom y plain SQL

#### Redis
- **Snapshots**: Backup de archivos RDB
- **Automatización**: BGSAVE automático sin interrumpir el servicio

#### Configuración del Sistema
- **Archivos de config**: docker-compose.yml, .env, Dockerfile, etc.
- **Compresión**: Archivos tar.gz comprimidos

#### Analytics Archive
- **Datos históricos**: Backup mensual de tablas de analytics
- **Archival**: Solo datos, preservando estructura

### 📊 Políticas de Retención

| Tipo de Backup | Retención Predeterminada | Configurable |
|---|---|---|
| PostgreSQL Full | 30 días | `BACKUP_RETENTION_FULL_DAYS` |
| PostgreSQL Incremental | 7 días | `BACKUP_RETENTION_INCREMENTAL_DAYS` |
| Redis Snapshots | 7 días | `BACKUP_RETENTION_REDIS_DAYS` |
| Configuración Sistema | 90 días | `BACKUP_RETENTION_CONFIG_DAYS` |
| Analytics Archive | 365 días | `BACKUP_RETENTION_ANALYTICS_DAYS` |

## Configuración

### Variables de Entorno Requeridas

```bash
# Directorios
BACKUP_DIR=./backups
BACKUP_TEMP_DIR=./backups/temp
BACKUP_ARCHIVE_DIR=./backups/archive

# PostgreSQL
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=your_password
DB_NAME=tucentropdf

# Redis  
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=your_redis_password

# Cifrado (REQUERIDO)
BACKUP_ENCRYPTION_KEY=your-32-character-or-longer-encryption-key

# Remoto (Rclone) - Opcional
BACKUP_REMOTE_ENABLED=true
RCLONE_REMOTE=drive:/tucentropdf_backups/
RCLONE_CONFIG=/path/to/rclone.conf

# Retención (Opcional)
BACKUP_RETENTION_FULL_DAYS=30
BACKUP_RETENTION_INCREMENTAL_DAYS=7
BACKUP_RETENTION_REDIS_DAYS=7
BACKUP_RETENTION_CONFIG_DAYS=90
BACKUP_RETENTION_ANALYTICS_DAYS=365

# Alertas
BACKUP_MIN_DISK_SPACE_GB=10
```

### Configuración de Rclone

```bash
# Instalar rclone
curl https://rclone.org/install.sh | sudo bash

# Configurar remoto (ej: Google Drive)
rclone config

# Verificar configuración
rclone listremotes
rclone lsf your-remote:/
```

## Uso del Sistema

### Integración en el Servidor Principal

```go
package main

import (
    "github.com/tucentropdf/engine-v2/internal/backup"
    "github.com/tucentropdf/engine-v2/internal/alerts"
    // otros imports...
)

func main() {
    // Crear instancias de dependencias
    db := setupDatabase()
    redis := setupRedis()  
    logger := setupLogger()
    alertService := alerts.NewService(db, redis, logger)
    
    // Crear módulo de backup
    backupModule := backup.NewBackupModule(db, redis, cfg, logger, alertService)
    
    // Iniciar módulo
    if err := backupModule.Start(); err != nil {
        log.Fatal("Failed to start backup module:", err)
    }
    defer backupModule.Stop()
    
    // Registrar rutas HTTP
    api := app.Group("/api/v1")
    backupModule.GetHandler().RegisterRoutes(api)
    
    // Servidor listo con backup enterprise
    app.Listen(":8080")
}
```

### API REST Endpoints

#### Estado del Sistema
```bash
# Estado general
GET /api/v1/backup/status

# Salud del sistema
GET /api/v1/backup/health
```

#### Operaciones de Backup
```bash
# Backup completo PostgreSQL
POST /api/v1/backup/run/full

# Backup incremental PostgreSQL  
POST /api/v1/backup/run/incremental

# Backup Redis
POST /api/v1/backup/run/redis

# Backup configuración
POST /api/v1/backup/run/config

# Backup analytics
POST /api/v1/backup/run/analytics
```

#### Restauración
```bash
# Restaurar backup específico
POST /api/v1/backup/restore/{type}/{filename}?target=/path/to/restore

# Listar backups disponibles
GET /api/v1/backup/list

# Verificar integridad
POST /api/v1/backup/verify/{type}/{filename}
```

#### Gestión de Retención
```bash
# Ejecutar limpieza manual
POST /api/v1/backup/cleanup

# Reporte de retención
GET /api/v1/backup/retention
```

#### Sincronización Remota
```bash
# Sincronizar al remoto
POST /api/v1/backup/sync?directory=/path/to/sync

# Listar backups remotos
GET /api/v1/backup/remote/list

# Información de cuota
GET /api/v1/backup/remote/quota
```

### Uso Programático

```go
// Disparar backup manual
if err := backupModule.TriggerFullBackup(); err != nil {
    log.Error("Manual backup failed:", err)
}

// Verificar estado
if !backupModule.IsHealthy() {
    log.Warning("Backup system unhealthy")
}

// Listar backups disponibles
backups, err := backupModule.ListBackups()
if err != nil {
    log.Error("Failed to list backups:", err)
}

// Restaurar backup específico
err := backupModule.RestoreBackup("postgresql", "postgresql_full_20250115_143022.sql.enc", "")
if err != nil {
    log.Error("Restore failed:", err)
}
```

## Estructura de Archivos

```
internal/backup/
├── module.go          # Módulo principal y API pública
├── service.go         # Servicio principal con lógica de negocio  
├── handler.go         # Manejadores HTTP REST
├── scheduler.go       # Programador automático de tareas
├── operations.go      # Operaciones de backup (PostgreSQL, Redis, etc.)
├── restore.go         # Sistema de restauración y recovery
├── retention.go       # Políticas de retención y limpieza
├── encrypt.go         # Cifrado AES256-GCM
├── rclone.go          # Gestión de sincronización remota
└── README.md          # Esta documentación
```

## Monitoreo y Alertas

### Integración con Sistema de Alertas
El sistema se integra automáticamente con el módulo de alertas interno:

```go
// Tipos de alertas enviadas
- BACKUP_CONFIG_ERROR: Error de configuración crítico
- BACKUP_PG_FULL_FAILED: Falla en backup completo PostgreSQL  
- BACKUP_PG_INCREMENTAL_FAILED: Falla en backup incremental PostgreSQL
- BACKUP_REDIS_FAILED: Falla en backup de Redis
- BACKUP_CONFIG_FAILED: Falla en backup de configuración
- BACKUP_ANALYTICS_FAILED: Falla en backup de analytics
- BACKUP_DISK_SPACE_LOW: Espacio en disco insuficiente
- BACKUP_DAILY_PARTIAL_FAILURE: Fallo parcial en rutina diaria
- BACKUP_CLEANUP_FAILED: Falla en limpieza de retención
```

### Verificaciones de Salud

El sistema verifica continuamente:
- ✅ Espacio disponible en disco
- ✅ Conectividad con PostgreSQL y Redis
- ✅ Integridad de la clave de cifrado
- ✅ Conectividad con remoto (rclone)
- ✅ Cumplimiento de políticas de retención
- ✅ Existencia de backups recientes

## Seguridad y Mejores Prácticas

### Gestión de Claves
```bash
# Generar clave segura de 32+ caracteres
openssl rand -base64 48

# Configurar como variable de entorno
export BACKUP_ENCRYPTION_KEY="your-generated-secure-key-here"
```

### Permisos de Archivos
- Directorios de backup: `0750` (rwxr-x---)
- Archivos cifrados: `0600` (rw-------)
- Archivos temporales: `0600` (rw-------)

### Backup de Configuración de Rclone
```bash
# Respaldar configuración de rclone
cp ~/.config/rclone/rclone.conf /secure/backup/location/

# Cifrar configuración
gpg --symmetric --armor rclone.conf
```

## Recuperación ante Desastres

### Escenarios Soportados

1. **Pérdida completa de base de datos PostgreSQL**
   ```bash
   POST /api/v1/backup/restore/postgresql/postgresql_full_latest.sql.enc
   ```

2. **Corrupción de Redis**
   ```bash
   POST /api/v1/backup/restore/redis/redis_snapshot_latest.rdb.enc
   ```

3. **Pérdida de configuración del sistema**
   ```bash
   POST /api/v1/backup/restore/config/system_config_latest.tar.gz.enc
   ```

4. **Migración a nuevo servidor**
   ```bash
   # 1. Configurar rclone en nuevo servidor
   # 2. Descargar backups desde remoto
   # 3. Restaurar en orden: config -> postgresql -> redis
   ```

### Plan de Recuperación

1. **Preparación**
   - Verificar clave de cifrado disponible
   - Confirmar conectividad con remoto
   - Validar espacio en disco suficiente

2. **Ejecución**
   - Detener servicios dependientes
   - Restaurar backups en orden de dependencias
   - Validar integridad post-restauración
   - Reiniciar servicios

3. **Verificación**
   - Confirmar funcionalidad de la aplicación
   - Validar integridad de datos
   - Reanudar operaciones normales

## Rendimiento y Optimización

### Consideraciones de Rendimiento
- **Backups incrementales**: Reducen tiempo de backup y espacio
- **Compresión**: tar.gz para backups de configuración
- **Cifrado**: AES256-GCM optimizado para rendimiento
- **Concurrencia**: Operaciones paralelas donde es posible

### Monitoreo de Uso de Recursos
- Espacio en disco monitoreado continuamente
- Alertas automáticas cuando espacio < 10GB
- Límites de tiempo para operaciones de backup

## Solución de Problemas

### Problemas Comunes

1. **Error de clave de cifrado**
   ```
   Error: BACKUP_ENCRYPTION_KEY is required
   Solución: Configurar variable de entorno con clave de 32+ caracteres
   ```

2. **Falla de conexión PostgreSQL**
   ```
   Error: pg_dump failed
   Solución: Verificar credenciales y conectividad DB
   ```

3. **Error de rclone**
   ```
   Error: rclone remote validation failed
   Solución: Ejecutar 'rclone config' y verificar configuración
   ```

4. **Espacio insuficiente en disco**
   ```
   Error: insufficient disk space
   Solución: Limpiar archivos antiguos o aumentar capacidad
   ```

### Logs y Debug

```bash
# Habilitar logging detallado
export LOG_LEVEL=debug

# Ver logs específicos de backup
grep "backup" /var/log/tucentropdf/app.log

# Verificar estado de rclone
rclone about your-remote:
```

## Roadmap y Mejoras Futuras

### Versión 1.1 (Planeada)
- [ ] Backup diferencial (además de incremental)
- [ ] Compresión avanzada con algoritmos optimizados
- [ ] Backup de archivos estáticos (PDFs procesados)
- [ ] Métricas avanzadas con Prometheus

### Versión 1.2 (Planeada)  
- [ ] Backup distribuido multi-nodo
- [ ] Restauración point-in-time
- [ ] Interfaz web para gestión de backups
- [ ] Integración con sistemas de ticketing

---

**Sistema Enterprise Backup para TuCentroPDF Engine V2** - Protección completa de datos con cifrado, automatización y recuperación ante desastres. Desarrollado con tecnologías modernas y mejores prácticas de seguridad.