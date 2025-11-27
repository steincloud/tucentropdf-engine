#!/usr/bin/pwsh
# TuCentroPDF Engine - http_server.ps1
# API HTTP modular
param([int]$Port = 8080)

$listener = [System.Net.HttpListener]::new()
$listener.Prefixes.Add("http://*:$Port/")
$listener.Start()
Write-Host "🚀 TuCentroPDF Engine HTTP server escuchando en puerto $Port ..."

function Send-JsonResponse($context, [int]$status, $data) {
    $response = $context.Response
    $json = ($data | ConvertTo-Json -Depth 5)
    $buffer = [System.Text.Encoding]::UTF8.GetBytes($json)
    $response.StatusCode = $status
    $response.ContentType = "application/json"
    $response.ContentLength64 = $buffer.Length
    $response.OutputStream.Write($buffer, 0, $buffer.Length)
    $response.Close()
}

function Run-Command($cmd) {
    try {
        $pinfo = New-Object System.Diagnostics.ProcessStartInfo
        $pinfo.FileName = "bash"
        $pinfo.Arguments = "-c `"$cmd`""
        $pinfo.RedirectStandardOutput = $true
        $pinfo.RedirectStandardError = $true
        $pinfo.UseShellExecute = $false
        $pinfo.CreateNoWindow = $true
        $process = New-Object System.Diagnostics.Process
        $process.StartInfo = $pinfo
        $process.Start() | Out-Null
        $stdout = $process.StandardOutput.ReadToEnd()
        $stderr = $process.StandardError.ReadToEnd()
        $process.WaitForExit()
        return @{ stdout = $stdout.Trim(); stderr = $stderr.Trim(); code = $process.ExitCode }
    }
    catch {
        return @{ stdout = ""; stderr = $_.Exception.Message; code = 1 }
    }
}

while ($true) {
    $context = $listener.GetContext()
    $request = $context.Request
    $path = $request.Url.AbsolutePath.ToLower()

    if ($request.HttpMethod -eq "POST" -and $path -eq "/task") {
        try {
            $body = New-Object System.IO.StreamReader($request.InputStream).ReadToEnd()
            $data = $body | ConvertFrom-Json
            $tool = $data.tool
            $input = $data.input
            $output = $data.output

            switch ($tool) {
                "compress" { $cmd = "pdfcpu compress `"$input`" `"$output`"" }
                "merge"    { $files = ($data.files -join " "); $cmd = "pdfcpu merge `"$output`" $files" }
                "split"    { $cmd = "pdfcpu split `"$input`" `"$output`"" }
                "info"     { $cmd = "pdfcpu info `"$input`"" }
                "ocr"      { $cmd = "tesseract `"$input`" `"$output`" -l spa" }
                "office"   { $cmd = "libreoffice --headless --convert-to pdf `"$input`" --outdir `"$output`"" }
                "html"     { $cmd = "wkhtmltopdf `"$input`" `"$output`"" }
                default    { Send-JsonResponse $context 400 @{ error = "Herramienta '$tool' no reconocida." }; continue }
            }

            $result = Run-Command $cmd
            $status = if ($result.code -eq 0) { 200 } else { 500 }
            Send-JsonResponse $context $status @{
                tool = $tool
                command = $cmd
                exitCode = $result.code
                stdout = $result.stdout
                stderr = $result.stderr
            }
        }
        catch {
            Send-JsonResponse $context 500 @{ error = $_.Exception.Message }
        }
    }
    else {
        Send-JsonResponse $context 404 @{ error = "Ruta no encontrada" }
    }
}
