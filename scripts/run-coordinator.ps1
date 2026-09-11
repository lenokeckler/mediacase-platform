# Corre el coordinador como proceso nativo del host (node-1) con infra/env/node1.env.
# Requiere: docker compose -f docker-compose.infra.yml up -d
. (Join-Path $PSScriptRoot 'env.ps1')
Import-DotEnv (Join-Path $PSScriptRoot '..\infra\env\node1.env')
Set-Location (Join-Path $PSScriptRoot '..')
# Compilar a un binario con nombre fijo (no 'go run': deja procesos huerfanos sin nombre en el puerto).
New-Item -ItemType Directory -Force bin | Out-Null
go build -o bin/mediacase-coordinator.exe ./cmd/coordinator
if ($LASTEXITCODE -ne 0) { Write-Error 'fallo la compilacion'; exit 1 }
& (Join-Path $PWD "bin/mediacase-coordinator.exe")
