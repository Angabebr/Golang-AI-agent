package agent

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
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
	retryStrategy string
}

func NewAgent(browser *browser.Browser, aiClient *ai.Client) *Agent {
	return &Agent{
		browser:       browser,
		aiClient:      aiClient,
		maxIterations: 50,
		maxErrors:     5, // Увеличено для лучшей адаптации
		retryStrategy:  "adaptive",
	}
}

func (a *Agent) Execute(ctx context.Context, task string) error {
	a.task = task
	a.errorCount = 0

	fmt.Printf("\n🤖 Начинаю выполнение задачи: %s\n\n", task)
	
	// Определяем тип под-агента и используем его, если нужно
	// Отладочный вывод для диагностики
	taskPreview := task
	if len(task) > 50 {
		taskPreview = task[:50] + "..."
	}
	fmt.Printf("🔍 Отладка: длина задачи = %d, первые символы = %q\n", len(task), taskPreview)
	subAgentType := DetectSubAgentType(task)
	fmt.Printf("🔍 Отладка: определен тип агента = %s\n", subAgentType)
	if subAgentType != SubAgentGeneric {
		subAgent := NewSubAgent(subAgentType, a.browser, a.aiClient)
		fmt.Printf("🎯 Использую специализированного агента: %s\n\n", subAgentType)
		return subAgent.Execute(ctx, task, a)
	}

	return a.executeTask(ctx, task)
}

// executeTask выполняет задачу (внутренний метод для использования sub-agents)
func (a *Agent) executeTask(ctx context.Context, task string) error {
	iteration := 0
	var history []string

	for iteration < a.maxIterations {
		iteration++

		// Сначала пытаемся получить быструю информацию
		quickInfo, quickErr := a.browser.GetQuickPageInfo()
		if quickErr != nil {
			// Если быстрый метод не работает, пробуем полный
			pageContent, err := a.browser.GetPageContent()
			if err != nil {
				// Если контекст браузера отменен, это критическая ошибка
				if strings.Contains(err.Error(), "browser context was canceled") {
					return fmt.Errorf("браузер недоступен после предыдущей задачи: %w. Возможно, браузер был закрыт или контекст отменен", err)
				}
				
				// При ошибках таймаута делаем еще одну попытку после паузы
				if strings.Contains(err.Error(), "deadline exceeded") || strings.Contains(err.Error(), "timeout") {
					a.errorCount++
					if a.errorCount < a.maxErrors {
						fmt.Printf("⚠️  Таймаут при получении контента, повторная попытка через 3 секунды...\n")
						time.Sleep(3 * time.Second)
						continue
					}
				}
				
				return fmt.Errorf("failed to get page content: %w", err)
			}
			
			// Используем полный контент
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
			
			// Обработка решения с полным контентом
			if err := a.processDecision(ctx, decision, history); err != nil {
				return err
			}
			
			a.errorCount = 0
			actionDesc := fmt.Sprintf("%s: %s", decision.Action, decision.Reasoning)
			history = append(history, actionDesc)
			time.Sleep(1 * time.Second)
			continue
		}
		
		// Используем быструю информацию для простых действий
		decision, err := a.aiClient.MakeDecision(ctx, task, quickInfo, history, 500)
		if err != nil {
			a.errorCount++
			if a.errorCount >= a.maxErrors {
				return fmt.Errorf("too many errors: %w", err)
			}
			fmt.Printf("⚠️  Ошибка при принятии решения: %v\n", err)
			time.Sleep(2 * time.Second)
			continue
		}

		// Обработка решения
		if err := a.processDecision(ctx, decision, history); err != nil {
			return err
		}
		
		// Сбрасываем счетчик ошибок при успешном выполнении
		a.errorCount = 0
		actionDesc := fmt.Sprintf("%s: %s", decision.Action, decision.Reasoning)
		history = append(history, actionDesc)
		time.Sleep(1 * time.Second)
	}

	return fmt.Errorf("достигнут максимум итераций (%d)", a.maxIterations)
}

