package subagents

import (
	"testing"
)

func TestNewRouter(t *testing.T) {
	router := NewRouter()
	if router == nil {
		t.Fatal("NewRouter returned nil")
	}
	if len(router.agents) == 0 {
		t.Error("Router should have agents")
	}
}

func TestEmailAgent(t *testing.T) {
	agent := NewEmailAgent()
	if agent.GetName() != "EmailAgent" {
		t.Errorf("Expected name 'EmailAgent', got '%s'", agent.GetName())
	}
	
	tests := []struct {
		task     string
		expected bool
	}{
		{"прочитай почту", true},
		{"удалить спам из email", true},
		{"письма в яндекс почте", true},
		{"gmail inbox", true},
		{"заказать еду", false},
		{"найти работу", false},
		{"", false},
	}
	
	for _, tt := range tests {
		result := agent.CanHandle(tt.task)
		if result != tt.expected {
			t.Errorf("EmailAgent.CanHandle(%q) = %v, expected %v", 
				tt.task, result, tt.expected)
		}
	}
}

func TestShoppingAgent(t *testing.T) {
	agent := NewShoppingAgent()
	if agent.GetName() != "ShoppingAgent" {
		t.Errorf("Expected name 'ShoppingAgent', got '%s'", agent.GetName())
	}
	
	tests := []struct {
		task     string
		expected bool
	}{
		{"заказать еду", true},
		{"купить бургер", true},
		{"доставка пиццы", true},
		{"add to cart", true},
		{"прочитай почту", false},
		{"найти работу", false},
		{"", false},
	}
	
	for _, tt := range tests {
		result := agent.CanHandle(tt.task)
		if result != tt.expected {
			t.Errorf("ShoppingAgent.CanHandle(%q) = %v, expected %v", 
				tt.task, result, tt.expected)
		}
	}
}

func TestJobSearchAgent(t *testing.T) {
	agent := NewJobSearchAgent()
	if agent.GetName() != "JobSearchAgent" {
		t.Errorf("Expected name 'JobSearchAgent', got '%s'", agent.GetName())
	}
	
	tests := []struct {
		task     string
		expected bool
	}{
		{"найти вакансии", true},
		{"hh.ru вакансии", true},
		{"откликнуться на работу", true},
		{"job search", true},
		{"прочитай почту", false},
		{"заказать еду", false},
		{"", false},
	}
	
	for _, tt := range tests {
		result := agent.CanHandle(tt.task)
		if result != tt.expected {
			t.Errorf("JobSearchAgent.CanHandle(%q) = %v, expected %v", 
				tt.task, result, tt.expected)
		}
	}
}

func TestRouterRoute(t *testing.T) {
	router := NewRouter()
	
	tests := []struct {
		task         string
		expectedName string
	}{
		{"прочитай почту", "EmailAgent"},
		{"заказать еду", "ShoppingAgent"},
		{"найти вакансии", "JobSearchAgent"},
		{"случайная задача", ""}, // nil agent
	}
	
	for _, tt := range tests {
		agent := router.Route(tt.task)
		if tt.expectedName == "" {
			if agent != nil {
				t.Errorf("Router.Route(%q) should return nil, got %v", tt.task, agent)
			}
		} else {
			if agent == nil {
				t.Errorf("Router.Route(%q) returned nil, expected %s", tt.task, tt.expectedName)
			} else if agent.GetName() != tt.expectedName {
				t.Errorf("Router.Route(%q) returned %s, expected %s", 
					tt.task, agent.GetName(), tt.expectedName)
			}
		}
	}
}

