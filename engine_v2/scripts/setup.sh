#!/bin/bash
# setup.sh - Script de configuración inicial para VPS

set -e

# Colores
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'

log() {
    echo -e "${GREEN}[$(date '+%Y-%m-%d %H:%M:%S')]${NC} $1"
}

warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

title() {
    echo -e "\n${CYAN}=== $1 ===${NC}\n"
}

# Verificar que se ejecuta como root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        error "Este script debe ejecutarse como root (sudo ./setup.sh)"
    fi
}

# Actualizar sistema
update_system() {
    title "Actualizando sistema"
    
    apt update
    apt upgrade -y
    
    log "✅ Sistema actualizado"
}

# Instalar dependencias
install_dependencies() {
    title "Instalando dependencias del sistema"
    
    local packages=(
        "curl"
        "wget"
        "git"
        "unzip"
        "apt-transport-https"
        "ca-certificates"
        "gnupg"
        "lsb-release"
        "software-properties-common"
        "ufw"
        "fail2ban"
        "htop"
        "ncdu"
        "tree"
        "jq"
        "nginx"
        "certbot"
        "python3-certbot-nginx"
        "redis-tools"
    )
    
    for package in "${packages[@]}"; do
        log "Instalando $package..."
        apt install -y "$package"
    done
    
    log "✅ Dependencias del sistema instaladas"
}

# Instalar Docker
install_docker() {
    title "Instalando Docker"
    
    if command -v docker &> /dev/null; then
        log "Docker ya está instalado"
        return 0
    fi
    
    # Agregar repositorio de Docker
    curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /usr/share/keyrings/docker-archive-keyring.gpg
    
    echo "deb [arch=amd64 signed-by=/usr/share/keyrings/docker-archive-keyring.gpg] https://download.docker.com/linux/ubuntu $(lsb_release -cs) stable" > /etc/apt/sources.list.d/docker.list
    
    apt update
    apt install -y docker-ce docker-ce-cli containerd.io
    
    # Instalar Docker Compose
    curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
    chmod +x /usr/local/bin/docker-compose
    
    # Iniciar Docker
    systemctl enable docker
    systemctl start docker
    
    log "✅ Docker instalado correctamente"
    docker --version
    docker-compose --version
}

# Crear usuario de aplicación
create_app_user() {
    title "Configurando usuario de aplicación"
    
    if id "tucentropdf" &>/dev/null; then
        log "Usuario tucentropdf ya existe"
    else
        useradd -m -s /bin/bash tucentropdf
        usermod -aG docker tucentropdf
        log "✅ Usuario tucentropdf creado"
    fi
    
    # Crear estructura de directorios
    local dirs=(
        "/opt/tucentropdf"
        "/opt/backups/tucentropdf"
        "/var/log/tucentropdf"
    )
    
    for dir in "${dirs[@]}"; do
        mkdir -p "$dir"
        chown tucentropdf:tucentropdf "$dir"
    done
    
    log "✅ Estructura de directorios creada"
}

# Configurar firewall
setup_firewall() {
    title "Configurando firewall"
    
    # Resetear UFW
    ufw --force reset
    
    # Políticas por defecto
    ufw default deny incoming
    ufw default allow outgoing
    
    # Puertos necesarios
    ufw allow ssh
    ufw allow 80/tcp
    ufw allow 443/tcp
    
    # Habilitar UFW
    ufw --force enable
    
    log "✅ Firewall configurado"
    ufw status
}

# Configurar fail2ban
setup_fail2ban() {
    title "Configurando fail2ban"
    
    cat > /etc/fail2ban/jail.local << 'EOF'
[DEFAULT]
bantime = 3600
findtime = 600
maxretry = 5
backend = systemd

[sshd]
enabled = true
port = ssh
logpath = %(sshd_log)s
backend = %(sshd_backend)s

[nginx-http-auth]
enabled = true

[nginx-limit-req]
enabled = true
filter = nginx-limit-req
logpath = /var/log/nginx/error.log

[nginx-botsearch]
enabled = true
filter = nginx-botsearch
logpath = /var/log/nginx/access.log
EOF
    
    systemctl enable fail2ban
    systemctl restart fail2ban
    
    log "✅ Fail2ban configurado"
}

