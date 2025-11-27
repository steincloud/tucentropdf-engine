# Test Suite - TuCentroPDF Engine V2

## Estructura de Tests

```
tests/
├── integration/       # Tests de integración E2E
│   └── e2e_test.go
├── load/             # Load tests con K6
│   └── k6-load-test.js
└── unit/             # Tests unitarios distribuidos
    ├── handlers_test.go (internal/api/handlers/)
    ├── storage_test.go (internal/storage/)
    └── validator_test.go (internal/utils/)
```

## Ejecutar Tests

### Tests Unitarios

```powershell
# Todos los tests unitarios
go test ./internal/... -v

# Tests específicos
go test ./internal/api/handlers -v
go test ./internal/storage -v
go test ./internal/utils -v

# Con coverage
go test ./internal/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Tests de Integración

```powershell
# Requiere servidor corriendo en localhost:8080
go test ./tests/integration/... -v

# Skip tests cortos (solo integration)
go test ./tests/integration/... -v -short=false
```

### Load Tests con K6

```powershell
# Instalar K6 (Windows)
# choco install k6

# Ejecutar load test
k6 run tests/load/k6-load-test.js

# Con variables de entorno
$env:BASE_URL="http://localhost:8080"
$env:API_KEY="your-api-key"
k6 run tests/load/k6-load-test.js

# Generar reporte HTML
k6 run --out json=results.json tests/load/k6-load-test.js
```

## Configuración de Tests

### Variables de Entorno para Integration Tests

```env
# En tests/.env
BASE_URL=http://localhost:8080
API_KEY=test-api-key
TEST_TIMEOUT=30s
```

### Configuración K6

El script K6 sigue este patrón de carga:
- 0-30s: Ramp up a 10 usuarios
- 30s-1m30s: Ramp up a 20 usuarios
- 1m30s-3m30s: Mantener 20 usuarios
- 3m30s-4m: Spike a 50 usuarios
- 4m-5m: Mantener 50 usuarios
- 5m-5m30s: Ramp down a 0

## Cobertura de Tests

### Handlers (internal/api/handlers_test.go)
- ✅ GetHealth
- ✅ GetInfo
- ✅ MergePDF
- ✅ SplitPDF
- ✅ OptimizePDF
- ✅ RotatePDF
- ✅ ValidateCaptcha

### Storage (internal/storage/service_test.go)
- ✅ SaveTemp
- ✅ GetPath
- ✅ Delete
- ✅ ValidateFile
- ✅ SanitizeFilename
- ✅ ValidateMimeType
- ✅ GetPlanLimits
- ✅ GenerateOutputPath
- ✅ ReadFile
- ✅ DeletePath

### Validator (internal/utils/validator_test.go)
- ✅ ValidateMimeType (all categories)
- ✅ IsAllowedMimeType
- ✅ DetectMimeTypeFromBytes
- ✅ ValidateFileExtensionMatch
- ✅ GetCategoryForMimeType
- ✅ DetectMimeType
- ✅ MIME type normalization

### Integration E2E (tests/integration/e2e_test.go)
- ✅ Complete PDF workflow (upload → merge → split → optimize → rotate)
- ✅ Authentication (with/without API key)
- ✅ Rate limiting
- ✅ Storage endpoints

## Métricas de Performance (K6)

### Thresholds Configurados
- `http_req_duration`: P95 < 5000ms
- `http_req_failed`: Error rate < 10%
- `errors`: Custom error rate < 10%

### Custom Metrics
- `pdf_processing_duration`: Tiempo de procesamiento PDF
- `errors`: Rate de errores personalizados

## CI/CD Integration

### GitHub Actions Workflow

```yaml
name: Tests

on: [push, pull_request]

jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
        with:
          go-version: '1.24'
      - run: go test ./internal/... -v -coverprofile=coverage.out
      - run: go tool cover -func=coverage.out

  integration-tests:
    runs-on: ubuntu-latest
    services:
      postgres:
        image: postgres:15
      redis:
        image: redis:7
    steps:
      - uses: actions/checkout@v3
      - uses: actions/setup-go@v4
      - run: go build -o bin/engine ./cmd/server
      - run: ./bin/engine &
      - run: sleep 10
      - run: go test ./tests/integration/... -v

  load-tests:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - uses: grafana/setup-k6@v1
      - run: k6 run tests/load/k6-load-test.js
```

## Debugging Tests

### Logs Detallados
```powershell
go test ./internal/... -v -log-level=debug
```

### Run Test Específico
```powershell
go test ./internal/api/handlers -run TestMergePDF -v
```

### Test con Race Detector
```powershell
go test ./internal/... -race
```

## Requisitos

- Go 1.24+
- K6 (para load tests)
- PostgreSQL (para integration tests)
- Redis (para integration tests)
- Servidor corriendo en localhost:8080 (para integration/load tests)

## Resultados Esperados

### Unit Tests
- Todos los tests deben pasar
- Coverage > 70% en handlers críticos
- Sin race conditions

### Integration Tests
- E2E workflow completo exitoso
- Autenticación correcta
- Rate limiting funcional

### Load Tests
- P95 response time < 5s
- Error rate < 10%
- Sistema estable bajo carga (50 usuarios concurrentes)
