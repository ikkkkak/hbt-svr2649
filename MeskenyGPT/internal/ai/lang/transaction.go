package lang

import "strings"

// normalizeArabicQuery folds common Arabic spelling variants for keyword matching.
func normalizeArabicQuery(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return ""
	}
	repl := strings.NewReplacer(
		"أ", "ا", "إ", "ا", "آ", "ا", "ٱ", "ا",
		"ى", "ي", "ئ", "ي", "ؤ", "و", "ة", "ه",
		"َ", "", "ُ", "", "ِ", "", "ّ", "", "ْ", "", "ـ", "",
	)
	return repl.Replace(s)
}

// MessageSignalsRent detects rent/lease intent in Mauritanian Arabic, French, and English.
func MessageSignalsRent(msg string) bool {
	if strings.TrimSpace(msg) == "" {
		return false
	}
	n := normalizeArabicQuery(msg)
	lower := strings.ToLower(msg)
	for _, k := range []string{
		"للايجار", "للإيجار", "للاجار", "للاجاره", "للإجاره",
		"كراء", "للكراء", "كرا", "للكرا",
		"إيجار", "ايجار", "اجار",
		"à louer", "a louer", "location", "for rent", "to rent", "rental", "rent",
	} {
		kn := normalizeArabicQuery(k)
		if kn != "" && strings.Contains(n, kn) {
			return true
		}
		if strings.Contains(lower, strings.ToLower(k)) {
			return true
		}
	}
	return false
}

// MessageSignalsBuy detects purchase/sale intent (not bare "sale" in English location names).
func MessageSignalsBuy(msg string) bool {
	if strings.TrimSpace(msg) == "" {
		return false
	}
	if MessageSignalsRent(msg) {
		return false
	}
	n := normalizeArabicQuery(msg)
	lower := strings.ToLower(msg)
	for _, k := range []string{
		"للبيع", "للبع", "شراء", "للشراء",
		"à vendre", "a vendre", "acheter", "achat", "for sale", "to buy",
		"want to buy", "wanna buy", "looking to buy",
		"تنباع", "تنبيع", "نبيع", "تبيع", "ينباع",
	} {
		kn := normalizeArabicQuery(k)
		if kn != "" && strings.Contains(n, kn) {
			return true
		}
		if strings.Contains(lower, strings.ToLower(k)) {
			return true
		}
	}
	// English "buy" / "sale" as words (avoid matching inside other tokens).
	if hasWord(lower, "buy") || hasWord(lower, "sell") {
		return true
	}
	if hasWord(lower, "sale") && !hasWord(lower, "wholesale") {
		return true
	}
	// Arabic bare "بيع" — require word boundary via spacing/punctuation context.
	if strings.Contains(n, "بيع") && !strings.Contains(n, "للبيع") {
		return strings.Contains(n, " للبيع") || strings.HasPrefix(n, "بيع ") ||
			strings.Contains(n, " بيع ") || strings.HasSuffix(n, " بيع")
	}
	return strings.Contains(n, "للبيع") || strings.Contains(n, "شراء")
}

// ReconcileTransactionFromMessage forces rent/buy from the current user text over session memory.
func ReconcileTransactionFromMessage(ctx MessageContext) MessageContext {
	raw := strings.TrimSpace(ctx.RawText)
	if raw == "" {
		return ctx
	}
	if MessageSignalsRent(raw) {
		ctx.Intent = IntentSearchRent
		return ctx
	}
	if MessageSignalsBuy(raw) {
		return applyBuyFamilyIntent(ctx)
	}
	return ctx
}

func applyBuyFamilyIntent(ctx MessageContext) MessageContext {
	t := strings.ToLower(strings.TrimSpace(ctx.Type))
	switch {
	case t == "land" || t == "terrain":
		ctx.Intent = IntentSearchLand
	case t == "boutique" || t == "commercial" || strings.Contains(t, "commercial"):
		ctx.Intent = IntentSearchCommercial
	default:
		ctx.Intent = IntentSearchBuy
	}
	return ctx
}

// ResolveTransactionIntent applies rent/buy detection; rent wins when both appear.
func ResolveTransactionIntent(ctx *MessageContext, msg string) {
	if ctx == nil {
		return
	}
	if strings.TrimSpace(msg) != "" {
		ctx.RawText = strings.TrimSpace(msg)
	}
	if MessageSignalsRent(msg) {
		ctx.Intent = IntentSearchRent
		return
	}
	if MessageSignalsBuy(msg) {
		*ctx = applyBuyFamilyIntent(*ctx)
	}
}
