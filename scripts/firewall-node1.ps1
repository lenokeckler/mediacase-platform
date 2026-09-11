# Correr UNA vez, como administrador. Permite que los otros nodos lleguen al coordinador (8080)
# y a MinIO (9000) en node-1. Solo node-1 necesita esto: los workers no abren puertos
# (conexion saliente). Cubre la red host-only de VirtualBox y las redes privadas de casa.
#Requires -RunAsAdministrator
$name = 'MediaCase node-1 (coordinator+minio)'
if (Get-NetFirewallRule -DisplayName $name -ErrorAction SilentlyContinue) {
    "La regla '$name' ya existe."
} else {
    New-NetFirewallRule -DisplayName $name -Direction Inbound -Protocol TCP -LocalPort 8080,9000 `
        -RemoteAddress 192.168.0.0/16,10.0.0.0/8,172.16.0.0/12 -Action Allow -Profile Any | Out-Null
    "Regla '$name' creada: TCP 8080,9000 desde redes privadas."
}
