foreach ($n in 'mediacase-coordinator','mediacase-worker-host','worker','coordinator','cloudflared') {
    Get-Process -Name $n -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
}
"procesos locales detenidos"
