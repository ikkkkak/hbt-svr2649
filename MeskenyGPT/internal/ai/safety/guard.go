package safety

import "strings"

// Guard performs basic safety checks (bad words etc.).
// This is intentionally tiny for now; you can port your existing bad-word
// logic from apartmentscloneserver/services/ai_service.go later.
type Guard interface {
	Check(msg string) (blocked bool, reason string)
}

type guard struct{}

func NewGuard() Guard {
	return &guard{}
}

func (g *guard) Check(msg string) (bool, string) {
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "fuck") || strings.Contains(lower, "shit") {
		return true, "Votre message contient un langage inapproprié."
	}
	return false, ""
}

