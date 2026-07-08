package lang

import "strings"

// ClearStaleType drops a persisted property type when the user's message does not
// reference it — e.g. session had type=land but user now asks for a generic property.
func ClearStaleType(ctx MessageContext, raw string) MessageContext {
	t := strings.TrimSpace(ctx.Type)
	if t == "" || raw == "" || strings.Contains(raw, "[MESKENY_PICKER]") {
		return ctx
	}
	if strings.Contains(raw, "Property context (shared from app card):") {
		return ctx
	}
	lower := strings.ToLower(raw)

	mentionsLand :=
		(strings.Contains(lower, "land") && !strings.Contains(lower, "landlord") && !strings.Contains(lower, "highland")) ||
			strings.Contains(raw, "أرض") || strings.Contains(raw, "ارض") ||
			strings.Contains(lower, "terrain") || strings.Contains(lower, "nimrou") ||
			strings.Contains(lower, "nemrou") || strings.Contains(raw, "تراب") || strings.Contains(raw, "اتراب")

	mentionsHouseOrApt :=
		hasAny(lower, raw, "house", "home", "villa", "maison", "apartment", "appartement", "studio", "property", "properties", "عقار", "عقارات", "منزل", "دار", "بيت", "شقة", "شقه", "فيلا")

	mentionsGenericBuy :=
		hasAny(lower, raw, "for sale", "property for sale", "buy", "purchase", "à vendre", "للبيع", "شراء")

	if t == "land" && (mentionsHouseOrApt || mentionsGenericBuy) && !mentionsLand {
		ctx.Type = ""
		if ctx.Intent == IntentSearchLand {
			ctx.Intent = IntentSearchBuy
		}
	}
	return ctx
}
