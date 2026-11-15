$ErrorActionPreference = "SilentlyContinue"
$conns = Get-NetTCPConnection -LocalPort 3078 -State Listen
if (-not $conns) { Write-Output "PORT_FREE"; exit 0 }
$pids = $conns | Select-Object -ExpandProperty OwningProcess | Sort-Object -Unique
foreach ($p in $pids) { Stop-Process -Id $p -Force; Write-Output ("killed " + $p) }
Start-Sleep -Seconds 2
Write-Output "DONE"