@echo off
title MediaCase - apagando node-1
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\stop-node1.ps1"
echo.
echo Esta ventana se cierra sola en 5 segundos.
timeout /t 5 /nobreak >nul
