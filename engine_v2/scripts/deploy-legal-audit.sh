#!/bin/bash

# =============================================================================
# SCRIPT DE DESPLIEGUE COMPLETO - TuCentroPDF Engine V2
# SISTEMA DE AUDITORÍA LEGAL Y EVIDENCIA DIGITAL
# =============================================================================

set -euo pipefail

# Colores para output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Variables de configuración
PROJECT_DIR="/opt/tucentropdf-engine-v2"
SERVICE_NAME="tucentropdf-engine-v2"
DB_NAME="tucentropdf_legal"
DB_USER="tucentropdf_audit"
BACKUP_DIR="/var/backups/tucentropdf"
LOG_DIR="/var/log/tucentropdf"

echo -e "${BLUE}🚀 Iniciando despliegue de TuCentroPDF Engine V2 con Auditoría Legal${NC}"

# =============================================================================
# 1. VERIFICAR PRERREQUISITOS
# =============================================================================

check_prerequisites() {
    echo -e "${BLUE}📋 Verificando prerrequisitos...${NC}"
    
    # Verificar que se ejecuta como root
    if [ "$EUID" -ne 0 ]; then
        echo -e "${RED}❌ Este script debe ejecutarse como root${NC}"
        exit 1
    fi
    
    # Verificar PostgreSQL
    if ! command -v psql &> /dev/null; then
        echo -e "${YELLOW}📦 Instalando PostgreSQL...${NC}"
        apt update
        apt install -y postgresql postgresql-contrib
    fi
    
    # Verificar Go
    if ! command -v go &> /dev/null; then
        echo -e "${YELLOW}📦 Instalando Go...${NC}"
        wget https://golang.org/dl/go1.21.0.linux-amd64.tar.gz
        tar -C /usr/local -xzf go1.21.0.linux-amd64.tar.gz
        echo 'export PATH=$PATH:/usr/local/go/bin' >> /etc/profile
        source /etc/profile
    fi
    
    # Verificar Git
    if ! command -v git &> /dev/null; then
        echo -e "${YELLOW}📦 Instalando Git...${NC}"
        apt install -y git
    fi
    
    echo -e "${GREEN}✅ Prerrequisitos verificados${NC}"
}

# =============================================================================
# 2. CONFIGURAR BASE DE DATOS POSTGRESQL
# =============================================================================

setup_database() {
    echo -e "${BLUE}🗃️ Configurando base de datos PostgreSQL...${NC}"
    
    # Iniciar servicio PostgreSQL
    systemctl start postgresql
    systemctl enable postgresql
    
    # Crear base de datos y usuario para auditoría legal
    sudo -u postgres psql << EOF
-- Crear base de datos
DROP DATABASE IF EXISTS ${DB_NAME};
CREATE DATABASE ${DB_NAME} WITH ENCODING 'UTF8' LOCALE 'en_US.UTF-8' TEMPLATE template0;

-- Crear usuario específico para auditoría legal
DROP ROLE IF EXISTS ${DB_USER};
CREATE ROLE ${DB_USER} WITH LOGIN PASSWORD 'secure_audit_password_2024';

-- Otorgar permisos
GRANT CONNECT ON DATABASE ${DB_NAME} TO ${DB_USER};
GRANT USAGE ON SCHEMA public TO ${DB_USER};
GRANT CREATE ON SCHEMA public TO ${DB_USER};

-- Configurar extensiones necesarias
\c ${DB_NAME}
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

EOF

    echo -e "${GREEN}✅ Base de datos configurada${NC}"
}

# =============================================================================
# 3. CONFIGURAR DIRECTORIOS DEL SISTEMA
# =============================================================================

