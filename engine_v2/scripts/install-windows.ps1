# =============================================================================
# SCRIPT DE INSTALACIÓN - TuCentroPDF Engine V2 con Auditoría Legal
# SISTEMA COMPLETO PARA WINDOWS
# =============================================================================

Write-Host "🚀 Instalando TuCentroPDF Engine V2 - Sistema de Auditoría Legal" -ForegroundColor Blue

# Verificar si se ejecuta como administrador
if (-NOT ([Security.Principal.WindowsPrincipal] [Security.Principal.WindowsIdentity]::GetCurrent()).IsInRole([Security.Principal.WindowsBuiltInRole] "Administrator")) {
    Write-Host "❌ Este script debe ejecutarse como Administrador" -ForegroundColor Red
    Write-Host "Reinicia PowerShell como Administrador y ejecuta el script nuevamente" -ForegroundColor Yellow
    pause
    exit 1
}

# =============================================================================
# 1. INSTALAR CHOCOLATEY (Si no está instalado)
# =============================================================================

Write-Host "📦 Verificando Chocolatey..." -ForegroundColor Blue
try {
    choco --version | Out-Null
    Write-Host "✅ Chocolatey ya está instalado" -ForegroundColor Green
} catch {
    Write-Host "📦 Instalando Chocolatey..." -ForegroundColor Yellow
    Set-ExecutionPolicy Bypass -Scope Process -Force
    [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072
    iex ((New-Object System.Net.WebClient).DownloadString('https://community.chocolatey.org/install.ps1'))
    Write-Host "✅ Chocolatey instalado" -ForegroundColor Green
}

# =============================================================================
# 2. INSTALAR DEPENDENCIAS
# =============================================================================

Write-Host "📦 Instalando dependencias..." -ForegroundColor Blue

# Instalar Go
Write-Host "🔧 Instalando Go..." -ForegroundColor Yellow
choco install golang -y

# Instalar Git (si no está instalado)
Write-Host "🔧 Instalando Git..." -ForegroundColor Yellow
choco install git -y

# Instalar PostgreSQL
Write-Host "🗃️ Instalando PostgreSQL..." -ForegroundColor Yellow
choco install postgresql -y --params '/Password:postgres123'

# Refrescar variables de entorno
$env:Path = [System.Environment]::GetEnvironmentVariable("Path","Machine") + ";" + [System.Environment]::GetEnvironmentVariable("Path","User")
refreshenv

Write-Host "✅ Dependencias instaladas" -ForegroundColor Green

# =============================================================================
# 3. CONFIGURAR POSTGRESQL
# =============================================================================

Write-Host "🗃️ Configurando PostgreSQL..." -ForegroundColor Blue

# Esperar a que PostgreSQL esté listo
Start-Sleep -Seconds 10

# Configurar base de datos
$env:PGPASSWORD = "postgres123"

try {
    # Crear base de datos y usuario
    & "C:\Program Files\PostgreSQL\15\bin\psql.exe" -U postgres -c "
    -- Crear base de datos
    DROP DATABASE IF EXISTS tucentropdf_legal;
    CREATE DATABASE tucentropdf_legal WITH ENCODING 'UTF8';
    
    -- Crear usuario específico para auditoría legal
    DROP ROLE IF EXISTS tucentropdf_audit;
    CREATE ROLE tucentropdf_audit WITH LOGIN PASSWORD 'secure_audit_password_2024';
    
    -- Otorgar permisos
    GRANT CONNECT ON DATABASE tucentropdf_legal TO tucentropdf_audit;
    GRANT USAGE ON SCHEMA public TO tucentropdf_audit;
    GRANT CREATE ON SCHEMA public TO tucentropdf_audit;
    "

    # Conectar a la nueva base de datos y crear extensiones
    & "C:\Program Files\PostgreSQL\15\bin\psql.exe" -U postgres -d tucentropdf_legal -c "
    -- Configurar extensiones necesarias
    CREATE EXTENSION IF NOT EXISTS \"uuid-ossp\";
    CREATE EXTENSION IF NOT EXISTS \"pgcrypto\";
    "
    
    Write-Host "✅ PostgreSQL configurado exitosamente" -ForegroundColor Green
} catch {
    Write-Host "❌ Error configurando PostgreSQL: $_" -ForegroundColor Red
}

# =============================================================================
# 4. CONFIGURAR DIRECTORIOS Y VARIABLES DE ENTORNO
# =============================================================================

Write-Host "📁 Configurando directorios..." -ForegroundColor Blue

# Crear directorios del sistema
$projectDir = "C:\tucentropdf-engine-v2"
$exportDir = "C:\tucentropdf\exports\legal"
$archiveDir = "C:\tucentropdf\archives\legal"
$logDir = "C:\tucentropdf\logs"

New-Item -ItemType Directory -Path $projectDir -Force | Out-Null
New-Item -ItemType Directory -Path $exportDir -Force | Out-Null
New-Item -ItemType Directory -Path $archiveDir -Force | Out-Null
New-Item -ItemType Directory -Path $logDir -Force | Out-Null

Write-Host "✅ Directorios creados" -ForegroundColor Green

# =============================================================================
# 5. GENERAR CLAVES DE SEGURIDAD
# =============================================================================

Write-Host "🔐 Generando claves de seguridad..." -ForegroundColor Blue

# Función para generar claves aleatorias
function Get-RandomKey($length) {
    $bytes = New-Object byte[] $length
    [System.Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($bytes)
    return [Convert]::ToBase64String($bytes)
}

$encryptionKey = Get-RandomKey(32)
$encryptionSalt = Get-RandomKey(64)
$hmacSecret = Get-RandomKey(32)
$jwtSecret = Get-RandomKey(32)

# Crear archivo .env
$envContent = @"
# =============================================================================
# CONFIGURACIÓN - TuCentroPDF Engine V2 - Auditoría Legal
# =============================================================================

# Base de datos PostgreSQL
DB_HOST=localhost
DB_PORT=5432
DB_NAME=tucentropdf_legal
DB_USER=tucentropdf_audit
DB_PASSWORD=secure_audit_password_2024
DB_SSLMODE=prefer

# Configuración del servidor
PORT=8080
ENVIRONMENT=development
GIN_MODE=debug

# Cifrado y seguridad criptográfica
LEGAL_AUDIT_ENCRYPTION_KEY=$encryptionKey
LEGAL_AUDIT_ENCRYPTION_SALT=$encryptionSalt
LEGAL_AUDIT_HMAC_SECRET=$hmacSecret
LEGAL_AUDIT_PBKDF2_ITERATIONS=100000

# Autenticación JWT para administradores
JWT_SECRET_KEY=$jwtSecret
JWT_ADMIN_TOKEN_DURATION=8
JWT_SIGNING_ALGORITHM=HS256
JWT_ISSUER=tucentropdf-engine-v2
JWT_ADMIN_AUDIENCE=tucentropdf-legal-audit

# Configuración de auditoría legal
LEGAL_AUDIT_RETENTION_DAYS=1095
LEGAL_AUDIT_EXPORT_DIR=$exportDir
LEGAL_AUDIT_ARCHIVE_DIR=$archiveDir
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

# Logging y monitoreo
LEGAL_AUDIT_LOG_LEVEL=INFO
LEGAL_AUDIT_DEBUG_MODE=true
LEGAL_AUDIT_PERFORMANCE_METRICS=true
LEGAL_AUDIT_LOG_DIR=$logDir

# Compliance
LEGAL_AUDIT_COMPLIANCE_STANDARD=GDPR,CCPA
LEGAL_AUDIT_LEGAL_REGION=EU,US
LEGAL_AUDIT_ENABLE_ANONYMIZATION=true
LEGAL_AUDIT_ANONYMIZED_RETENTION_DAYS=2555
"@

$envContent | Out-File -FilePath "$projectDir\.env" -Encoding UTF8

Write-Host "✅ Configuración de seguridad generada" -ForegroundColor Green

# =============================================================================
# 6. COPIAR CÓDIGO FUENTE
# =============================================================================

Write-Host "📥 Preparando código fuente..." -ForegroundColor Blue

# Copiar código del proyecto actual
$currentDir = Get-Location
Copy-Item -Path "$currentDir\*" -Destination $projectDir -Recurse -Force

Write-Host "✅ Código fuente copiado" -ForegroundColor Green

# =============================================================================
# 7. EJECUTAR MIGRACIONES
# =============================================================================

Write-Host "🔧 Ejecutando migraciones de base de datos..." -ForegroundColor Blue

try {
    $migrationScript = "$projectDir\scripts\legal_audit_migration.sql"
    if (Test-Path $migrationScript) {
        & "C:\Program Files\PostgreSQL\15\bin\psql.exe" -U postgres -d tucentropdf_legal -f $migrationScript
        Write-Host "✅ Migraciones ejecutadas exitosamente" -ForegroundColor Green
    } else {
        Write-Host "⚠️ Archivo de migración no encontrado" -ForegroundColor Yellow
    }
} catch {
    Write-Host "❌ Error ejecutando migraciones: $_" -ForegroundColor Red
}

# =============================================================================
# 8. COMPILAR Y EJECUTAR APLICACIÓN
# =============================================================================

Write-Host "🔨 Compilando aplicación..." -ForegroundColor Blue

Set-Location $projectDir

# Instalar dependencias Go
Write-Host "📦 Instalando dependencias Go..." -ForegroundColor Yellow
& go mod download
& go mod tidy

# Compilar aplicación
Write-Host "🔨 Compilando aplicación..." -ForegroundColor Yellow
if (Test-Path "cmd\server\main_with_legal_audit.go") {
    & go build -o bin\tucentropdf-engine-v2.exe cmd\server\main_with_legal_audit.go
    Write-Host "✅ Aplicación compilada exitosamente" -ForegroundColor Green
} else {
    Write-Host "⚠️ Usando main.go principal" -ForegroundColor Yellow
    & go build -o bin\tucentropdf-engine-v2.exe cmd\server\main.go
}

# =============================================================================
# 9. CREAR SCRIPT DE INICIO
# =============================================================================

Write-Host "🚀 Creando scripts de inicio..." -ForegroundColor Blue

$startScript = @"
@echo off
title TuCentroPDF Engine V2 - Sistema de Auditoría Legal
echo 🚀 Iniciando TuCentroPDF Engine V2 con Auditoría Legal...
cd /d "$projectDir"
bin\tucentropdf-engine-v2.exe
pause
"@

$startScript | Out-File -FilePath "$projectDir\start.bat" -Encoding ASCII

# Crear script PowerShell para desarrollo
$startPSScript = @"
# Script de inicio para TuCentroPDF Engine V2
Write-Host "🚀 Iniciando TuCentroPDF Engine V2 - Sistema de Auditoría Legal" -ForegroundColor Blue

# Cargar variables de entorno desde .env
Get-Content .env | ForEach-Object {
    if ($_ -match "^([^#].*)=(.*)$") {
        [Environment]::SetEnvironmentVariable($matches[1], $matches[2])
    }
}

# Ejecutar aplicación
Write-Host "🌐 Servidor iniciándose en puerto 8080..." -ForegroundColor Green
Write-Host "📋 APIs de auditoría legal disponibles en /api/v2/legal-audit/" -ForegroundColor Green
Write-Host "🔐 Token de admin de ejemplo: admin-token-123" -ForegroundColor Yellow
Write-Host ""
.\bin\tucentropdf-engine-v2.exe
"@

$startPSScript | Out-File -FilePath "$projectDir\start.ps1" -Encoding UTF8

Write-Host "✅ Scripts de inicio creados" -ForegroundColor Green

# =============================================================================
# 10. MOSTRAR INFORMACIÓN FINAL
# =============================================================================

Write-Host ""
Write-Host "🎉 ¡INSTALACIÓN COMPLETADA EXITOSAMENTE!" -ForegroundColor Green -BackgroundColor DarkGreen
Write-Host ""
Write-Host "📊 Sistema de Auditoría Legal instalado con:" -ForegroundColor Blue
Write-Host "   ✓ Logs inmutables con triggers PostgreSQL" -ForegroundColor Green
Write-Host "   ✓ Cifrado AES256-GCM para datos sensibles" -ForegroundColor Green
Write-Host "   ✓ Firmas digitales HMAC-SHA256" -ForegroundColor Green
Write-Host "   ✓ Retención legal de 3 años" -ForegroundColor Green
Write-Host "   ✓ Exportación de evidencia legal" -ForegroundColor Green
Write-Host "   ✓ APIs de administración seguras" -ForegroundColor Green
Write-Host ""
Write-Host "🚀 Para iniciar el servidor:" -ForegroundColor Blue
Write-Host "   cd $projectDir" -ForegroundColor Yellow
Write-Host "   .\start.ps1" -ForegroundColor Yellow
Write-Host ""
Write-Host "🌐 Acceso una vez iniciado:" -ForegroundColor Blue
Write-Host "   - API Principal: http://localhost:8080" -ForegroundColor Cyan
Write-Host "   - Health Check:  http://localhost:8080/api/v2/legal-audit/health" -ForegroundColor Cyan
Write-Host "   - Admin Logs:    http://localhost:8080/api/v2/legal-audit/admin/logs" -ForegroundColor Cyan
Write-Host ""
Write-Host "🔐 Token de administrador para testing:" -ForegroundColor Blue
Write-Host "   Authorization: Bearer admin-token-123" -ForegroundColor Yellow
Write-Host ""
Write-Host "📁 Ubicaciones importantes:" -ForegroundColor Blue
Write-Host "   - Aplicación: $projectDir" -ForegroundColor White
Write-Host "   - Exports:    $exportDir" -ForegroundColor White
Write-Host "   - Archives:   $archiveDir" -ForegroundColor White
Write-Host "   - Logs:       $logDir" -ForegroundColor White
Write-Host ""
Write-Host "⚠️  IMPORTANTE PARA PRODUCCIÓN:" -ForegroundColor Red
Write-Host "   - Cambiar contraseñas por defecto" -ForegroundColor Yellow
Write-Host "   - Configurar HTTPS con certificados válidos" -ForegroundColor Yellow
Write-Host "   - Implementar autenticación JWT real" -ForegroundColor Yellow
Write-Host "   - Configurar backup automático" -ForegroundColor Yellow
Write-Host ""

pause