// processDecision обрабатывает решение AI
func (a *Agent) processDecision(ctx context.Context, decision *ai.Decision, history []string) error {
	fmt.Printf("💭 Решение: %s\n", decision.Action)
	if decision.Reasoning != "" {
		fmt.Printf("   Обоснование: %s\n", decision.Reasoning)
	}

	if decision.IsComplete {
		// Проверяем, действительно ли задача выполнена
		// Если в истории много завершений подряд - это зацикливание
		recentCompletes := 0
		for i := len(history) - 1; i >= 0 && i >= len(history)-5; i-- {
			if strings.Contains(history[i], "complete") || strings.Contains(history[i], "Задача выполнена") {
				recentCompletes++
			}
		}
		
		if recentCompletes >= 3 {
			fmt.Printf("\n⚠️  Обнаружено зацикливание завершения задачи. Продолжаю выполнение...\n")
			// Не завершаем, продолжаем работу - сбрасываем IsComplete
			decision.IsComplete = false
			// Добавляем в историю, что было зацикливание
			history = append(history, "ОБНАРУЖЕНО зацикливание завершения - продолжаю работу")
		} else {
			fmt.Printf("\n✅ Задача выполнена!\n")
			if decision.Summary != "" {
				fmt.Printf("📋 Резюме: %s\n", decision.Summary)
			}
			return nil
		}
	}

	if decision.NeedsInput {
		fmt.Printf("\n❓ Требуется ввод от пользователя: %s\n", decision.InputPrompt)
		return fmt.Errorf("needs user input")
	}
	
	// Если действие "complete" но IsComplete=false (после сброса зацикливания), пропускаем
	if decision.Action == "complete" && !decision.IsComplete {
		fmt.Printf("⚠️  Действие 'complete' пропущено из-за зацикливания\n")
		return fmt.Errorf("complete action skipped due to loop detection")
	}

	// Проверка на деструктивные действия
	if a.isDestructiveAction(decision) {
		quickInfo, _ := a.browser.GetQuickPageInfo()
		contextStr := ""
		if quickInfo != nil {
			contextStr = fmt.Sprintf("URL: %s, Title: %s", quickInfo.URL, quickInfo.Title)
		}
		
		confirmed, err := a.checkDestructiveAction(ctx, decision, contextStr)
		if err != nil {
			fmt.Printf("⚠️  Ошибка при проверке деструктивного действия: %v\n", err)
			confirmed = false
		}
		
		if !confirmed {
			fmt.Printf("🚫 Деструктивное действие отменено пользователем\n")
			history = append(history, fmt.Sprintf("ОТМЕНЕНО деструктивное действие: %s", decision.Action))
			time.Sleep(1 * time.Second)
			return fmt.Errorf("destructive action canceled")
		}
	}

	if err := a.executeAction(ctx, decision); err != nil {
		a.errorCount++
		fmt.Printf("❌ Ошибка при выполнении действия: %v\n", err)

		// Адаптивная обработка ошибок
		retryDelay := a.calculateRetryDelay(a.errorCount)
		errorDesc := fmt.Sprintf("ОШИБКА при '%s': %v. Стратегия: %s", decision.Action, err, a.adaptToError(err, decision))
		history = append(history, errorDesc)

		if a.errorCount >= a.maxErrors {
			return fmt.Errorf("too many consecutive errors: %w", err)
		}

		fmt.Printf("⏳ Ожидание перед повтором (%v)...\n", retryDelay)
		time.Sleep(retryDelay)
		return err
	}

	return nil
}

func (a *Agent) executeAction(ctx context.Context, decision *ai.Decision) error {
	switch decision.Action {
	case "navigate":
		if decision.URL == "" {
			return fmt.Errorf("URL не указан для навигации. Используй поле 'url' с адресом (можно прямой URL или из списка links)")
		}
		
		// Нормализуем URL - добавляем https:// если отсутствует
		url := decision.URL
		if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
			// Если это домен без протокола, добавляем https://
			if strings.Contains(url, ".") && !strings.Contains(url, " ") {
				url = "https://" + url
			}
		}
		
		fmt.Printf("🌐 Переход на: %s\n", url)
		return a.browser.Navigate(url)

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

	case "press_key":
		if decision.Key == "" {
			return fmt.Errorf("не указана клавиша для нажатия (key пустое). Используй поле 'key' с названием клавиши (delete, enter, escape и т.д.)")
		}
		fmt.Printf("⌨️  Нажатие клавиши: %s\n", decision.Key)
		return a.browser.PressKey(decision.Key)

	case "switch_tab":
		if decision.TabIndex <= 0 {
			return fmt.Errorf("не указан индекс вкладки (tab_index пустое или неверное). Используй поле 'tab_index' с номером вкладки (1, 2, 3...)")
		}
		// Получаем список вкладок
		tabs, err := a.browser.GetAllTabs()
		if err != nil {
			return fmt.Errorf("не удалось получить список вкладок: %w", err)
		}
		if decision.TabIndex > len(tabs) {
			return fmt.Errorf("неверный индекс вкладки: %d (всего вкладок: %d)", decision.TabIndex, len(tabs))
		}
		targetTab := tabs[decision.TabIndex-1]
		fmt.Printf("🔄 Переключение на вкладку %d: %s\n", decision.TabIndex, targetTab.Title)
		return a.browser.SwitchToTab(targetTab.ID)

	case "close_tab":
		if decision.TabIndex <= 0 {
			return fmt.Errorf("не указан индекс вкладки (tab_index пустое или неверное). Используй поле 'tab_index' с номером вкладки (1, 2, 3...)")
		}
		// Получаем список вкладок
		tabs, err := a.browser.GetAllTabs()
		if err != nil {
			return fmt.Errorf("не удалось получить список вкладок: %w", err)
		}
		if decision.TabIndex > len(tabs) {
			return fmt.Errorf("неверный индекс вкладки: %d (всего вкладок: %d)", decision.TabIndex, len(tabs))
		}
		if len(tabs) == 1 {
			return fmt.Errorf("нельзя закрыть единственную открытую вкладку")
		}
		targetTab := tabs[decision.TabIndex-1]
		if targetTab.IsActive {
			// Если закрываем активную вкладку, сначала переключимся на другую
			newActiveIndex := 0
			if decision.TabIndex == 1 {
				newActiveIndex = 1 // переключимся на следующую
			}
			if err := a.browser.SwitchToTab(tabs[newActiveIndex].ID); err != nil {
				return fmt.Errorf("не удалось переключиться перед закрытием: %w", err)
			}
		}
		fmt.Printf("❌ Закрытие вкладки %d: %s\n", decision.TabIndex, targetTab.Title)
		return a.browser.CloseTab(targetTab.ID)

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

	case "complete":
		// Действие "complete" должно обрабатываться в processDecision, но на случай если попало сюда
		return nil

	default:
		return fmt.Errorf("неизвестное действие: %s", decision.Action)
	}
}