setup_directories() {
    echo -e "${BLUE}📁 Configurando directorios del sistema...${NC}"
    
    # Crear directorios principales
    mkdir -p ${PROJECT_DIR}
    mkdir -p ${LOG_DIR}/legal-audit
    mkdir -p /var/tucentropdf/exports/legal
    mkdir -p /var/tucentropdf/archives/legal
    mkdir -p ${BACKUP_DIR}
    
    # Crear usuario del sistema
    if ! id "tucentropdf" &>/dev/null; then
        useradd -r -s /bin/false -d ${PROJECT_DIR} tucentropdf
    fi
    
    # Configurar permisos
    chown -R tucentropdf:tucentropdf ${PROJECT_DIR}
    chown -R tucentropdf:tucentropdf /var/tucentropdf
    chown -R tucentropdf:tucentropdf ${LOG_DIR}
    chown -R tucentropdf:tucentropdf ${BACKUP_DIR}
    
    chmod 750 ${PROJECT_DIR}
    chmod 755 /var/tucentropdf
    chmod 750 /var/tucentropdf/exports
    chmod 750 /var/tucentropdf/archives
    chmod 755 ${LOG_DIR}
    
    echo -e "${GREEN}✅ Directorios configurados${NC}"
}

# =============================================================================
# 4. GENERAR CLAVES DE SEGURIDAD
# =============================================================================

generate_security_keys() {
    echo -e "${BLUE}🔐 Generando claves de seguridad...${NC}"
    
    # Generar claves criptográficas seguras
    ENCRYPTION_KEY=$(openssl rand -base64 32)
    ENCRYPTION_SALT=$(openssl rand -base64 64)
    HMAC_SECRET=$(openssl rand -base64 32)
    JWT_SECRET=$(openssl rand -base64 32)
    
    # Crear archivo de variables de entorno
    cat > ${PROJECT_DIR}/.env << EOF
# =============================================================================
# CONFIGURACIÓN PRODUCCIÓN - TuCentroPDF Engine V2 - Auditoría Legal
# =============================================================================

# Base de datos PostgreSQL
DB_HOST=localhost
DB_PORT=5432
DB_NAME=${DB_NAME}
DB_USER=${DB_USER}
DB_PASSWORD=secure_audit_password_2024
DB_SSLMODE=require

# Configuración del servidor
PORT=8080
ENVIRONMENT=production
GIN_MODE=release

# Cifrado y seguridad criptográfica
LEGAL_AUDIT_ENCRYPTION_KEY=${ENCRYPTION_KEY}
LEGAL_AUDIT_ENCRYPTION_SALT=${ENCRYPTION_SALT}
LEGAL_AUDIT_HMAC_SECRET=${HMAC_SECRET}
LEGAL_AUDIT_PBKDF2_ITERATIONS=100000

# Autenticación JWT para administradores
JWT_SECRET_KEY=${JWT_SECRET}
JWT_ADMIN_TOKEN_DURATION=8
JWT_SIGNING_ALGORITHM=HS256
JWT_ISSUER=tucentropdf-engine-v2
JWT_ADMIN_AUDIENCE=tucentropdf-legal-audit

# Configuración de auditoría legal
LEGAL_AUDIT_RETENTION_DAYS=1095
LEGAL_AUDIT_EXPORT_DIR=/var/tucentropdf/exports/legal
LEGAL_AUDIT_ARCHIVE_DIR=/var/tucentropdf/archives/legal
LEGAL_AUDIT_COMPRESSION_LEVEL=6
LEGAL_AUDIT_ENCRYPT_ARCHIVES=true

# Detección de abuso y seguridad
LEGAL_AUDIT_ABUSE_DETECTION=true
LEGAL_AUDIT_RATE_LIMIT_FREE=10
LEGAL_AUDIT_RATE_LIMIT_BASIC=30
LEGAL_AUDIT_RATE_LIMIT_PRO=60
LEGAL_AUDIT_RATE_LIMIT_ENTERPRISE=120
LEGAL_AUDIT_RATE_LIMIT_API=300

# Configuración de exportación
LEGAL_AUDIT_DOWNLOAD_TOKEN_EXPIRY=7
LEGAL_AUDIT_MAX_EXPORT_SIZE_MB=1000
LEGAL_AUDIT_EXPORT_FORMATS=json,csv,xml
LEGAL_AUDIT_EXPORT_SIGNATURES=true

# Archivado automático
LEGAL_AUDIT_AUTO_ARCHIVE=true
LEGAL_AUDIT_ARCHIVE_AFTER_DAYS=90
LEGAL_AUDIT_ARCHIVE_TIME=02:00
LEGAL_AUDIT_CLEANUP_EXPIRED_EXPORTS=true

# Alertas y notificaciones
LEGAL_AUDIT_ALERT_EMAIL=legal@tucentropdf.com
LEGAL_AUDIT_INTEGRITY_ALERTS=true
LEGAL_AUDIT_INTEGRITY_ALERT_THRESHOLD=95

# Logging y monitoreo
LEGAL_AUDIT_LOG_LEVEL=INFO
LEGAL_AUDIT_DEBUG_MODE=false
LEGAL_AUDIT_PERFORMANCE_METRICS=true
LEGAL_AUDIT_LOG_DIR=${LOG_DIR}/legal-audit

# Compliance
LEGAL_AUDIT_COMPLIANCE_STANDARD=GDPR,CCPA
LEGAL_AUDIT_LEGAL_REGION=EU,US
LEGAL_AUDIT_ENABLE_ANONYMIZATION=true
LEGAL_AUDIT_ANONYMIZED_RETENTION_DAYS=2555

EOF

    # Proteger archivo de configuración
    chmod 600 ${PROJECT_DIR}/.env
    chown tucentropdf:tucentropdf ${PROJECT_DIR}/.env
    
    echo -e "${GREEN}✅ Claves de seguridad generadas${NC}"
}

