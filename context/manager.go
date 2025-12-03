package context

import (
	"fmt"
	"strings"
)

// Manager управляет контекстом с учетом ограничений по токенам
type Manager struct {
	maxTokens     int
	currentTokens int
	history       []string
	maxHistory    int
}

// NewManager создает новый менеджер контекста
func NewManager(maxTokens int) *Manager {
	return &Manager{
		maxTokens:  maxTokens,
		maxHistory: 10, // Храним последние 10 действий
	}
}

// AddAction добавляет действие в историю
func (m *Manager) AddAction(action string) {
	m.history = append(m.history, action)
	if len(m.history) > m.maxHistory {
		m.history = m.history[1:]
	}
}

// GetHistory возвращает историю действий
func (m *Manager) GetHistory() []string {
	return m.history
}

// SummarizePageContent создает краткое описание страницы для экономии токенов
func (m *Manager) SummarizePageContent(content interface{}) string {
	// Преобразуем в строку и обрезаем если слишком длинная
	str := fmt.Sprintf("%+v", content)
	
	// Ограничиваем размер описания страницы
	maxPageDesc := 2000
	if len(str) > maxPageDesc {
		str = str[:maxPageDesc] + "... (truncated)"
	}
	
	return str
}

// BuildContext строит контекст для AI с учетом лимитов
func (m *Manager) BuildContext(task string, pageContent interface{}, recentActions []string) string {
	var sb strings.Builder

	// Задача всегда включается
	sb.WriteString(fmt.Sprintf("Задача: %s\n\n", task))

	// Недавние действия (последние 3-5)
	if len(recentActions) > 0 {
		sb.WriteString("Последние действия:\n")
		start := len(recentActions) - 5
		if start < 0 {
			start = 0
		}
		for i := start; i < len(recentActions); i++ {
			sb.WriteString(fmt.Sprintf("- %s\n", recentActions[i]))
		}
		sb.WriteString("\n")
	}

	// Краткое описание страницы
	pageDesc := m.SummarizePageContent(pageContent)
	sb.WriteString(fmt.Sprintf("Текущая страница:\n%s\n", pageDesc))

	return sb.String()
}

// EstimateTokens приблизительно оценивает количество токенов в тексте
func (m *Manager) EstimateTokens(text string) int {
	// Простая оценка: ~4 символа на токен для английского, ~2 для русского
	// Используем среднее значение
	return len(text) / 3
}

// CanAdd проверяет, можно ли добавить еще контент
func (m *Manager) CanAdd(additionalText string) bool {
	estimated := m.EstimateTokens(additionalText)
	return m.currentTokens+estimated < m.maxTokens
}

// Reset сбрасывает состояние
func (m *Manager) Reset() {
	m.currentTokens = 0
	m.history = []string{}
}

