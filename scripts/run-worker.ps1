# Corre el worker local del host (node-1) con infra/env/worker-host.env.
. (Join-Path $PSScriptRoot 'env.ps1')
Import-DotEnv (Join-Path $PSScriptRoot '..\infra\env\worker-host.env')
Set-Location (Join-Path $PSScriptRoot '..')
go run ./cmd/worker
