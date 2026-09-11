# Corre el coordinador como proceso nativo del host (node-1) con infra/env/node1.env.
# Requiere: docker compose -f docker-compose.infra.yml up -d
. (Join-Path $PSScriptRoot 'env.ps1')
Import-DotEnv (Join-Path $PSScriptRoot '..\infra\env\node1.env')
Set-Location (Join-Path $PSScriptRoot '..')
go run ./cmd/coordinator
