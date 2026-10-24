# =====================================================================
# Script para detener los servidores en segundo plano
# =====================================================================
Write-Host "Deteniendo los servidores..." -ForegroundColor Cyan

Get-Process -Name server -ErrorAction SilentlyContinue | Stop-Process -Force
Get-Process -Name bot -ErrorAction SilentlyContinue | Stop-Process -Force

Write-Host "[OK] Todos los servidores han sido detenidos correctamente." -ForegroundColor Green
