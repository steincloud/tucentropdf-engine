# Guía de Deployment - TuCentroPDF Engine V2

Esta guía explica cómo desplegar TuCentroPDF Engine V2 en un VPS y conectarlo con tu aplicación web.

## 📋 Tabla de Contenidos

- [Requisitos del VPS](#requisitos-del-vps)
- [Preparación del Entorno](#preparación-del-entorno)
- [Deployment con Docker](#deployment-con-docker)
- [Configuración de Nginx](#configuración-de-nginx)
- [Variables de Entorno](#variables-de-entorno)
- [Integración con tu Web](#integración-con-tu-web)
- [Monitoreo y Logs](#monitoreo-y-logs)
- [Seguridad](#seguridad)
- [Backup y Mantenimiento](#backup-y-mantenimiento)

## 🖥️ Requisitos del VPS

### Especificaciones Mínimas
- **CPU**: 2 vCPU
- **RAM**: 4 GB
- **Almacenamiento**: 40 GB SSD
- **SO**: Ubuntu 22.04 LTS o CentOS 8+
- **Ancho de banda**: 100 Mbps

### Especificaciones Recomendadas (Producción)
- **CPU**: 4 vCPU
- **RAM**: 8 GB
- **Almacenamiento**: 80 GB SSD
- **SO**: Ubuntu 22.04 LTS
- **Ancho de banda**: 1 Gbps

### Proveedores Recomendados
- **DigitalOcean**: Droplets desde $20/mes
- **Vultr**: High Performance desde $24/mes
- **Linode**: Dedicated CPU desde $30/mes
- **AWS**: t3.medium (para empezar)

## 🔧 Preparación del Entorno

### 1. Conexión inicial al VPS

```bash
# Conectar por SSH (reemplaza con tu IP)
ssh root@YOUR_VPS_IP

# Actualizar sistema
apt update && apt upgrade -y

# Crear usuario para la aplicación
adduser tucentropdf
usermod -aG sudo tucentropdf
su - tucentropdf
```

### 2. Instalar dependencias

```bash
# Docker y Docker Compose
curl -fsSL https://get.docker.com -o get-docker.sh
sh get-docker.sh
sudo usermod -aG docker tucentropdf

# Docker Compose
sudo curl -L "https://github.com/docker/compose/releases/latest/download/docker-compose-$(uname -s)-$(uname -m)" -o /usr/local/bin/docker-compose
sudo chmod +x /usr/local/bin/docker-compose

# Nginx
sudo apt install nginx -y

# Certbot para SSL
sudo apt install snapd -y
sudo snap install core; sudo snap refresh core
sudo snap install --classic certbot
sudo ln -s /snap/bin/certbot /usr/bin/certbot

# Git
sudo apt install git -y

# Herramientas adicionales
sudo apt install htop curl wget unzip -y
```

### 3. Configurar firewall

```bash
# UFW (Ubuntu Firewall)
sudo ufw enable
sudo ufw allow ssh
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw allow 8080/tcp  # Puerto temporal para testing
sudo ufw status
```

## 🐳 Deployment con Docker

### 1. Clonar el repositorio

```bash
# Crear directorio de aplicación
sudo mkdir -p /opt/tucentropdf
sudo chown tucentropdf:tucentropdf /opt/tucentropdf
cd /opt/tucentropdf

# Clonar tu repositorio
git clone https://github.com/tu-usuario/tucentropdf-engine.git
cd tucentropdf-engine/engine_v2
```

### 2. Crear archivo docker-compose.yml

```yaml
# /opt/tucentropdf/tucentropdf-engine/engine_v2/docker-compose.prod.yml
version: '3.8'

services:
  # Aplicación principal
  tucentropdf-engine:
    build:
      context: .
      dockerfile: Dockerfile
    container_name: tucentropdf-engine
    restart: unless-stopped
    ports:
      - "8080:8080"
    environment:
      - ENVIRONMENT=production
      - HOST=0.0.0.0
      - PORT=8080
      - CORS_ORIGINS=https://tucentropdf.com,https://www.tucentropdf.com
      - REDIS_HOST=redis
      - REDIS_PORT=6379
      - STORAGE_TEMP_DIR=/tmp/tucentropdf
      - STORAGE_UPLOADS_DIR=/uploads
      - OFFICE_LIBREOFFICE_PATH=/usr/bin/libreoffice
      - LOG_LEVEL=info
      - LOG_FORMAT=json
    env_file:
      - .env.production
    volumes:
      - ./uploads:/uploads
      - ./temp:/tmp/tucentropdf
      - ./logs:/app/logs
    depends_on:
      - redis
    networks:
      - tucentropdf-network
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/health"]
      interval: 30s
      timeout: 10s
      retries: 3

  # Redis para rate limiting y cache
  redis:
    image: redis:7-alpine
    container_name: tucentropdf-redis
    restart: unless-stopped
    ports:
      - "127.0.0.1:6379:6379"
    command: redis-server --appendonly yes --requirepass ${REDIS_PASSWORD}
    volumes:
      - redis_data:/data
    networks:
      - tucentropdf-network
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 30s
      timeout: 10s
      retries: 3

  # Nginx reverse proxy
  nginx:
    image: nginx:alpine
    container_name: tucentropdf-nginx
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./nginx/nginx.conf:/etc/nginx/nginx.conf:ro
      - ./nginx/sites-available:/etc/nginx/sites-available:ro
      - ./nginx/ssl:/etc/nginx/ssl:ro
      - /etc/letsencrypt:/etc/letsencrypt:ro
    depends_on:
      - tucentropdf-engine
    networks:
      - tucentropdf-network

volumes:
  redis_data:
    driver: local

networks:
  tucentropdf-network:
    driver: bridge
```

### 3. Crear archivo de producción .env

```bash
# .env.production
ENVIRONMENT=production
OPENAI_API_KEY=tu-api-key-real-aqui
OPENAI_MODEL=gpt-4o-mini
REDIS_PASSWORD=tu-password-redis-seguro
JWT_SECRET=tu-jwt-secret-super-seguro-aqui
CORS_ORIGINS=https://tucentropdf.com,https://www.tucentropdf.com,https://api.tucentropdf.com
```

## 🌐 Configuración de Nginx

### 1. Estructura de directorios

```bash
mkdir -p nginx/{sites-available,ssl}
```

### 2. Configuración principal

```nginx
# nginx/nginx.conf
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

    # Logging
    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" "$http_x_forwarded_for"';
    access_log /var/log/nginx/access.log main;

    # Performance
    sendfile on;
    tcp_nopush on;
    tcp_nodelay on;
    keepalive_timeout 65;
    types_hash_max_size 2048;
    client_max_body_size 100M;

    # Gzip
    gzip on;
    gzip_vary on;
    gzip_min_length 10240;
    gzip_proxied expired no-cache no-store private must-revalidate auth;
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

    # Rate limiting
    limit_req_zone $binary_remote_addr zone=api:10m rate=10r/s;
    limit_req_zone $binary_remote_addr zone=upload:10m rate=1r/s;

    # Include sites
    include /etc/nginx/sites-available/*;
}
```

### 3. Configuración del sitio

```nginx
# nginx/sites-available/tucentropdf-api.conf
upstream tucentropdf_backend {
    server tucentropdf-engine:8080;
    keepalive 32;
}

# HTTP -> HTTPS redirect
server {
    listen 80;
    server_name api.tucentropdf.com;
    return 301 https://$server_name$request_uri;
}

# HTTPS API Server
server {
    listen 443 ssl http2;
    server_name api.tucentropdf.com;

    # SSL Configuration
    ssl_certificate /etc/letsencrypt/live/api.tucentropdf.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/api.tucentropdf.com/privkey.pem;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers ECDHE-RSA-AES256-GCM-SHA512:DHE-RSA-AES256-GCM-SHA512:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers off;
    ssl_session_cache shared:SSL:10m;
    ssl_session_timeout 10m;

    # Security Headers
    add_header X-Frame-Options DENY;
    add_header X-Content-Type-Options nosniff;
    add_header X-XSS-Protection "1; mode=block";
    add_header Strict-Transport-Security "max-age=31536000; includeSubdomains";
    add_header Referrer-Policy "strict-origin-when-cross-origin";

    # CORS Headers
    add_header Access-Control-Allow-Origin "https://tucentropdf.com";
    add_header Access-Control-Allow-Methods "GET, POST, OPTIONS";
    add_header Access-Control-Allow-Headers "Origin, X-Requested-With, Content-Type, Accept, Authorization, X-User-Plan";

    # Rate limiting
    limit_req zone=api burst=20 nodelay;

    # Proxy settings
    proxy_http_version 1.1;
    proxy_cache_bypass $http_upgrade;
    proxy_set_header Upgrade $http_upgrade;
    proxy_set_header Connection 'upgrade';
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;

    # API Routes
    location /api/ {
        proxy_pass http://tucentropdf_backend;
        proxy_read_timeout 300s;
        proxy_connect_timeout 75s;
    }

    # Upload endpoints (special rate limiting)
    location ~ ^/api/v2/(pdf|office|ocr)/ {
        limit_req zone=upload burst=5 nodelay;
        proxy_pass http://tucentropdf_backend;
        proxy_read_timeout 600s;
        proxy_connect_timeout 75s;
        client_max_body_size 100M;
    }

    # Health checks (no rate limiting)
    location ~ ^/(health|ready|metrics)$ {
        proxy_pass http://tucentropdf_backend;
        access_log off;
    }

    # Block access to sensitive paths
    location ~ /\. {
        deny all;
        access_log off;
        log_not_found off;
    }
}
```

## ⚙️ Variables de Entorno

### Configuración de producción completa

```bash
# .env.production (COMPLETO)
# Entorno
ENVIRONMENT=production
HOST=0.0.0.0
PORT=8080

# CORS (tu dominio web)
CORS_ORIGINS=https://tucentropdf.com,https://www.tucentropdf.com

# OpenAI Configuration
OPENAI_API_KEY=sk-tu-api-key-real-aqui
OPENAI_MODEL=gpt-4o-mini

# Redis Configuration
REDIS_HOST=redis
REDIS_PORT=6379
REDIS_PASSWORD=tu-password-redis-super-seguro
REDIS_DB=0

# Storage Configuration
STORAGE_TEMP_DIR=/tmp/tucentropdf
STORAGE_UPLOADS_DIR=/uploads
STORAGE_MAX_SIZE_MB=100

# Office Configuration
OFFICE_LIBREOFFICE_PATH=/usr/bin/libreoffice
OFFICE_TIMEOUT=30

# Logging
LOG_LEVEL=info
LOG_FORMAT=json
LOG_OUTPUT=stdout

# Security
JWT_SECRET=tu-jwt-secret-de-al-menos-32-caracteres-super-seguro

# Plan Limits (ajustar según tu modelo de negocio)
PLAN_FREE_MAX_SIZE_MB=5
PLAN_FREE_MAX_FILES_DAY=10
PLAN_FREE_MAX_AI_PAGES=0

PLAN_PREMIUM_MAX_SIZE_MB=25
PLAN_PREMIUM_MAX_FILES_DAY=100
PLAN_PREMIUM_MAX_AI_PAGES=3

PLAN_PRO_MAX_SIZE_MB=100
PLAN_PRO_MAX_FILES_DAY=1000
PLAN_PRO_MAX_AI_PAGES=20
```

## 🔗 Integración con tu Web

### 1. Client JavaScript para tu web

```javascript
// tucentropdf-client.js
class TuCentroPDFClient {
    constructor(apiUrl, userPlan = 'free') {
        this.apiUrl = apiUrl.replace(/\/$/, ''); // Remove trailing slash
        this.userPlan = userPlan;
    }

    // Helper para hacer requests
    async makeRequest(endpoint, options = {}) {
        const url = `${this.apiUrl}${endpoint}`;
        
        const defaultHeaders = {
            'X-User-Plan': this.userPlan,
            'X-User-ID': this.getCurrentUserId()
        };

        const config = {
            ...options,
            headers: {
                ...defaultHeaders,
                ...options.headers
            }
        };

        try {
            const response = await fetch(url, config);
            
            if (!response.ok) {
                const error = await response.json().catch(() => ({}));
                throw new Error(error.message || `HTTP ${response.status}`);
            }

            return await response.json();
        } catch (error) {
            console.error('TuCentroPDF API Error:', error);
            throw error;
        }
    }

    // Obtener ID del usuario actual (implementar según tu sistema)
    getCurrentUserId() {
        return localStorage.getItem('user_id') || 'anonymous';
    }

    // ======================
    // OPERACIONES PDF
    // ======================

    async mergePDFs(files, filename = 'merged.pdf') {
        const formData = new FormData();
        
        files.forEach((file, index) => {
            formData.append(`file_${index}`, file);
        });
        formData.append('output_filename', filename);

        return this.makeRequest('/api/v2/pdf/merge', {
            method: 'POST',
            body: formData
        });
    }

    async splitPDF(file, pagesPerFile = 1) {
        const formData = new FormData();
        formData.append('file', file);
        formData.append('pages_per_file', pagesPerFile.toString());

        return this.makeRequest('/api/v2/pdf/split', {
            method: 'POST',
            body: formData
        });
    }

    async optimizePDF(file, quality = 'medium') {
        const formData = new FormData();
        formData.append('file', file);
        formData.append('quality', quality);

        return this.makeRequest('/api/v2/pdf/optimize', {
            method: 'POST',
            body: formData
        });
    }

    async getPDFInfo(file) {
        const formData = new FormData();
        formData.append('file', file);

        return this.makeRequest('/api/v2/pdf/info', {
            method: 'POST',
            body: formData
        });
    }

    // ======================
    // CONVERSIÓN OFFICE
    // ======================

    async convertOfficeToPDF(file) {
        const formData = new FormData();
        formData.append('file', file);

        return this.makeRequest('/api/v2/office/convert', {
            method: 'POST',
            body: formData
        });
    }

    async getSupportedFormats() {
        return this.makeRequest('/api/v2/office/formats');
    }

    // ======================
    // OCR
    // ======================

    async extractTextOCR(file, options = {}) {
        const formData = new FormData();
        formData.append('file', file);
        
        if (options.language) {
            formData.append('language', options.language);
        }
        if (options.engine) {
            formData.append('engine', options.engine); // 'classic' or 'ai'
        }

        return this.makeRequest('/api/v2/ocr/extract', {
            method: 'POST',
            body: formData
        });
    }

    async extractStructuredData(file, documentType = 'general') {
        const formData = new FormData();
        formData.append('file', file);
        formData.append('document_type', documentType);

        return this.makeRequest('/api/v2/ocr/extract-structured', {
            method: 'POST',
            body: formData
        });
    }

    // ======================
    // UTILIDADES
    // ======================

    async checkHealth() {
        return this.makeRequest('/health');
    }

    async getQuota() {
        return this.makeRequest('/api/v2/user/quota');
    }
}

// Uso en tu página web
window.TuCentroPDFClient = TuCentroPDFClient;
```

### 2. Ejemplo de integración en tu página

```html
<!DOCTYPE html>
<html>
<head>
    <title>TuCentroPDF - Herramientas PDF</title>
    <script src="tucentropdf-client.js"></script>
</head>
<body>
    <h1>Herramientas PDF</h1>
    
    <!-- Merge PDFs -->
    <section id="merge-section">
        <h2>Combinar PDFs</h2>
        <input type="file" id="merge-files" multiple accept=".pdf">
        <button onclick="mergePDFs()">Combinar</button>
        <div id="merge-result"></div>
    </section>

    <!-- Convert Office -->
    <section id="convert-section">
        <h2>Convertir a PDF</h2>
        <input type="file" id="office-file" accept=".txt,.doc,.docx,.xls,.xlsx,.ppt,.pptx">
        <button onclick="convertToPDF()">Convertir</button>
        <div id="convert-result"></div>
    </section>

    <!-- OCR -->
    <section id="ocr-section">
        <h2>Extraer Texto (OCR)</h2>
        <input type="file" id="ocr-file" accept="image/*,.pdf">
        <select id="ocr-engine">
            <option value="classic">OCR Clásico</option>
            <option value="ai">OCR con IA</option>
        </select>
        <button onclick="extractText()">Extraer Texto</button>
        <div id="ocr-result"></div>
    </section>

    <script>
        // Inicializar cliente
        const pdfClient = new TuCentroPDFClient('https://api.tucentropdf.com', 'premium');

        // Funciones de ejemplo
        async function mergePDFs() {
            const files = document.getElementById('merge-files').files;
            if (files.length < 2) {
                alert('Selecciona al menos 2 archivos PDF');
                return;
            }

            try {
                document.getElementById('merge-result').innerHTML = 'Procesando...';
                const result = await pdfClient.mergePDFs(Array.from(files));
                
                if (result.success) {
                    document.getElementById('merge-result').innerHTML = 
                        `<a href="${result.download_url}" download>Descargar PDF combinado</a>`;
                } else {
                    document.getElementById('merge-result').innerHTML = 'Error: ' + result.message;
                }
            } catch (error) {
                document.getElementById('merge-result').innerHTML = 'Error: ' + error.message;
            }
        }

        async function convertToPDF() {
            const file = document.getElementById('office-file').files[0];
            if (!file) {
                alert('Selecciona un archivo');
                return;
            }

            try {
                document.getElementById('convert-result').innerHTML = 'Convirtiendo...';
                const result = await pdfClient.convertOfficeToPDF(file);
                
                if (result.success) {
                    document.getElementById('convert-result').innerHTML = 
                        `<a href="${result.download_url}" download>Descargar PDF</a>`;
                } else {
                    document.getElementById('convert-result').innerHTML = 'Error: ' + result.message;
                }
            } catch (error) {
                document.getElementById('convert-result').innerHTML = 'Error: ' + error.message;
            }
        }

        async function extractText() {
            const file = document.getElementById('ocr-file').files[0];
            const engine = document.getElementById('ocr-engine').value;
            
            if (!file) {
                alert('Selecciona una imagen o PDF');
                return;
            }

            try {
                document.getElementById('ocr-result').innerHTML = 'Extrayendo texto...';
                const result = await pdfClient.extractTextOCR(file, { engine });
                
                if (result.success) {
                    document.getElementById('ocr-result').innerHTML = 
                        `<h3>Texto extraído:</h3><pre>${result.text}</pre>`;
                } else {
                    document.getElementById('ocr-result').innerHTML = 'Error: ' + result.message;
                }
            } catch (error) {
                document.getElementById('ocr-result').innerHTML = 'Error: ' + error.message;
            }
        }

        // Verificar estado del servicio al cargar
        document.addEventListener('DOMContentLoaded', async () => {
            try {
                const health = await pdfClient.checkHealth();
                console.log('API Status:', health);
            } catch (error) {
                console.error('API no disponible:', error);
            }
        });
    </script>
</body>
</html>
```

## 🚀 Scripts de Deployment

### 1. Script de deployment inicial

```bash
#!/bin/bash
# deploy.sh

set -e

echo "🚀 Iniciando deployment de TuCentroPDF Engine V2..."

# Variables
REPO_URL="https://github.com/tu-usuario/tucentropdf-engine.git"
APP_DIR="/opt/tucentropdf"
BACKUP_DIR="/opt/tucentropdf/backups"

# Crear backup si existe instalación previa
if [ -d "$APP_DIR/tucentropdf-engine" ]; then
    echo "📦 Creando backup..."
    mkdir -p $BACKUP_DIR
    sudo cp -r $APP_DIR/tucentropdf-engine $BACKUP_DIR/backup-$(date +%Y%m%d-%H%M%S)
fi

# Clonar/actualizar repositorio
echo "📥 Descargando código..."
cd $APP_DIR
if [ -d "tucentropdf-engine" ]; then
    cd tucentropdf-engine
    git pull origin main
else
    git clone $REPO_URL
    cd tucentropdf-engine
fi

cd engine_v2

# Verificar archivos de configuración
echo "⚙️  Verificando configuración..."
if [ ! -f ".env.production" ]; then
    echo "❌ Falta archivo .env.production"
    echo "Copia .env.example a .env.production y configura las variables"
    exit 1
fi

# Crear directorios necesarios
echo "📁 Creando directorios..."
sudo mkdir -p uploads temp logs nginx/ssl
sudo chown -R tucentropdf:tucentropdf uploads temp logs

# Construir y lanzar contenedores
echo "🔨 Construyendo aplicación..."
docker-compose -f docker-compose.prod.yml build --no-cache

echo "🚀 Iniciando servicios..."
docker-compose -f docker-compose.prod.yml up -d

# Esperar a que los servicios estén listos
echo "⏳ Esperando servicios..."
sleep 30

# Verificar salud de los servicios
echo "🏥 Verificando salud..."
if curl -f http://localhost:8080/health > /dev/null 2>&1; then
    echo "✅ Aplicación funcionando correctamente"
else
    echo "❌ Error: Aplicación no responde"
    echo "📋 Logs de la aplicación:"
    docker-compose -f docker-compose.prod.yml logs tucentropdf-engine --tail=50
    exit 1
fi

echo "🎉 ¡Deployment completado exitosamente!"
echo "📊 Estado de los servicios:"
docker-compose -f docker-compose.prod.yml ps
```

### 2. Script de actualización

```bash
#!/bin/bash
# update.sh

set -e

echo "🔄 Actualizando TuCentroPDF Engine V2..."

cd /opt/tucentropdf/tucentropdf-engine/engine_v2

# Backup de la base de datos Redis
echo "💾 Backup de Redis..."
docker exec tucentropdf-redis redis-cli BGSAVE

# Actualizar código
echo "📥 Actualizando código..."
git pull origin main

# Rebuild solo si hay cambios en Dockerfile o dependencias
if git diff HEAD~1 --name-only | grep -E "(Dockerfile|go\.mod|go\.sum)"; then
    echo "🔨 Reconstruyendo imagen..."
    docker-compose -f docker-compose.prod.yml build tucentropdf-engine
fi

# Restart con zero-downtime (rolling update)
echo "♻️  Actualizando servicios..."
docker-compose -f docker-compose.prod.yml up -d

# Verificar que el update fue exitoso
echo "🏥 Verificando..."
sleep 15

if curl -f http://localhost:8080/health > /dev/null 2>&1; then
    echo "✅ Actualización exitosa"
    
    # Limpiar imágenes antigas
    docker image prune -f
else
    echo "❌ Error en actualización"
    echo "📋 Logs:"
    docker-compose -f docker-compose.prod.yml logs --tail=50
    exit 1
fi

echo "🎉 ¡Actualización completada!"
```

## 📊 Monitoreo y Logs

### 1. Script de monitoreo

```bash
#!/bin/bash
# monitor.sh

echo "📊 Estado de TuCentroPDF Engine V2"
echo "=================================="

# Estado de contenedores
echo "🐳 Contenedores:"
docker-compose -f /opt/tucentropdf/tucentropdf-engine/engine_v2/docker-compose.prod.yml ps

# Uso de recursos
echo -e "\n💻 Recursos del sistema:"
echo "CPU: $(top -bn1 | grep "Cpu(s)" | awk '{print $2}' | cut -d'%' -f1)%"
echo "RAM: $(free | grep Mem | awk '{printf "%.1f%%", $3/$2 * 100.0}')"
echo "Disco: $(df -h / | awk 'NR==2{printf "%s", $5}')"

# Estado de la API
echo -e "\n🌐 Estado de la API:"
if curl -f http://localhost:8080/health > /dev/null 2>&1; then
    echo "✅ API funcionando"
    # Obtener métricas
    curl -s http://localhost:8080/metrics | grep -E "(requests_total|response_time)" | head -5
else
    echo "❌ API no responde"
fi

# Logs recientes
echo -e "\n📋 Logs recientes (últimos 10):"
docker logs tucentropdf-engine --tail=10 --timestamps

# Espacio en disco de uploads
echo -e "\n📁 Almacenamiento:"
du -sh /opt/tucentropdf/tucentropdf-engine/engine_v2/uploads
du -sh /opt/tucentropdf/tucentropdf-engine/engine_v2/temp
```

### 2. Configurar cron para monitoreo

```bash
# Agregar al crontab del usuario tucentropdf
crontab -e

# Monitoreo cada 5 minutos
*/5 * * * * /opt/tucentropdf/scripts/monitor.sh >> /var/log/tucentropdf-monitor.log 2>&1

# Limpieza de archivos temporales diaria a las 2 AM
0 2 * * * find /opt/tucentropdf/tucentropdf-engine/engine_v2/temp -type f -mtime +1 -delete

# Backup de Redis diario a las 3 AM
0 3 * * * docker exec tucentropdf-redis redis-cli BGSAVE
```

## 🔒 Configuración SSL

### 1. Obtener certificados SSL

```bash
# Asegúrate de que tu dominio apunte a la IP del VPS
# Detener nginx temporalmente
sudo systemctl stop nginx

# Obtener certificados para tu dominio API
sudo certbot certonly --standalone -d api.tucentropdf.com

# Reiniciar nginx
sudo systemctl start nginx

# Auto-renovación (agregar a cron)
sudo crontab -e
# Agregar: 0 12 * * * /usr/bin/certbot renew --quiet --post-hook "systemctl reload nginx"
```

### 2. Configurar renovación automática

```bash
#!/bin/bash
# /opt/tucentropdf/scripts/renew-ssl.sh

# Renovar certificados
certbot renew --quiet --post-hook "docker-compose -f /opt/tucentropdf/tucentropdf-engine/engine_v2/docker-compose.prod.yml restart nginx"

# Log del resultado
echo "$(date): SSL renewal check completed" >> /var/log/ssl-renewal.log
```

## 🛡️ Seguridad Adicional

### 1. Configurar fail2ban

```bash
# Instalar fail2ban
sudo apt install fail2ban -y

# Configurar para nginx
sudo tee /etc/fail2ban/jail.local << EOF
[nginx-limit-req]
enabled = true
filter = nginx-limit-req
action = iptables-multiport[name=ReqLimit, port="http,https", protocol=tcp]
logpath = /var/log/nginx/*error.log
findtime = 600
bantime = 7200
maxretry = 10
EOF

sudo systemctl restart fail2ban
```

### 2. Configurar backups automáticos

```bash
#!/bin/bash
# /opt/tucentropdf/scripts/backup.sh

BACKUP_DIR="/opt/backups"
DATE=$(date +%Y%m%d-%H%M%S)

mkdir -p $BACKUP_DIR

# Backup de archivos de configuración
tar -czf $BACKUP_DIR/config-$DATE.tar.gz -C /opt/tucentropdf/tucentropdf-engine/engine_v2 .env.production nginx/

# Backup de Redis
docker exec tucentropdf-redis redis-cli --rdb /data/backup-$DATE.rdb

# Backup de uploads importantes (últimos 7 días)
find /opt/tucentropdf/tucentropdf-engine/engine_v2/uploads -mtime -7 -type f | tar -czf $BACKUP_DIR/uploads-$DATE.tar.gz -T -

# Limpiar backups antiguos (mantener últimos 7 días)
find $BACKUP_DIR -type f -mtime +7 -delete

echo "Backup completado: $DATE"
```

## 🚀 Pasos Finales de Deployment

### 1. Lista de verificación

- [ ] VPS configurado con requisitos mínimos
- [ ] Docker y Docker Compose instalados
- [ ] Nginx configurado con SSL
- [ ] Variables de entorno configuradas
- [ ] Dominio DNS apuntando al VPS
- [ ] Firewall configurado
- [ ] Certificados SSL obtenidos
- [ ] Servicios iniciados y funcionando
- [ ] Monitoreo configurado
- [ ] Backups configurados

### 2. Comandos finales

```bash
# En tu VPS, ejecutar todos los pasos:
chmod +x /opt/tucentropdf/scripts/*.sh
sudo /opt/tucentropdf/scripts/deploy.sh

# Verificar que todo funciona
curl https://api.tucentropdf.com/health

# Ver logs en tiempo real
docker-compose -f /opt/tucentropdf/tucentropdf-engine/engine_v2/docker-compose.prod.yml logs -f
```

¡Listo! Tu TuCentroPDF Engine V2 estará funcionando en `https://api.tucentropdf.com` y podrás integrarlo con tu web usando el cliente JavaScript proporcionado.

¿Necesitas ayuda con algún paso específico del deployment?