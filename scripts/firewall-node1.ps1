#Requires -RunAsAdministrator
$name = 'MediaCase node-1 (coordinator+minio)'
if (Get-NetFirewallRule -DisplayName $name -ErrorAction SilentlyContinue) {
    "La regla '$name' ya existe."
} else {
    New-NetFirewallRule -DisplayName $name -Direction Inbound -Protocol TCP -LocalPort 8080,9000 `
        -RemoteAddress 192.168.0.0/16,10.0.0.0/8,172.16.0.0/12 -Action Allow -Profile Any | Out-Null
    "Regla '$name' creada: TCP 8080,9000 desde redes privadas."
}
