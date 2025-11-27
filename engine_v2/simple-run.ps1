Write-Host "=== TuCentroPDF Engine V2 ===" -ForegroundColor Green

# Buscar Go
$goExe = $null
$paths = @("C:\Go\bin\go.exe", "C:\Program Files\Go\bin\go.exe")

foreach ($path in $paths) {
    if (Test-Path $path) {
        $goExe = $path
        break
    }
}

if (-not $goExe) {
    Write-Host "Go no encontrado. Instalando..." -ForegroundColor Yellow
    exit 1
}

Write-Host "Go encontrado: $goExe" -ForegroundColor Green

# Ejecutar
Write-Host "Ejecutando servidor..." -ForegroundColor Yellow
& $goExe version
& $goExe mod tidy
& $goExe run cmd/server/main.go