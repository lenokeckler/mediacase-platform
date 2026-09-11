# Detiene el coordinador y los workers locales (los binarios con nombre fijo de bin/).
foreach ($n in 'mediacase-coordinator','mediacase-worker-host','worker','coordinator') {
    Get-Process -Name $n -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
}
"procesos locales detenidos"
