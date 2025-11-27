#!/bin/bash
# deploy.sh - Script de deployment inicial para TuCentroPDF Engine V2

set -e

# Variables de configuración
REPO_URL="https://github.com/tu-usuario/tucentropdf-engine.git"
APP_DIR="/opt/tucentropdf"
BACKUP_DIR="/opt/tucentropdf/backups"
LOG_FILE="/var/log/tucentropdf-deploy.log"

# Colores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Función de logging
log() {
    echo -e "${GREEN}[$(date '+%Y-%m-%d %H:%M:%S')]${NC} $1" | tee -a "$LOG_FILE"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1" | tee -a "$LOG_FILE"
    exit 1
}

warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1" | tee -a "$LOG_FILE"
}

# Verificar si es root
if [[ $EUID -eq 0 ]]; then
    error "Este script no debe ejecutarse como root. Usa el usuario tucentropdf."
fi

# Función para verificar dependencias
check_dependencies() {
    log "🔍 Verificando dependencias..."
    
    # Docker
    if ! command -v docker &> /dev/null; then
        error "Docker no está instalado"
    fi
    
    # Docker Compose
    if ! command -v docker-compose &> /dev/null; then
        error "Docker Compose no está instalado"
    fi
    
    # Git
    if ! command -v git &> /dev/null; then
        error "Git no está instalado"
    fi
    
    # Nginx
    if ! command -v nginx &> /dev/null; then
        warning "Nginx no está instalado - se instalará con Docker"
    fi
    
    log "✅ Dependencias verificadas"
}

# Función para crear directorios
setup_directories() {
    log "📁 Configurando directorios..."
    
    sudo mkdir -p "$APP_DIR" "$BACKUP_DIR" /opt/tucentropdf/data/{redis,prometheus,grafana}
    sudo chown -R tucentropdf:tucentropdf /opt/tucentropdf
    
    mkdir -p "$APP_DIR/tucentropdf-engine/engine_v2/"{uploads,temp,logs,nginx/{sites-available,ssl,logs}}
    mkdir -p "$APP_DIR/tucentropdf-engine/engine_v2/monitoring"
    
    log "✅ Directorios creados"
}

# Función para clonar/actualizar repositorio
setup_code() {
    log "📥 Configurando código fuente..."
    
    cd "$APP_DIR"
    
    if [ -d "tucentropdf-engine" ]; then
        log "📦 Creando backup del código actual..."
        cp -r tucentropdf-engine "$BACKUP_DIR/code-backup-$(date +%Y%m%d-%H%M%S)"
        
        cd tucentropdf-engine
        log "🔄 Actualizando repositorio..."
        git fetch origin
        git reset --hard origin/main
        git pull origin main
    else
        log "📥 Clonando repositorio..."
        git clone "$REPO_URL"
        cd tucentropdf-engine
    fi
    
    cd engine_v2
    log "✅ Código fuente configurado"
}

# Función para verificar archivos de configuración
check_config() {
    log "⚙️ Verificando configuración..."
    
    if [ ! -f ".env.production" ]; then
        log "📝 Creando archivo .env.production desde ejemplo..."
        cp .env.example .env.production
        
        warning "IMPORTANTE: Debes editar .env.production con tus valores reales:"
        echo "  - OPENAI_API_KEY"
        echo "  - REDIS_PASSWORD"
        echo "  - JWT_SECRET"
        echo "  - CORS_ORIGINS"
        
        read -p "¿Has configurado .env.production? (y/N): " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Yy]$ ]]; then
            error "Configura .env.production antes de continuar"
        fi
    fi
    
    log "✅ Configuración verificada"
}

