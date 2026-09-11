# Carga un archivo .env (KEY=VALUE, # comentarios) en el entorno del proceso actual.
function Import-DotEnv([string]$Path) {
    if (-not (Test-Path $Path)) {
        Write-Error "Falta $Path - copia el .example que esta al lado y ajusta las IPs."
        exit 1
    }
    Get-Content $Path | Where-Object { $_ -match '^\s*[^#].*=' } | ForEach-Object {
        $k, $v = $_ -split '=', 2
        [Environment]::SetEnvironmentVariable($k.Trim(), $v.Trim(), 'Process')
    }
}
