package rag

import (
	"context"
	"fmt"
	"strings"

	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
	"apartments-clone-server/models"

	"gorm.io/gorm"
)

// maxKnowledgeChars caps injected admin knowledge so system prompts stay token-efficient.
const maxKnowledgeChars = 2400

// NewCompositeRetriever returns Mauritania static context plus DB-backed admin snippets.
func NewCompositeRetriever(db *gorm.DB) Retriever {
	return &compositeRetriever{db: db, base: &mauritaniaRetriever{}}
}

type compositeRetriever struct {
	db   *gorm.DB
	base Retriever
}

func (c *compositeRetriever) Retrieve(ctx context.Context, msgCtx lang.MessageContext) (RAGContext, error) {
	out, err := c.base.Retrieve(ctx, msgCtx)
	if err != nil {
		out = RAGContext{}
	}
	if c.db == nil {
		return out, nil
	}
	snips := selectKnowledgeSnippets(c.db, msgCtx, maxKnowledgeChars)
	out.FAQSnippets = append(out.FAQSnippets, snips...)
	return out, nil
}

func langCode(l lang.Lang) string {
	switch l {
	case lang.LangAR:
		return "ar"
	case lang.LangEN:
		return "en"
	default:
		return "fr"
	}
}

func intentScopeKey(i lang.Intent) string {
	switch i {
	case lang.IntentSearchRent:
		return "search_rent"
	case lang.IntentSearchBuy:
		return "search_buy"
	case lang.IntentSearchAny:
		return "search_any"
	case lang.IntentSearchLand:
		return "search_land"
	case lang.IntentSearchCommercial:
		return "search_commercial"
	case lang.IntentSearchByBudget:
		return "search_budget"
	case lang.IntentSearchByLocation:
		return "search_location"
	case lang.IntentSearchByRooms:
		return "search_rooms"
	case lang.IntentSearchByType:
		return "search_type"
	case lang.IntentGreeting:
		return "greeting"
	case lang.IntentHelp:
		return "help"
	default:
		return "unknown"
	}
}

func intentMatches(scope, intentKey string) bool {
	scope = strings.TrimSpace(strings.ToLower(scope))
	if scope == "" || scope == "all" {
		return true
	}
	intentKey = strings.ToLower(intentKey)
	for _, p := range strings.Split(scope, ",") {
		t := strings.TrimSpace(p)
		if t == "" {
			continue
		}
		if t == intentKey {
			return true
		}
		// Dashboard label "General search" (search_any): treat as a broad bucket so FAQ-style
		// rows still apply when AnalyzeMessage returns unknown/help/greeting, not only IntentSearchAny.
		if t == "search_any" {
			if intentKey == "search_any" || intentKey == "unknown" || intentKey == "help" || intentKey == "greeting" {
				return true
			}
			if strings.HasPrefix(intentKey, "search_") {
				return true
			}
		}
	}
	return false
}

func keywordsMatch(kwCSV, lowerUser string) bool {
	kwCSV = strings.TrimSpace(kwCSV)
	if kwCSV == "" {
		return true
	}
	for _, k := range strings.Split(kwCSV, ",") {
		kk := strings.TrimSpace(strings.ToLower(k))
		if kk != "" && strings.Contains(lowerUser, kk) {
			return true
		}
	}
	return false
}

func formatKnowledgeEntry(e *models.MeskenyKnowledgeEntry) string {
	return fmt.Sprintf("[%s | %s]\n%s",
		strings.ToUpper(strings.TrimSpace(e.DocType)),
		strings.TrimSpace(e.Title),
		strings.TrimSpace(e.Body))
}

func selectKnowledgeSnippets(db *gorm.DB, msgCtx lang.MessageContext, maxChars int) []string {
	loc := langCode(msgCtx.Lang)
	ik := intentScopeKey(msgCtx.Intent)
	lowerUser := strings.ToLower(strings.TrimSpace(msgCtx.RawText))

	var rows []models.MeskenyKnowledgeEntry
	q := db.Model(&models.MeskenyKnowledgeEntry{}).
		Where("active = ?", true).
		Where("(locale = ? OR locale = ?)", "any", loc).
		Order("priority DESC, id DESC").
		Limit(80)
	if err := q.Find(&rows).Error; err != nil {
		return nil
	}

	var matched []models.MeskenyKnowledgeEntry
	for i := range rows {
		e := rows[i]
		if !intentMatches(e.IntentScope, ik) {
			continue
		}
		if !keywordsMatch(e.MatchKeywords, lowerUser) {
			continue
		}
		matched = append(matched, e)
		if len(matched) >= 12 {
			break
		}
	}

	var out []string
	used := 0
	for i := range matched {
		block := formatKnowledgeEntry(&matched[i])
		if len(block) > maxChars-used {
			if used >= maxChars {
				break
			}
			remain := maxChars - used - 20
			if remain < 40 {
				break
			}
			block = block[:remain] + "\n…"
		}
		out = append(out, block)
		used += len(block) + 4
		if used >= maxChars {
			break
		}
	}
	return out
}
