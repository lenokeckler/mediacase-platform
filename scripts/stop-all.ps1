foreach ($n in 'mediacase-coordinator','mediacase-worker-host','worker','coordinator','cloudflared') {
    Get-Process -Name $n -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue
}
"procesos locales detenidos"
$launchers = Get-CimInstance Win32_Process -Filter "Name='powershell.exe' or Name='cmd.exe'" -ErrorAction SilentlyContinue |
    Where-Object { $_.ProcessId -ne $PID -and $_.CommandLine -match 'scripts\\(run-coordinator|run-worker|tunnel)\.ps1|\\MediaCase\.bat' }
$launchers | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
if ($launchers) { "ventanas cerradas: $(@($launchers).Count)" }
