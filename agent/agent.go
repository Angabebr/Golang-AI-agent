package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/Angabebr/Golang-AI-agent/ai"
	"github.com/Angabebr/Golang-AI-agent/browser"
	ctxmgr "github.com/Angabebr/Golang-AI-agent/context"
	"github.com/Angabebr/Golang-AI-agent/security"
)

// Agent основной автономный агент
type Agent struct {
	browser       *browser.Browser
	aiClient      *ai.Client
	ctxManager    *ctxmgr.Manager
	security      *security.Layer
	task          string
	maxIterations int
	errorCount    int
	maxErrors     int
}

// NewAgent создает новый экземпляр агента
func NewAgent(
	browser *browser.Browser,
	aiClient *ai.Client,
	ctxManager *ctxmgr.Manager,
	security *security.Layer,
) *Agent {
	return &Agent{
		browser:       browser,
		aiClient:      aiClient,
		ctxManager:    ctxManager,
		security:      security,
		maxIterations: 50, // Максимум итераций для предотвращения бесконечных циклов
		maxErrors:     3,  // Максимум ошибок подряд
	}
}

// Execute выполняет задачу автономно
func (a *Agent) Execute(ctx context.Context, task string) error {
	a.task = task
	a.ctxManager.Reset()
	a.errorCount = 0

	fmt.Printf("\n🤖 Начинаю выполнение задачи: %s\n\n", task)

	iteration := 0
	for iteration < a.maxIterations {
		iteration++

		// Получаем текущее состояние страницы
		pageContent, err := a.browser.GetPageContent()
		if err != nil {
			return fmt.Errorf("failed to get page content: %w", err)
		}

		// Строим контекст с учетом лимитов токенов
		history := a.ctxManager.GetHistory()

		// Принимаем решение о следующем действии
		decision, err := a.aiClient.MakeDecision(ctx, task, pageContent, history, 500)
		if err != nil {
			a.errorCount++
			if a.errorCount >= a.maxErrors {
				return fmt.Errorf("too many errors: %w", err)
			}
			fmt.Printf("⚠️  Ошибка при принятии решения: %v\n", err)
			time.Sleep(2 * time.Second)
			continue
		}

		// Выводим решение
		fmt.Printf("💭 Решение: %s\n", decision.Action)
		if decision.Reasoning != "" {
			fmt.Printf("   Обоснование: %s\n", decision.Reasoning)
		}

		// Проверяем, завершена ли задача
		if decision.IsComplete {
			fmt.Printf("\n✅ Задача выполнена!\n")
			if decision.Summary != "" {
				fmt.Printf("📋 Резюме: %s\n", decision.Summary)
			}
			return nil
		}

		// Проверяем, нужен ли ввод от пользователя
		if decision.NeedsInput {
			fmt.Printf("\n❓ Требуется ввод от пользователя: %s\n", decision.InputPrompt)
			// Здесь можно добавить логику получения ввода от пользователя
			continue
		}

		// Выполняем действие
		if err := a.executeAction(ctx, decision); err != nil {
			a.errorCount++
			fmt.Printf("❌ Ошибка при выполнении действия: %v\n", err)

			if a.errorCount >= a.maxErrors {
				return fmt.Errorf("too many consecutive errors: %w", err)
			}

			// Агент пытается адаптироваться - ждем и пробуем снова
			fmt.Printf("⏳ Ожидание перед повтором...\n")
			time.Sleep(3 * time.Second)
			continue
		}

		// Успешное выполнение - сбрасываем счетчик ошибок
		a.errorCount = 0

		// Добавляем действие в историю
		actionDesc := fmt.Sprintf("%s: %s", decision.Action, decision.Reasoning)
		a.ctxManager.AddAction(actionDesc)

		// Небольшая пауза между действиями
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("достигнут максимум итераций (%d)", a.maxIterations)
}

// executeAction выполняет конкретное действие
func (a *Agent) executeAction(ctx context.Context, decision *ai.Decision) error {
	// Проверяем деструктивные действия
	if a.security.IsDestructiveAction(decision.Action) {
		isDestructive, description, err := a.aiClient.CheckDestructiveAction(ctx, decision.Action, decision.Reasoning)
		if err == nil && isDestructive {
			confirmed, err := a.security.CheckAction(decision.Action, description, true)
			if err != nil {
				return err
			}
			if !confirmed {
				return fmt.Errorf("действие отменено пользователем")
			}
		}
	}

	switch decision.Action {
	case "navigate":
		if decision.URL == "" {
			return fmt.Errorf("URL не указан для навигации")
		}
		fmt.Printf("🌐 Переход на: %s\n", decision.URL)
		return a.browser.Navigate(decision.URL)

	case "click":
		if decision.Text != "" {
			fmt.Printf("🖱️  Клик по тексту: %s\n", decision.Text)
			return a.browser.ClickByText(decision.Text)
		} else if decision.Selector != "" {
			fmt.Printf("🖱️  Клик по селектору: %s\n", decision.Selector)
			return a.browser.ClickElement(decision.Selector)
		}
		return fmt.Errorf("не указан текст или селектор для клика")

	case "fill":
		if decision.Selector != "" {
			fmt.Printf("✍️  Заполнение поля: %s = %s\n", decision.Selector, decision.Value)
			return a.browser.FillInput(decision.Selector, decision.Value)
		} else if decision.Text != "" {
			fmt.Printf("✍️  Заполнение поля по placeholder: %s = %s\n", decision.Text, decision.Value)
			return a.browser.FillInputByPlaceholder(decision.Text, decision.Value)
		}
		return fmt.Errorf("не указан селектор или placeholder для заполнения")

	case "wait":
		if decision.WaitFor != "" {
			fmt.Printf("⏳ Ожидание элемента: %s\n", decision.WaitFor)
			return a.browser.WaitForElement(decision.WaitFor, 10*time.Second)
		}
		fmt.Printf("⏳ Ожидание 2 секунды...\n")
		time.Sleep(2 * time.Second)
		return nil

	case "extract":
		fmt.Printf("📄 Извлечение информации со страницы...\n")
		// Информация уже извлечена в GetPageContent
		return nil

	default:
		return fmt.Errorf("неизвестное действие: %s", decision.Action)
	}
}

// GetBrowser возвращает экземпляр браузера
func (a *Agent) GetBrowser() *browser.Browser {
	return a.browser
}
