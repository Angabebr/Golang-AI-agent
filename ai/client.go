package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/sashabaranov/go-openai"
)

// Client обертка над OpenAI API
type Client struct {
	client *openai.Client
	model  string
}

// NewClient создает новый AI клиент
func NewClient(apiKey, model string) *Client {
	if model == "" {
		model = "gpt-4-turbo-preview"
	}

	return &Client{
		client: openai.NewClient(apiKey),
		model:  model,
	}
}

// Decision представляет решение агента о следующем действии
type Decision struct {
	Action      string            `json:"action"`
	Reasoning   string            `json:"reasoning"`
	Selector    string            `json:"selector,omitempty"`
	Text        string            `json:"text,omitempty"`
	Value       string            `json:"value,omitempty"`
	URL         string            `json:"url,omitempty"`
	WaitFor     string            `json:"wait_for,omitempty"`
	NeedsInput  bool              `json:"needs_input"`
	InputPrompt string            `json:"input_prompt,omitempty"`
	IsComplete  bool              `json:"is_complete"`
	Summary     string            `json:"summary,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// MakeDecision принимает решение о следующем действии на основе текущего состояния страницы
func (c *Client) MakeDecision(ctx context.Context, task string, pageContent interface{}, history []string, maxTokens int) (*Decision, error) {
	// Формируем промпт для AI
	prompt := c.buildPrompt(task, pageContent, history)

	messages := []openai.ChatCompletionMessage{
		{
			Role: openai.ChatMessageRoleSystem,
			Content: `Ты - автономный AI-агент, который управляет веб-браузером для выполнения задач пользователя.

Твоя задача - анализировать текущее состояние веб-страницы и АВТОНОМНО принимать решения о следующих действиях, БЕЗ использования заготовленных планов или шаблонов.

Доступные действия:
1. navigate - перейти на URL (используй url из списка links на странице)
2. click - кликнуть на элемент (ПРЕДПОЧТИТЕЛЬНО используй text - видимый текст кнопки/ссылки)
3. fill - заполнить поле ввода (используй placeholder или name из списка inputs)
4. wait - подождать появления элемента или просто подождать
5. extract - извлечь информацию со страницы (уже сделано, можно пропустить)
6. complete - задача выполнена

КРИТИЧЕСКИ ВАЖНО:
- НЕ используй заготовленные селекторы типа a[data-qa='vacancy'] или предопределенные пути
- НЕ используй подсказки о структуре сайта (например, что вакансии на /vacancies)
- Анализируй ТОЛЬКО то, что видишь на текущей странице в предоставленных данных
- Для клика ПРЕДПОЧТИТЕЛЬНО используй поле "text" с видимым текстом элемента из списка buttons/links
- Для заполнения используй placeholder или name из списка inputs
- Для навигации используй href из списка links
- Если элемент не найден в списках, можешь попробовать selector, но это последний вариант
- Принимай решения на основе текущего контекста, а не общих знаний о сайтах
- Отвечай ТОЛЬКО в формате JSON, без дополнительного текста до или после JSON

Формат ответа (строго валидный JSON):
{
  "action": "click|navigate|fill|wait|extract|complete",
  "reasoning": "подробное объяснение почему выбрано это действие на основе анализа страницы",
  "selector": "CSS селектор (только если text не работает)",
  "text": "текст элемента для поиска (ПРЕДПОЧТИТЕЛЬНО)",
  "value": "значение для заполнения",
  "url": "URL для перехода (из списка links)",
  "wait_for": "селектор элемента для ожидания",
  "needs_input": false,
  "input_prompt": "если нужен ввод от пользователя",
  "is_complete": false,
  "summary": "краткое резюме выполненной работы (если is_complete=true)",
  "metadata": {}
}`,
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: prompt,
		},
	}

	resp, err := c.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model:       c.model,
			Messages:    messages,
			Temperature: 0.7,
			MaxTokens:   maxTokens,
		},
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get AI response: %w", err)
	}

	// Парсим JSON ответ
	content := resp.Choices[0].Message.Content
	decision, err := parseDecision(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse decision: %w", err)
	}

	return decision, nil
}

// AnalyzePage анализирует страницу и определяет, что на ней находится
func (c *Client) AnalyzePage(ctx context.Context, pageContent interface{}, task string) (string, error) {
	prompt := fmt.Sprintf(`Проанализируй эту веб-страницу и опиши, что на ней находится и как можно выполнить задачу: "%s"

Содержимое страницы:
%+v

Дай краткое описание страницы и возможных действий.`, task, pageContent)

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleUser,
			Content: prompt,
		},
	}

	resp, err := c.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model:       c.model,
			Messages:    messages,
			Temperature: 0.5,
			MaxTokens:   500,
		},
	)

	if err != nil {
		return "", fmt.Errorf("failed to analyze page: %w", err)
	}

	return resp.Choices[0].Message.Content, nil
}

// CheckDestructiveAction проверяет, является ли действие деструктивным
func (c *Client) CheckDestructiveAction(ctx context.Context, action string, context string) (bool, string, error) {
	prompt := fmt.Sprintf(`Проверь, является ли это действие деструктивным (удаление, оплата, отправка важных данных, изменение настроек):

Действие: %s
Контекст: %s

Ответь в формате JSON:
{
  "is_destructive": true/false,
  "description": "описание что произойдет",
  "confirmation_question": "вопрос для пользователя"
}`, action, context)

	messages := []openai.ChatCompletionMessage{
		{
			Role: openai.ChatMessageRoleSystem,
			Content: "Ты проверяешь действия на деструктивность. Отвечай только в формате JSON.",
		},
		{
			Role:    openai.ChatMessageRoleUser,
			Content: prompt,
		},
	}

	resp, err := c.client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model:       c.model,
			Messages:    messages,
			Temperature: 0.3,
			MaxTokens:   200,
		},
	)

	if err != nil {
		return false, "", fmt.Errorf("failed to check destructive action: %w", err)
	}

	// Парсим ответ (упрощенная версия)
	content := resp.Choices[0].Message.Content
	isDestructive := strings.Contains(strings.ToLower(content), `"is_destructive": true`) ||
		strings.Contains(strings.ToLower(content), `is_destructive: true`)

	description := "Действие может привести к необратимым изменениям"
	if strings.Contains(content, `"description"`) {
		// Простое извлечение описания
		parts := strings.Split(content, `"description"`)
		if len(parts) > 1 {
			desc := strings.Trim(parts[1], `":,}`)
			if desc != "" {
				description = desc
			}
		}
	}

	return isDestructive, description, nil
}

