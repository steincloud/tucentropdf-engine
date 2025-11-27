# Guía Rápida de Implementación VPS
## TuCentroPDF Engine V2

### 📋 Prerrequisitos

- **VPS con Ubuntu 20.04/22.04**
- **Mínimo 2GB RAM, 2 CPU cores, 20GB disco**
- **Acceso root via SSH**
- **Dominio configurado** (opcional pero recomendado para SSL)

### 🚀 Implementación Rápida (5 pasos)

#### 1. Configuración inicial del servidor
```bash
# Conectar al VPS
ssh root@tu-servidor

# Descargar y ejecutar script de configuración
curl -fsSL https://raw.githubusercontent.com/tu-usuario/tu-repo/main/engine_v2/scripts/setup.sh -o setup.sh
chmod +x setup.sh
sudo ./setup.sh
```

#### 2. Configurar aplicación
```bash
# Cambiar al usuario de aplicación
sudo su - tucentropdf

# Clonar repositorio
git clone https://github.com/tu-usuario/tu-repo.git /opt/tucentropdf/tucentropdf-engine

# Ir al directorio de la aplicación
cd /opt/tucentropdf/tucentropdf-engine/engine_v2
```

#### 3. Configurar variables de entorno
```bash
# Copiar archivo de ejemplo
cp .env.example .env.production

# Editar configuración
nano .env.production
```

**Variables críticas a configurar:**
```bash
# Configuración básica
APP_ENV=production
APP_HOST=0.0.0.0
APP_PORT=8080
APP_DOMAIN=tu-dominio.com

# Base de datos Redis
REDIS_HOST=redis
REDIS_PORT=6379
REDIS_PASSWORD=tu-password-seguro

# Seguridad
JWT_SECRET=tu-jwt-secret-muy-largo
API_KEY=tu-api-key-secreta

# Límites
MAX_FILE_SIZE=50MB
UPLOAD_TIMEOUT=300s
OCR_TIMEOUT=60s
OFFICE_TIMEOUT=120s

# Monitoreo
MONITORING_ENABLED=true
```

#### 4. Desplegar aplicación
```bash
# Hacer script ejecutable
chmod +x scripts/deploy.sh

# Ejecutar despliegue
./scripts/deploy.sh
```

#### 5. Configurar SSL (si tienes dominio)
```bash
# Volver a root
exit

# Configurar SSL con Certbot
sudo certbot --nginx -d tu-dominio.com

# Reiniciar Nginx
sudo systemctl restart nginx
```

### ✅ Verificación de Instalación

#### Comprobar servicios
```bash
# Estado general
tucentropdf-status

# Verificar contenedores Docker
docker ps

# Verificar logs
tail -f /var/log/tucentropdf/*.log
```

#### Probar API
```bash
# Health check
curl http://localhost:8080/health

# Si tienes dominio:
curl https://tu-dominio.com/api/v2/health
```

### 🔧 Configuración Nginx para tu dominio

**Editar configuración:**
```bash
sudo nano /etc/nginx/sites-available/tucentropdf
```

**Contenido del archivo:**
```nginx
server {
    listen 80;
    server_name tu-dominio.com www.tu-dominio.com;
    return 301 https://$server_name$request_uri;
}

server {
    listen 443 ssl http2;
    server_name tu-dominio.com www.tu-dominio.com;

    # SSL será configurado por Certbot automáticamente
    
    # Security headers
    add_header X-Frame-Options DENY;
    add_header X-Content-Type-Options nosniff;
    add_header X-XSS-Protection "1; mode=block";
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains";

    # Rate limiting
    limit_req zone=api burst=20 nodelay;

    # API routes
    location /api/ {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;
        
        # Timeouts para operaciones largas
        proxy_connect_timeout 300s;
        proxy_send_timeout 300s;
        proxy_read_timeout 300s;
    }

    # Upload endpoint con límites especiales
    location /api/v2/upload {
        limit_req zone=upload burst=5 nodelay;
        client_max_body_size 50M;
        
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        
        # Timeouts extendidos para uploads
        proxy_connect_timeout 600s;
        proxy_send_timeout 600s;
        proxy_read_timeout 600s;
    }

    # Static files (si tienes una interfaz web)
    location / {
        root /var/www/tucentropdf;
        index index.html;
        try_files $uri $uri/ =404;
    }
}
```

**Activar configuración:**
```bash
sudo ln -s /etc/nginx/sites-available/tucentropdf /etc/nginx/sites-enabled/
sudo nginx -t
sudo systemctl reload nginx
```

### 🌐 Integración con tu Web

