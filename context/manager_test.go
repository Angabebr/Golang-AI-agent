package context

import (
	"fmt"
	"testing"
)

func TestNewManager(t *testing.T) {
	manager := NewManager(1000)
	if manager == nil {
		t.Fatal("NewManager returned nil")
	}
	if manager.maxTokens != 1000 {
		t.Errorf("Expected maxTokens=1000, got %d", manager.maxTokens)
	}
}

func TestAddAction(t *testing.T) {
	manager := NewManager(1000)
	manager.AddAction("test action")
	
	history := manager.GetHistory()
	if len(history) != 1 {
		t.Errorf("Expected history length 1, got %d", len(history))
	}
	if history[0] != "test action" {
		t.Errorf("Expected 'test action', got '%s'", history[0])
	}
}

func TestHistoryLimit(t *testing.T) {
	manager := NewManager(1000)
	// Добавляем больше действий, чем maxHistory (10)
	for i := 0; i < 15; i++ {
		manager.AddAction(fmt.Sprintf("action %d", i))
	}
	
	history := manager.GetHistory()
	if len(history) > 10 {
		t.Errorf("Expected history length <= 10, got %d", len(history))
	}
	// Проверяем, что последние действия сохранены
	if history[len(history)-1] != "action 14" {
		t.Errorf("Expected last action 'action 14', got '%s'", history[len(history)-1])
	}
}

func TestEstimateTokens(t *testing.T) {
	manager := NewManager(1000)
	
	tests := []struct {
		text     string
		minTokens int
		maxTokens int
	}{
		{"short", 1, 5},
		{"this is a longer text with more words", 10, 20},
		{"", 0, 1},
	}
	
	for _, tt := range tests {
		tokens := manager.EstimateTokens(tt.text)
		if tokens < tt.minTokens || tokens > tt.maxTokens {
			t.Errorf("EstimateTokens(%q) = %d, expected between %d and %d", 
				tt.text, tokens, tt.minTokens, tt.maxTokens)
		}
	}
}

func TestBuildContext(t *testing.T) {
	manager := NewManager(1000)
	manager.AddAction("action 1")
	manager.AddAction("action 2")
	
	pageContent := map[string]string{"title": "Test Page"}
	context := manager.BuildContext("test task", pageContent, manager.GetHistory())
	
	if context == "" {
		t.Fatal("BuildContext returned empty string")
	}
	if !contains(context, "test task") {
		t.Error("Context should contain task")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && 
		(s == substr || 
		 (len(s) > len(substr) && 
		  (s[:len(substr)] == substr || 
		   s[len(s)-len(substr):] == substr ||
		   findSubstring(s, substr))))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

