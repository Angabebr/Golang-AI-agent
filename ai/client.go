package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/Angabebr/Golang-AI-agent/browser"
	"github.com/sashabaranov/go-openai"
)

type Client struct {
	client      *openai.Client
	model       string
	systemPrompt string
}

func NewClient(apiKey, model string) *Client {
	if model == "" {
		model = "gpt-4-turbo-preview"
	}

	return &Client{
		client: openai.NewClient(apiKey),
		model:  model,
		systemPrompt: "", // Будет использован дефолтный из MakeDecision
	}
}

// GetSystemPrompt возвращает текущий системный промпт
func (c *Client) GetSystemPrompt() string {
	return c.systemPrompt
}

// SetSystemPrompt устанавливает кастомный системный промпт
func (c *Client) SetSystemPrompt(prompt string) {
	c.systemPrompt = prompt
}

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

func (c *Client) MakeDecision(ctx context.Context, task string, pageContent interface{}, history []string, maxTokens int) (*Decision, error) {
	prompt := c.buildPrompt(task, pageContent, history)

	// Используем кастомный системный промпт, если он установлен, иначе дефолтный
	systemContent := c.systemPrompt
	if systemContent == "" {
		systemContent = `Ты - автономный AI-агент, который управляет веб-браузером для выполнения задач пользователя.

Твоя задача - анализировать текущее состояние веб-страницы и АВТОНОМНО принимать решения о следующих действиях, БЕЗ использования заготовленных планов или шаблонов.

Доступные действия:
1. navigate - перейти на URL
   - Можешь использовать URL из списка links.href ИЛИ указать прямой URL (например, "https://mail.ru")
   - Заполни: "url" (полный URL, например "https://mail.ru" или из списка links)
   
2. click - кликнуть на элемент
   - ОБЯЗАТЕЛЬНО заполни: "text" (видимый текст из списка buttons или links)
   - Или если text не работает: "selector" (CSS селектор)
   
3. fill - заполнить поле ввода
   - ОБЯЗАТЕЛЬНО заполни: "text" (placeholder, name, aria-label из списка inputs)
   - ОБЯЗАТЕЛЬНО заполни: "value" (значение для ввода)
   - Для полей поиска можно использовать общие термины: "искать", "search", "поиск"
   - Или если text не работает: "selector" (CSS селектор) + "value"
   
4. wait - подождать
   - Опционально: "wait_for" (селектор элемента)
   
5. extract - извлечь информацию (уже сделано автоматически)
6. complete - задача выполнена ТОЛЬКО когда задача действительно выполнена

КРИТИЧЕСКИ ВАЖНО - ПРАВИЛА ЗАПОЛНЕНИЯ ПОЛЕЙ:
- Для действия "navigate": Можешь использовать URL из списка links ИЛИ указать прямой URL (например, "https://mail.ru", "https://e.mail.ru")
- Для действия "click": ВСЕГДА заполняй "text" из списка buttons/links
- Для действия "fill": ВСЕГДА заполняй "text" (из inputs.placeholder, inputs.name, inputs.aria-label) И "value"
- Для полей поиска можно использовать общие термины: "искать", "search", "поиск" - система найдет поле автоматически
- НЕ завершай задачу (complete) если просто не можешь найти ссылку - используй navigate с прямым URL
- НЕ используй заготовленные селекторы - анализируй ТОЛЬКО данные текущей страницы
- Отвечай ТОЛЬКО в формате JSON, без дополнительного текста до или после JSON

Формат ответа (строго валидный JSON):

Для клика:
{
  "action": "click",
  "reasoning": "объяснение",
  "text": "ОБЯЗАТЕЛЬНО: текст кнопки из списка buttons или links"
}

Для заполнения:
{
  "action": "fill",
  "reasoning": "объяснение",
  "text": "ОБЯЗАТЕЛЬНО: placeholder или name из списка inputs",
  "value": "ОБЯЗАТЕЛЬНО: что вводить"
}

Для навигации:
{
  "action": "navigate",
  "reasoning": "объяснение",
  "url": "полный URL (можно из списка links ИЛИ прямой URL, например 'https://mail.ru')"
}

Для ожидания:
{
  "action": "wait",
  "reasoning": "объяснение",
  "wait_for": "опционально: селектор элемента"
}

Для завершения:
{
  "action": "complete",
  "reasoning": "объяснение",
  "is_complete": true,
  "summary": "что было выполнено"
}`
	}

	messages := []openai.ChatCompletionMessage{
		{
			Role:    openai.ChatMessageRoleSystem,
			Content: systemContent,
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

	content := resp.Choices[0].Message.Content
	decision, err := parseDecision(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse decision: %w", err)
	}

	return decision, nil
}

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
			Role:    openai.ChatMessageRoleSystem,
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

	content := resp.Choices[0].Message.Content
	isDestructive := strings.Contains(strings.ToLower(content), `"is_destructive": true`) ||
		strings.Contains(strings.ToLower(content), `is_destructive: true`)

	description := "Действие может привести к необратимым изменениям"
	if strings.Contains(content, `"description"`) {
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

func (c *Client) buildPrompt(task string, pageContent interface{}, history []string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("Задача пользователя: %s\n\n", task))

	// История действий (только последние 5-7 для экономии токенов)
	if len(history) > 0 {
		sb.WriteString("История последних действий:\n")
		startIdx := len(history) - 7
		if startIdx < 0 {
			startIdx = 0
		}
		for i := startIdx; i < len(history); i++ {
			sb.WriteString(fmt.Sprintf("- %s\n", history[i]))
		}
		sb.WriteString("\n")
	}

	// Умное форматирование содержимого страницы
	sb.WriteString("Текущее состояние страницы:\n")
	
	// Проверяем, быстрая ли это информация или полная
	if quickInfo, ok := pageContent.(*browser.QuickPageInfo); ok {
		// Быстрая информация для простых действий
		sb.WriteString(fmt.Sprintf("URL: %s\n", quickInfo.URL))
		sb.WriteString(fmt.Sprintf("Title: %s\n", quickInfo.Title))
		
		if len(quickInfo.Links) > 0 {
			sb.WriteString("\nДоступные ссылки (первые 15):\n")
			maxLinks := 15
			if len(quickInfo.Links) < maxLinks {
				maxLinks = len(quickInfo.Links)
			}
			for i := 0; i < maxLinks; i++ {
				link := quickInfo.Links[i]
				sb.WriteString(fmt.Sprintf("  - %s -> %s\n", link.Text, link.Href))
			}
		}
		
		if len(quickInfo.Buttons) > 0 {
			sb.WriteString("\nДоступные кнопки:\n")
			for _, btn := range quickInfo.Buttons {
				sb.WriteString(fmt.Sprintf("  - %s\n", btn))
			}
		}
	} else if pc, ok := pageContent.(*browser.PageContent); ok {
		sb.WriteString(fmt.Sprintf("URL: %s\n", pc.URL))
		sb.WriteString(fmt.Sprintf("Title: %s\n", pc.Title))
		
		if len(pc.Headings) > 0 {
			sb.WriteString("\nЗаголовки:\n")
			for _, h := range pc.Headings {
				sb.WriteString(fmt.Sprintf("  %s: %s\n", h.Level, h.Text))
			}
		}
		
		if len(pc.Buttons) > 0 {
			sb.WriteString("\nДоступные кнопки:\n")
			for _, btn := range pc.Buttons {
				sb.WriteString(fmt.Sprintf("  - %s\n", btn.Text))
			}
		}
		
		if len(pc.Links) > 0 {
			sb.WriteString("\nДоступные ссылки (первые 15):\n")
			maxLinks := 15
			if len(pc.Links) < maxLinks {
				maxLinks = len(pc.Links)
			}
			for i := 0; i < maxLinks; i++ {
				link := pc.Links[i]
				sb.WriteString(fmt.Sprintf("  - %s -> %s\n", link.Text, link.Href))
			}
		}
		
		if len(pc.Inputs) > 0 {
			sb.WriteString("\nДоступные поля ввода:\n")
			for _, inp := range pc.Inputs {
				label := inp.Label
				if label == "" {
					label = inp.Placeholder
				}
				if label == "" {
					label = inp.Name
				}
				if label == "" {
					label = inp.ID
				}
				sb.WriteString(fmt.Sprintf("  - %s (%s)\n", label, inp.Type))
			}
		}
		
		// Краткий текст страницы (первые 3000 символов)
		if len(pc.Text) > 0 {
			textPreview := pc.Text
			if len(textPreview) > 3000 {
				textPreview = textPreview[:3000] + "..."
			}
			sb.WriteString(fmt.Sprintf("\nТекст страницы:\n%s\n", textPreview))
		}
		
		// Списки и таблицы для структурированных данных
		if len(pc.Lists) > 0 {
			sb.WriteString("\nСписки на странице:\n")
			for i, list := range pc.Lists {
				if i >= 3 {
					break
				}
				for j, item := range list {
					if j >= 5 {
						break
					}
					sb.WriteString(fmt.Sprintf("  - %s\n", item))
				}
			}
		}
		
		// Таблицы (трехмерный массив: таблицы -> строки -> ячейки)
		if len(pc.Tables) > 0 {
			sb.WriteString("\nТаблицы на странице:\n")
			for i, table := range pc.Tables {
				if i >= 2 {
					break
				}
				for j, row := range table {
					if j >= 5 {
						break
					}
					rowStr := strings.Join(row, " | ")
					sb.WriteString(fmt.Sprintf("  %s\n", rowStr))
				}
			}
		}
	} else {
		// Fallback для других типов
		sb.WriteString(fmt.Sprintf("%+v\n", pageContent))
	}

	sb.WriteString("\nКакое следующее действие нужно выполнить? Ответь в формате JSON.")

	return sb.String()
}

func parseDecision(content string) (*Decision, error) {
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

	jsonRegex := regexp.MustCompile(`\{[^{}]*"action"[^{}]*\}`)
	jsonMatch := jsonRegex.FindString(content)
	if jsonMatch == "" {
		jsonRegex = regexp.MustCompile(`\{[\s\S]*?\}`)
		jsonMatch = jsonRegex.FindString(content)
	}

	if jsonMatch != "" {
		content = jsonMatch
	}

	decision := &Decision{
		Action:     "wait",
		IsComplete: false,
		Metadata:   make(map[string]string),
	}

	if err := json.Unmarshal([]byte(content), decision); err != nil {
		return parseDecisionFallback(content)
	}

	if decision.Metadata == nil {
		decision.Metadata = make(map[string]string)
	}

	return decision, nil
}

func parseDecisionFallback(content string) (*Decision, error) {
	decision := &Decision{
		Action:     "wait",
		IsComplete: false,
		Metadata:   make(map[string]string),
	}

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
