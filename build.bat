@echo off
echo Building Golang AI Agent...
echo.

REM Создаем директорию для билдов
if not exist "bin" mkdir bin

REM Собираем для Windows
echo Building for Windows...
go build -o bin\golang-ai-agent.exe -ldflags="-s -w" .

if %ERRORLEVEL% EQU 0 (
    echo.
    echo ✅ Build successful!
    echo Executable: bin\golang-ai-agent.exe
    echo.
    echo To run: bin\golang-ai-agent.exe
) else (
    echo.
    echo ❌ Build failed!
    exit /b 1
)

