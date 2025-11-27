@echo off
echo Buscando Go...

REM Buscar Go en ubicaciones comunes
if exist "C:\Go\bin\go.exe" (
    echo Go encontrado en C:\Go\bin\
    set GOPATH_FOUND=C:\Go\bin
    goto :found
)

if exist "C:\Program Files\Go\bin\go.exe" (
    echo Go encontrado en C:\Program Files\Go\bin\
    set GOPATH_FOUND=C:\Program Files\Go\bin
    goto :found
)

if exist "%USERPROFILE%\go\bin\go.exe" (
    echo Go encontrado en %USERPROFILE%\go\bin\
    set GOPATH_FOUND=%USERPROFILE%\go\bin
    goto :found
)

echo Go no encontrado en ubicaciones comunes.
echo Por favor instale Go desde https://golang.org/dl/
pause
exit /b 1

:found
echo Probando Go...
"%GOPATH_FOUND%\go.exe" version
if errorlevel 1 (
    echo Error al ejecutar Go
    pause
    exit /b 1
)

echo.
echo Go funciona correctamente!
echo Navegando al directorio del proyecto...
cd /d "%~dp0"

echo.
echo Verificando go.mod...
if not exist "go.mod" (
    echo go.mod no encontrado. Creando...
    "%GOPATH_FOUND%\go.exe" mod init github.com/tucentropdf/engine-v2
)

echo.
echo Descargando dependencias...
"%GOPATH_FOUND%\go.exe" mod tidy

echo.
echo Intentando ejecutar el servidor...
"%GOPATH_FOUND%\go.exe" run cmd\server\main.go
if errorlevel 1 (
    echo.
    echo Error con main.go, intentando con main_with_legal_audit.go...
    "%GOPATH_FOUND%\go.exe" run cmd\server\main_with_legal_audit.go
)

pause