# =============================================================================
# 5. CLONAR Y COMPILAR CÓDIGO FUENTE
# =============================================================================

deploy_application() {
    echo -e "${BLUE}📥 Desplegando aplicación...${NC}"
    
    # Navegar al directorio del proyecto
    cd ${PROJECT_DIR}
    
    # Clonar código fuente (reemplazar con tu repositorio real)
    if [ ! -d "engine_v2" ]; then
        echo -e "${YELLOW}📁 Clonando código fuente...${NC}"
        # git clone https://github.com/tuuser/tucentropdf-engine.git .
        # Por ahora, copiar desde directorio actual
        cp -r /path/to/your/source/engine_v2 .
    fi
    
    cd engine_v2
    
    # Instalar dependencias Go
    echo -e "${YELLOW}📦 Instalando dependencias Go...${NC}"
    go mod download
    go mod tidy
    
    # Compilar aplicación
    echo -e "${YELLOW}🔨 Compilando aplicación...${NC}"
    go build -o bin/tucentropdf-engine-v2 cmd/server/main_with_legal_audit.go
    
    # Hacer ejecutable
    chmod +x bin/tucentropdf-engine-v2
    
    echo -e "${GREEN}✅ Aplicación desplegada${NC}"
}

# =============================================================================
# 6. EJECUTAR MIGRACIONES DE BASE DE DATOS
# =============================================================================

run_migrations() {
    echo -e "${BLUE}🔧 Ejecutando migraciones de base de datos...${NC}"
    
    cd ${PROJECT_DIR}/engine_v2
    
    # Ejecutar migración SQL de auditoría legal
    if [ -f "scripts/legal_audit_migration.sql" ]; then
        echo -e "${YELLOW}📊 Ejecutando migración de auditoría legal...${NC}"
        sudo -u postgres psql -d ${DB_NAME} -f scripts/legal_audit_migration.sql
    else
        echo -e "${RED}❌ Archivo de migración no encontrado${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}✅ Migraciones completadas${NC}"
}

# =============================================================================
# 7. CONFIGURAR SERVICIO SYSTEMD
# =============================================================================

