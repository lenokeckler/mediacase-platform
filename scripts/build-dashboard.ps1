Set-Location (Join-Path $PSScriptRoot '..\dashboard')
if (-not (Test-Path node_modules)) { npm install }
npm run build
if ($LASTEXITCODE -ne 0) { Write-Error 'fallo el build del dashboard'; exit 1 }
