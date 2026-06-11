package rules

import (
	"strings"

	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
	"apartments-clone-server/models"

	"gorm.io/gorm"
)

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
		t := strings.TrimSpace(strings.ToLower(p))
		if t == "" {
			continue
		}
		if t == intentKey {
			return true
		}
		// Treat search_any as broad bucket (covers search_* + unknown/help/greeting).
		if t == "search_any" {
			if strings.HasPrefix(intentKey, "search_") || intentKey == "unknown" || intentKey == "help" || intentKey == "greeting" {
				return true
			}
		}
	}
	return false
}

func keywordsMatch(kwCSV, lowerUser string) bool {
	kwCSV = strings.TrimSpace(strings.ToLower(kwCSV))
	if kwCSV == "" {
		return true
	}
	for _, k := range strings.Split(kwCSV, ",") {
		kk := strings.TrimSpace(k)
		if kk != "" && strings.Contains(lowerUser, kk) {
			return true
		}
	}
	return false
}

// ApplyAdminSearchRules expands deterministic DB filters using admin-authored rules.
//
// This intentionally does NOT call any LLM and does not consume tokens.
// It is meant to optimize retrieval/search behavior (RAG-for-search).
//
// Supported mini-format in knowledge entry Body (doc_type=zones|product):
//
//   ZONE_OR:<trigger>=<pipe-separated patterns>
//
// Example:
//   ZONE_OR:اسكان=اسكان|الإسكان|Iskan|النسيم|Alnesim
//
// If user's RawText contains <trigger> (case-insensitive for latin), we append the patterns
// to ctx.Zone (OR list) so DB queries match titles/addresses/districts accordingly.
func ApplyAdminSearchRules(db *gorm.DB, msgCtx *lang.MessageContext) {
	if db == nil || msgCtx == nil {
		return
	}
	raw := strings.TrimSpace(msgCtx.RawText)
	if raw == "" {
		return
	}
	lower := strings.ToLower(raw)

	loc := langCode(msgCtx.Lang)
	ik := intentScopeKey(msgCtx.Intent)

	// Only pull rule-like docs (short list, ordered).
	var rows []models.MeskenyKnowledgeEntry
	if err := db.Model(&models.MeskenyKnowledgeEntry{}).
		Where("active = ?", true).
		Where("doc_type IN (?, ?)", "zones", "product").
		Where("(locale = ? OR locale = ?)", "any", loc).
		Order("priority DESC, id DESC").
		Limit(40).
		Find(&rows).Error; err != nil {
		return
	}

	for i := range rows {
		e := rows[i]
		if !intentMatches(e.IntentScope, ik) {
			continue
		}
		if !keywordsMatch(e.MatchKeywords, lower) {
			continue
		}
		body := e.Body
		if strings.TrimSpace(body) == "" {
			continue
		}
		lines := strings.Split(body, "\n")
		for _, ln := range lines {
			ln = strings.TrimSpace(ln)
			if ln == "" {
				continue
			}
			// Accept both "ZONE_OR:" and "zone_or:" etc.
			up := strings.ToUpper(ln)
			if !strings.HasPrefix(up, "ZONE_OR:") {
				continue
			}
			rest := strings.TrimSpace(ln[len("ZONE_OR:"):])
			parts := strings.SplitN(rest, "=", 2)
			if len(parts) != 2 {
				continue
			}
			trigger := strings.TrimSpace(parts[0])
			patterns := strings.TrimSpace(parts[1])
			if trigger == "" || patterns == "" {
				continue
			}

			// Trigger match: exact substring in raw, and case-insensitive for latin in lower.
			if !strings.Contains(raw, trigger) && !strings.Contains(lower, strings.ToLower(trigger)) {
				continue
			}

			// City default: if admin zone rules fire and city not set, assume Nouakchott.
			if msgCtx.City == "" {
				msgCtx.City = "nouakchott"
			}

			if msgCtx.Zone == "" {
				msgCtx.Zone = patterns
			} else if !strings.Contains(msgCtx.Zone, patterns) {
				msgCtx.Zone = msgCtx.Zone + "|" + patterns
			}
		}
	}
}

