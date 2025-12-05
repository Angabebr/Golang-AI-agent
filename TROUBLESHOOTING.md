# Устранение проблем

## Проблема: "OPENAI_API_KEY не установлен"

### Решение:
1. Убедитесь, что файл `.env` существует в корне проекта
2. Проверьте, что файл сохранен в кодировке **UTF-8 без BOM**
3. Убедитесь, что в файле есть строка:
   ```
   OPENAI_API_KEY=your_api_key_here
   ```

### Как исправить BOM в .env:
**Windows (PowerShell):**
```powershell
$content = Get-Content .env -Raw
$content = $content.TrimStart([char]0xFEFF)
[System.IO.File]::WriteAllText("$PWD\.env", $content, [System.Text.UTF8Encoding]::new($false))
```

**Или пересоздайте файл:**
1. Удалите старый `.env`
2. Скопируйте `.env.example` в `.env`
3. Добавьте ваш API ключ

## Проблема: "Не удалось создать каталог данных" / "cannot perform read and write operations"

### Решение:
Эта ошибка означает, что Chrome не может получить доступ к директории `browser_data`.

**Автоматическое исправление (встроено в код):**
- Код теперь автоматически создает директорию
- Проверяет права на запись
- Использует абсолютный путь

**Если проблема сохраняется:**

1. **Проверьте права доступа:**
   ```powershell
   # Windows - проверьте права на папку
   icacls browser_data
   ```

2. **Удалите и пересоздайте директорию:**
   ```powershell
   # Windows
   Remove-Item -Recurse -Force browser_data -ErrorAction SilentlyContinue
   New-Item -ItemType Directory -Path browser_data
   ```

3. **Используйте другую директорию:**
   В `.env` файле укажите абсолютный путь:
   ```env
   BROWSER_USER_DATA_DIR=C:\Users\YourName\browser_data
   ```

4. **⚠️ НЕ используйте стандартную директорию Chrome:**
   **НЕ указывайте:**
   ```env
   BROWSER_USER_DATA_DIR=C:\Users\YourName\AppData\Local\Google\Chrome\User Data
   ```
   
   **Почему:**
   - Chrome блокирует доступ к этой директории, если он запущен
   - Может привести к конфликтам и потере данных
   - Агент должен использовать отдельную директорию
   
   **Решение:**
   - Используйте `./browser_data` (по умолчанию)
   - Или укажите другую отдельную директорию
   - Убедитесь, что Chrome закрыт, если всё же используете стандартную директорию

4. **Проверьте антивирус:**
   - Некоторые антивирусы блокируют создание директорий
   - Добавьте исключение для вашего проекта

## Проблема: Окно выбора профиля Chrome

### Симптомы:
- При запуске агента появляется окно выбора профиля Chrome
- Браузер не запускается автоматически
- Ошибки "context canceled" или таймауты

### Решение:
**Автоматическое исправление (встроено в код):**
- Код теперь автоматически пропускает окно выбора профиля
- Использует профиль "Default" по умолчанию
- Добавлены флаги для автоматического запуска

**Если проблема сохраняется:**

1. **Закройте все окна Chrome:**
   ```powershell
   # Проверьте процессы Chrome
   Get-Process chrome -ErrorAction SilentlyContinue
   
   # Закройте все процессы
   Stop-Process -Name chrome -Force -ErrorAction SilentlyContinue
   ```

2. **Используйте отдельную директорию для агента:**
   В `.env` укажите:
   ```env
   BROWSER_USER_DATA_DIR=./browser_data
   ```
   Это создаст новый профиль специально для агента.

3. **Удалите старую директорию browser_data:**
   ```powershell
   Remove-Item -Recurse -Force browser_data -ErrorAction SilentlyContinue
   ```
   При следующем запуске агент создаст новый профиль без окна выбора.

## Проблема: "failed to start browser" / "chrome failed to start"

### Решение:
1. **Установите Chrome или Chromium:**
   - Chrome: https://www.google.com/chrome/
   - Chromium: https://www.chromium.org/getting-involved/download-chromium

2. **Проверьте, что Chrome установлен:**
   ```powershell
   # Windows
   where.exe chrome
   
   # Или проверьте пути:
   Test-Path "C:\Program Files\Google\Chrome\Application\chrome.exe"
   Test-Path "$env:LOCALAPPDATA\Google\Chrome\Application\chrome.exe"
   ```

3. **Если Chrome установлен, но не найден:**
   - Добавьте Chrome в PATH
   - Или укажите путь явно в коде (требует изменения)

4. **Проверьте антивирус:**
   - Некоторые антивирусы блокируют автоматический запуск Chrome
   - Добавьте исключение для вашего .exe файла

5. **Закройте все окна Chrome перед запуском агента:**
   - Окно выбора профиля может блокировать автоматизацию
   - Убедитесь, что Chrome полностью закрыт

## Проблема: ".env file not found: unexpected character"

Это означает, что в `.env` файле есть BOM (Byte Order Mark) или неправильная кодировка.

### Решение:
Пересоздайте `.env` файл в кодировке UTF-8 без BOM:
1. Откройте `.env` в Notepad++
2. Кодировка → Преобразовать в UTF-8 без BOM
3. Сохраните

Или используйте команду выше для удаления BOM.

## Проверка работоспособности

### Шаг 1: Проверка .env
```bash
# Проверьте содержимое
cat .env  # Linux/Mac
type .env  # Windows

# Должно быть:
OPENAI_API_KEY=sk-...
OPENAI_MODEL=gpt-4-turbo-preview
```

### Шаг 2: Проверка Chrome
```bash
# Windows
where.exe chrome

# Linux
which google-chrome
which chromium

# Mac
which "Google Chrome"
```

### Шаг 3: Запуск
```bash
# Из исходников
go run main.go

# Или из .exe
.\Golang-AI-agent.exe  # Windows
./golang-ai-agent  # Linux/Mac
```

## Дополнительная диагностика

Если проблемы продолжаются:

1. **Проверьте логи:**
   - Запустите с выводом ошибок: `.\Golang-AI-agent.exe 2>&1 | tee error.log`

2. **Проверьте переменные окружения:**
   ```powershell
   # Windows
   $env:OPENAI_API_KEY
   
   # Linux/Mac
   echo $OPENAI_API_KEY
   ```

3. **Пересоберите проект:**
   ```bash
   go clean
   go build -o golang-ai-agent.exe .
   ```

## Контакты

Если проблема не решена, создайте issue на GitHub с:
- Версией ОС
- Версией Go (`go version`)
- Полным текстом ошибки
- Содержимым `.env.example` (БЕЗ реального API ключа!)