// buildPrompt формирует промпт для принятия решения
func (c *Client) buildPrompt(task string, pageContent interface{}, history []string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Задача пользователя: %s\n\n", task))

	if len(history) > 0 {
		sb.WriteString("История действий:\n")
		for i, h := range history {
			if i >= len(history)-5 { // Последние 5 действий
				sb.WriteString(fmt.Sprintf("- %s\n", h))
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("Текущее состояние страницы:\n")
	sb.WriteString(fmt.Sprintf("%+v\n\n", pageContent))

	sb.WriteString("Какое следующее действие нужно выполнить? Ответь в формате JSON.")

	return sb.String()
}

// parseDecision парсит JSON ответ от AI
func parseDecision(content string) (*Decision, error) {
	// Убираем markdown код блоки если есть
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```json") {
		content = strings.TrimPrefix(content, "```json")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	} else if strings.HasPrefix(content, "```") {
		content = strings.TrimPrefix(content, "```")
		content = strings.TrimSuffix(content, "```")
		content = strings.TrimSpace(content)
	}

	// Пытаемся найти JSON объект в тексте
	jsonRegex := regexp.MustCompile(`\{[^{}]*"action"[^{}]*\}`)
	jsonMatch := jsonRegex.FindString(content)
	if jsonMatch == "" {
		// Если не нашли компактный JSON, ищем многострочный
		jsonRegex = regexp.MustCompile(`\{[\s\S]*?\}`)
		jsonMatch = jsonRegex.FindString(content)
	}

	if jsonMatch != "" {
		content = jsonMatch
	}

	// Парсим JSON
	decision := &Decision{
		Action:     "wait",
		IsComplete: false,
		Metadata:   make(map[string]string),
	}

	if err := json.Unmarshal([]byte(content), decision); err != nil {
		// Если парсинг не удался, используем fallback
		return parseDecisionFallback(content)
	}

	// Инициализируем Metadata если nil
	if decision.Metadata == nil {
		decision.Metadata = make(map[string]string)
	}

	return decision, nil
}

// parseDecisionFallback используется если основной парсинг не удался
func parseDecisionFallback(content string) (*Decision, error) {
	decision := &Decision{
		Action:     "wait",
		IsComplete: false,
		Metadata:   make(map[string]string),
	}

	// Простой regex-based парсинг как fallback
	extractString := func(key string) string {
		re := regexp.MustCompile(fmt.Sprintf(`"%s"\s*:\s*"([^"]*)"`, key))
		matches := re.FindStringSubmatch(content)
		if len(matches) > 1 {
			return matches[1]
		}
		return ""
	}

	extractBool := func(key string) bool {
		re := regexp.MustCompile(fmt.Sprintf(`"%s"\s*:\s*(true|false)`, key))
		matches := re.FindStringSubmatch(content)
		if len(matches) > 1 {
			return matches[1] == "true"
		}
		return false
	}

	decision.Action = extractString("action")
	if decision.Action == "" {
		decision.Action = "wait"
	}

	decision.Reasoning = extractString("reasoning")
	decision.Text = extractString("text")
	decision.Selector = extractString("selector")
	decision.Value = extractString("value")
	decision.URL = extractString("url")
	decision.Summary = extractString("summary")
	decision.InputPrompt = extractString("input_prompt")
	decision.WaitFor = extractString("wait_for")
	decision.IsComplete = extractBool("is_complete")
	decision.NeedsInput = extractBool("needs_input")

	return decision, nil
}

