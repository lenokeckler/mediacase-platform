# Enciende node-1 completo con un doble clic (lo llama MediaCase.bat):
#   1. Docker Desktop (si no esta corriendo) y la infra: Postgres, Redis, MinIO, Prometheus, Grafana
#   2. detecta la IP de esta maquina en el WiFi (MINIO_PUBLIC_ENDPOINT=auto)
#   3. compila (si hay Go) y abre el coordinador y el worker local, cada uno en su ventana
#   4. abre el dashboard en el navegador
# Para apagar todo: MediaCase-detener.bat. Sin tildes: PowerShell 5.1 lee .ps1 sin BOM como ANSI.
$ErrorActionPreference = 'Continue'
$root = Split-Path $PSScriptRoot -Parent
Set-Location $root
. (Join-Path $PSScriptRoot 'env.ps1')
$Host.UI.RawUI.WindowTitle = 'MediaCase - encendiendo node-1'

function Step([string]$msg) { Write-Host ""; Write-Host "==> $msg" -ForegroundColor Cyan }
function Ok([string]$msg) { Write-Host "    $msg" -ForegroundColor Green }
function Warn([string]$msg) { Write-Host "    $msg" -ForegroundColor Yellow }

# ── 1. Docker Desktop e infraestructura ────────────────────────────────────────────────────
Step 'Docker Desktop'
$dockerOk = $false
function Test-Docker { $p = Start-Process docker -ArgumentList 'info' -NoNewWindow -Wait -PassThru -RedirectStandardOutput "$env:TEMP\mc-docker.out" -RedirectStandardError "$env:TEMP\mc-docker.err" -ErrorAction SilentlyContinue; return ($p -and $p.ExitCode -eq 0) }
$dockerOk = Test-Docker
if (-not $dockerOk) {
    $dd = Join-Path $env:ProgramFiles 'Docker\Docker\Docker Desktop.exe'
    if (-not (Test-Path $dd)) { Write-Error "No encuentro Docker Desktop en $dd"; exit 1 }
    Write-Host '    arrancando Docker Desktop (tarda ~1 min la primera vez)...'
    Start-Process $dd | Out-Null
    for ($i = 0; $i -lt 60 -and -not $dockerOk; $i++) {
        Start-Sleep 3
        $dockerOk = Test-Docker
    }
    if (-not $dockerOk) { Write-Error 'Docker Desktop no respondio en 3 minutos.'; exit 1 }
}
Ok 'Docker listo'

Step 'Infraestructura (Postgres, Redis, MinIO, Prometheus, Grafana)'
docker compose -f docker-compose.infra.yml up -d
if ($LASTEXITCODE -ne 0) { Write-Error 'fallo docker compose'; exit 1 }
Ok 'contenedores arriba'

# ── 2. IP de la red ────────────────────────────────────────────────────────────────────────
Step 'Red'
$ip = Get-LanIPv4
if (-not $ip) { Warn 'no se detecto IP de red: el dashboard funciona en localhost, pero otras PCs no van a poder conectarse' }
else { Ok "esta maquina en la red: $ip" }
$fw = Get-NetFirewallRule -DisplayName 'MediaCase node-1*' -ErrorAction SilentlyContinue
if (-not $fw) { Warn 'falta la regla de firewall (una sola vez, como administrador): scripts\firewall-node1.ps1' }

# ── 3. Coordinador y worker local, cada uno en su propia ventana ───────────────────────────
Step 'Coordinador y worker local'
if (Get-Process -Name 'mediacase-coordinator' -ErrorAction SilentlyContinue) {
    Warn 'el coordinador ya estaba corriendo; no se abre otro'
} else {
    $go = Get-Command go -ErrorAction SilentlyContinue
    if (-not $go -and -not (Test-Path 'bin\mediacase-coordinator.exe')) {
        Write-Error 'No hay Go instalado ni binarios en bin\. Instalar Go o copiar los binarios.'; exit 1
    }
    # La ruta lleva comillas: Start-Process une los argumentos con espacios y la carpeta puede tenerlos.
    Start-Process powershell -ArgumentList "-NoExit -ExecutionPolicy Bypass -File `"$(Join-Path $PSScriptRoot 'run-coordinator.ps1')`"" `
        -WorkingDirectory $root -WindowStyle Minimized | Out-Null
    # Hasta 3 min: la ventana compila antes de arrancar y la primera compilacion tras cambios
    # en el codigo (o con la cache de Go vacia) pasa del minuto.
    Write-Host '    compilando y arrancando el coordinador (hasta 3 min la primera vez)...'
    $up = $false
    for ($i = 0; $i -lt 90 -and -not $up; $i++) {
        Start-Sleep 2
        try { Invoke-WebRequest -UseBasicParsing -TimeoutSec 2 'http://localhost:8080/api/stats' | Out-Null; $up = $true } catch {}
    }
    if (-not $up) { Write-Error 'el coordinador no respondio en 3 min; mirar su ventana minimizada'; exit 1 }
    Ok 'coordinador en http://localhost:8080'
}
if (Get-Process -Name 'mediacase-worker-host' -ErrorAction SilentlyContinue) {
    Warn 'el worker local ya estaba corriendo'
} else {
    # La ruta lleva comillas: Start-Process une los argumentos con espacios y la carpeta puede tenerlos.
    Start-Process powershell -ArgumentList "-NoExit -ExecutionPolicy Bypass -File `"$(Join-Path $PSScriptRoot 'run-worker.ps1')`"" `
        -WorkingDirectory $root -WindowStyle Minimized | Out-Null
    Ok 'worker local (video) arrancando'
}

# ── 4. Navegador ───────────────────────────────────────────────────────────────────────────
Step 'Dashboard'
Start-Process 'http://localhost:8080'
Write-Host ""
Write-Host '  node-1 encendido.' -ForegroundColor Green
Write-Host ''
Write-Host "  Dashboard local:        http://localhost:8080"
if ($ip) { Write-Host "  Para otras PCs (WiFi):  http://${ip}:8080/connect" }
Write-Host '  Desde otra red:         Monitor -> Compartir -> "Publicar en internet"'
Write-Host ''
Write-Host '  El coordinador y el worker quedaron en dos ventanas minimizadas.'
Write-Host '  Para apagar todo: MediaCase-detener.bat'
Write-Host ''
