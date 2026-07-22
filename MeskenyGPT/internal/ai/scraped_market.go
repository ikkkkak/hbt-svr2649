package ai

import (
	"context"
	"fmt"
	"strings"

	"apartments-clone-server/MeskenyGPT/internal/ai/lang"

	"gorm.io/gorm"
)

// scrapedMarketRow is a lightweight projection of scraped_listings (no model
// import — keeps the internal AI module decoupled from the server's models).
type scrapedMarketRow struct {
	Title     string
	PriceText string
	Location  string
	City      string
	SourceURL string
	Kind      string
}

// retrieveScrapedMarketBlock returns a system-prompt block of real, admin-
// scraped market listings relevant to the user's message, each with its
// source URL. Injecting this lets MeskenyGPT REASON over current market data
// (prices, comparables) and cite it — the difference between advising from
// live data and guessing. Returns "" when nothing relevant or DB absent.
func retrieveScrapedMarketBlock(ctx context.Context, gdb *gorm.DB, mc lang.MessageContext) string {
	if gdb == nil {
		return ""
	}

	// Administrative/procedure questions use the STRICT grounding block instead
	// (procedureGroundingBlock) — never the softer "inform the answer" market
	// framing, which risks the model padding official steps with guesses.
	if mc.Intent == lang.IntentInfoProcedure {
		return ""
	}

	kind := ""
	switch mc.Intent {
	case lang.IntentSearchLand:
		kind = "land_sale"
	}

	q := gdb.WithContext(ctx).
		Table("scraped_listings AS sl").
		Select("sl.title, sl.price_text, sl.location, sl.city, sl.source_url, sl.kind").
		Joins("JOIN scraped_sources ss ON ss.id = sl.source_id AND ss.active = true").
		Where("sl.deleted_at IS NULL")

	if kind != "" {
		q = q.Where("sl.kind = ?", kind)
	}

	// Relevance: match the user's message keywords across title, DESCRIPTION
	// (crawled page text — ministry/cadastre/procedures info lives here),
	// location and city. City/zone from the parsed context are weighted in too.
	terms := promptTokens(mc.RawText)
	if c := strings.TrimSpace(strings.ToLower(firstNonEmptyStr(mc.City, mc.Zone))); c != "" {
		terms = append([]string{c}, terms...)
	}
	if len(terms) > 0 {
		or := gdb.Session(&gorm.Session{NewDB: true})
		first := true
		for _, t := range terms {
			like := "%" + t + "%"
			cond := "LOWER(sl.title) LIKE ? OR LOWER(sl.description) LIKE ? OR LOWER(sl.location) LIKE ? OR LOWER(sl.city) LIKE ?"
			if first {
				or = or.Where(cond, like, like, like, like)
				first = false
			} else {
				or = or.Or(cond, like, like, like, like)
			}
		}
		q = q.Where(or)
	}

	var rows []scrapedMarketRow
	if err := q.Order("sl.scraped_at DESC").Limit(6).Scan(&rows).Error; err != nil || len(rows) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString("\n\n=== SCRAPED KNOWLEDGE & MARKET DATA (real pages: ministry/cadastre/land procedures + market listings; use to inform the answer and CITE the source URL when you reference one) ===\n")
	for i, r := range rows {
		loc := firstNonEmptyStr(r.Location, r.City)
		b.WriteString(fmt.Sprintf("%d. %s", i+1, strings.TrimSpace(r.Title)))
		if r.PriceText != "" {
			b.WriteString(" — " + strings.TrimSpace(r.PriceText))
		}
		if loc != "" {
			b.WriteString(" (" + strings.TrimSpace(loc) + ")")
		}
		if r.SourceURL != "" {
			b.WriteString("\n   source: " + r.SourceURL)
		}
		b.WriteString("\n")
	}
	b.WriteString("Only cite these when genuinely relevant; never invent listings or prices.\n")
	return b.String()
}

