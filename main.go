package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Angabebr/Golang-AI-agent/agent"
	"github.com/Angabebr/Golang-AI-agent/ai"
	"github.com/Angabebr/Golang-AI-agent/browser"
	ctxmgr "github.com/Angabebr/Golang-AI-agent/context"
	"github.com/Angabebr/Golang-AI-agent/security"
	"github.com/Angabebr/Golang-AI-agent/subagents"
	"github.com/joho/godotenv"
)

// ErrorFilterWriter фильтрует некритичные ошибки chromedp
type ErrorFilterWriter struct {
	original io.Writer
}

func (w *ErrorFilterWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	// Фильтруем некритичные ошибки парсинга событий
	if strings.Contains(msg, "ERROR: could not unmarshal event") ||
		strings.Contains(msg, "parse error: expected string") ||
		strings.Contains(msg, "unknown IPAddressSpace value: Loopback") {
		// Пропускаем эти ошибки - они не критичны
		return len(p), nil
	}
	// Выводим все остальное
	return w.original.Write(p)
}

func main() {
	// Примечание: chromedp может выводить ошибки парсинга событий в stderr
	// Эти ошибки ("could not unmarshal event", "unknown IPAddressSpace") не критичны
	// и не влияют на функциональность - они связаны с парсингом DevTools Protocol
	// Можно игнорировать их или фильтровать через перенаправление stderr при запуске:
	// .\Golang-AI-agent.exe 2>nul  (Windows)
	// ./golang-ai-agent 2>/dev/null  (Linux/Mac)

	// Загружаем переменные окружения
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found or error loading: %v", err)
		log.Println("Попытка продолжить с переменными окружения системы...")
	}

	// Получаем конфигурацию
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal(`
❌ OPENAI_API_KEY не установлен!

Создайте файл .env в корне проекта со следующим содержимым:
OPENAI_API_KEY=your_api_key_here
OPENAI_MODEL=gpt-4-turbo-preview
BROWSER_USER_DATA_DIR=./browser_data
ENABLE_SECURITY_LAYER=true
START_URL=https://www.google.com

Или установите переменную окружения:
set OPENAI_API_KEY=your_api_key_here (Windows)
export OPENAI_API_KEY=your_api_key_here (Linux/Mac)
`)
	}

	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-4-turbo-preview"
	}

	userDataDir := os.Getenv("BROWSER_USER_DATA_DIR")
	if userDataDir == "" {
		userDataDir = "./browser_data"
	}

	// Преобразуем относительный путь в абсолютный
	if !filepath.IsAbs(userDataDir) {
		absPath, err := filepath.Abs(userDataDir)
		if err != nil {
			log.Fatalf("Не удалось получить абсолютный путь для browser_data: %v", err)
		}
		userDataDir = absPath
	}

	// Предупреждение, если используется стандартная директория Chrome
	chromeUserData := filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "User Data")
	if userDataDir == chromeUserData {
		fmt.Println("⚠️  ВНИМАНИЕ: Используется стандартная директория Chrome!")
		fmt.Println("   Убедитесь, что Chrome полностью закрыт перед запуском агента.")
		fmt.Println("   Рекомендуется использовать отдельную директорию для агента.")
		fmt.Println("   Для этого в .env укажите: BROWSER_USER_DATA_DIR=./browser_data")
		fmt.Println()
	}

	// Создаем директорию browser_data если её нет
	if err := os.MkdirAll(userDataDir, 0755); err != nil {
		log.Fatalf("Не удалось создать директорию browser_data (%s): %v\n\nПроверьте права доступа к директории.", userDataDir, err)
	}

	// Проверяем права на запись
	testFile := filepath.Join(userDataDir, ".test_write")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		log.Fatalf("Нет прав на запись в директорию browser_data (%s): %v\n\nПроверьте права доступа.", userDataDir, err)
	}
	os.Remove(testFile) // Удаляем тестовый файл

	enableSecurity := os.Getenv("ENABLE_SECURITY_LAYER")
	securityEnabled := enableSecurity != "false"

	// Опция: оставить браузер открытым после завершения программы
	keepBrowserOpen := os.Getenv("KEEP_BROWSER_OPEN") == "true"

	// Создаем компоненты
	fmt.Println("🚀 Инициализация AI-агента...")
	fmt.Printf("📁 Директория браузера: %s\n", userDataDir)

	// Создаем браузер (не headless, чтобы видеть процесс)
	fmt.Println("🌐 Запуск браузера...")
	browserInstance, err := browser.NewBrowser(userDataDir, false)
	if err != nil {
		log.Fatalf("\n❌ Не удалось запустить браузер: %v\n\nУбедитесь, что Chrome/Chromium установлен и доступен.", err)
	}

	// Закрываем браузер при завершении программы (если не указано иное)
	if !keepBrowserOpen {
		defer browserInstance.Close()
	} else {
		fmt.Println("ℹ️  Браузер останется открытым после завершения программы")
	}

	fmt.Println("✅ Браузер запущен")

	// Создаем AI клиент
	aiClient := ai.NewClient(apiKey, model)
	fmt.Println("✅ AI клиент инициализирован")

	// Создаем менеджер контекста
	ctxManager := ctxmgr.NewManager(8000) // ~8000 токенов максимум
	fmt.Println("✅ Менеджер контекста создан")

	// Создаем security layer
	securityLayer := security.NewLayer(securityEnabled)
	fmt.Println("✅ Security layer активирован")

	// Создаем роутер под-агентов
	subAgentRouter := subagents.NewRouter()
	fmt.Println("✅ Роутер под-агентов создан")

	// Создаем основной агент
	mainAgent := agent.NewAgent(browserInstance, aiClient, ctxManager, securityLayer)
	fmt.Println("✅ Основной агент создан")

	// Обработка сигналов для корректного завершения
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Запускаем интерактивный режим
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("🤖 AI-агент готов к работе!")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("\n📝 Как использовать:")
	fmt.Println("   Просто введите задачу текстом и нажмите Enter")
	fmt.Println("   Агент будет выполнять её автономно в браузере")
	fmt.Println("\n💡 Примеры команд:")
	fmt.Println("   • Прочитай последние 10 писем в яндекс почте и удали спам")
	fmt.Println("   • Закажи мне BBQ-бургер и картошку фри")
	fmt.Println("   • Найди 3 подходящие вакансии AI-инженера на hh.ru")
	fmt.Println("\n⚙️  Служебные команды:")
	fmt.Println("   • help / помощь - показать эту справку")
	fmt.Println("   • exit / quit / выход - завершить работу")
	fmt.Println(strings.Repeat("=", 60) + "\n")

	// Переходим на стартовую страницу (можно изменить)
	startURL := os.Getenv("START_URL")
	if startURL == "" {
		startURL = "https://www.google.com"
	}

	fmt.Printf("🌐 Переход на стартовую страницу: %s\n", startURL)
	if err := browserInstance.Navigate(startURL); err != nil {
		log.Printf("⚠️  Warning: не удалось перейти на стартовую страницу: %v", err)
		log.Println("   Агент продолжит работу. Вы можете указать URL в команде.")
	} else {
		fmt.Println("✅ Стартовая страница загружена")
		// Даем браузеру дополнительное время для стабилизации
		// Это гарантирует, что браузер останется открытым
		time.Sleep(1 * time.Second)
	}

	// Основной цикл
	scanner := bufio.NewScanner(os.Stdin)

	go func() {
		<-sigChan
		fmt.Println("\n\n🛑 Получен сигнал завершения...")
		if !keepBrowserOpen {
			fmt.Println("   Браузер будет закрыт...")
			browserInstance.Close()
		} else {
			fmt.Println("   Браузер останется открытым")
		}
		os.Exit(0)
	}()

	for {
		fmt.Print("\n> ")
		if !scanner.Scan() {
			break
		}

		task := strings.TrimSpace(scanner.Text())
		if task == "" {
			continue
		}

		// Обработка служебных команд
		taskLower := strings.ToLower(task)
		if taskLower == "exit" || taskLower == "quit" || taskLower == "выход" {
			fmt.Println("👋 До свидания!")
			if !keepBrowserOpen {
				fmt.Println("   Браузер будет закрыт...")
			} else {
				fmt.Println("   Браузер останется открытым")
			}
			// defer browserInstance.Close() закроет браузер автоматически (если keepBrowserOpen = false)
			break
		}

		if taskLower == "help" || taskLower == "помощь" || taskLower == "справка" {
			fmt.Println("\n" + strings.Repeat("=", 60))
			fmt.Println("📖 Справка по использованию агента")
			fmt.Println(strings.Repeat("=", 60))
			fmt.Println("\n🎯 Как давать команды:")
			fmt.Println("   Просто опишите задачу на русском или английском языке")
			fmt.Println("   Агент сам поймет, что нужно сделать")
			fmt.Println("\n📋 Примеры задач:")
			fmt.Println("   1. Удаление спама:")
			fmt.Println("      \"Прочитай последние 10 писем в яндекс почте и удали спам\"")
			fmt.Println("\n   2. Заказ еды:")
			fmt.Println("      \"Закажи мне BBQ-бургер и картошку фри из того места,")
			fmt.Println("       откуда я заказывал на прошлой неделе\"")
			fmt.Println("\n   3. Поиск вакансий:")
			fmt.Println("      \"Найди 3 подходящие вакансии AI-инженера на hh.ru")
			fmt.Println("       и откликнись на них с сопроводительным письмом\"")
			fmt.Println("\n   4. Навигация:")
			fmt.Println("      \"Перейди на сайт github.com и найди репозиторий golang\"")
			fmt.Println("\n⚙️  Служебные команды:")
			fmt.Println("   help / помощь - показать эту справку")
			fmt.Println("   exit / quit / выход - завершить работу")
			fmt.Println("\n💡 Советы:")
			fmt.Println("   • Будьте конкретны в описании задачи")
			fmt.Println("   • Агент работает автономно - просто наблюдайте")
			fmt.Println("   • При деструктивных действиях агент спросит подтверждение")
			fmt.Println("   • Можно давать несколько задач подряд")
			fmt.Println(strings.Repeat("=", 60) + "\n")
			continue
		}

		// Проверяем, есть ли специализированный агент для этой задачи
		subAgent := subAgentRouter.Route(task)
		if subAgent != nil {
			fmt.Printf("🎯 Используется специализированный агент: %s\n\n", subAgent.GetName())
		}

		// Выполняем задачу
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)

		startTime := time.Now()
		err := mainAgent.Execute(ctx, task)
		cancel()

		duration := time.Since(startTime)

		if err != nil {
			fmt.Printf("\n❌ Ошибка при выполнении задачи: %v\n", err)
			fmt.Printf("⏱️  Время выполнения: %v\n", duration)
		} else {
			fmt.Printf("\n⏱️  Время выполнения: %v\n", duration)
		}

		fmt.Println("\n" + strings.Repeat("-", 60))
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Ошибка при чтении ввода: %v", err)
	}
}
