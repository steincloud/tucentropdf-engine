#!/bin/bash
# update.sh - Script para actualizar TuCentroPDF Engine V2

set -e

# Variables
APP_DIR="/opt/tucentropdf/tucentropdf-engine/engine_v2"
BACKUP_DIR="/opt/tucentropdf/backups"
LOG_FILE="/var/log/tucentropdf-update.log"

# Colores
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
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

cd "$APP_DIR" || error "No se pudo acceder al directorio de la aplicación"

log "🔄 Iniciando actualización de TuCentroPDF Engine V2..."

# 1. Crear backup de la configuración actual
log "💾 Creando backup de configuración..."
BACKUP_NAME="backup-$(date +%Y%m%d-%H%M%S)"
mkdir -p "$BACKUP_DIR/$BACKUP_NAME"
cp -r .env.production nginx/ "$BACKUP_DIR/$BACKUP_NAME/" 2>/dev/null || true

# 2. Backup de Redis
log "💾 Backup de base de datos Redis..."
if docker ps | grep -q tucentropdf-redis; then
    docker exec tucentropdf-redis redis-cli BGSAVE || warning "No se pudo hacer backup de Redis"
fi

# 3. Obtener cambios del repositorio
log "📥 Obteniendo cambios del repositorio..."
git fetch origin
CURRENT_COMMIT=$(git rev-parse HEAD)
LATEST_COMMIT=$(git rev-parse origin/main)

if [ "$CURRENT_COMMIT" = "$LATEST_COMMIT" ]; then
    log "✅ Ya estás en la última versión"
    exit 0
fi

log "📊 Cambios encontrados:"
git log --oneline $CURRENT_COMMIT..$LATEST_COMMIT

# 4. Actualizar código
log "🔄 Actualizando código..."
git reset --hard origin/main
git pull origin main

# 5. Verificar si hay cambios que requieren rebuild
NEEDS_REBUILD=false
if git diff $CURRENT_COMMIT..$LATEST_COMMIT --name-only | grep -E "(Dockerfile|go\.mod|go\.sum|cmd/|internal/|pkg/)"; then
    log "🔨 Cambios detectados en código - requiere rebuild"
    NEEDS_REBUILD=true
fi

# 6. Verificar cambios en configuración
if git diff $CURRENT_COMMIT..$LATEST_COMMIT --name-only | grep -E "(docker-compose|nginx/)"; then
    log "⚙️ Cambios detectados en configuración"
    
    # Backup de configuraciones existentes
    cp docker-compose.prod.yml "$BACKUP_DIR/$BACKUP_NAME/docker-compose.prod.yml.old" 2>/dev/null || true
    
    warning "Revisa manualmente los cambios en docker-compose.prod.yml y nginx/"
fi

# 7. Rebuild si es necesario
if [ "$NEEDS_REBUILD" = true ]; then
    log "🔨 Reconstruyendo imagen de la aplicación..."
    docker-compose -f docker-compose.prod.yml build --no-cache tucentropdf-engine
fi

# 8. Rolling update de los servicios
log "♻️ Actualizando servicios..."

# Actualizar aplicación con zero-downtime
log "🔄 Actualizando aplicación principal..."
docker-compose -f docker-compose.prod.yml up -d --force-recreate tucentropdf-engine

# Esperar a que la aplicación esté lista
log "⏳ Esperando a que la aplicación esté lista..."
sleep 20

# Verificar que la aplicación está funcionando
RETRIES=0
MAX_RETRIES=12
while [ $RETRIES -lt $MAX_RETRIES ]; do
    if curl -f http://localhost:8080/health > /dev/null 2>&1; then
        log "✅ Aplicación funcionando correctamente"
        break
    else
        RETRIES=$((RETRIES + 1))
        log "⏳ Esperando aplicación ($RETRIES/$MAX_RETRIES)..."
        sleep 10
    fi
done

if [ $RETRIES -eq $MAX_RETRIES ]; then
    error "❌ La aplicación no responde después de la actualización"
fi

# 9. Actualizar nginx si es necesario
if docker ps | grep -q tucentropdf-nginx; then
    log "🌐 Actualizando proxy nginx..."
    docker-compose -f docker-compose.prod.yml up -d nginx
fi

# 10. Actualizar Redis si hay cambios
if git diff $CURRENT_COMMIT..$LATEST_COMMIT --name-only | grep -q redis; then
    log "📊 Actualizando Redis..."
    docker-compose -f docker-compose.prod.yml up -d redis
fi

# 11. Verificación post-update
log "🔍 Verificando estado post-actualización..."

# Estado de contenedores
docker-compose -f docker-compose.prod.yml ps

# Test de endpoints críticos
ENDPOINTS=("/health" "/ready" "/api/v2/office/formats")
for endpoint in "${ENDPOINTS[@]}"; do
    if curl -f "http://localhost:8080$endpoint" > /dev/null 2>&1; then
        log "✅ Endpoint $endpoint: OK"
    else
        warning "⚠️ Endpoint $endpoint: No responde"
    fi
done

# 12. Limpiar recursos antiguos
log "🧹 Limpiando recursos antiguos..."
docker image prune -f
docker volume prune -f

# 13. Mostrar información de la actualización
log "📊 Información de la actualización:"
echo "=================================="
echo "🔄 Versión anterior: $CURRENT_COMMIT"
echo "🔄 Versión actual: $LATEST_COMMIT"
echo "📦 Backup guardado en: $BACKUP_DIR/$BACKUP_NAME"

# 14. Logs recientes
log "📋 Logs recientes de la aplicación:"
docker-compose -f docker-compose.prod.yml logs --tail=20 tucentropdf-engine

log "🎉 ¡Actualización completada exitosamente!"

# Opcional: Notificar a servicios externos
if [ -n "$WEBHOOK_URL" ]; then
    curl -X POST "$WEBHOOK_URL" \
         -H "Content-Type: application/json" \
         -d "{\"text\":\"TuCentroPDF Engine V2 actualizado exitosamente de $CURRENT_COMMIT a $LATEST_COMMIT\"}" \
         > /dev/null 2>&1 || true
fi