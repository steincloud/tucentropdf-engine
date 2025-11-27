# TuCentroPDF Engine V2 - Ejecutar Servidor
# Script simplificado para Windows

Write-Host "=== TuCentroPDF Engine V2 - Iniciando Servidor ===" -ForegroundColor Green

# Buscar Go en ubicaciones comunes
$goPaths = @(
    "C:\Go\bin\go.exe",
    "C:\Program Files\Go\bin\go.exe",
    "$env:USERPROFILE\go\bin\go.exe",
    "$env:LOCALAPPDATA\Go\bin\go.exe"
)

$goExe = $null
foreach ($path in $goPaths) {
    if (Test-Path $path) {
        $goExe = $path
        Write-Host "✅ Go encontrado en: $path" -ForegroundColor Green
        break
    }
}

if (-not $goExe) {
    # Intentar buscar en PATH
    try {
        $goFromPath = Get-Command go -ErrorAction SilentlyContinue
        if ($goFromPath) {
            $goExe = $goFromPath.Source
            Write-Host "✅ Go encontrado en PATH: $goExe" -ForegroundColor Green
        }
    } catch {}
}

if (-not $goExe) {
    Write-Host "❌ Go no encontrado" -ForegroundColor Red
    Write-Host "Por favor instale Go desde: https://golang.org/dl/" -ForegroundColor Yellow
    Write-Host "O descargue el instalador para Windows x64" -ForegroundColor Yellow
    Read-Host "Presiona Enter para salir"
    exit 1
}

# Verificar versión de Go
try {
    $version = & $goExe version 2>&1
    Write-Host "📦 Versión: $version" -ForegroundColor Cyan
} catch {
    Write-Host "❌ Error al verificar versión de Go: $_" -ForegroundColor Red
    Read-Host "Presiona Enter para salir"
    exit 1
}
}

# Verificar directorio del proyecto
if (-not (Test-Path "go.mod")) {
    Write-Host "❌ go.mod no encontrado. ¿Estás en el directorio correcto?" -ForegroundColor Red
    Write-Host "Directorio actual: $(Get-Location)" -ForegroundColor Yellow
    Read-Host "Presiona Enter para salir"
    exit 1
}

Write-Host "📁 Directorio del proyecto verificado" -ForegroundColor Green

# Descargar dependencias
Write-Host "📦 Descargando dependencias..." -ForegroundColor Yellow
try {
    & $goExe mod download 2>&1
    & $goExe mod tidy 2>&1
    Write-Host "✅ Dependencias descargadas" -ForegroundColor Green
} catch {
    Write-Host "⚠️ Error descargando dependencias: $_" -ForegroundColor Yellow
}

# Verificar archivos main
$mainFiles = @(
    "cmd\server\main.go",
    "cmd\server\main_with_legal_audit.go"
)

$mainToUse = $null
foreach ($file in $mainFiles) {
    if (Test-Path $file) {
        $mainToUse = $file
        Write-Host "📄 Archivo principal encontrado: $file" -ForegroundColor Green
        break
    }
}

if (-not $mainToUse) {
    Write-Host "❌ No se encontró archivo main.go" -ForegroundColor Red
    Read-Host "Presiona Enter para salir"
    exit 1
}

# Ejecutar servidor
Write-Host "🚀 Ejecutando TuCentroPDF Engine V2..." -ForegroundColor Blue
Write-Host "🌐 El servidor estará disponible en: http://localhost:8080" -ForegroundColor Cyan
Write-Host "📝 APIs de auditoría legal en: http://localhost:8080/api/v2/legal-audit/" -ForegroundColor Cyan
Write-Host "🛑 Presiona Ctrl+C para detener el servidor" -ForegroundColor Yellow
Write-Host ""

try {
    & $goExe run $mainToUse
} catch {
    Write-Host "❌ Error ejecutando el servidor: $_" -ForegroundColor Red
    if ($mainToUse -eq "cmd\server\main.go") {
        Write-Host "🔄 Intentando con archivo alternativo..." -ForegroundColor Yellow
        if (Test-Path "cmd\server\main_with_legal_audit.go") {
            try {
                & $goExe run "cmd\server\main_with_legal_audit.go"
            } catch {
                Write-Host "❌ Error también con archivo alternativo: $_" -ForegroundColor Red
            }
        }
    }
}
}

Write-Host ""
Write-Host "🛑 Servidor detenido" -ForegroundColor Yellow
Read-Host "Presiona Enter para salir"