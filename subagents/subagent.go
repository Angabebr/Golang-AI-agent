package subagents

import (
	"context"
	"fmt"
	"strings"

	"github.com/Angabebr/Golang-AI-agent/ai"
	"github.com/Angabebr/Golang-AI-agent/browser"
)

// SubAgent специализированный агент для конкретного типа задач
type SubAgent interface {
	Execute(ctx context.Context, task string, browser *browser.Browser, aiClient *ai.Client) error
	CanHandle(task string) bool
	GetName() string
}

// Router определяет, какой под-агент должен обработать задачу
type Router struct {
	agents []SubAgent
}

// NewRouter создает новый роутер под-агентов
func NewRouter() *Router {
	return &Router{
		agents: []SubAgent{
			NewEmailAgent(),
			NewShoppingAgent(),
			NewJobSearchAgent(),
		},
	}
}

// Route определяет подходящего агента для задачи
func (r *Router) Route(task string) SubAgent {
	for _, agent := range r.agents {
		if agent.CanHandle(task) {
			return agent
		}
	}
	return nil // Используем основной агент
}

// EmailAgent специализированный агент для работы с почтой
type EmailAgent struct {
	name string
}

func NewEmailAgent() *EmailAgent {
	return &EmailAgent{
		name: "EmailAgent",
	}
}

func (e *EmailAgent) GetName() string {
	return e.name
}

func (e *EmailAgent) CanHandle(task string) bool {
	keywords := []string{"почт", "email", "письм", "спам", "mail", "яндекс.почт", "gmail"}
	taskLower := task
	for _, keyword := range keywords {
		if contains(taskLower, keyword) {
			return true
		}
	}
	return false
}

func (e *EmailAgent) Execute(ctx context.Context, task string, browser *browser.Browser, aiClient *ai.Client) error {
	// EmailAgent использует общую логику основного агента
	// но может иметь специализированные промпты или стратегии
	fmt.Printf("📧 EmailAgent обрабатывает задачу: %s\n", task)
	return nil
}

// ShoppingAgent специализированный агент для покупок и заказов
type ShoppingAgent struct {
	name string
}

func NewShoppingAgent() *ShoppingAgent {
	return &ShoppingAgent{
		name: "ShoppingAgent",
	}
}

func (s *ShoppingAgent) GetName() string {
	return s.name
}

func (s *ShoppingAgent) CanHandle(task string) bool {
	keywords := []string{"заказ", "купить", "корзин", "доставк", "еда", "ресторан", "бургер", "order", "cart", "delivery"}
	taskLower := task
	for _, keyword := range keywords {
		if contains(taskLower, keyword) {
			return true
		}
	}
	return false
}

func (s *ShoppingAgent) Execute(ctx context.Context, task string, browser *browser.Browser, aiClient *ai.Client) error {
	fmt.Printf("🛒 ShoppingAgent обрабатывает задачу: %s\n", task)
	return nil
}

// JobSearchAgent специализированный агент для поиска работы
type JobSearchAgent struct {
	name string
}

func NewJobSearchAgent() *JobSearchAgent {
	return &JobSearchAgent{
		name: "JobSearchAgent",
	}
}

func (j *JobSearchAgent) GetName() string {
	return j.name
}

func (j *JobSearchAgent) CanHandle(task string) bool {
	keywords := []string{"ваканс", "резюме", "hh.ru", "работа", "job", "vacancy", "отклик", "рекрутер"}
	taskLower := task
	for _, keyword := range keywords {
		if contains(taskLower, keyword) {
			return true
		}
	}
	return false
}

func (j *JobSearchAgent) Execute(ctx context.Context, task string, browser *browser.Browser, aiClient *ai.Client) error {
	fmt.Printf("💼 JobSearchAgent обрабатывает задачу: %s\n", task)
	return nil
}

// contains проверяет, содержит ли строка подстроку (без учета регистра)
func contains(s, substr string) bool {
	s = strings.ToLower(s)
	substr = strings.ToLower(substr)
	return strings.Contains(s, substr)
}

