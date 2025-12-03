package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
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

func main() {
	// Загружаем переменные окружения
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}

	// Получаем конфигурацию
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal("OPENAI_API_KEY не установлен. Создайте файл .env с вашим API ключом.")
	}

	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-4-turbo-preview"
	}

	userDataDir := os.Getenv("BROWSER_USER_DATA_DIR")
	if userDataDir == "" {
		userDataDir = "./browser_data"
	}

	enableSecurity := os.Getenv("ENABLE_SECURITY_LAYER")
	securityEnabled := enableSecurity != "false"

	// Создаем компоненты
	fmt.Println("🚀 Инициализация AI-агента...")

	// Создаем браузер (не headless, чтобы видеть процесс)
	browserInstance, err := browser.NewBrowser(userDataDir, false)
	if err != nil {
		log.Fatalf("Не удалось запустить браузер: %v", err)
	}
	defer browserInstance.Close()

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
	fmt.Println("Введите задачу для выполнения или 'exit' для выхода")
	fmt.Println(strings.Repeat("=", 60) + "\n")

	// Переходим на стартовую страницу (можно изменить)
	startURL := os.Getenv("START_URL")
	if startURL == "" {
		startURL = "https://www.google.com"
	}

	if err := browserInstance.Navigate(startURL); err != nil {
		log.Printf("Warning: не удалось перейти на стартовую страницу: %v", err)
	}

	// Основной цикл
	scanner := bufio.NewScanner(os.Stdin)

	go func() {
		<-sigChan
		fmt.Println("\n\n🛑 Получен сигнал завершения...")
		browserInstance.Close()
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

		if task == "exit" || task == "quit" || task == "выход" {
			fmt.Println("👋 До свидания!")
			break
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
