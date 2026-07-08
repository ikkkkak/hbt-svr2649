package escalation

import (
	"context"
	"fmt"
	"strings"
	"time"

	"apartments-clone-server/models"
	"apartments-clone-server/storage"

	"gorm.io/gorm"
)

var escalationKeywords = []string{
	"negotiate", "negotiation", "dispute", "lawsuit", "legal", "fraud", "scam",
	"complaint", "refund", "lawyer", "attorney", "human agent", "real person",
	"speak to someone", "manager", "supervisor", "frustrated", "angry",
	"مفاوض", "نزاع", "قانون", "محام", "شخص حقيقي", "مختص", "غاضب",
	"négoci", "litige", "avocat", "personne réelle", "spécialiste", "frustr",
}

type Message struct {
	Role    string
	Content string
}

type Trigger struct {
	Type    string
	Score   float64
	Reason  string
	Urgency string
	Context string
}

type Engine struct {
	db *gorm.DB
}

func NewEngine(db *gorm.DB) *Engine {
	if db == nil {
		db = storage.DB
	}
	return &Engine{db: db}
}

func Enabled() bool {
	return true
}

func (e *Engine) Evaluate(_ context.Context, messages []Message) *Trigger {
	if !Enabled() || len(messages) == 0 {
		return nil
	}
	last := strings.ToLower(messages[len(messages)-1].Content)
	for _, kw := range escalationKeywords {
		if strings.Contains(last, strings.ToLower(kw)) {
			return &Trigger{
				Type:    "explicit",
				Score:   1,
				Reason:  fmt.Sprintf("Escalation keyword detected: %s", kw),
				Urgency: urgencyFromText(last),
				Context: summarize(messages),
			}
		}
	}
	neg := 0
	negWords := []string{"bad", "terrible", "awful", "hate", "worst", "useless", "disappointed"}
	for _, m := range messages {
		lower := strings.ToLower(m.Content)
		for _, w := range negWords {
			if strings.Contains(lower, w) {
				neg++
			}
		}
	}
	if neg >= 2 {
		return &Trigger{
			Type:    "sentiment",
			Score:   0.75,
			Reason:  fmt.Sprintf("Negative sentiment indicators: %d", neg),
			Urgency: "medium",
			Context: summarize(messages),
		}
	}
	if len(messages) >= 30 {
		return &Trigger{
			Type:    "length",
			Score:   0.6,
			Reason:  "Long conversation without resolution",
			Urgency: "medium",
			Context: summarize(messages),
		}
	}
	return nil
}

func (e *Engine) Execute(ctx context.Context, trigger *Trigger, sessionID string, userID *uint) (*models.AIEscalation, error) {
	if trigger == nil {
		return nil, fmt.Errorf("nil trigger")
	}
	row := &models.AIEscalation{
		SessionID:      sessionID,
		UserID:         userID,
		TriggerType:    trigger.Type,
		TriggerScore:   trigger.Score,
		Reason:         trigger.Reason,
		Urgency:        trigger.Urgency,
		ContextSummary: trigger.Context,
		Status:         "pending",
	}
	if err := e.db.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func urgencyFromText(s string) string {
	for _, kw := range []string{"urgent", "emergency", "asap", "immediately", "now", "عاجل", "فور"} {
		if strings.Contains(s, kw) {
			return "urgent"
		}
	}
	if strings.Contains(s, "frustrated") || strings.Contains(s, "angry") || strings.Contains(s, "غاضب") {
		return "high"
	}
	return "medium"
}

func summarize(messages []Message) string {
	start := 0
	if len(messages) > 10 {
		start = len(messages) - 10
	}
	var b strings.Builder
	b.WriteString("Conversation summary:\n")
	for i, m := range messages[start:] {
		role := "User"
		if m.Role == "assistant" {
			role = "AI"
		}
		content := m.Content
		if len(content) > 200 {
			content = content[:200] + "..."
		}
		fmt.Fprintf(&b, "%d. [%s]: %s\n", i+1, role, content)
	}
	return b.String()
}

func (e *Engine) GetByID(ctx context.Context, id uint) (*models.AIEscalation, error) {
	var row models.AIEscalation
	if err := e.db.WithContext(ctx).First(&row, id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (e *Engine) Resolve(ctx context.Context, id uint, notes string) error {
	now := time.Now()
	return e.db.WithContext(ctx).Model(&models.AIEscalation{}).Where("id = ?", id).Updates(map[string]any{
		"status":           "resolved",
		"resolution_notes": notes,
		"resolved_at":      &now,
	}).Error
}
