package lang

import "strings"

// ClearStaleQuartier drops a persisted quartier when the user message does not
// reference it — avoids searching sector 6 when they only said "house for sale".
func ClearStaleQuartier(ctx MessageContext, raw string) MessageContext {
	q := strings.TrimSpace(ctx.Quartier)
	if q == "" {
		return ctx
	}
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.Contains(raw, "[MESKENY_PICKER]") {
		return ctx
	}
	lower := strings.ToLower(raw)
	if strings.Contains(lower, strings.ToLower(q)) {
		return ctx
	}
	if strings.Contains(raw, "سكتور") || strings.Contains(raw, "سكتر") ||
		strings.Contains(lower, "secteur") || strings.Contains(lower, "sector") ||
		strings.Contains(lower, "quartier") || strings.Contains(raw, "حي") {
		return ctx
	}
	ctx.Quartier = ""
	return ctx
}
