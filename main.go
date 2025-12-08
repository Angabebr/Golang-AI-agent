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
	"github.com/joho/godotenv"
)

type ErrorFilterWriter struct {
	original io.Writer
}

func (w *ErrorFilterWriter) Write(p []byte) (n int, err error) {
	msg := string(p)
	if strings.Contains(msg, "ERROR: could not unmarshal event") ||
		strings.Contains(msg, "parse error: expected string") ||
		strings.Contains(msg, "unknown IPAddressSpace value: Loopback") {
		return len(p), nil
	}
	return w.original.Write(p)
}

func main() {
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found or error loading: %v", err)
		log.Println("Попытка продолжить с переменными окружения системы...")
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		log.Fatal(`
❌ OPENAI_API_KEY не установлен!

Создайте файл .env в корне проекта со следующим содержимым:
OPENAI_API_KEY=your_api_key_here
OPENAI_MODEL=gpt-4-turbo-preview
BROWSER_USER_DATA_DIR=./browser_data
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

	if !filepath.IsAbs(userDataDir) {
		absPath, err := filepath.Abs(userDataDir)
		if err != nil {
			log.Fatalf("Не удалось получить абсолютный путь для browser_data: %v", err)
		}
		userDataDir = absPath
	}

	chromeUserData := filepath.Join(os.Getenv("LOCALAPPDATA"), "Google", "Chrome", "User Data")
	if userDataDir == chromeUserData {
		fmt.Println("⚠️  ВНИМАНИЕ: Используется стандартная директория Chrome!")
		fmt.Println("   Убедитесь, что Chrome полностью закрыт перед запуском агента.")
		fmt.Println("   Рекомендуется использовать отдельную директорию для агента.")
		fmt.Println("   Для этого в .env укажите: BROWSER_USER_DATA_DIR=./browser_data")
		fmt.Println()
	}

	if err := os.MkdirAll(userDataDir, 0755); err != nil {
		log.Fatalf("Не удалось создать директорию browser_data (%s): %v\n\nПроверьте права доступа к директории.", userDataDir, err)
	}

	testFile := filepath.Join(userDataDir, ".test_write")
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		log.Fatalf("Нет прав на запись в директорию browser_data (%s): %v\n\nПроверьте права доступа.", userDataDir, err)
	}
	os.Remove(testFile)

	keepBrowserOpen := os.Getenv("KEEP_BROWSER_OPEN") == "true"

	fmt.Println("🚀 Инициализация AI-агента...")
	fmt.Printf("📁 Директория браузера: %s\n", userDataDir)
	fmt.Println("🌐 Запуск браузера...")

	browserInstance, err := browser.NewBrowser(userDataDir, false)
	if err != nil {
		log.Fatalf("\n❌ Не удалось запустить браузер: %v\n\nУбедитесь, что Chrome/Chromium установлен и доступен.", err)
	}

	if !keepBrowserOpen {
		defer browserInstance.Close()
	} else {
		fmt.Println("ℹ️  Браузер останется открытым после завершения программы")
	}

	fmt.Println("✅ Браузер запущен")

	aiClient := ai.NewClient(apiKey, model)
	fmt.Println("✅ AI клиент инициализирован")

	mainAgent := agent.NewAgent(browserInstance, aiClient)
	fmt.Println("✅ Основной агент создан")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

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

	startURL := os.Getenv("START_URL")
	if startURL == "" {
		startURL = "https://www.google.com"
	}

	fmt.Printf("🌐 Переход на стартовую страницу: %s\n", startURL)
	navErr := browserInstance.Navigate(startURL)
	if navErr != nil {
		log.Printf("⚠️  Warning: не удалось перейти на стартовую страницу: %v", navErr)
		log.Println("   Агент продолжит работу. Вы можете указать URL в команде.")
	} else {
		fmt.Println("✅ Стартовая страница загружена")
		time.Sleep(1 * time.Second)
	}

	time.Sleep(500 * time.Millisecond)

	scanner := bufio.NewScanner(os.Stdin)

	go func() {
		<-sigChan
		fmt.Println("\n\n🛑 Получен сигнал завершения (Ctrl+C)...")
		if !keepBrowserOpen {
			fmt.Println("   Браузер будет закрыт...")
			browserInstance.Close()
		} else {
			fmt.Println("   Браузер останется открытым")
		}
		os.Exit(0)
	}()

	fmt.Println("\n🎯 Агент готов к вводу команд. Введите задачу или 'help' для справки:")

	for {
		fmt.Print("\n> ")

		scanResult := scanner.Scan()

		if !scanResult {
			if err := scanner.Err(); err != nil {
				fmt.Printf("\n❌ Ошибка при чтении ввода: %v\n", err)
			} else {
				fmt.Println("\n⚠️  Ввод завершен (EOF) - stdin закрыт")
			}
			break
		}

		task := strings.TrimSpace(scanner.Text())
		if task == "" {
			continue
		}

		taskLower := strings.ToLower(task)
		if taskLower == "exit" || taskLower == "quit" || taskLower == "выход" {
			fmt.Println("👋 До свидания!")
			if !keepBrowserOpen {
				fmt.Println("   Браузер будет закрыт...")
			} else {
				fmt.Println("   Браузер останется открытым")
			}
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
			fmt.Println("   • Можно давать несколько задач подряд")
			fmt.Println(strings.Repeat("=", 60) + "\n")
			continue
		}

		// Проверка состояния браузера перед задачей
		url, urlErr := browserInstance.GetCurrentURL()
		if urlErr != nil {
			fmt.Printf("⚠️  Предупреждение: не удалось получить URL перед задачей: %v\n", urlErr)
		} else {
			fmt.Printf("📍 Текущий URL перед задачей: %s\n", url)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)

		startTime := time.Now()
		err := mainAgent.Execute(ctx, task)
		cancel()

		duration := time.Since(startTime)

		if err != nil {
			fmt.Printf("\n❌ Ошибка при выполнении задачи: %v\n", err)
			fmt.Printf("⏱️  Время выполнения: %v\n", duration)
		} else {
			fmt.Printf("\n✅ Задача выполнена успешно\n")
			fmt.Printf("⏱️  Время выполнения: %v\n", duration)
		}

		// Проверка состояния браузера после задачи
		url, urlErr = browserInstance.GetCurrentURL()
		if urlErr != nil {
			fmt.Printf("⚠️  ВНИМАНИЕ: после задачи не удалось получить URL: %v\n", urlErr)
			fmt.Printf("   Браузер может быть в нерабочем состоянии!\n")
		} else {
			fmt.Printf("📍 Текущий URL после задачи: %s\n", url)
		}

		// Проверка доступности контента после задачи
		pageContent, contentErr := browserInstance.GetPageContent()
		if contentErr != nil {
			fmt.Printf("❌ КРИТИЧЕСКАЯ ОШИБКА: после задачи не удалось получить контент: %v\n", contentErr)
			fmt.Printf("   Браузер недоступен для следующих задач!\n")
		} else {
			fmt.Printf("✅ Браузер доступен для следующих задач (URL: %s)\n", pageContent.URL)
		}

		fmt.Println("\n" + strings.Repeat("-", 60))
	}

	fmt.Println("\n👋 Программа завершена")
	if !keepBrowserOpen {
		fmt.Println("   Закрываем браузер...")
	} else {
		fmt.Println("   Браузер останется открытым")
	}

	fmt.Println("\nНажмите Enter для выхода...")
	bufio.NewReader(os.Stdin).ReadBytes('\n')
}
