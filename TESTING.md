# Руководство по тестированию

## Запуск тестов

### Все тесты
```bash
go test ./... -v
```

### Конкретный пакет
```bash
go test ./context -v
go test ./security -v
go test ./subagents -v
```

### С покрытием кода
```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out -o coverage.html
```

Откройте `coverage.html` в браузере для просмотра покрытия.

## Структура тестов

### context/manager_test.go
Тестирует:
- Создание менеджера контекста
- Добавление действий в историю
- Ограничение размера истории
- Оценка токенов
- Построение контекста

### security/layer_test.go
Тестирует:
- Создание security layer
- Определение деструктивных действий
- Работу при отключенном security layer

### subagents/subagent_test.go
Тестирует:
- Создание роутера под-агентов
- Определение типа задачи (EmailAgent, ShoppingAgent, JobSearchAgent)
- Маршрутизацию задач к правильным агентам

## Сборка и проверка .exe

### Windows
```bash
# Сборка
go build -o bin\golang-ai-agent.exe .

# Проверка
Test-Path bin\golang-ai-agent.exe
Get-Item bin\golang-ai-agent.exe
```

### Linux/Mac
```bash
# Сборка
go build -o bin/golang-ai-agent .

# Проверка
ls -lh bin/golang-ai-agent
file bin/golang-ai-agent
```

## Проверка работоспособности

После сборки .exe файла можно проверить его работоспособность:

1. Убедитесь, что создан файл `.env` с API ключом
2. Запустите собранный файл
3. Проверьте, что браузер открывается
4. Проверьте, что агент ждет ввода команд

## Примечания

- Тесты для `browser` и `agent` требуют реального браузера и API ключа, поэтому не включены в unit-тесты
- Для интеграционных тестов можно создать отдельный пакет `integration/`
- `.exe` файлы автоматически игнорируются git (см. `.gitignore`)

