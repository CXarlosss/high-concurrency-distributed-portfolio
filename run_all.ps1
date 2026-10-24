# =====================================================================
# Script para iniciar los 3 servidores en segundo plano
# =====================================================================
Write-Host "Iniciando los 3 microservicios de alto rendimiento..." -ForegroundColor Cyan

# 1. Pixel War (Puerto 8080)
Start-Process -FilePath ".\server.exe" -WorkingDirectory "$PSScriptRoot\01-pixel-war" -WindowStyle Hidden
Write-Host "  [OK] Pixel War escuchando en: http://localhost:8080" -ForegroundColor Green

# 2. Arena .io (Puerto 8081)
Start-Process -FilePath ".\server.exe" -WorkingDirectory "$PSScriptRoot\02-arena-io" -WindowStyle Hidden
Write-Host "  [OK] Arena .io escuchando en: http://localhost:8081" -ForegroundColor Green

# 3. Uber Matching Radar (Puerto 8082)
Start-Process -FilePath ".\server.exe" -WorkingDirectory "$PSScriptRoot\03-uber-matching" -WindowStyle Hidden
Write-Host "  [OK] Uber Radar escuchando en: http://localhost:8082" -ForegroundColor Green

# Abrir el Portfolio Hub en el navegador
Start-Sleep -Seconds 1
Start-Process "$PSScriptRoot\index.html"
Write-Host "`nPortfolio Hub abierto en tu navegador." -ForegroundColor Yellow
Write-Host "Para detener los servidores en cualquier momento ejecuta: .\stop_all.ps1" -ForegroundColor Gray
