# Compila el dashboard (React/Vite) a dashboard/dist, que sirve el coordinador en http://<ip>:8080.
# Correr antes de commitear cambios del dashboard (dist/ esta versionado para no exigir Node a quien clone).
Set-Location (Join-Path $PSScriptRoot '..\dashboard')
if (-not (Test-Path node_modules)) { npm install }
npm run build
if ($LASTEXITCODE -ne 0) { Write-Error 'fallo el build del dashboard'; exit 1 }
