#!/bin/bash
# backup.sh - Script de backup para TuCentroPDF Engine V2

set -e

# Variables de configuración
BACKUP_ROOT="/opt/backups/tucentropdf"
APP_DIR="/opt/tucentropdf/tucentropdf-engine/engine_v2"
TIMESTAMP=$(date +%Y%m%d-%H%M%S)
BACKUP_DIR="$BACKUP_ROOT/$TIMESTAMP"
RETENTION_DAYS=${BACKUP_RETENTION_DAYS:-7}
LOG_FILE="/var/log/tucentropdf-backup.log"

# Colores
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
NC='\033[0m'

log() {
    echo -e "${GREEN}[$(date '+%Y-%m-%d %H:%M:%S')]${NC} $1" | tee -a "$LOG_FILE"
}

warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1" | tee -a "$LOG_FILE"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1" | tee -a "$LOG_FILE"
    exit 1
}

# Verificar si es el usuario correcto
if [[ $EUID -eq 0 ]]; then
    error "No ejecutes como root. Usa el usuario tucentropdf."
fi

# Crear directorio de backup
setup_backup_dir() {
    log "📁 Configurando directorio de backup..."
    
    sudo mkdir -p "$BACKUP_ROOT"
    sudo chown -R tucentropdf:tucentropdf "$BACKUP_ROOT"
    mkdir -p "$BACKUP_DIR"
    
    log "✅ Directorio de backup: $BACKUP_DIR"
}

# Backup de configuración
backup_config() {
    log "⚙️ Backup de archivos de configuración..."
    
    cd "$APP_DIR" || error "No se pudo acceder al directorio de la aplicación"
    
    mkdir -p "$BACKUP_DIR/config"
    
    # Archivos de configuración críticos
    local config_files=(
        ".env.production"
        "docker-compose.prod.yml"
        "nginx/nginx.conf"
        "nginx/sites-available/"
        "monitoring/prometheus.yml"
    )
    
    for file in "${config_files[@]}"; do
        if [ -e "$file" ]; then
            cp -r "$file" "$BACKUP_DIR/config/" 2>/dev/null || warning "No se pudo copiar $file"
            log "✅ Backup: $file"
        else
            warning "⚠️ Archivo no encontrado: $file"
        fi
    done
    
    # Crear archivo de metadatos
    cat > "$BACKUP_DIR/config/backup_info.txt" << EOF
Backup TuCentroPDF Engine V2
============================
Fecha: $(date)
Host: $(hostname)
Usuario: $(whoami)
Versión Git: $(git rev-parse HEAD 2>/dev/null || echo "N/A")
Docker Images:
$(docker images | grep tucentropdf)

Configuración incluida:
- Variables de entorno (.env.production)
- Configuración Docker Compose
- Configuración Nginx
- Configuración Prometheus
EOF
    
    log "✅ Configuración respaldada"
}

# Backup de base de datos Redis
backup_redis() {
    log "📊 Backup de base de datos Redis..."
    
    mkdir -p "$BACKUP_DIR/redis"
    
    if docker ps | grep -q tucentropdf-redis; then
        # Forzar guardado en Redis
        docker exec tucentropdf-redis redis-cli BGSAVE > /dev/null 2>&1 || warning "BGSAVE falló"
        
        # Esperar a que termine el guardado
        sleep 5
        
        # Copiar archivo RDB
        if docker exec tucentropdf-redis test -f /data/dump.rdb; then
            docker cp tucentropdf-redis:/data/dump.rdb "$BACKUP_DIR/redis/dump.rdb"
            log "✅ Redis RDB copiado"
        else
            warning "⚠️ No se encontró archivo dump.rdb"
        fi
        
        # Exportar configuración de Redis
        docker exec tucentropdf-redis redis-cli CONFIG GET "*" > "$BACKUP_DIR/redis/redis_config.txt" 2>/dev/null || true
        
        # Información de Redis
        docker exec tucentropdf-redis redis-cli INFO > "$BACKUP_DIR/redis/redis_info.txt" 2>/dev/null || true
        
        log "✅ Base de datos Redis respaldada"
    else
        warning "⚠️ Redis no está ejecutándose - saltando backup"
    fi
}

