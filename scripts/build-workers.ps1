Set-Location (Join-Path $PSScriptRoot '..')
New-Item -ItemType Directory -Force bin | Out-Null
$env:CGO_ENABLED = '0'
$targets = @(
    @{ GOOS = 'linux';   GOARCH = 'amd64'; Out = 'bin/worker-linux-amd64' },
    @{ GOOS = 'windows'; GOARCH = 'amd64'; Out = 'bin/worker-windows-amd64.exe' }
)
foreach ($t in $targets) {
    $env:GOOS = $t.GOOS; $env:GOARCH = $t.GOARCH
    go build -ldflags '-s -w' -o $t.Out ./cmd/worker
    if ($LASTEXITCODE -ne 0) { Write-Error "fallo compilando $($t.Out)"; exit 1 }
}
Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
Get-ChildItem bin | Select-Object Name, @{ n = 'MB'; e = { [math]::Round($_.Length / 1MB, 1) } } | Format-Table -AutoSize
