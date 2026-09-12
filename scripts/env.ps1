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
    Resolve-AutoEndpoints
}

# IP de esta maquina en la red donde estan los demas nodos: el adaptador fisico con puerta de
# enlace (el WiFi o el cable). Se descartan los virtuales, que tambien tienen puerta de enlace
# cuando estan activos: la VPN de WARP, VirtualBox, WSL/Hyper-V.
function Get-LanIPv4 {
    $cfgs = Get-NetIPConfiguration -ErrorAction SilentlyContinue | Where-Object {
        $_.IPv4DefaultGateway -and $_.NetAdapter.Status -eq 'Up' -and
        $_.InterfaceAlias -notmatch 'WARP|VirtualBox|vEthernet|WSL|Hyper-V|VMware|Loopback'
    }
    $ip = $cfgs | ForEach-Object { $_.IPv4Address.IPAddress } | Select-Object -First 1
    if (-not $ip) {
        # Sin puerta de enlace (sin internet): cualquier IPv4 privada de un adaptador fisico.
        $ip = Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue | Where-Object {
            $_.InterfaceAlias -notmatch 'WARP|VirtualBox|vEthernet|WSL|Hyper-V|VMware|Loopback' -and
            $_.IPAddress -match '^(10\.|192\.168\.|172\.(1[6-9]|2\d|3[01])\.)'
        } | ForEach-Object { $_.IPAddress } | Select-Object -First 1
    }
    return $ip
}

# MINIO_PUBLIC_ENDPOINT=auto (o auto:9000) se reemplaza por la IP detectada, para no editar el
# .env cada vez que la laptop cambia de red.
function Resolve-AutoEndpoints {
    $v = [Environment]::GetEnvironmentVariable('MINIO_PUBLIC_ENDPOINT', 'Process')
    if ($v -and $v -match '^auto(:(\d+))?$') {
        $port = if ($Matches[2]) { $Matches[2] } else { '9000' }
        $ip = Get-LanIPv4
        if (-not $ip) { $ip = 'localhost'; Write-Warning 'no se detecto IP de red; los otros nodos no van a poder bajar archivos' }
        [Environment]::SetEnvironmentVariable('MINIO_PUBLIC_ENDPOINT', "${ip}:${port}", 'Process')
        Write-Host "MINIO_PUBLIC_ENDPOINT=auto -> ${ip}:${port}"
    }
}