# Backup de uploads importantes
backup_uploads() {
    log "📁 Backup de archivos de usuarios..."
    
    local uploads_dir="$APP_DIR/uploads"
    
    if [ -d "$uploads_dir" ]; then
        mkdir -p "$BACKUP_DIR/uploads"
        
        # Backup de archivos recientes (últimos 7 días)
        local recent_files
        recent_files=$(find "$uploads_dir" -type f -mtime -7 2>/dev/null)
        
        if [ -n "$recent_files" ]; then
            echo "$recent_files" | tar -czf "$BACKUP_DIR/uploads/recent_uploads.tar.gz" -T - 2>/dev/null || warning "Error en tar de uploads"
            
            local file_count
            file_count=$(echo "$recent_files" | wc -l)
            log "✅ Respaldados $file_count archivos recientes"
        else
            log "ℹ️ No hay archivos recientes para respaldar"
        fi
        
        # Estadísticas de uploads
        local total_files
        total_files=$(find "$uploads_dir" -type f 2>/dev/null | wc -l)
        local total_size
        total_size=$(du -sh "$uploads_dir" 2>/dev/null | cut -f1)
        
        cat > "$BACKUP_DIR/uploads/stats.txt" << EOF
Estadísticas de Uploads
======================
Total de archivos: $total_files
Tamaño total: $total_size
Backup realizado: $(date)
Criterio: Archivos de los últimos 7 días
EOF
        
    else
        warning "⚠️ Directorio uploads no existe"
    fi
}

# Backup de logs críticos
backup_logs() {
    log "📋 Backup de logs críticos..."
    
    mkdir -p "$BACKUP_DIR/logs"
    
    # Logs de la aplicación
    if [ -d "$APP_DIR/logs" ]; then
        # Solo logs de los últimos 3 días para no hacer el backup muy grande
        find "$APP_DIR/logs" -name "*.log" -mtime -3 -exec cp {} "$BACKUP_DIR/logs/" \; 2>/dev/null || true
    fi
    
    # Logs del sistema relacionados
    local system_logs=(
        "/var/log/tucentropdf-deploy.log"
        "/var/log/tucentropdf-update.log"
        "/var/log/tucentropdf-monitor.log"
        "/var/log/nginx/access.log"
        "/var/log/nginx/error.log"
    )
    
    for log_file in "${system_logs[@]}"; do
        if [ -f "$log_file" ]; then
            # Solo últimas 1000 líneas para no hacer backup muy pesado
            tail -n 1000 "$log_file" > "$BACKUP_DIR/logs/$(basename "$log_file")" 2>/dev/null || true
        fi
    done
    
    # Logs de Docker
    docker logs tucentropdf-engine --tail 500 > "$BACKUP_DIR/logs/docker-app.log" 2>/dev/null || true
    docker logs tucentropdf-redis --tail 100 > "$BACKUP_DIR/logs/docker-redis.log" 2>/dev/null || true
    docker logs tucentropdf-nginx --tail 100 > "$BACKUP_DIR/logs/docker-nginx.log" 2>/dev/null || true
    
    log "✅ Logs críticos respaldados"
}

# Backup de monitoreo
backup_monitoring() {
    log "📊 Backup de datos de monitoreo..."
    
    mkdir -p "$BACKUP_DIR/monitoring"
    
    # Estado actual del sistema
    {
        echo "=== ESTADO DEL SISTEMA ==="
        echo "Fecha: $(date)"
        echo "Uptime: $(uptime)"
        echo "Memoria: $(free -h)"
        echo "Disco: $(df -h)"
        echo ""
        
        echo "=== CONTENEDORES DOCKER ==="
        docker ps -a
        echo ""
        
        echo "=== IMÁGENES DOCKER ==="
        docker images
        echo ""
        
        echo "=== PROCESOS ==="
        ps aux | head -20
        echo ""
        
        echo "=== PUERTOS ==="
        netstat -tuln | grep -E "(80|443|8080|6379)"
        
    } > "$BACKUP_DIR/monitoring/system_status.txt"
    
    # Backup de métricas si Prometheus está corriendo
    if docker ps | grep -q tucentropdf-prometheus; then
        # Snapshot de Prometheus
        docker exec tucentropdf-prometheus promtool tsdb create-blocks-from openmetrics /prometheus /tmp/backup 2>/dev/null || true
    fi
    
    log "✅ Datos de monitoreo respaldados"
}

