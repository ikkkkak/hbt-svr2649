package lang

import "strings"

// HistoryTurn is one prior chat message for context merging.
type HistoryTurn struct {
	Role    string
	Content string
}

// EnrichContextFromHistory keeps city/zone/budget/purpose/type from recent turns
// when the user sends a short follow-up (e.g. "buy", "house", "كراء").
func EnrichContextFromHistory(ctx MessageContext, history []HistoryTurn, raw string) MessageContext {
	if strings.TrimSpace(raw) != "" {
		ctx.RawText = strings.TrimSpace(raw)
	}
	if strings.Contains(ctx.RawText, "[MESKENY_PICKER]") {
		if city, zone, quartier, bmin, bmax, ok := parsePickerTag(ctx.RawText); ok {
			ctx.City = city
			ctx.Zone = zone
			ctx.Quartier = quartier
			ctx.BudgetMin = bmin
			ctx.BudgetMax = bmax
			ctx.FromPicker = true
			if !IsPropertySearchIntent(ctx.Intent) {
				ctx.Intent = IntentSearchAny
			}
		}
	}

	needLoc := strings.TrimSpace(ctx.City) == "" && strings.TrimSpace(ctx.Zone) == "" && strings.TrimSpace(ctx.Quartier) == ""
	needBudget := ctx.BudgetMRU == 0 && ctx.BudgetMin == 0 && ctx.BudgetMax == 0
	needPurpose := !IsExplicitPurposeIntent(ctx.Intent)
	needType := strings.TrimSpace(ctx.Type) == "" && ctx.Intent != IntentSearchLand

	if !needLoc && !needBudget && !needPurpose && !needType {
		return ctx
	}

	for i := len(history) - 1; i >= 0; i-- {
		if strings.ToLower(strings.TrimSpace(history[i].Role)) != "user" {
			continue
		}
		content := strings.TrimSpace(history[i].Content)
		if content == "" {
			continue
		}
		if strings.Contains(content, "[MESKENY_PICKER]") {
			if city, zone, quartier, bmin, bmax, ok := parsePickerTag(content); ok {
				if needLoc {
					ctx.City = city
					ctx.Zone = zone
					ctx.Quartier = quartier
					needLoc = false
				}
				if needBudget && (bmin > 0 || bmax > 0) {
					ctx.BudgetMin = bmin
					ctx.BudgetMax = bmax
					needBudget = false
				}
			}
		}
		prev := AnalyzeMessage(content)
		if needPurpose && IsExplicitPurposeIntent(prev.Intent) {
			ctx.Intent = prev.Intent
			needPurpose = false
		}
		if needType && strings.TrimSpace(prev.Type) != "" {
			ctx.Type = prev.Type
			needType = false
		}
		if needLoc {
			if strings.TrimSpace(prev.City) != "" {
				ctx.City = prev.City
			}
			if strings.TrimSpace(prev.Zone) != "" {
				ctx.Zone = prev.Zone
			}
			if strings.TrimSpace(prev.Quartier) != "" {
				ctx.Quartier = prev.Quartier
			}
			if strings.TrimSpace(ctx.City) != "" || strings.TrimSpace(ctx.Zone) != "" {
				needLoc = false
			}
		}
		if needBudget && !looksLikeRoomCount(content) {
			if prev.BudgetMRU > 0 && isPlausibleBudgetMRU(prev.BudgetMRU) {
				ctx.BudgetMRU = prev.BudgetMRU
				needBudget = false
			} else if prev.BudgetMin >= minPlausibleListingBudgetMRU || prev.BudgetMax >= minPlausibleListingBudgetMRU {
				ctx.BudgetMin = prev.BudgetMin
				ctx.BudgetMax = prev.BudgetMax
				needBudget = false
			}
		}
		if !needLoc && !needBudget && !needPurpose && !needType {
			break
		}
	}
	return ReconcileTransactionFromMessage(SanitizeBudgetContext(ctx, raw))
}