func (a *Agent) GetBrowser() *browser.Browser {
	return a.browser
}

// isDestructiveAction проверяет, является ли действие деструктивным
func (a *Agent) isDestructiveAction(decision *ai.Decision) bool {
	action := strings.ToLower(decision.Action)
	text := strings.ToLower(decision.Text)
	reasoning := strings.ToLower(decision.Reasoning)
	
	destructiveKeywords := []string{
		"удалить", "delete", "remove", "удаление",
		"оплатить", "pay", "payment", "купить", "buy", "purchase",
		"подтвердить", "confirm", "submit", "отправить",
		"отменить", "cancel", "отмена",
		"изменить", "change", "modify", "редактировать",
		"сохранить", "save", "сохранение",
	}
	
	for _, keyword := range destructiveKeywords {
		if strings.Contains(action, keyword) || 
		   strings.Contains(text, keyword) || 
		   strings.Contains(reasoning, keyword) {
			return true
		}
	}
	
	// Проверка на действия с оплатой или удалением
	if strings.Contains(text, "корзина") && (strings.Contains(text, "оформить") || strings.Contains(text, "заказать")) {
		return true
	}
	
	if strings.Contains(text, "удалить") || strings.Contains(text, "delete") {
		return true
	}
	
	return false
}

// checkDestructiveAction запрашивает подтверждение у пользователя
func (a *Agent) checkDestructiveAction(ctx context.Context, decision *ai.Decision, contextStr string) (bool, error) {
	isDestructive, description, err := a.aiClient.CheckDestructiveAction(ctx, decision.Action, contextStr)
	if err != nil {
		return false, err
	}
	
	if !isDestructive {
		return true, nil
	}
	
	fmt.Printf("\n⚠️  ВНИМАНИЕ: Деструктивное действие обнаружено!\n")
	fmt.Printf("   Действие: %s\n", decision.Action)
	fmt.Printf("   Описание: %s\n", description)
	if decision.Text != "" {
		fmt.Printf("   Элемент: %s\n", decision.Text)
	}
	fmt.Printf("\n❓ Подтвердите действие (yes/no): ")
	
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}
	
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "yes" || response == "y" || response == "да" || response == "д", nil
}

// calculateRetryDelay вычисляет задержку перед повтором с экспоненциальным backoff
func (a *Agent) calculateRetryDelay(errorCount int) time.Duration {
	baseDelay := 2 * time.Second
	maxDelay := 10 * time.Second
	
	delay := time.Duration(errorCount) * baseDelay
	if delay > maxDelay {
		delay = maxDelay
	}
	
	return delay
}

// adaptToError определяет стратегию адаптации к ошибке
func (a *Agent) adaptToError(err error, decision *ai.Decision) string {
	errStr := strings.ToLower(err.Error())
	
	if strings.Contains(errStr, "not found") || strings.Contains(errStr, "не найден") {
		return "элемент не найден - попробую найти альтернативный способ"
	}
	
	if strings.Contains(errStr, "timeout") || strings.Contains(errStr, "таймаут") {
		return "таймаут - увеличу время ожидания"
	}
	
	if strings.Contains(errStr, "visible") || strings.Contains(errStr, "видимый") {
		return "элемент не видим - подожду загрузки страницы"
	}
	
	return "повторю попытку с задержкой"
}
