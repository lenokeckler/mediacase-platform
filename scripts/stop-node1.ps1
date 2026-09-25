param([switch]$KeepDocker)
$root = Split-Path $PSScriptRoot -Parent
Set-Location $root
& (Join-Path $PSScriptRoot 'stop-all.ps1')
$p = Start-Process docker -ArgumentList 'info' -NoNewWindow -Wait -PassThru -RedirectStandardOutput "$env:TEMP\mc-docker.out" -RedirectStandardError "$env:TEMP\mc-docker.err" -ErrorAction SilentlyContinue
$dockerOk = ($p -and $p.ExitCode -eq 0)
if ($dockerOk) {
    docker compose -f docker-compose.infra.yml stop
    "contenedores detenidos (los datos se conservan)"
    if (-not $KeepDocker) {
        foreach ($n in 'Docker Desktop', 'com.docker.backend', 'com.docker.build', 'com.docker.dev-envs') {
            Get-Process -Name $n -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
        }
        "Docker Desktop cerrado"
    }
}
"node-1 apagado."
