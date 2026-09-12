# Corre el worker local del host (node-1) con infra/env/worker-host.env.
. (Join-Path $PSScriptRoot 'env.ps1')
Import-DotEnv (Join-Path $PSScriptRoot '..\infra\env\worker-host.env')
Set-Location (Join-Path $PSScriptRoot '..')
# Compilar a un binario con nombre fijo (no 'go run': deja procesos huerfanos sin nombre en el puerto).
New-Item -ItemType Directory -Force bin | Out-Null
if (Get-Command go -ErrorAction SilentlyContinue) {
    go build -o bin/mediacase-worker-host.exe ./cmd/worker
    if ($LASTEXITCODE -ne 0) { Write-Error 'fallo la compilacion'; exit 1 }
} elseif (-not (Test-Path bin/mediacase-worker-host.exe)) {
    Write-Error 'no hay Go instalado ni bin/mediacase-worker-host.exe'; exit 1
}
& (Join-Path $PWD "bin/mediacase-worker-host.exe")