setup_systemd_service() {
    echo -e "${BLUE}🔧 Configurando servicio systemd...${NC}"
    
    # Crear archivo de servicio
    cat > /etc/systemd/system/${SERVICE_NAME}.service << EOF
[Unit]
Description=TuCentroPDF Engine V2 with Legal Audit System
Documentation=https://tucentropdf.com/docs
After=network.target postgresql.service
Requires=postgresql.service

[Service]
Type=simple
User=tucentropdf
Group=tucentropdf
WorkingDirectory=${PROJECT_DIR}/engine_v2
ExecStart=${PROJECT_DIR}/engine_v2/bin/tucentropdf-engine-v2
EnvironmentFile=${PROJECT_DIR}/.env
Restart=always
RestartSec=10
StartLimitBurst=3
StartLimitInterval=60

# Configuración de seguridad
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=true
ProtectSystem=strict
ReadWritePaths=/var/tucentropdf /var/log/tucentropdf /tmp

# Configuración de recursos
LimitNOFILE=65536
LimitNPROC=4096

# Configuración de logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=tucentropdf-engine-v2

[Install]
WantedBy=multi-user.target
EOF

    # Recargar systemd y habilitar servicio
    systemctl daemon-reload
    systemctl enable ${SERVICE_NAME}
    
    echo -e "${GREEN}✅ Servicio systemd configurado${NC}"
}

# =============================================================================
# 8. CONFIGURAR NGINX (OPCIONAL)
# =============================================================================

setup_nginx() {
    echo -e "${BLUE}🌐 Configurando Nginx...${NC}"
    
    # Instalar Nginx si no está instalado
    if ! command -v nginx &> /dev/null; then
        apt install -y nginx
    fi
    
    # Crear configuración de sitio
    cat > /etc/nginx/sites-available/tucentropdf << EOF
server {
    listen 80;
    server_name localhost tucentropdf.local;
    
    # Redireccionar a HTTPS
    return 301 https://\$server_name\$request_uri;
}

server {
    listen 443 ssl http2;
    server_name localhost tucentropdf.local;
    
    # Certificados SSL (generar con Let's Encrypt en producción)
    ssl_certificate /etc/ssl/certs/tucentropdf.crt;
    ssl_certificate_key /etc/ssl/private/tucentropdf.key;
    
    # Configuración SSL segura
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-RSA-AES128-GCM-SHA256:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;
    
    # Configuración de seguridad
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Frame-Options DENY;
    add_header X-Content-Type-Options nosniff;
    add_header X-XSS-Protection "1; mode=block";
    
    # Límites de request
    client_max_body_size 500M;
    client_body_timeout 60s;
    client_header_timeout 60s;
    
    location / {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_cache_bypass \$http_upgrade;
        proxy_read_timeout 3600s;
        proxy_send_timeout 3600s;
    }
    
    # Endpoint específico para auditoría legal con autenticación extra
    location /api/v2/legal-audit {
        proxy_pass http://localhost:8080;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        
        # Logging extra para auditoría
        access_log /var/log/nginx/tucentropdf_legal_audit.log combined;
    }
}
EOF

    # Habilitar sitio
    ln -sf /etc/nginx/sites-available/tucentropdf /etc/nginx/sites-enabled/
    rm -f /etc/nginx/sites-enabled/default
    
    # Generar certificado SSL autofirmado para desarrollo
    openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
        -keyout /etc/ssl/private/tucentropdf.key \
        -out /etc/ssl/certs/tucentropdf.crt \
        -subj "/C=US/ST=State/L=City/O=TuCentroPDF/CN=localhost"
    
    # Test configuración y reload
    nginx -t
    systemctl enable nginx
    systemctl restart nginx
    
    echo -e "${GREEN}✅ Nginx configurado${NC}"
}

# =============================================================================
# 9. CONFIGURAR MONITOREO Y LOGS
# =============================================================================

