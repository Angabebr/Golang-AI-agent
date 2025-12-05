# Скрипт проверки окружения

Write-Host "=== Проверка окружения ===" -ForegroundColor Cyan
Write-Host ""

# Проверка .env
Write-Host "1. Проверка .env файла:" -ForegroundColor Yellow
if (Test-Path .env) {
    $content = Get-Content .env -Raw
    if ($content -match 'OPENAI_API_KEY\s*=') {
        Write-Host "   [OK] .env файл найден и содержит OPENAI_API_KEY" -ForegroundColor Green
    } else {
        Write-Host "   [ERROR] .env файл не содержит OPENAI_API_KEY" -ForegroundColor Red
    }
} else {
    Write-Host "   [ERROR] .env файл не найден" -ForegroundColor Red
    Write-Host "   Создайте .env файл из .env.example" -ForegroundColor Yellow
}

Write-Host ""

# Проверка Chrome
Write-Host "2. Проверка Chrome/Chromium:" -ForegroundColor Yellow
$chromePaths = @(
    "C:\Program Files\Google\Chrome\Application\chrome.exe",
    "C:\Program Files (x86)\Google\Chrome\Application\chrome.exe",
    "$env:LOCALAPPDATA\Google\Chrome\Application\chrome.exe",
    "C:\Program Files\Chromium\Application\chrome.exe"
)

$found = $false
foreach ($path in $chromePaths) {
    if (Test-Path $path) {
        Write-Host "   [OK] Chrome найден: $path" -ForegroundColor Green
        $found = $true
        break
    }
}

if (-not $found) {
    Write-Host "   [ERROR] Chrome не найден" -ForegroundColor Red
    Write-Host "   Установите Chrome: https://www.google.com/chrome/" -ForegroundColor Yellow
}

Write-Host ""

# Проверка Go
Write-Host "3. Проверка Go:" -ForegroundColor Yellow
$goVersion = go version 2>&1
if ($LASTEXITCODE -eq 0) {
    Write-Host "   [OK] $goVersion" -ForegroundColor Green
} else {
    Write-Host "   [ERROR] Go не установлен или не в PATH" -ForegroundColor Red
}

Write-Host ""
Write-Host "=== Конец проверки ===" -ForegroundColor Cyan

