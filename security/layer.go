package security

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Layer проверяет деструктивные действия и запрашивает подтверждение
type Layer struct {
	enabled bool
}

// NewLayer создает новый security layer
func NewLayer(enabled bool) *Layer {
	return &Layer{
		enabled: enabled,
	}
}

// CheckAction проверяет действие и запрашивает подтверждение если нужно
func (s *Layer) CheckAction(action, description string, isDestructive bool) (bool, error) {
	if !s.enabled {
		return true, nil
	}

	if !isDestructive {
		return true, nil
	}

	// Показываем предупреждение
	fmt.Println("\n⚠️  ВНИМАНИЕ: Деструктивное действие обнаружено!")
	fmt.Printf("Действие: %s\n", action)
	fmt.Printf("Описание: %s\n", description)
	fmt.Print("\nПродолжить? (yes/no): ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, fmt.Errorf("failed to read user input: %w", err)
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "yes" || response == "y" || response == "да", nil
}

// IsDestructiveAction проверяет, является ли действие деструктивным по ключевым словам
func (s *Layer) IsDestructiveAction(action string) bool {
	if !s.enabled {
		return false
	}

	actionLower := strings.ToLower(action)
	destructiveKeywords := []string{
		"delete", "удалить", "удаление",
		"remove", "убрать",
		"pay", "оплатить", "оплата", "payment",
		"submit", "отправить", "подтвердить",
		"confirm", "подтверждение",
		"purchase", "покупка", "купить",
		"send", "отправить",
		"clear", "очистить",
		"reset", "сбросить",
	}

	for _, keyword := range destructiveKeywords {
		if strings.Contains(actionLower, keyword) {
			return true
		}
	}

	return false
}