setup_monitoring() {
    echo -e "${BLUE}📊 Configurando monitoreo...${NC}"
    
    # Configurar logrotate para logs de auditoría legal
    cat > /etc/logrotate.d/tucentropdf << EOF
${LOG_DIR}/legal-audit/*.log {
    daily
    missingok
    rotate 365
    compress
    delaycompress
    notifempty
    create 0640 tucentropdf tucentropdf
    postrotate
        systemctl reload ${SERVICE_NAME}
    endscript
}

/var/log/nginx/tucentropdf_legal_audit.log {
    daily
    missingok
    rotate 90
    compress
    delaycompress
    notifempty
    create 0644 www-data www-data
    postrotate
        systemctl reload nginx
    endscript
}
EOF

    # Configurar cron para limpieza automática
    cat > /etc/cron.d/tucentropdf-maintenance << EOF
# Limpieza de exportaciones expiradas (cada 6 horas)
0 */6 * * * tucentropdf curl -s http://localhost:8080/internal/cleanup-exports

# Archivado automático (diario a las 2:00 AM)
0 2 * * * tucentropdf curl -s http://localhost:8080/internal/auto-archive

# Actualización de estadísticas diarias (cada hora)
0 * * * * tucentropdf curl -s http://localhost:8080/internal/update-stats

# Backup de base de datos (diario a las 1:00 AM)
0 1 * * * root ${PROJECT_DIR}/scripts/backup-database.sh
EOF

    echo -e "${GREEN}✅ Monitoreo configurado${NC}"
}

# =============================================================================
# 10. CREAR SCRIPTS DE MANTENIMIENTO
# =============================================================================

