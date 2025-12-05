package security

import (
	"testing"
)

func TestNewLayer(t *testing.T) {
	layer := NewLayer(true)
	if layer == nil {
		t.Fatal("NewLayer returned nil")
	}
	if !layer.enabled {
		t.Error("Expected enabled=true")
	}
	
	layerDisabled := NewLayer(false)
	if layerDisabled.enabled {
		t.Error("Expected enabled=false")
	}
}

func TestIsDestructiveAction(t *testing.T) {
	layer := NewLayer(true)
	
	tests := []struct {
		action   string
		expected bool
	}{
		{"delete", true},
		{"удалить", true},
		{"pay", true},
		{"оплатить", true},
		{"submit", true},
		{"click", false},
		{"navigate", false},
		{"read", false},
		{"", false},
	}
	
	for _, tt := range tests {
		result := layer.IsDestructiveAction(tt.action)
		if result != tt.expected {
			t.Errorf("IsDestructiveAction(%q) = %v, expected %v", 
				tt.action, result, tt.expected)
		}
	}
}

func TestIsDestructiveActionDisabled(t *testing.T) {
	layer := NewLayer(false)
	
	// Когда security layer отключен, все действия не деструктивные
	if layer.IsDestructiveAction("delete") {
		t.Error("Expected false when layer is disabled")
	}
}

