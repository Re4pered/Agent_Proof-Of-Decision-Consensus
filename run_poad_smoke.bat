@echo off
rem Run independent strict PoAD 80-payment completion smoke test.
rem The PowerShell runner archives previous smoke outputs and captures logs.
cd /d "%~dp0"
powershell.exe -NoProfile -ExecutionPolicy Bypass -File ".\scripts\run_poad_smoke.ps1"
exit /b %ERRORLEVEL%
