package lang

import "strings"

// minPlausibleListingBudgetMRU — amounts below this are almost never real MRU budgets
// (e.g. "3 غرف" mistaken as 3 MRU).
const minPlausibleListingBudgetMRU int64 = 100_000

// SanitizeBudgetContext clears bogus budget fields after parsing / history merge.
func SanitizeBudgetContext(ctx MessageContext, raw string) MessageContext {
	if looksLikeRoomCount(raw) {
		ctx.Budget = ""
		ctx.BudgetMRU = 0
		ctx.BudgetMin = 0
		ctx.BudgetMax = 0
		return ctx
	}
	if ctx.BudgetMRU > 0 && ctx.BudgetMRU < minPlausibleListingBudgetMRU {
		if !messageHasExplicitCurrency(raw) {
			ctx.Budget = ""
			ctx.BudgetMRU = 0
			ctx.BudgetMin = 0
			ctx.BudgetMax = 0
		}
	}
	if ctx.BudgetMin > 0 && ctx.BudgetMin < minPlausibleListingBudgetMRU &&
		ctx.BudgetMax > 0 && ctx.BudgetMax < minPlausibleListingBudgetMRU &&
		!messageHasExplicitCurrency(raw) {
		ctx.BudgetMin = 0
		ctx.BudgetMax = 0
	}
	return ctx
}

func messageHasExplicitCurrency(raw string) bool {
	norm := strings.ToLower(normalizeDigits(raw))
	for _, sig := range append(mruSignals, mroSignals...) {
		if strings.Contains(norm, sig) {
			return true
		}
	}
	return strings.Contains(norm, "mru") || strings.Contains(norm, "mro") ||
		strings.Contains(raw, "أوقية") || strings.Contains(raw, "اوقية")
}

func isPlausibleBudgetMRU(v int64) bool {
	return v >= minPlausibleListingBudgetMRU
}