#### JavaScript Client
```javascript
class TuCentroPDFClient {
    constructor(apiUrl, apiKey) {
        this.apiUrl = apiUrl;
        this.apiKey = apiKey;
    }

    async uploadFile(file, options = {}) {
        const formData = new FormData();
        formData.append('file', file);
        
        if (options.operation) {
            formData.append('operation', options.operation);
        }
        
        const response = await fetch(`${this.apiUrl}/api/v2/upload`, {
            method: 'POST',
            headers: {
                'X-API-Key': this.apiKey
            },
            body: formData
        });
        
        if (!response.ok) {
            throw new Error(`Upload failed: ${response.statusText}`);
        }
        
        return await response.json();
    }

    async processOperation(operationType, fileId, options = {}) {
        const response = await fetch(`${this.apiUrl}/api/v2/${operationType}`, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-API-Key': this.apiKey
            },
            body: JSON.stringify({
                file_id: fileId,
                ...options
            })
        });
        
        if (!response.ok) {
            throw new Error(`Operation failed: ${response.statusText}`);
        }
        
        return await response.json();
    }

    async downloadResult(resultId) {
        const response = await fetch(`${this.apiUrl}/api/v2/download/${resultId}`, {
            headers: {
                'X-API-Key': this.apiKey
            }
        });
        
        if (!response.ok) {
            throw new Error(`Download failed: ${response.statusText}`);
        }
        
        return response.blob();
    }
}

// Uso ejemplo
const client = new TuCentroPDFClient('https://tu-dominio.com', 'tu-api-key');

// Upload y procesar
async function processPDF() {
    try {
        const fileInput = document.getElementById('pdfFile');
        const file = fileInput.files[0];
        
        // Upload
        const uploadResult = await client.uploadFile(file);
        console.log('File uploaded:', uploadResult.file_id);
        
        // Procesar (ejemplo: OCR)
        const processResult = await client.processOperation('ocr', uploadResult.file_id, {
            language: 'es',
            output_format: 'text'
        });
        
        console.log('Processing started:', processResult.task_id);
        
        // Descargar resultado cuando esté listo
        setTimeout(async () => {
            const result = await client.downloadResult(processResult.result_id);
            // Manejar resultado...
        }, 5000);
        
    } catch (error) {
        console.error('Error:', error);
    }
}
```

### 📊 Monitoreo y Mantenimiento

#### Comandos útiles diarios
```bash
# Estado general del sistema
tucentropdf-status

# Logs en tiempo real
tail -f /var/log/tucentropdf/application.log

# Uso de recursos
htop

# Espacio en disco
df -h

# Estado de contenedores
docker ps -a
```

#### Backups automáticos
Los backups se ejecutan automáticamente:
- **Diario**: 2:00 AM
- **Semanal completo**: Domingos 1:00 AM

**Verificar backups:**
```bash
ls -la /opt/backups/tucentropdf/
```

**Restaurar backup:**
```bash
cd /opt/backups/tucentropdf
tar -xzf 20240315-020000_tucentropdf_backup.tar.gz
```

#### Actualizaciones
```bash
# Actualización automática
sudo su - tucentropdf
cd /opt/tucentropdf/tucentropdf-engine/engine_v2
./scripts/update.sh
```

### 🆘 Solución de Problemas

#### Problemas comunes

**1. Aplicación no responde:**
```bash
docker restart tucentropdf-engine
docker logs tucentropdf-engine --tail 100
```

**2. Error de permisos:**
```bash
sudo chown -R tucentropdf:tucentropdf /opt/tucentropdf
```

**3. Nginx error:**
```bash
sudo nginx -t
sudo systemctl status nginx
tail -f /var/log/nginx/error.log
```

**4. Espacio en disco lleno:**
```bash
# Limpiar logs antiguos
sudo journalctl --vacuum-time=7d

# Limpiar contenedores Docker
docker system prune -a

# Limpiar uploads antiguos
find /opt/tucentropdf/uploads -type f -mtime +7 -delete
```

#### Logs importantes
- **Aplicación**: `/var/log/tucentropdf/application.log`
- **Nginx**: `/var/log/nginx/error.log`
- **Docker**: `docker logs tucentropdf-engine`
- **Sistema**: `journalctl -u tucentropdf-engine`

### 📞 Contacto y Soporte

Para problemas o dudas:
1. Revisar logs de aplicación
2. Verificar estado de servicios con `tucentropdf-status`
3. Consultar documentación completa en `docs/`
4. Crear issue en el repositorio si persiste el problema

---

**¡Tu TuCentroPDF Engine V2 está listo para producción! 🎉**