create_maintenance_scripts() {
    echo -e "${BLUE}🔧 Creando scripts de mantenimiento...${NC}"
    
    mkdir -p ${PROJECT_DIR}/scripts
    
    # Script de backup
    cat > ${PROJECT_DIR}/scripts/backup-database.sh << 'EOF'
#!/bin/bash
BACKUP_DIR="/var/backups/tucentropdf"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
DB_NAME="tucentropdf_legal"

mkdir -p ${BACKUP_DIR}

# Backup completo de la base de datos
sudo -u postgres pg_dump ${DB_NAME} | gzip > ${BACKUP_DIR}/db_backup_${TIMESTAMP}.sql.gz

# Mantener solo los últimos 30 días de backups
find ${BACKUP_DIR} -name "db_backup_*.sql.gz" -mtime +30 -delete

echo "Backup completado: db_backup_${TIMESTAMP}.sql.gz"
EOF

    # Script de verificación de integridad
    cat > ${PROJECT_DIR}/scripts/integrity-check.sh << 'EOF'
#!/bin/bash
LOG_FILE="/var/log/tucentropdf/integrity-check.log"

echo "$(date): Iniciando verificación de integridad" >> ${LOG_FILE}

# Verificar integridad de registros de auditoría legal
RESULT=$(curl -s -H "Authorization: Bearer admin-token-123" \
    http://localhost:8080/api/v2/legal-audit/admin/integrity/verify?limit=1000)

if echo "$RESULT" | grep -q '"verified":true'; then
    echo "$(date): Verificación de integridad exitosa" >> ${LOG_FILE}
else
    echo "$(date): ERROR - Fallos de integridad detectados" >> ${LOG_FILE}
    echo "$RESULT" >> ${LOG_FILE}
fi
EOF

    # Hacer scripts ejecutables
    chmod +x ${PROJECT_DIR}/scripts/*.sh
    chown -R tucentropdf:tucentropdf ${PROJECT_DIR}/scripts
    
    echo -e "${GREEN}✅ Scripts de mantenimiento creados${NC}"
}

# =============================================================================
# 11. INICIAR SERVICIOS
# =============================================================================

start_services() {
    echo -e "${BLUE}🚀 Iniciando servicios...${NC}"
    
    # Iniciar base de datos
    systemctl start postgresql
    
    # Iniciar aplicación
    systemctl start ${SERVICE_NAME}
    
    # Verificar estado
    if systemctl is-active --quiet ${SERVICE_NAME}; then
        echo -e "${GREEN}✅ TuCentroPDF Engine V2 iniciado correctamente${NC}"
    else
        echo -e "${RED}❌ Error al iniciar la aplicación${NC}"
        journalctl -u ${SERVICE_NAME} --no-pager -l
        exit 1
    fi
    
    # Iniciar Nginx
    systemctl start nginx
    
    echo -e "${GREEN}✅ Todos los servicios iniciados${NC}"
}

# =============================================================================
# 12. VERIFICAR DESPLIEGUE
# =============================================================================

verify_deployment() {
    echo -e "${BLUE}🔍 Verificando despliegue...${NC}"
    
    # Esperar a que la aplicación esté lista
    sleep 10
    
    # Verificar endpoint de salud
    if curl -f -s http://localhost:8080/api/v2/legal-audit/health > /dev/null; then
        echo -e "${GREEN}✅ Endpoint de salud responde correctamente${NC}"
    else
        echo -e "${RED}❌ Endpoint de salud no responde${NC}"
        exit 1
    fi
    
    # Verificar conexión a base de datos
    if sudo -u postgres psql -d ${DB_NAME} -c "SELECT 1;" > /dev/null; then
        echo -e "${GREEN}✅ Conexión a base de datos funcionando${NC}"
    else
        echo -e "${RED}❌ Error de conexión a base de datos${NC}"
        exit 1
    fi
    
    # Verificar directorios críticos
    if [ -d "/var/tucentropdf/exports/legal" ] && [ -d "/var/tucentropdf/archives/legal" ]; then
        echo -e "${GREEN}✅ Directorios de auditoría legal creados${NC}"
    else
        echo -e "${RED}❌ Directorios críticos faltantes${NC}"
        exit 1
    fi
    
    echo -e "${GREEN}✅ Despliegue verificado exitosamente${NC}"
}

# =============================================================================
# FUNCIÓN PRINCIPAL
# =============================================================================

main() {
    echo -e "${BLUE}"
    echo "=================================="
    echo "  TuCentroPDF Engine V2 Deploy"
    echo "  Sistema de Auditoría Legal"
    echo "=================================="
    echo -e "${NC}"
    
    check_prerequisites
    setup_database
    setup_directories
    generate_security_keys
    deploy_application
    run_migrations
    setup_systemd_service
    setup_nginx
    setup_monitoring
    create_maintenance_scripts
    start_services
    verify_deployment
    
    echo -e "${GREEN}"
    echo "🎉 ¡DESPLIEGUE COMPLETADO EXITOSAMENTE!"
    echo ""
    echo "📊 Sistema de Auditoría Legal activado con:"
    echo "   ✓ Logs inmutables con triggers PostgreSQL"
    echo "   ✓ Cifrado AES256-GCM para datos sensibles"
    echo "   ✓ Firmas digitales HMAC-SHA256"
    echo "   ✓ Retención legal de 3 años"
    echo "   ✓ Exportación de evidencia legal"
    echo "   ✓ APIs de administración seguras"
    echo ""
    echo "🌐 Acceso:"
    echo "   - HTTP:  http://localhost"
    echo "   - HTTPS: https://localhost"
    echo "   - API:   http://localhost:8080"
    echo ""
    echo "🔐 APIs de Auditoría Legal:"
    echo "   - Logs:      GET /api/v2/legal-audit/admin/logs"
    echo "   - Export:    POST /api/v2/legal-audit/admin/export"
    echo "   - Integridad: GET /api/v2/legal-audit/admin/integrity/verify"
    echo ""
    echo "📁 Ubicaciones importantes:"
    echo "   - Aplicación: ${PROJECT_DIR}"
    echo "   - Logs:       ${LOG_DIR}"
    echo "   - Exports:    /var/tucentropdf/exports/legal"
    echo "   - Archives:   /var/tucentropdf/archives/legal"
    echo ""
    echo "🔑 Token de admin de ejemplo: admin-token-123"
    echo ""
    echo "⚠️  IMPORTANTE:"
    echo "   - Cambiar contraseñas por defecto en producción"
    echo "   - Configurar certificados SSL válidos"
    echo "   - Revisar y ajustar configuración de seguridad"
    echo "   - Configurar alertas de monitoreo"
    echo -e "${NC}"
}

# Ejecutar función principal
main "$@"