@echo off
rem ============================================================
rem  START Sistem Monitoring COD
rem  Membersihkan proses lama yang nyangkut, menjalankan server,
rem  lalu membuka dashboard di browser.
rem
rem  Buka UI lewat alamat ROOT:  http://localhost:8080/
rem  (jalur /api/... adalah JSON untuk perangkat, bukan tampilan)
rem ============================================================
title Sistem Monitoring COD - Server

echo [1/3] Mematikan proses aplikasi lama (bila ada)...
taskkill /IM monitoring_app.exe /F >nul 2>&1
powershell -NoProfile -Command "Get-NetTCPConnection -LocalPort 8080 -State Listen -ErrorAction SilentlyContinue | ForEach-Object { Stop-Process -Id $_.OwningProcess -Force -ErrorAction SilentlyContinue }"
timeout /t 1 /nobreak >nul

echo [2/3] Menjalankan server monitoring_app.exe ...
start "Sistem Monitoring COD - Server" monitoring_app.exe

echo [3/3] Membuka browser...
timeout /t 3 /nobreak >nul
start "" http://localhost:8080/

echo.
echo Server aktif di http://localhost:8080/
echo Gunakan http://<IP-INI>:8080 dari laptop lain di jaringan yang sama.
echo Tekan tombol apa saja untuk menutup jendela ini (server tetap berjalan)...
pause >nul