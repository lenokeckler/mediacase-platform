@echo off
title MediaCase - apagando node-1
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\stop-node1.ps1"
echo.
pause