# Comprimir backup
compress_backup() {
    log "🗜️ Comprimiendo backup..."
    
    cd "$BACKUP_ROOT"
    tar -czf "${TIMESTAMP}_tucentropdf_backup.tar.gz" "$TIMESTAMP/"
    
    if [ $? -eq 0 ]; then
        rm -rf "$TIMESTAMP/"
        
        local compressed_size
        compressed_size=$(du -sh "${TIMESTAMP}_tucentropdf_backup.tar.gz" | cut -f1)
        log "✅ Backup comprimido: ${TIMESTAMP}_tucentropdf_backup.tar.gz ($compressed_size)"
    else
        error "❌ Error al comprimir backup"
    fi
}

# Limpiar backups antiguos
cleanup_old_backups() {
    log "🧹 Limpiando backups antiguos (>$RETENTION_DAYS días)..."
    
    cd "$BACKUP_ROOT"
    
    local old_backups
    old_backups=$(find . -name "*_tucentropdf_backup.tar.gz" -mtime +$RETENTION_DAYS 2>/dev/null)
    
    if [ -n "$old_backups" ]; then
        echo "$old_backups" | while read -r backup; do
            rm -f "$backup"
            log "🗑️ Eliminado: $backup"
        done
    else
        log "ℹ️ No hay backups antiguos para eliminar"
    fi
}

# Verificar integridad del backup
verify_backup() {
    log "✅ Verificando integridad del backup..."
    
    cd "$BACKUP_ROOT"
    local backup_file="${TIMESTAMP}_tucentropdf_backup.tar.gz"
    
    if [ -f "$backup_file" ]; then
        # Verificar que el archivo no esté corrupto
        if tar -tzf "$backup_file" > /dev/null 2>&1; then
            log "✅ Backup verificado correctamente"
            
            # Mostrar contenido
            log "📋 Contenido del backup:"
            tar -tzf "$backup_file" | head -20
            
            local file_size
            file_size=$(du -sh "$backup_file" | cut -f1)
            log "📦 Tamaño final: $file_size"
            
            return 0
        else
            error "❌ El backup está corrupto"
        fi
    else
        error "❌ El backup no se creó correctamente"
    fi
}

# Enviar notificación (opcional)
send_notification() {
    local status="$1"
    local message="$2"
    
    if [ -n "$WEBHOOK_URL" ]; then
        curl -X POST "$WEBHOOK_URL" \
             -H "Content-Type: application/json" \
             -d "{\"text\":\"TuCentroPDF Backup: $status - $message\"}" \
             > /dev/null 2>&1 || true
    fi
    
    if [ -n "$EMAIL_NOTIFY" ] && command -v mail &> /dev/null; then
        echo "$message" | mail -s "TuCentroPDF Backup: $status" "$EMAIL_NOTIFY" 2>/dev/null || true
    fi
}

# Mostrar resumen
show_summary() {
    log "📊 Resumen del backup:"
    echo "======================="
    echo "🕒 Inicio: $TIMESTAMP"
    echo "📁 Directorio: $BACKUP_ROOT"
    echo "🗜️ Archivo: ${TIMESTAMP}_tucentropdf_backup.tar.gz"
    echo "📦 Tamaño: $(du -sh "$BACKUP_ROOT/${TIMESTAMP}_tucentropdf_backup.tar.gz" 2>/dev/null | cut -f1 || echo 'N/A')"
    echo "⏱️ Duración: $(($(date +%s) - START_TIME))s"
    echo "💾 Retención: $RETENTION_DAYS días"
    echo "📋 Log: $LOG_FILE"
}

# Función principal
main() {
    local START_TIME
    START_TIME=$(date +%s)
    
    log "🚀 Iniciando backup de TuCentroPDF Engine V2..."
    
    setup_backup_dir
    backup_config
    backup_redis
    backup_uploads
    backup_logs
    backup_monitoring
    compress_backup
    verify_backup
    cleanup_old_backups
    show_summary
    
    send_notification "SUCCESS" "Backup completado exitosamente: ${TIMESTAMP}_tucentropdf_backup.tar.gz"
    
    log "✅ Backup completado exitosamente"
}

# Manejo de errores
trap 'send_notification "ERROR" "Backup falló en línea $LINENO"; error "Backup falló"' ERR

# Ejecutar función principal
main "$@"