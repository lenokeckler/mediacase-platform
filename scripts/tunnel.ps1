# Publica el coordinador (puerto 8080) en internet con un "quick tunnel" de Cloudflare, sin abrir
# puertos ni tener cuenta. Imprime la URL https://xxx.trycloudflare.com; con ella, una PC en OTRA
# red abre el dashboard, baja el ZIP de /connect (que queda apuntando al tunel) y se conecta por wss.
#
# Requiere cloudflared:  winget install --id Cloudflare.cloudflared
# Uso:  scripts\tunnel.ps1          (Ctrl+C para cerrar el tunel)
#
# Limite conocido: los workers remotos tambien necesitan MinIO (9000) para bajar entradas y subir
# resultados; el tunel solo cubre el 8080. Ver docs/informe-pruebas.md, seccion "tunel".
$cf = Get-Command cloudflared -ErrorAction SilentlyContinue
if (-not $cf) {
    Write-Error "Falta cloudflared. Instalar con:  winget install --id Cloudflare.cloudflared"
    exit 1
}
Write-Host "Abriendo tunel hacia http://localhost:8080 ... (Ctrl+C para cerrar)"
& $cf.Source tunnel --url http://localhost:8080 2>&1 | ForEach-Object {
    if ($_ -match 'https://[a-z0-9-]+\.trycloudflare\.com') {
        Write-Host ""
        Write-Host "  URL publica: $($Matches[0])" -ForegroundColor Green
        Write-Host "  Dashboard:   $($Matches[0])/"
        Write-Host "  Conectar PC: $($Matches[0])/connect"
        Write-Host ""
    }
    $_
}