# Configurar Nginx básico
setup_nginx() {
    title "Configurando Nginx"
    
    # Backup de configuración original
    cp /etc/nginx/nginx.conf /etc/nginx/nginx.conf.backup
    
    # Configuración base de Nginx
    cat > /etc/nginx/nginx.conf << 'EOF'
user www-data;
worker_processes auto;
pid /run/nginx.pid;
include /etc/nginx/modules-enabled/*.conf;

events {
    worker_connections 1024;
    use epoll;
    multi_accept on;
}

http {
    # Basic Settings
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    types_hash_max_size 2048;
    server_tokens off;
    
    # MIME
    include /etc/nginx/mime.types;
    default_type application/octet-stream;
    
    # SSL Settings
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_prefer_server_ciphers off;
    
    # Logging Settings
    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" "$http_x_forwarded_for"';
    
    access_log /var/log/nginx/access.log main;
    error_log /var/log/nginx/error.log;
    
    # Gzip Settings
    gzip on;
    gzip_vary on;
    gzip_proxied any;
    gzip_comp_level 6;
    gzip_types
        text/plain
        text/css
        text/xml
        text/javascript
        application/json
        application/javascript
        application/xml+rss
        application/atom+xml
        image/svg+xml;
    
    # Rate Limiting
    limit_req_zone $binary_remote_addr zone=api:10m rate=10r/s;
    limit_req_zone $binary_remote_addr zone=upload:10m rate=2r/s;
    
    # Include configs
    include /etc/nginx/conf.d/*.conf;
    include /etc/nginx/sites-enabled/*;
}
EOF
    
    # Configuración por defecto
    cat > /etc/nginx/sites-available/default << 'EOF'
server {
    listen 80 default_server;
    listen [::]:80 default_server;
    
    server_name _;
    
    # Security headers
    add_header X-Frame-Options DENY;
    add_header X-Content-Type-Options nosniff;
    add_header X-XSS-Protection "1; mode=block";
    
    # Redirect to HTTPS
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2 default_server;
    listen [::]:443 ssl http2 default_server;
    
    server_name _;
    
    # Placeholder SSL (será reemplazado por Certbot)
    ssl_certificate /etc/ssl/certs/ssl-cert-snakeoil.pem;
    ssl_certificate_key /etc/ssl/private/ssl-cert-snakeoil.key;
    
    root /var/www/html;
    index index.html index.htm index.nginx-debian.html;
    
    location / {
        try_files $uri $uri/ =404;
    }
}
EOF
    
    # Crear página por defecto
    cat > /var/www/html/index.html << 'EOF'
<!DOCTYPE html>
<html>
<head>
    <title>TuCentroPDF Engine V2</title>
    <style>
        body { font-family: Arial, sans-serif; text-align: center; margin-top: 50px; }
        .container { max-width: 600px; margin: 0 auto; }
        .status { color: #28a745; font-weight: bold; }
    </style>
</head>
<body>
    <div class="container">
        <h1>TuCentroPDF Engine V2</h1>
        <p class="status">Servidor configurado correctamente</p>
        <p>El motor de PDFs está en proceso de configuración.</p>
        <p>Una vez completada la instalación, este sitio mostrará la interfaz de la aplicación.</p>
    </div>
</body>
</html>
EOF
    
    # Verificar configuración
    nginx -t
    
    systemctl enable nginx
    systemctl restart nginx
    
    log "✅ Nginx configurado"
}

# Configurar herramientas de monitoreo
setup_monitoring() {
    title "Configurando herramientas de monitoreo"
    
    # Script de monitoreo básico
    cat > /usr/local/bin/tucentropdf-status << 'EOF'
#!/bin/bash

echo "=== Estado TuCentroPDF Engine V2 ==="
echo "Fecha: $(date)"
echo ""

echo "=== Sistema ==="
echo "Uptime: $(uptime)"
echo "Memoria: $(free -h | head -2)"
echo "Disco: $(df -h / | tail -1)"
echo ""

echo "=== Contenedores Docker ==="
if command -v docker &> /dev/null; then
    docker ps --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
else
    echo "Docker no instalado"
fi
echo ""

echo "=== Nginx ==="
if systemctl is-active --quiet nginx; then
    echo "✅ Nginx activo"
else
    echo "❌ Nginx inactivo"
fi

echo "=== Puertos ==="
ss -tuln | grep -E "(80|443|8080|6379)" | head -10
EOF
    
    chmod +x /usr/local/bin/tucentropdf-status
    
    # Crear alias útiles
    cat > /etc/profile.d/tucentropdf.sh << 'EOF'
alias tucentro-status='tucentropdf-status'
alias tucentro-logs='tail -f /var/log/tucentropdf/*.log'
alias tucentro-nginx='tail -f /var/log/nginx/error.log'
alias tucentro-docker='docker ps -a'
EOF
    
    log "✅ Herramientas de monitoreo configuradas"
}

# Configurar logs centralizados
setup_logging() {
    title "Configurando sistema de logs"
    
    # Configuración de logrotate para logs de la aplicación
    cat > /etc/logrotate.d/tucentropdf << 'EOF'
/var/log/tucentropdf/*.log {
    daily
    missingok
    rotate 30
    compress
    delaycompress
    notifempty
    copytruncate
}
EOF
    
    # Configurar rsyslog para logs de aplicación
    cat > /etc/rsyslog.d/50-tucentropdf.conf << 'EOF'
# TuCentroPDF Engine logs
:programname,isequal,"tucentropdf" /var/log/tucentropdf/application.log
:programname,isequal,"tucentropdf" stop
EOF
    
    systemctl restart rsyslog
    
    log "✅ Sistema de logs configurado"
}

# Configurar backups automáticos
setup_automated_backups() {
    title "Configurando backups automáticos"
    
    # Configurar crontab para backups
    cat > /etc/cron.d/tucentropdf-backup << 'EOF'
# TuCentroPDF Engine V2 - Backups automáticos
# Backup diario a las 2:00 AM
0 2 * * * tucentropdf /opt/tucentropdf/tucentropdf-engine/engine_v2/scripts/backup.sh >/dev/null 2>&1

# Backup semanal completo los domingos a las 1:00 AM
0 1 * * 0 tucentropdf /opt/tucentropdf/tucentropdf-engine/engine_v2/scripts/backup.sh --full >/dev/null 2>&1
EOF
    
    # Configuración de backup por defecto
    cat > /opt/tucentropdf/.backup-config << 'EOF'
# Configuración de backups
BACKUP_RETENTION_DAYS=7
WEBHOOK_URL=""
EMAIL_NOTIFY=""
FULL_BACKUP_RETENTION=4
EOF
    
    chown tucentropdf:tucentropdf /opt/tucentropdf/.backup-config
    
    log "✅ Backups automáticos configurados"
}

# Mostrar resumen final
show_final_summary() {
    title "Configuración completada"
    
    local server_ip
    server_ip=$(curl -s ifconfig.me 2>/dev/null || echo "N/A")
    
    echo "🎉 ¡Configuración inicial completada!"
    echo ""
    echo "📋 Resumen de configuración:"
    echo "============================="
    echo "🖥️  IP del servidor: $server_ip"
    echo "👤 Usuario de aplicación: tucentropdf"
    echo "📁 Directorio de aplicación: /opt/tucentropdf"
    echo "🗂️  Directorio de backups: /opt/backups/tucentropdf"
    echo "📋 Logs de sistema: /var/log/tucentropdf"
    echo "🐳 Docker: $(docker --version | cut -d' ' -f3)"
    echo "🐙 Docker Compose: $(docker-compose --version | cut -d' ' -f3)"
    echo "🌐 Nginx: Configurado y ejecutándose"
    echo "🔒 Firewall: UFW habilitado (puertos 22, 80, 443)"
    echo "🛡️  Fail2ban: Configurado para SSH y Nginx"
    echo ""
    echo "🔧 Próximos pasos:"
    echo "=================="
    echo "1. Cambiar al usuario tucentropdf:"
    echo "   sudo su - tucentropdf"
    echo ""
    echo "2. Clonar el repositorio:"
    echo "   git clone <tu-repositorio> /opt/tucentropdf/tucentropdf-engine"
    echo ""
    echo "3. Configurar el dominio y SSL:"
    echo "   # Editar /etc/nginx/sites-available/default"
    echo "   # Ejecutar: certbot --nginx -d tu-dominio.com"
    echo ""
    echo "4. Ejecutar el script de despliegue:"
    echo "   cd /opt/tucentropdf/tucentropdf-engine/engine_v2"
    echo "   ./scripts/deploy.sh"
    echo ""
    echo "📚 Comandos útiles:"
    echo "=================="
    echo "• Estado del sistema: tucentropdf-status"
    echo "• Ver logs: tail -f /var/log/tucentropdf/*.log"
    echo "• Estado de Docker: docker ps"
    echo "• Estado de Nginx: systemctl status nginx"
    echo ""
    echo "⚠️  Recordatorios importantes:"
    echo "============================"
    echo "• Configura un dominio antes de usar SSL"
    echo "• Cambia las contraseñas por defecto"
    echo "• Configura las notificaciones de backup"
    echo "• Revisa los logs regularmente"
    echo ""
}

# Función principal
main() {
    log "🚀 Iniciando configuración del servidor VPS para TuCentroPDF Engine V2"
    
    check_root
    update_system
    install_dependencies
    install_docker
    create_app_user
    setup_firewall
    setup_fail2ban
    setup_nginx
    setup_monitoring
    setup_logging
    setup_automated_backups
    show_final_summary
    
    log "✅ Configuración inicial completada exitosamente"
}

# Manejo de errores
trap 'error "Setup falló en línea $LINENO"' ERR

# Ejecutar función principal
main "$@"