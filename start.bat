@echo off
cd /d "%~dp0"
go run ./cmd/setup
if errorlevel 1 goto failed
echo StyleShop: http://127.0.0.1:8082
go run ./cmd/server
:failed
pause
