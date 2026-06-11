package agent

import (
	"strings"

	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
)

const (
	RolePropertySearcher = "PropertySearcher"
	RolePropertyAdvisor  = "PropertyAdvisor"
	RoleMarketAnalyst    = "MarketAnalyst"
)

func RouteRole(msgCtx lang.MessageContext, text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	if isMarketAnalyst(lower, text) {
		return RoleMarketAnalyst
	}
	if lang.IsPropertySearchIntent(msgCtx.Intent) {
		return RolePropertySearcher
	}
	if isPropertyAdvisor(msgCtx, lower, text) {
		return RolePropertyAdvisor
	}
	return RolePropertyAdvisor
}

func isMarketAnalyst(lower, raw string) bool {
	if strings.Contains(raw, "market value") || strings.Contains(raw, "valuation") ||
		strings.Contains(raw, "تقييم") || strings.Contains(raw, "القيمة السوقية") {
		return true
	}
	for _, k := range []string{
		"market trend", "market stats", "price evolution", "investment",
		"marché", "tendance", "évolution des prix", "statistique",
		"سوق", "اتجاه", "مقارنة", "استثمار",
	} {
		if strings.Contains(lower, k) || strings.Contains(raw, k) {
			return true
		}
	}
	return false
}

func isPropertyAdvisor(msgCtx lang.MessageContext, lower, raw string) bool {
	if msgCtx.Intent == lang.IntentHelp || msgCtx.Intent == lang.IntentGreeting {
		return true
	}
	for _, k := range []string{
		"recommend", "advice", "should i", "which one", "compare",
		"conseil", "recommand", "quelle", "meilleur", "نصيحة", "أنصح",
	} {
		if strings.Contains(lower, k) || strings.Contains(raw, k) {
			return true
		}
	}
	return false
}
