$cf = Get-Command cloudflared -ErrorAction SilentlyContinue
if (-not $cf) {
    $cf = Get-Command "$env:ProgramFiles (x86)\cloudflared\cloudflared.exe", "$env:ProgramFiles\cloudflared\cloudflared.exe" -ErrorAction SilentlyContinue | Select-Object -First 1
}
if (-not $cf) {
    Write-Error "Falta cloudflared. Instalar con:  winget install --id Cloudflare.cloudflared"
    exit 1
}
$root = Join-Path $PSScriptRoot '..'
$envFile = Join-Path $root 'infra\env\tunnel.env'
$logDir = Join-Path $env:TEMP 'mediacase-tunnel'
New-Item -ItemType Directory -Force $logDir | Out-Null

function Start-Tunnel([string]$name, [int]$port) {
    $log = Join-Path $logDir "$name.log"
    if (Test-Path $log) { Remove-Item $log -Force }
    $p = Start-Process -FilePath $cf.Source -ArgumentList "tunnel --protocol http2 --url http://localhost:$port" `
        -RedirectStandardError $log -NoNewWindow -PassThru
    $url = $null
    for ($i = 0; $i -lt 60 -and -not $url; $i++) {
        Start-Sleep -Milliseconds 500
        if (Test-Path $log) {
            $m = Select-String -Path $log -Pattern 'https://[a-z0-9-]+\.trycloudflare\.com' | Select-Object -First 1
            if ($m) { $url = $m.Matches[0].Value }
        }
        if ($p.HasExited) { break }
    }
    if (-not $url) {
        Write-Error "cloudflared no dio URL para el puerto $port (ver $log)"
        if (-not $p.HasExited) { Stop-Process -Id $p.Id -Force }
        exit 1
    }
    return @{ Process = $p; Url = $url }
}

Write-Host "Abriendo tuneles hacia localhost:8080 (coordinador) y localhost:9000 (MinIO)..."
$coord = Start-Tunnel 'coordinator' 8080
$minio = Start-Tunnel 'minio' 9000
$minioHost = ([uri]$minio.Url).Host

@(
    "# Escrito por scripts/tunnel.ps1 (se borra al cerrar el tunel). Lo lee el coordinador al generar el ZIP.",
    "COORDINATOR_TUNNEL_URL=$($coord.Url)",
    "MINIO_TUNNEL_HOST=$minioHost"
) | Set-Content -Path $envFile -Encoding ascii

Write-Host ""
Write-Host "  Dashboard:   $($coord.Url)/" -ForegroundColor Green
Write-Host "  Conectar PC: $($coord.Url)/connect" -ForegroundColor Green
Write-Host "  MinIO:       $($minio.Url)"
Write-Host ""
Write-Host "  $envFile escrito. Ctrl+C cierra los dos tuneles."
Write-Host ""

try {
    while (-not $coord.Process.HasExited -and -not $minio.Process.HasExited) { Start-Sleep 2 }
    Write-Warning "un tunel se cerro solo (ver $logDir)"
} finally {
    foreach ($t in @($coord, $minio)) {
        if (-not $t.Process.HasExited) { Stop-Process -Id $t.Process.Id -Force -ErrorAction SilentlyContinue }
    }
    Remove-Item $envFile -Force -ErrorAction SilentlyContinue
    Write-Host "tuneles cerrados."
}
