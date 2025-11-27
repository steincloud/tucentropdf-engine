# Manual Testing Script para TuCentroPDF Engine V2
# Este script ejecuta pruebas manuales de todos los componentes

param(
    [string]$BaseURL = "http://localhost:8080",
    [string]$TestPlan = "premium",
    [switch]$SkipAITests = $false,
    [switch]$Verbose = $false
)

# Configuración
$ErrorActionPreference = "Continue"
$Script:TestResults = @()
$Script:PassedTests = 0
$Script:FailedTests = 0

# Colores para output
function Write-Success($message) {
    Write-Host "✅ $message" -ForegroundColor Green
}

function Write-Error($message) {
    Write-Host "❌ $message" -ForegroundColor Red
}

function Write-Warning($message) {
    Write-Host "⚠️  $message" -ForegroundColor Yellow
}

function Write-Info($message) {
    Write-Host "ℹ️  $message" -ForegroundColor Cyan
}

function Write-Header($message) {
    Write-Host "`n" + "="*60 -ForegroundColor Magenta
    Write-Host $message -ForegroundColor Magenta
    Write-Host "="*60 -ForegroundColor Magenta
}

# Función para ejecutar test
function Invoke-Test {
    param(
        [string]$Name,
        [scriptblock]$TestBlock
    )
    
    Write-Info "Ejecutando: $Name"
    
    try {
        $result = & $TestBlock
        if ($result -eq $true) {
            Write-Success "$Name - PASSED"
            $Script:PassedTests++
            $Script:TestResults += [PSCustomObject]@{
                Test = $Name
                Status = "PASSED"
                Error = $null
            }
        } else {
            Write-Error "$Name - FAILED"
            $Script:FailedTests++
            $Script:TestResults += [PSCustomObject]@{
                Test = $Name
                Status = "FAILED"
                Error = "Test returned false"
            }
        }
    }
    catch {
        Write-Error "$Name - ERROR: $($_.Exception.Message)"
        $Script:FailedTests++
        $Script:TestResults += [PSCustomObject]@{
            Test = $Name
            Status = "ERROR"
            Error = $_.Exception.Message
        }
    }
}

# Función para hacer request HTTP
function Invoke-APIRequest {
    param(
        [string]$Method = "GET",
        [string]$Endpoint,
        [object]$Body = $null,
        [hashtable]$Headers = @{},
        [string]$ContentType = "application/json"
    )
    
    $uri = "$BaseURL$Endpoint"
    $defaultHeaders = @{
        "X-User-Plan" = $TestPlan
        "X-User-ID" = "test-user-123"
    }
    
    $allHeaders = $defaultHeaders + $Headers
    
    try {
        $params = @{
            Uri = $uri
            Method = $Method
            Headers = $allHeaders
        }
        
        if ($Body) {
            if ($ContentType -eq "application/json") {
                $params.Body = ($Body | ConvertTo-Json -Depth 10)
                $params.ContentType = $ContentType
            } else {
                $params.Body = $Body
                $params.ContentType = $ContentType
            }
        }
        
        if ($Verbose) {
            Write-Info "Request: $Method $uri"
        }
        
        $response = Invoke-RestMethod @params
        return $response
    }
    catch {
        if ($Verbose) {
            Write-Warning "Request failed: $($_.Exception.Message)"
        }
        throw
    }
}

