@echo off
title Editor OPD - Servidor local
cd /d "%~dp0"
powershell -NoProfile -STA -ExecutionPolicy Bypass -File "%~dp0EditorOPD-Servidor.ps1"
echo.
echo El servidor se detuvo. Pulsa una tecla para cerrar.
pause >nul
