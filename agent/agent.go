package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/Angabebr/Golang-AI-agent/ai"
	"github.com/Angabebr/Golang-AI-agent/browser"
)

type Agent struct {
	browser       *browser.Browser
	aiClient      *ai.Client
	task          string
	maxIterations int
	errorCount    int
	maxErrors     int
}

func NewAgent(browser *browser.Browser, aiClient *ai.Client) *Agent {
	return &Agent{
		browser:       browser,
		aiClient:      aiClient,
		maxIterations: 50,
		maxErrors:     3,
	}
}

func (a *Agent) Execute(ctx context.Context, task string) error {
	a.task = task
	a.errorCount = 0

	fmt.Printf("\n🤖 Начинаю выполнение задачи: %s\n\n", task)

	iteration := 0
	var history []string

	for iteration < a.maxIterations {
		iteration++

		pageContent, err := a.browser.GetPageContent()
		if err != nil {
			return fmt.Errorf("failed to get page content: %w", err)
		}

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

		fmt.Printf("💭 Решение: %s\n", decision.Action)
		if decision.Reasoning != "" {
			fmt.Printf("   Обоснование: %s\n", decision.Reasoning)
		}

		if decision.IsComplete {
			fmt.Printf("\n✅ Задача выполнена!\n")
			if decision.Summary != "" {
				fmt.Printf("📋 Резюме: %s\n", decision.Summary)
			}
			return nil
		}

		if decision.NeedsInput {
			fmt.Printf("\n❓ Требуется ввод от пользователя: %s\n", decision.InputPrompt)
			continue
		}

		if err := a.executeAction(ctx, decision); err != nil {
			a.errorCount++
			fmt.Printf("❌ Ошибка при выполнении действия: %v\n", err)

			errorDesc := fmt.Sprintf("ОШИБКА при '%s': %v", decision.Action, err)
			history = append(history, errorDesc)

			if a.errorCount >= a.maxErrors {
				return fmt.Errorf("too many consecutive errors: %w", err)
			}

			fmt.Printf("⏳ Ожидание перед повтором...\n")
			time.Sleep(3 * time.Second)
			continue
		}

		a.errorCount = 0

		actionDesc := fmt.Sprintf("%s: %s", decision.Action, decision.Reasoning)
		history = append(history, actionDesc)

		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("достигнут максимум итераций (%d)", a.maxIterations)
}

func (a *Agent) executeAction(ctx context.Context, decision *ai.Decision) error {
	switch decision.Action {
	case "navigate":
		if decision.URL == "" {
			return fmt.Errorf("URL не указан для навигации. Используй поле 'url' с адресом из списка links")
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
		return fmt.Errorf("не указан текст или селектор для клика. Используй поле 'text' с текстом кнопки/ссылки из списка buttons/links, или поле 'selector' с CSS селектором")

	case "fill":
		if decision.Value == "" {
			return fmt.Errorf("не указано значение для заполнения (value пустое)")
		}
		if decision.Selector != "" {
			fmt.Printf("✍️  Заполнение поля: %s = %s\n", decision.Selector, decision.Value)
			return a.browser.FillInput(decision.Selector, decision.Value)
		} else if decision.Text != "" {
			fmt.Printf("✍️  Заполнение поля по placeholder: %s = %s\n", decision.Text, decision.Value)
			return a.browser.FillInputByPlaceholder(decision.Text, decision.Value)
		}
		return fmt.Errorf("не указан селектор или placeholder для заполнения. Используй поле 'text' с placeholder/name из списка inputs, или поле 'selector' с CSS селектором")

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
		return nil

	default:
		return fmt.Errorf("неизвестное действие: %s", decision.Action)
	}
}

func (a *Agent) GetBrowser() *browser.Browser {
	return a.browser
}