# Función para hacer request con archivo
function Invoke-FileUploadRequest {
    param(
        [string]$Endpoint,
        [string]$FilePath,
        [hashtable]$AdditionalFields = @{},
        [hashtable]$Headers = @{}
    )
    
    if (-not (Test-Path $FilePath)) {
        throw "File not found: $FilePath"
    }
    
    $uri = "$BaseURL$Endpoint"
    $defaultHeaders = @{
        "X-User-Plan" = $TestPlan
        "X-User-ID" = "test-user-123"
    }
    
    $allHeaders = $defaultHeaders + $Headers
    
    try {
        # Crear multipart form data
        $boundary = [System.Guid]::NewGuid().ToString()
        $LF = "`n"
        
        $bodyLines = @(
            "--$boundary"
            "Content-Disposition: form-data; name=`"file`"; filename=`"$(Split-Path $FilePath -Leaf)`""
            "Content-Type: application/octet-stream"
            ""
        )
        
        $bodyString = $bodyLines -join $LF
        $fileBytes = [System.IO.File]::ReadAllBytes($FilePath)
        
        $bodyLines = @(
            ""
            "--$boundary--"
        )
        $bodyStringEnd = $bodyLines -join $LF
        
        # Combinar todo
        $bodyBytes = [System.Text.Encoding]::UTF8.GetBytes($bodyString) + $fileBytes + [System.Text.Encoding]::UTF8.GetBytes($bodyStringEnd)
        
        # Agregar campos adicionales
        foreach ($field in $AdditionalFields.GetEnumerator()) {
            $fieldData = @(
                "--$boundary"
                "Content-Disposition: form-data; name=`"$($field.Key)`""
                ""
                $field.Value
            ) -join $LF
            $fieldBytes = [System.Text.Encoding]::UTF8.GetBytes($fieldData)
            $bodyBytes = $fieldBytes + $bodyBytes
        }
        
        $allHeaders["Content-Type"] = "multipart/form-data; boundary=$boundary"
        
        $response = Invoke-WebRequest -Uri $uri -Method POST -Body $bodyBytes -Headers $allHeaders
        
        if ($response.StatusCode -ge 200 -and $response.StatusCode -lt 300) {
            return ($response.Content | ConvertFrom-Json)
        } else {
            throw "HTTP $($response.StatusCode): $($response.StatusDescription)"
        }
    }
    catch {
        if ($Verbose) {
            Write-Warning "File upload failed: $($_.Exception.Message)"
        }
        throw
    }
}

# Tests de Health Check
Write-Header "HEALTH CHECKS"

Invoke-Test "Health Check" {
    try {
        $response = Invoke-APIRequest -Endpoint "/health"
        return $response.status -eq "ok"
    }
    catch {
        return $false
    }
}

Invoke-Test "Ready Check" {
    try {
        $response = Invoke-APIRequest -Endpoint "/ready"
        return $response.status -eq "ready"
    }
    catch {
        return $false
    }
}

# Tests de PDF
Write-Header "PDF OPERATIONS TESTS"

# Crear archivos de prueba si no existen
$testDataDir = "testdata"
if (-not (Test-Path $testDataDir)) {
    New-Item -ItemType Directory -Path $testDataDir -Force | Out-Null
}

# Crear PDF de prueba simple
$testPDFPath = "$testDataDir/test_manual.pdf"
if (-not (Test-Path $testPDFPath)) {
    $pdfContent = "%PDF-1.4`n1 0 obj<</Type/Catalog/Pages 2 0 R>>endobj`n2 0 obj<</Type/Pages/Kids[3 0 R]/Count 1>>endobj`n3 0 obj<</Type/Page/Parent 2 0 R/MediaBox[0 0 612 792]>>endobj`nxref`n0 4`n0000000000 65535 f `n0000000010 00000 n `n0000000053 00000 n `n0000000103 00000 n `ntrailer<</Size 4/Root 1 0 R>>`nstartxref`n151`n%%EOF"
    [System.IO.File]::WriteAllText($testPDFPath, $pdfContent)
}

Invoke-Test "PDF Merge" {
    try {
        $response = Invoke-FileUploadRequest -Endpoint "/api/v2/pdf/merge" -FilePath $testPDFPath -AdditionalFields @{
            "files" = "2"
        }
        return $response.success -eq $true
    }
    catch {
        return $false
    }
}

Invoke-Test "PDF Split" {
    try {
        $response = Invoke-FileUploadRequest -Endpoint "/api/v2/pdf/split" -FilePath $testPDFPath -AdditionalFields @{
            "pages_per_file" = "1"
        }
        return $response.success -eq $true
    }
    catch {
        return $false
    }
}

Invoke-Test "PDF Optimize" {
    try {
        $response = Invoke-FileUploadRequest -Endpoint "/api/v2/pdf/optimize" -FilePath $testPDFPath -AdditionalFields @{
            "quality" = "medium"
        }
        return $response.success -eq $true
    }
    catch {
        return $false
    }
}

Invoke-Test "PDF Info" {
    try {
        $response = Invoke-FileUploadRequest -Endpoint "/api/v2/pdf/info" -FilePath $testPDFPath
        return $response.page_count -gt 0
    }
    catch {
        return $false
    }
}

# Tests de Office
Write-Header "OFFICE CONVERSION TESTS"

# Crear archivo de texto de prueba
$testTxtPath = "$testDataDir/test_manual.txt"
if (-not (Test-Path $testTxtPath)) {
    "Este es un documento de prueba para conversión a PDF.`nContiene múltiples líneas.`nY algunos caracteres especiales: áéíóú ñ" | Out-File -FilePath $testTxtPath -Encoding UTF8
}

Invoke-Test "Office Text to PDF" {
    try {
        $response = Invoke-FileUploadRequest -Endpoint "/api/v2/office/convert" -FilePath $testTxtPath
        return $response.success -eq $true
    }
    catch {
        return $false
    }
}

Invoke-Test "Office Supported Formats" {
    try {
        $response = Invoke-APIRequest -Endpoint "/api/v2/office/formats"
        return $response.formats.Count -gt 0
    }
    catch {
        return $false
    }
}

# Tests de OCR (si no se saltan)
if (-not $SkipAITests) {
    Write-Header "OCR TESTS"

    # Crear imagen de prueba (texto simple)
    $testImagePath = "$testDataDir/test_text.png"
    if (-not (Test-Path $testImagePath)) {
        Write-Warning "Imagen de prueba no encontrada en $testImagePath"
        Write-Info "Creando imagen de texto simple..."
        
        # Crear una imagen simple con texto usando PowerShell y .NET
        Add-Type -AssemblyName System.Drawing
        $bitmap = New-Object System.Drawing.Bitmap(400, 200)
        $graphics = [System.Drawing.Graphics]::FromImage($bitmap)
        $graphics.Clear([System.Drawing.Color]::White)
        
        $font = New-Object System.Drawing.Font("Arial", 20)
        $brush = New-Object System.Drawing.SolidBrush([System.Drawing.Color]::Black)
        $graphics.DrawString("TEXTO DE PRUEBA", $font, $brush, 50, 50)
        $graphics.DrawString("Manual Testing", $font, $brush, 50, 100)
        
        $bitmap.Save($testImagePath, [System.Drawing.Imaging.ImageFormat]::Png)
        $bitmap.Dispose()
        $graphics.Dispose()
    }

    Invoke-Test "OCR Classic" {
        try {
            $response = Invoke-FileUploadRequest -Endpoint "/api/v2/ocr/extract" -FilePath $testImagePath -AdditionalFields @{
                "language" = "spa"
                "engine" = "classic"
            }
            return $response.text.Length -gt 0
        }
        catch {
            return $false
        }
    }

    Invoke-Test "OCR AI" {
        try {
            $response = Invoke-FileUploadRequest -Endpoint "/api/v2/ocr/extract" -FilePath $testImagePath -AdditionalFields @{
                "engine" = "ai"
            }
            return $response.text.Length -gt 0
        }
        catch {
            return $false
        }
    }

    Invoke-Test "OCR Structured Data" {
        try {
            $response = Invoke-FileUploadRequest -Endpoint "/api/v2/ocr/extract-structured" -FilePath $testImagePath -AdditionalFields @{
                "document_type" = "general"
            }
            return $response.data -ne $null
        }
        catch {
            return $false
        }
    }
} else {
    Write-Warning "Saltando tests de AI OCR (usar -SkipAITests:$false para incluir)"
}

# Tests de límites de plan
Write-Header "PLAN LIMITS TESTS"

# Test con archivo demasiado grande (simulado)
Invoke-Test "File Size Limit Check" {
    try {
        # Intentar subir con header que simula archivo grande
        $headers = @{
            "Content-Length" = "50000000"  # 50MB
        }
        
        if ($TestPlan -eq "free") {
            # Para plan free, debería fallar
            $response = Invoke-FileUploadRequest -Endpoint "/api/v2/pdf/info" -FilePath $testPDFPath -Headers $headers
            return $false  # Si llega aquí, el límite no funcionó
        } else {
            # Para otros planes, podría funcionar
            return $true
        }
    }
    catch {
        # Se espera error para plan free
        return $TestPlan -eq "free"
    }
}

# Tests de métricas
Write-Header "METRICS AND MONITORING"

Invoke-Test "Metrics Endpoint" {
    try {
        $response = Invoke-WebRequest -Uri "$BaseURL/metrics" -Method GET
        return $response.StatusCode -eq 200
    }
    catch {
        return $false
    }
}

# Tests de seguridad básicos
Write-Header "SECURITY TESTS"

Invoke-Test "CORS Headers" {
    try {
        $response = Invoke-WebRequest -Uri "$BaseURL/health" -Method OPTIONS
        $corsHeader = $response.Headers["Access-Control-Allow-Origin"]
        return $corsHeader -ne $null
    }
    catch {
        return $false
    }
}

Invoke-Test "Security Headers" {
    try {
        $response = Invoke-WebRequest -Uri "$BaseURL/health" -Method GET
        $securityHeaders = @(
            "X-Content-Type-Options",
            "X-Frame-Options",
            "X-XSS-Protection"
        )
        
        foreach ($header in $securityHeaders) {
            if (-not $response.Headers.ContainsKey($header)) {
                Write-Warning "Missing security header: $header"
            }
        }
        return $true
    }
    catch {
        return $false
    }
}

# Test de carga de archivos maliciosos
Invoke-Test "Malicious File Rejection" {
    try {
        $maliciousPath = "$testDataDir/malicious.exe"
        "MZ" | Out-File -FilePath $maliciousPath -Encoding ASCII -NoNewline
        
        $response = Invoke-FileUploadRequest -Endpoint "/api/v2/pdf/info" -FilePath $maliciousPath
        return $false  # No debería llegar aquí
    }
    catch {
        # Se espera que falle
        Remove-Item $maliciousPath -ErrorAction SilentlyContinue
        return $true
    }
}

# Resumen final
Write-Header "TEST SUMMARY"

$totalTests = $Script:PassedTests + $Script:FailedTests
Write-Info "Tests ejecutados: $totalTests"
Write-Success "Tests exitosos: $Script:PassedTests"
if ($Script:FailedTests -gt 0) {
    Write-Error "Tests fallidos: $Script:FailedTests"
} else {
    Write-Success "Tests fallidos: $Script:FailedTests"
}

if ($Script:FailedTests -eq 0) {
    Write-Success "🎉 TODOS LOS TESTS PASARON!"
} else {
    Write-Error "🚨 ALGUNOS TESTS FALLARON"
    
    Write-Host "`nTests fallidos:" -ForegroundColor Red
    $Script:TestResults | Where-Object { $_.Status -ne "PASSED" } | ForEach-Object {
        Write-Host "  - $($_.Test): $($_.Status)" -ForegroundColor Red
        if ($_.Error) {
            Write-Host "    Error: $($_.Error)" -ForegroundColor DarkRed
        }
    }
}

# Guardar reporte
$reportPath = "test-report-$(Get-Date -Format 'yyyyMMdd-HHmmss').json"
$Script:TestResults | ConvertTo-Json -Depth 3 | Out-File -FilePath $reportPath -Encoding UTF8
Write-Info "Reporte guardado en: $reportPath"

# Limpiar archivos temporales
Remove-Item "$testDataDir/test_manual.*" -ErrorAction SilentlyContinue
Write-Info "Archivos temporales eliminados"

# Exit code basado en resultados
exit $Script:FailedTests