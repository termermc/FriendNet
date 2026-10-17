@echo off
setlocal enableextensions

cd /d "%~dp0" || exit /b 1

go run build.go %*