# Función para configurar nginx
setup_nginx_config() {
    log "🌐 Configurando Nginx..."
    
    # Crear configuración de nginx si no existe
    if [ ! -f "nginx/nginx.conf" ]; then
        cat > nginx/nginx.conf << 'EOF'
user nginx;
worker_processes auto;
error_log /var/log/nginx/error.log warn;
pid /var/run/nginx.pid;

events {
    worker_connections 1024;
    use epoll;
    multi_accept on;
}

http {
    include /etc/nginx/mime.types;
    default_type application/octet-stream;

    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" "$http_x_forwarded_for"';
    access_log /var/log/nginx/access.log main;

    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    types_hash_max_size 2048;
    client_max_body_size 100M;

    gzip on;
    gzip_vary on;
    gzip_min_length 10240;
    gzip_proxied any;
    gzip_types text/plain text/css text/xml text/javascript application/json application/javascript application/xml+rss application/atom+xml image/svg+xml;

    limit_req_zone $binary_remote_addr zone=api:10m rate=10r/s;
    limit_req_zone $binary_remote_addr zone=upload:10m rate=1r/s;

    include /etc/nginx/sites-available/*;
}
EOF
    fi
    
    # Crear configuración del sitio si no existe
    read -p "Ingresa tu dominio para la API (ej: api.tucentropdf.com): " API_DOMAIN
    
    if [ ! -f "nginx/sites-available/tucentropdf.conf" ]; then
        cat > nginx/sites-available/tucentropdf.conf << EOF
upstream tucentropdf_backend {
    server tucentropdf-engine:8080;
    keepalive 32;
}

server {
    listen 80;
    server_name $API_DOMAIN;
    
    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
    }
    
    location / {
        return 301 https://\$server_name\$request_uri;
    }
}

server {
    listen 443 ssl http2;
    server_name $API_DOMAIN;

    # SSL será configurado después de obtener certificados
    # ssl_certificate /etc/letsencrypt/live/$API_DOMAIN/fullchain.pem;
    # ssl_certificate_key /etc/letsencrypt/live/$API_DOMAIN/privkey.pem;

    # Security Headers
    add_header X-Frame-Options DENY;
    add_header X-Content-Type-Options nosniff;
    add_header X-XSS-Protection "1; mode=block";
    add_header Referrer-Policy "strict-origin-when-cross-origin";

    # Rate limiting
    limit_req zone=api burst=20 nodelay;

    # Proxy settings
    proxy_http_version 1.1;
    proxy_cache_bypass \$http_upgrade;
    proxy_set_header Upgrade \$http_upgrade;
    proxy_set_header Connection 'upgrade';
    proxy_set_header Host \$host;
    proxy_set_header X-Real-IP \$remote_addr;
    proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto \$scheme;

    location /api/ {
        proxy_pass http://tucentropdf_backend;
        proxy_read_timeout 300s;
        proxy_connect_timeout 75s;
    }

    location ~ ^/api/v2/(pdf|office|ocr)/ {
        limit_req zone=upload burst=5 nodelay;
        proxy_pass http://tucentropdf_backend;
        proxy_read_timeout 600s;
        client_max_body_size 100M;
    }

    location ~ ^/(health|ready|metrics)\$ {
        proxy_pass http://tucentropdf_backend;
        access_log off;
    }
}
EOF
    fi
    
    log "✅ Configuración de Nginx creada"
}

# Función para construir y lanzar servicios
deploy_services() {
    log "🔨 Construyendo aplicación..."
    
    # Construir imagen
    docker-compose -f docker-compose.prod.yml build --no-cache tucentropdf-engine
    
    log "🚀 Iniciando servicios..."
    
    # Iniciar servicios core primero
    docker-compose -f docker-compose.prod.yml up -d redis tucentropdf-engine
    
    # Esperar a que los servicios estén listos
    log "⏳ Esperando servicios core..."
    sleep 30
    
    # Verificar salud
    local retries=0
    local max_retries=10
    
    while [ $retries -lt $max_retries ]; do
        if curl -f http://localhost:8080/health > /dev/null 2>&1; then
            log "✅ Aplicación respondiendo correctamente"
            break
        else
            retries=$((retries + 1))
            log "⏳ Reintentando verificación de salud ($retries/$max_retries)..."
            sleep 15
        fi
    done
    
    if [ $retries -eq $max_retries ]; then
        error "La aplicación no responde después de $max_retries intentos"
    fi
    
    # Iniciar nginx si todo está bien
    docker-compose -f docker-compose.prod.yml up -d nginx
    
    log "✅ Todos los servicios iniciados"
}

# Función para configurar SSL
setup_ssl() {
    log "🔒 Configurando SSL..."
    
    read -p "¿Quieres configurar SSL con Let's Encrypt ahora? (y/N): " -n 1 -r
    echo
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        # Detener nginx temporalmente
        docker-compose -f docker-compose.prod.yml stop nginx
        
        # Obtener certificados
        sudo certbot certonly --standalone -d "$API_DOMAIN"
        
        # Habilitar configuración SSL en nginx
        sed -i 's/# ssl_certificate/ssl_certificate/' nginx/sites-available/tucentropdf.conf
        sed -i 's/# ssl_certificate_key/ssl_certificate_key/' nginx/sites-available/tucentropdf.conf
        
        # Reiniciar nginx
        docker-compose -f docker-compose.prod.yml up -d nginx
        
        log "✅ SSL configurado"
    else
        log "⚠️ SSL no configurado. Recuerda configurarlo manualmente."
    fi
}

# Función para configurar monitoreo (opcional)
setup_monitoring() {
    log "📊 Configurando monitoreo..."
    
    read -p "¿Quieres habilitar monitoreo con Prometheus/Grafana? (y/N): " -n 1 -r
    echo
    
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        # Crear configuración de Prometheus
        cat > monitoring/prometheus.yml << 'EOF'
global:
  scrape_interval: 15s
  evaluation_interval: 15s

scrape_configs:
  - job_name: 'tucentropdf-engine'
    static_configs:
      - targets: ['tucentropdf-engine:8080']
    metrics_path: '/metrics'
    scrape_interval: 30s

  - job_name: 'redis'
    static_configs:
      - targets: ['redis:6379']
    scrape_interval: 30s
EOF
        
        # Iniciar servicios de monitoreo
        docker-compose -f docker-compose.prod.yml --profile monitoring up -d
        
        log "✅ Monitoreo configurado"
        log "📊 Prometheus: http://localhost:9090"
        log "📈 Grafana: http://localhost:3000 (admin/admin123)"
    else
        log "⚠️ Monitoreo no habilitado"
    fi
}

# Función para verificar deployment
verify_deployment() {
    log "🔍 Verificando deployment..."
    
    # Estado de contenedores
    docker-compose -f docker-compose.prod.yml ps
    
    # Test de endpoints
    local health_status
    health_status=$(curl -s http://localhost:8080/health | grep -o '"status":"[^"]*"' | cut -d'"' -f4)
    
    if [ "$health_status" = "ok" ]; then
        log "✅ Health check: OK"
    else
        warning "⚠️ Health check: $health_status"
    fi
    
    # Test de ready endpoint
    local ready_status
    ready_status=$(curl -s http://localhost:8080/ready | grep -o '"status":"[^"]*"' | cut -d'"' -f4)
    
    if [ "$ready_status" = "ready" ]; then
        log "✅ Ready check: OK"
    else
        warning "⚠️ Ready check: $ready_status"
    fi
    
    log "✅ Verificación completada"
}

# Función para mostrar información final
show_final_info() {
    log "🎉 ¡Deployment completado exitosamente!"
    echo
    echo "📊 INFORMACIÓN DEL DEPLOYMENT:"
    echo "=================================="
    echo "🌐 API URL: http://localhost:8080"
    if [ -n "$API_DOMAIN" ]; then
        echo "🌐 Dominio: https://$API_DOMAIN"
    fi
    echo "📋 Health: http://localhost:8080/health"
    echo "📋 Ready: http://localhost:8080/ready"
    echo "📊 Metrics: http://localhost:8080/metrics"
    echo "📁 Logs: $APP_DIR/tucentropdf-engine/engine_v2/logs"
    echo "💾 Uploads: $APP_DIR/tucentropdf-engine/engine_v2/uploads"
    echo
    echo "🔧 COMANDOS ÚTILES:"
    echo "==================="
    echo "Ver logs: docker-compose -f docker-compose.prod.yml logs -f"
    echo "Reiniciar: docker-compose -f docker-compose.prod.yml restart"
    echo "Actualizar: ./scripts/update.sh"
    echo "Monitoreo: ./scripts/monitor.sh"
    echo "Backup: ./scripts/backup.sh"
    echo
    echo "⚠️ PRÓXIMOS PASOS:"
    echo "=================="
    echo "1. Configura tu DNS para que $API_DOMAIN apunte a este servidor"
    echo "2. Configura SSL con: sudo certbot --nginx -d $API_DOMAIN"
    echo "3. Configura backups automáticos"
    echo "4. Configura monitoreo de logs"
    echo "5. Prueba la integración con tu aplicación web"
}

# Función principal
main() {
    log "🚀 Iniciando deployment de TuCentroPDF Engine V2..."
    
    check_dependencies
    setup_directories
    setup_code
    check_config
    setup_nginx_config
    deploy_services
    setup_ssl
    setup_monitoring
    verify_deployment
    show_final_info
    
    log "✅ Deployment completado"
}

# Ejecutar función principal
main "$@"