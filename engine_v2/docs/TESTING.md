# Testing Guide - TuCentroPDF Engine V2

Esta guía explica cómo ejecutar y mantener la suite completa de tests para TuCentroPDF Engine V2.

## Tabla de Contenidos

- [Configuración Inicial](#configuración-inicial)
- [Tipos de Tests](#tipos-de-tests)
- [Ejecución de Tests](#ejecución-de-tests)
- [Tests Manuales](#tests-manuales)
- [CI/CD](#cicd)
- [Datos de Prueba](#datos-de-prueba)
- [Troubleshooting](#troubleshooting)

## Configuración Inicial

### Prerrequisitos

1. **Go 1.24+**
2. **Redis** (para tests de integración)
3. **LibreOffice** (para tests de Office)
4. **Tesseract OCR** (para tests de OCR clásico)
5. **OpenAI API Key** (para tests de AI OCR)

### Instalación de Dependencias

```bash
# Instalar dependencias Go
make deps

# Instalar herramientas de desarrollo
make install-tools
```

### Variables de Entorno para Testing

```bash
# .env.test
ENVIRONMENT=test
REDIS_HOST=localhost
REDIS_PORT=6379
OPENAI_API_KEY=your-test-api-key
OFFICE_LIBREOFFICE_PATH=soffice
LOG_LEVEL=error
```

## Tipos de Tests

### 1. Tests Unitarios
- **Ubicación**: `*_test.go` en cada paquete
- **Objetivo**: Probar funciones y métodos individuales
- **Mocks**: Utilizan interfaces mockeadas para dependencias externas

### 2. Tests de Integración
- **Ubicación**: Archivos con `Integration` en el nombre
- **Objetivo**: Probar interacciones entre componentes
- **Dependencias**: Redis, LibreOffice, Tesseract

### 3. Tests de Rendimiento
- **Ubicación**: Funciones `Benchmark*`
- **Objetivo**: Medir performance y detectar regresiones

### 4. Tests Manuales
- **Ubicación**: `test-manual.ps1`
- **Objetivo**: Validación end-to-end con API HTTP

## Ejecución de Tests

### Tests Básicos

```bash
# Todos los tests
make test

# Solo tests unitarios (rápidos)
make test-unit

# Tests con coverage
make test-coverage

# Tests específicos por componente
make test-pdf
make test-ocr
make test-office
make test-middleware
make test-config
```

### Tests de Integración

```bash
# Iniciar dependencias
make redis-start

# Ejecutar tests de integración
make test-integration

# Limpiar dependencias
make redis-stop
```

### Benchmarks

```bash
# Todos los benchmarks
make benchmark

# Benchmarks específicos
make benchmark-pdf
make benchmark-ocr
make benchmark-office
```

### Tests con Configuración Personalizada

```bash
# Tests cortos (sin dependencias externas)
make test-short

# Tests con timeout extendido
GOTEST_TIMEOUT=60m make test

# Tests con race detection
make race-test
```

## Tests Manuales

### Ejecución con PowerShell

```powershell
# Ejecutar todos los tests manuales
.\test-manual.ps1

# Ejecutar con plan específico
.\test-manual.ps1 -TestPlan "pro"

# Saltar tests de AI
.\test-manual.ps1 -SkipAITests

# Modo verbose
.\test-manual.ps1 -Verbose

# URL personalizada
.\test-manual.ps1 -BaseURL "https://api.tucentropdf.com"
```

### Tests Cubiertos por Script Manual

1. **Health Checks**
   - `/health` - Estado general del servicio
   - `/ready` - Disponibilidad para recibir tráfico

2. **PDF Operations**
   - Merge - Combinación de PDFs
   - Split - División de PDFs
   - Optimize - Optimización de tamaño
   - Info - Extracción de metadatos

3. **Office Conversion**
   - Text to PDF
   - Formatos soportados

4. **OCR Processing**
   - OCR Clásico (Tesseract)
   - OCR con IA (OpenAI)
   - Extracción estructurada

5. **Security & Limits**
   - Límites de plan
   - Validación de archivos
   - Headers de seguridad

## CI/CD

### GitHub Actions Workflow

El pipeline de CI/CD incluye:

1. **Test Job**: Ejecuta toda la suite de tests
2. **Lint Job**: Análisis de código con golangci-lint
3. **Security Job**: Escaneo de seguridad con gosec
4. **Build Job**: Compilación para múltiples plataformas
5. **Docker Job**: Construcción de imágenes Docker
6. **Performance Job**: Tests de rendimiento
7. **Release Job**: Publicación de artefactos

### Comandos de CI Local

```bash
# Simular pipeline CI completo
make ci-build

# Solo tests de CI
make ci-test

# Verificación de calidad
make check
```

### Configuración de Secrets

En GitHub Actions, configura estos secrets:

- `OPENAI_API_KEY_TEST`: API key de OpenAI para tests
- `CODECOV_TOKEN`: Token para reportes de coverage
- `GITHUB_TOKEN`: Token automático para releases

## Datos de Prueba

### Generación Automática

```bash
# Generar todos los archivos de test
make testdata-generate

# Verificar que existen los datos
make testdata-verify

# Limpiar datos de prueba
make testdata-clean
```

### Estructura de Testdata

```
testdata/
├── pdf/
│   ├── sample1.pdf      # PDF de 2 páginas
│   ├── sample2.pdf      # PDF de 5 páginas
│   ├── corrupted.pdf    # PDF dañado
│   └── encrypted.pdf    # PDF con contraseña
├── office/
│   ├── sample.txt       # Archivo de texto
│   ├── sample.docx      # Documento Word
│   ├── sample.xlsx      # Hoja de cálculo
│   └── sample.pptx      # Presentación
└── ocr/
    ├── classic/
    │   ├── clean_document.png     # Texto claro
    │   ├── multilingual.png       # Múltiples idiomas
    │   └── poor_quality.png       # Baja calidad
    └── ai/
        ├── invoice.png            # Factura
        ├── id_document.png        # Documento ID
        └── receipt.png            # Recibo
```

### Archivos de Test Personalizados

Para crear tus propios archivos de test:

1. Coloca archivos en el directorio `testdata/`
2. Sigue la convención de nombres
3. Actualiza los tests para incluir nuevos casos

## Troubleshooting

### Tests Fallidos Comunes

#### 1. Redis no disponible
```
Error: dial tcp [::1]:6379: connect: connection refused
```
**Solución**: Iniciar Redis con `make redis-start`

#### 2. LibreOffice no encontrado
```
Error: executable file not found in $PATH
```
**Solución**: Instalar LibreOffice o configurar `OFFICE_LIBREOFFICE_PATH`

#### 3. Tesseract no disponible
```
Error: exec: "tesseract": executable file not found in $PATH
```
**Solución**: Instalar Tesseract OCR

#### 4. OpenAI API Key inválida
```
Error: 401 Unauthorized
```
**Solución**: Configurar `OPENAI_API_KEY` válida

### Tests Lentos

Si los tests tardan mucho:

```bash
# Ejecutar solo tests rápidos
make test-short

# Tests sin benchmarks
go test -short ./...

# Tests de un componente específico
go test ./internal/pdf/...
```

### Problemas de Memory/Race Conditions

```bash
# Tests con race detection
make race-test

# Profiling de memoria
make memory-profile

# Profiling de CPU
make cpu-profile
```

### Depuración de Tests

```bash
# Tests con output verbose
make test-verbose

# Test específico con debug
go test -v -run TestSpecificFunction ./internal/pdf/

# Tests con logs
LOG_LEVEL=debug make test-unit
```

## Configuración IDE

### VS Code

Configuración recomendada en `.vscode/settings.json`:

```json
{
  "go.testFlags": ["-v", "-race"],
  "go.testTimeout": "30m",
  "go.testOnSave": true,
  "go.coverOnSave": true,
  "go.coverageDecorator": "highlight"
}
```

### Testing Tasks

Configuración de tasks en `.vscode/tasks.json`:

```json
{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "Test All",
      "type": "shell",
      "command": "make test",
      "group": "test"
    },
    {
      "label": "Test Current Package",
      "type": "shell",
      "command": "go test -v ./...",
      "options": {
        "cwd": "${fileDirname}"
      },
      "group": "test"
    }
  ]
}
```

## Métricas y Reporting

### Coverage Reports

```bash
# Generar reporte HTML
make test-coverage-html

# Ver reporte
open coverage/coverage.html
```

### Benchmark Reports

```bash
# Ejecutar benchmarks y guardar resultados
go test -bench=. -benchmem ./... > benchmark_results.txt

# Comparar con resultados anteriores
benchcmp old_results.txt benchmark_results.txt
```

## Contribuir a los Tests

### Guidelines para Nuevos Tests

1. **Nomenclatura**: Usar `TestComponentName_FunctionName`
2. **Table-driven tests**: Para múltiples casos
3. **Mocks**: Para dependencias externas
4. **Cleanup**: Limpiar recursos creados
5. **Deterministic**: Tests que siempre den el mismo resultado

### Estructura de Test Recomendada

```go
func TestService_Method(t *testing.T) {
    // Setup
    service := setupTestService()
    defer service.cleanup()

    // Table-driven tests
    tests := []struct {
        name        string
        input       InputType
        expectError bool
        expected    ExpectedType
    }{
        // Test cases
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            // Execute
            result, err := service.Method(tt.input)
            
            // Assert
            if tt.expectError {
                assert.Error(t, err)
                return
            }
            
            assert.NoError(t, err)
            assert.Equal(t, tt.expected, result)
        })
    }
}
```

---

Para más información, consulta la documentación específica de cada componente o el código fuente de los tests.