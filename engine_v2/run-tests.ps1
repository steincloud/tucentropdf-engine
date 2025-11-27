#!/usr/bin/env pwsh
# Test Execution Script - TuCentroPDF Engine V2
# Usage: .\run-tests.ps1 [-Type unit|integration|load|all] [-Coverage]

param(
    [Parameter()]
    [ValidateSet('unit', 'integration', 'load', 'all')]
    [string]$Type = 'unit',
    
    [Parameter()]
    [switch]$Coverage = $false,
    
    [Parameter()]
    [switch]$Verbose = $false
)

$ErrorActionPreference = "Continue"

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "TuCentroPDF Engine V2 - Test Suite" -ForegroundColor Cyan
Write-Host "========================================`n" -ForegroundColor Cyan

# Colors
function Write-Success { param($msg) Write-Host "✓ $msg" -ForegroundColor Green }
function Write-Error { param($msg) Write-Host "✗ $msg" -ForegroundColor Red }
function Write-Info { param($msg) Write-Host "→ $msg" -ForegroundColor Yellow }
function Write-Section { param($msg) Write-Host "`n=== $msg ===" -ForegroundColor Cyan }

# Check Go installation
function Test-GoInstalled {
    try {
        $goVersion = go version
        Write-Success "Go detected: $goVersion"
        return $true
    } catch {
        Write-Error "Go not found. Please install Go 1.24+"
        return $false
    }
}

# Check K6 installation
function Test-K6Installed {
    try {
        $k6Version = k6 version
        Write-Success "K6 detected: $k6Version"
        return $true
    } catch {
        Write-Error "K6 not found. Install with: choco install k6"
        return $false
    }
}

# Run Unit Tests
function Invoke-UnitTests {
    Write-Section "Running Unit Tests"
    
    $testPaths = @(
        "./internal/api/handlers",
        "./internal/storage",
        "./internal/utils"
    )
    
    $allPassed = $true
    
    foreach ($path in $testPaths) {
        Write-Info "Testing: $path"
        
        if ($Coverage) {
            $coverFile = "coverage_$(Split-Path $path -Leaf).out"
            $result = go test $path -v -coverprofile=$coverFile 2>&1
        } elseif ($Verbose) {
            $result = go test $path -v 2>&1
        } else {
            $result = go test $path 2>&1
        }
        
        Write-Host $result
        
        if ($LASTEXITCODE -eq 0) {
            Write-Success "Tests passed: $path"
        } else {
            Write-Error "Tests failed: $path"
            $allPassed = $false
        }
        Write-Host ""
    }
    
    # Generate combined coverage
    if ($Coverage -and $allPassed) {
        Write-Info "Generating coverage report..."
        go test ./internal/... -coverprofile=coverage_combined.out | Out-Null
        go tool cover -html=coverage_combined.out -o coverage.html
        Write-Success "Coverage report: coverage.html"
    }
    
    return $allPassed
}

# Run Integration Tests
function Invoke-IntegrationTests {
    Write-Section "Running Integration Tests"
    
    Write-Info "Checking if server is running on localhost:8080..."
    try {
        $response = Invoke-WebRequest -Uri "http://localhost:8080/api/v2/health" -TimeoutSec 5 -ErrorAction Stop
        Write-Success "Server is running"
    } catch {
        Write-Error "Server not detected on localhost:8080"
        Write-Info "Start server with: .\start-server.ps1 or go run ./cmd/server"
        return $false
    }
    
    Write-Info "Running E2E tests..."
    if ($Verbose) {
        $result = go test ./tests/integration/... -v 2>&1
    } else {
        $result = go test ./tests/integration/... 2>&1
    }
    
    Write-Host $result
    
    if ($LASTEXITCODE -eq 0) {
        Write-Success "Integration tests passed"
        return $true
    } else {
        Write-Error "Integration tests failed"
        return $false
    }
}

# Run Load Tests
function Invoke-LoadTests {
    Write-Section "Running Load Tests (K6)"
    
    if (-not (Test-K6Installed)) {
        return $false
    }
    
    Write-Info "Checking if server is running on localhost:8080..."
    try {
        $response = Invoke-WebRequest -Uri "http://localhost:8080/api/v2/health" -TimeoutSec 5 -ErrorAction Stop
        Write-Success "Server is running"
    } catch {
        Write-Error "Server not detected on localhost:8080"
        Write-Info "Start server with: .\start-server.ps1 or go run ./cmd/server"
        return $false
    }
    
    Write-Info "Starting K6 load test (5+ minutes)..."
    Write-Info "Load pattern: 10→20→20→50→50→0 users"
    
    $env:BASE_URL = "http://localhost:8080"
    if (-not $env:API_KEY) {
        $env:API_KEY = "test-api-key"
        Write-Info "Using default API key: test-api-key"
    }
    
    $result = k6 run --out json=load-test-results.json tests/load/k6-load-test.js 2>&1
    Write-Host $result
    
    if ($LASTEXITCODE -eq 0) {
        Write-Success "Load tests completed"
        Write-Info "Results: load-test-results.json"
        return $true
    } else {
        Write-Error "Load tests failed"
        return $false
    }
}

# Main execution
function Main {
    if (-not (Test-GoInstalled)) {
        exit 1
    }
    
    $success = $true
    
    switch ($Type) {
        'unit' {
            $success = Invoke-UnitTests
        }
        'integration' {
            $success = Invoke-IntegrationTests
        }
        'load' {
            $success = Invoke-LoadTests
        }
        'all' {
            $unitSuccess = Invoke-UnitTests
            $integrationSuccess = Invoke-IntegrationTests
            $loadSuccess = Invoke-LoadTests
            $success = $unitSuccess -and $integrationSuccess -and $loadSuccess
        }
    }
    
    Write-Host "`n========================================" -ForegroundColor Cyan
    if ($success) {
        Write-Success "All tests completed successfully!"
        Write-Host "========================================`n" -ForegroundColor Green
        exit 0
    } else {
        Write-Error "Some tests failed. Check output above."
        Write-Host "========================================`n" -ForegroundColor Red
        exit 1
    }
}

# Run
Main
