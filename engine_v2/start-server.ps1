# =============================================================================
# TuCentroPDF Engine V2 - Script de Inicio para Windows
# =============================================================================

Write-Host "========================================" -ForegroundColor Blue
Write-Host "   TuCentroPDF Engine V2 - Servidor" -ForegroundColor Blue  
Write-Host "========================================" -ForegroundColor Blue
Write-Host ""

# Configurar PATH para Go
$env:PATH += ";C:\Program Files\Go\bin"

# Verificar Go
Write-Host "🔍 Verificando Go..." -ForegroundColor Yellow
try {
    $goVersion = go version 2>$null
    if ($LASTEXITCODE -eq 0) {
        Write-Host "✅ $goVersion" -ForegroundColor Green
    } else {
        throw "Go no encontrado"
    }
} catch {
    Write-Host "❌ ERROR: Go no está instalado o no se encuentra en el PATH" -ForegroundColor Red
    Write-Host "Por favor instale Go desde https://golang.org/dl/" -ForegroundColor Yellow
    pause
    exit 1
}

# Verificar directorio del proyecto
if (-not (Test-Path "go.mod")) {
    Write-Host "❌ ERROR: go.mod no encontrado" -ForegroundColor Red
    Write-Host "Ejecute este script desde el directorio del proyecto" -ForegroundColor Yellow
    pause
    exit 1
}

# Crear directorio bin si no existe
if (-not (Test-Path "bin")) {
    New-Item -ItemType Directory -Path "bin" | Out-Null
}

Write-Host "🔨 Compilando aplicación..." -ForegroundColor Yellow
Write-Host ""

# Intentar compilar servidor principal
Write-Host "📦 Intentando compilar servidor principal..." -ForegroundColor Cyan
$compileResult = go build -o bin\tucentropdf-engine-v2.exe cmd\server\main.go 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "⚠️ Compilación del servidor principal falló (ciclos de importación)" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "📦 Compilando servidor con auditoría legal..." -ForegroundColor Cyan
    
    # Usar servidor alternativo
    $compileResult = go build -o bin\tucentropdf-engine-v2.exe cmd\server\main_with_legal_audit.go 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "❌ Error en compilación del servidor alternativo:" -ForegroundColor Red
        Write-Host $compileResult -ForegroundColor Red
        Write-Host ""
        Write-Host "📦 Descargando dependencias..." -ForegroundColor Yellow
        go mod download
        go mod tidy
        Write-Host ""
        Write-Host "📦 Reintentando compilación..." -ForegroundColor Cyan
        $compileResult = go build -o bin\tucentropdf-engine-v2.exe cmd\server\main_with_legal_audit.go 2>&1
        if ($LASTEXITCODE -ne 0) {
            Write-Host "❌ ERROR FINAL: No se pudo compilar la aplicación" -ForegroundColor Red
            Write-Host $compileResult -ForegroundColor Red
            pause
            exit 1
        }
    }
}

# Verificar ejecutable
if (-not (Test-Path "bin\tucentropdf-engine-v2.exe")) {
    Write-Host "❌ ERROR: El ejecutable no fue creado" -ForegroundColor Red
    pause
    exit 1
}

Write-Host ""
Write-Host "✅ Compilación exitosa!" -ForegroundColor Green
Write-Host ""

Write-Host "🚀 Iniciando TuCentroPDF Engine V2..." -ForegroundColor Blue
Write-Host ""
Write-Host "📊 Sistema incluye:" -ForegroundColor Cyan
Write-Host "   ✓ Motor PDF nativo con pdfcpu" -ForegroundColor Green
Write-Host "   ✓ OCR con Tesseract + IA OpenAI" -ForegroundColor Green
Write-Host "   ✓ Conversión Office (Word, Excel, PowerPoint)" -ForegroundColor Green
Write-Host "   ✓ Sistema de Analytics" -ForegroundColor Green
Write-Host "   ✓ Auditoría Legal Inmutable" -ForegroundColor Green
Write-Host "   ✓ Límites por plan y rate limiting" -ForegroundColor Green
Write-Host ""
Write-Host "🌐 El servidor estará disponible en:" -ForegroundColor Cyan
Write-Host "   - Principal: http://localhost:8080" -ForegroundColor White
Write-Host "   - APIs: http://localhost:8080/api/v2/" -ForegroundColor White
Write-Host "   - Docs: http://localhost:8080/docs/" -ForegroundColor White
Write-Host "   - Legal: http://localhost:8080/api/v2/legal-audit/" -ForegroundColor White
Write-Host ""
Write-Host "💡 Para detener el servidor presiona Ctrl+C" -ForegroundColor Yellow
Write-Host ""

# Ejecutar la aplicación
try {
    & .\bin\tucentropdf-engine-v2.exe
} catch {
    Write-Host "❌ Error al ejecutar la aplicación: $_" -ForegroundColor Red
}

Write-Host ""
Write-Host "🛑 El servidor se ha detenido." -ForegroundColor Yellow
pause