// procedureGroundingBlock builds the STRICT, anti-hallucination context for a
// real-estate administrative-procedure question. Government procedures must be
// answered from real scraped data or not fabricated at all — a wrong step or
// an invented URL/fee makes users distrust the whole product. So:
//   - If we have scraped official content: the model may answer ONLY from it
//     and cite ONLY those exact URLs.
//   - If we have nothing: the model is explicitly forbidden from inventing
//     steps, documents, fees, percentages, office names, or URLs.
func procedureGroundingBlock(ctx context.Context, gdb *gorm.DB, mc lang.MessageContext) (block string, found bool, sourceURLs []string) {
	if gdb == nil || mc.Intent != lang.IntentInfoProcedure {
		return "", false, nil
	}

	type procRow struct {
		Title       string
		Description string
		SourceURL   string
	}
	q := gdb.WithContext(ctx).
		Table("scraped_listings AS sl").
		Select("sl.title, sl.description, sl.source_url").
		Joins("JOIN scraped_sources ss ON ss.id = sl.source_id AND ss.active = true").
		Where("sl.deleted_at IS NULL")

	terms := promptTokens(mc.RawText)
	if len(terms) > 0 {
		or := gdb.Session(&gorm.Session{NewDB: true})
		first := true
		for _, t := range terms {
			like := "%" + t + "%"
			cond := "LOWER(sl.title) LIKE ? OR LOWER(sl.description) LIKE ?"
			if first {
				or = or.Where(cond, like, like)
				first = false
			} else {
				or = or.Or(cond, like, like)
			}
		}
		q = q.Where(or)
	}

	var rows []procRow
	_ = q.Order("sl.scraped_at DESC").Limit(8).Scan(&rows).Error

	if len(rows) == 0 {
		return "\n\n=== ADMINISTRATIVE PROCEDURE — GENERAL GUIDANCE (NO OFFICIAL SOURCE ON FILE) ===\n" +
			"The user is asking HOW to complete a real-estate administrative procedure. This is a NATIONAL procedure — it does NOT depend on the property type (apartment/house/land) or the city. " +
			"NEVER ask 'is it an apartment or land?' or 'which city?' — that is robotic and wrong here. Answer immediately and helpfully.\n" +
			"We do NOT have an official documented source for this on file. Give the GENERAL steps that apply in Mauritania (typically: prepare identity documents + the existing title/ownership deed → draft the transfer/sale/gift contract before a notary «كاتب العدل» → pay the registration/mutation duties → register at the land-registry office «مصلحة التسجيل العقاري / المحافظة العقارية» → collect the updated title). " +
			"You MUST NOT invent specific fee amounts/percentages, exact office addresses, processing days, article numbers, or URLs — never fabricate a procedures.gov.mr link, and do NOT state a specific number of days/weeks as if it were official. Keep documents generic; do not present a precise official document checklist. End by recommending the user confirm exact details with the land registry or a notary, and offer to connect them with a Meskeny specialist.\n", false, nil
	}

	var b strings.Builder
	b.WriteString("\n\n=== OFFICIAL PROCEDURE DATA (AUTHORITATIVE — GROUND YOUR ANSWER ONLY IN THIS) ===\n")
	b.WriteString("Answer the procedure question ONLY from the content below. You MUST cite the exact source URL(s) shown here INLINE in your answer (write a line like 'المصدر / Source: <url>') so the user can verify it — this is mandatory when you use this data. Never invent or guess a URL. Do NOT add documents, fees, percentages, deadlines, or office names that are not present in this content; if a detail the user needs is missing here, say it must be verified with the authority rather than guessing. Do NOT show the honesty/⚠️ disclaimer when you have this official data.\n\n")
	for i, r := range rows {
		b.WriteString(fmt.Sprintf("%d. %s\n", i+1, strings.TrimSpace(r.Title)))
		if d := strings.TrimSpace(r.Description); d != "" {
			b.WriteString(clipRunes(d, 1000) + "\n")
		}
		if u := strings.TrimSpace(r.SourceURL); u != "" {
			b.WriteString("source: " + u + "\n")
			sourceURLs = append(sourceURLs, u)
		}
		b.WriteString("\n")
	}
	return b.String(), true, sourceURLs
}

// procedureDisclaimer / procedureSourceLabel — localized strings used to
// GUARANTEE the honesty marker / citation in code (not left to the LLM).
func procedureDisclaimer(l lang.Lang) string {
	switch l {
	case lang.LangAR:
		return "⚠️ إرشادات عامة — لم نتحقق بعد من هذا الإجراء مقابل مصدر رسمي في مسكني. تأكد من التفاصيل الدقيقة لدى الجهة المختصة."
	case lang.LangEN:
		return "⚠️ General guidance — not yet verified against an official source on Meskeny. Confirm exact details with the competent authority."
	default:
		return "⚠️ Indications générales — non encore vérifiées auprès d'une source officielle sur Meskeny. Confirmez les détails exacts auprès de l'autorité compétente."
	}
}

func procedureSourceLabel(l lang.Lang) string {
	switch l {
	case lang.LangAR:
		return "المصدر:"
	default:
		return "Source:"
	}
}

// EnforceProcedureHonesty GUARANTEES the trust markers regardless of what the
// LLM did: a general (ungrounded) procedure answer always carries the ⚠️
// disclaimer up front; a grounded one always shows a real source URL. This is
// the machine-enforced version of the prompt rules the model kept ignoring.
func EnforceProcedureHonesty(mc lang.MessageContext, answer string, grounded bool, sourceURLs []string) string {
	if mc.Intent != lang.IntentInfoProcedure {
		return answer
	}
	a := strings.TrimSpace(answer)
	if a == "" {
		return a
	}
	if !grounded {
		if !strings.Contains(a, "⚠️") {
			a = procedureDisclaimer(mc.Lang) + "\n\n" + a
		}
		return a
	}
	// Grounded: make sure a real source URL is present so the user can verify.
	for _, u := range sourceURLs {
		if u != "" && strings.Contains(a, u) {
			return a // already cited
		}
	}
	for _, u := range sourceURLs {
		if strings.TrimSpace(u) != "" {
			return a + "\n\n" + procedureSourceLabel(mc.Lang) + " " + strings.TrimSpace(u)
		}
	}
	return a
}

func clipRunes(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n]) + "…"
}

func firstNonEmptyStr(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

var promptStop = map[string]bool{
	"the": true, "and": true, "for": true, "with": true, "you": true, "what": true,
	"how": true, "are": true, "can": true, "que": true, "les": true, "des": true,
	"pour": true, "une": true, "comment": true, "quel": true, "quelle": true,
}

// promptTokens extracts up-to-6 meaningful search tokens (latin + arabic) from
// the user's message for matching against scraped content.
func promptTokens(msg string) []string {
	msg = strings.ToLower(strings.TrimSpace(msg))
	fields := strings.FieldsFunc(msg, func(r rune) bool {
		return !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') &&
			!(r >= 0x0600 && r <= 0x06FF)
	})
	out := make([]string, 0, 6)
	seen := map[string]bool{}
	for _, f := range fields {
		if len(f) < 4 || promptStop[f] || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
		if len(out) >= 6 {
			break
		}
	}
	return out
}
