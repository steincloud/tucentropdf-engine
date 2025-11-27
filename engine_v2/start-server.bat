@echo off
title TuCentroPDF Engine V2 - Server Principal

echo ========================================
echo   TuCentroPDF Engine V2 - Servidor
echo ========================================

REM Agregar Go al PATH para esta sesión
set PATH=%PATH%;C:\Program Files\Go\bin

REM Verificar que Go funciona
echo Verificando Go...
go version
if errorlevel 1 (
    echo ERROR: Go no está instalado o no se encuentra en el PATH
    echo Por favor instale Go desde https://golang.org/dl/
    pause
    exit /b 1
)

echo Go encontrado y funcionando correctamente!
echo.

REM Navegar al directorio del proyecto
cd /d "%~dp0"
if not exist "go.mod" (
    echo ERROR: Archivo go.mod no encontrado
    echo Asegurate de ejecutar este script desde el directorio correcto
    pause
    exit /b 1
)

echo Compilando aplicación...
echo.

REM Intentar compilar el servidor principal
go build -o bin\tucentropdf-engine-v2.exe cmd\server\main.go
if errorlevel 1 (
    echo.
    echo ERROR: La compilación falló debido a ciclos de importación
    echo Intentando con el servidor alternativo...
    echo.
    
    REM Usar el servidor con auditoría legal que no tiene ciclos
    go build -o bin\tucentropdf-engine-v2.exe cmd\server\main_with_legal_audit.go
    if errorlevel 1 (
        echo ERROR: Falló la compilación del servidor alternativo
        echo.
        echo Esto puede deberse a dependencias faltantes.
        echo Ejecutando go mod download...
        go mod download
        go mod tidy
        echo.
        echo Reintentando compilación...
        go build -o bin\tucentropdf-engine-v2.exe cmd\server\main_with_legal_audit.go
        if errorlevel 1 (
            echo ERROR FINAL: No se pudo compilar la aplicación
            echo Revisa los logs arriba para más detalles
            pause
            exit /b 1
        )
    )
)

echo.
echo ✅ Compilación exitosa!
echo.

REM Verificar que el ejecutable existe
if not exist "bin\tucentropdf-engine-v2.exe" (
    echo ERROR: El archivo ejecutable no fue creado
    pause
    exit /b 1
)

echo 🚀 Iniciando TuCentroPDF Engine V2...
echo.
echo 📊 Sistema incluye:
echo    ✓ Motor PDF nativo con pdfcpu
echo    ✓ OCR con Tesseract + IA OpenAI
echo    ✓ Conversión Office (Word, Excel, PowerPoint)
echo    ✓ Sistema de Analytics
echo    ✓ Auditoría Legal Inmutable
echo    ✓ Límites por plan y rate limiting
echo.
echo 🌐 El servidor estará disponible en:
echo    - http://localhost:8080
echo    - APIs: http://localhost:8080/api/v2/
echo    - Docs: http://localhost:8080/docs/
echo    - Legal: http://localhost:8080/api/v2/legal-audit/
echo.
echo 💡 Para detener el servidor presiona Ctrl+C
echo.

REM Ejecutar la aplicación
bin\tucentropdf-engine-v2.exe

echo.
echo El servidor se ha detenido.
pause