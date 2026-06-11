package lang

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// AnalyzeMessage detects intent, language, and search context from user input.
// Trained on Mauritanian expressions: Arabic (MSA/dialect), French, and English.
func AnalyzeMessage(msg string) MessageContext {
	l := DetectLang(msg)
	lower := strings.ToLower(msg)

	ctx := MessageContext{
		Lang:   l,
		Intent: IntentUnknown,
	}

	// ── Deterministic picker tag parsing ─────────────────────────────────────
	// Mobile sends an authoritative filter block:
	// [MESKENY_PICKER]
	// city_name=...
	// zone_name=...
	// quartier_name=...
	// budget_min_mru=0
	// budget_max_mru=0
	// [/MESKENY_PICKER]
	//
	// If present, we trust it and skip fuzzy keyword parsing.
	if city, zone, quartier, budgetMin, budgetMax, ok := parsePickerTag(msg); ok {
		ctx.Intent = IntentSearchAny
		ctx.City = city
		ctx.Zone = zone
		ctx.Quartier = quartier
		ctx.BudgetMRU = 0
		ctx.BudgetMin = budgetMin
		ctx.BudgetMax = budgetMax
		ctx.FromPicker = true
		return ctx
	}

	// Greetings — Arabic, French, English
	if hasAny(lower, msg, "bonjour", "salut", "bonsoir", "coucou", "hey", "hello", "hi", "howdy",
		"السلام", "سلام", "سلام عليكم", "عليكم السلام", "مرحبا", "أهلا", "مرحباً", "كيف حالك", "ازيك", "كيفك") {
		ctx.Intent = IntentGreeting
		return ctx
	}

	// Help
	if hasAny(lower, msg, "aide", "help", "مساعدة", "مساعده", "كيف") && len(msg) < 30 {
		ctx.Intent = IntentHelp
		return ctx
	}

	// Rent
	if hasAny(lower, msg, "للايجار", "للإيجار", "إيجار", "اجار", "à louer", "location", "rent", "rental", "for rent") {
		if ctx.Intent == IntentUnknown {
			ctx.Intent = IntentSearchRent
		}
	}

	// Buy (incl. Hassaniya / dialect: تنباع، تنبيع، نبيع، ينباع)
	if hasAny(lower, msg, "للبيع", "بيع", "شراء", "à vendre", "acheter", "buy", "achat", "for sale", "sale", "vente", "sell", "selling",
		"تنباع", "تنبيع", "نبيع", "تبيع", "ينباع") {
		if ctx.Intent == IntentUnknown {
			ctx.Intent = IntentSearchBuy
		}
	}

	// Property type hints
	if hasAny(lower, msg, "شقة", "شقه", "appartement", "apartment", "ستوديو", "studio") {
		if ctx.Type == "" {
			ctx.Type = "appartement"
		}
	}
	if hasAny(lower, msg, "فيلا", "villa") {
		ctx.Type = "villa"
	} else if hasAny(lower, msg, "منزل", "دار", "maison", "house", "بيت") {
		if ctx.Type == "" {
			ctx.Type = "house"
		}
	}
	if hasAny(lower, msg, "محل", "بوتيك", "boutique", "مكتب", "bureau", "تجاري", "commercial") {
		if ctx.Intent == IntentUnknown {
			ctx.Intent = IntentSearchCommercial
		}
		if ctx.Type == "" {
			ctx.Type = "boutique"
		}
	}

	// Land/terrain — only when the user is not asking for a house/apartment/villa.
	// Place names may contain "اتراب" while the user wants a منزل (house).
	if ctx.Type == "" {
		if strings.Contains(msg, "أرض") || strings.Contains(msg, "ارض") || strings.Contains(msg, "تراب") ||
			strings.Contains(msg, "اتراب") || strings.Contains(msg, "اترب") ||
			strings.Contains(msg, "نيمرو") || strings.Contains(msg, "نمره") || strings.Contains(msg, "نمرة") ||
			strings.Contains(lower, "nimrou") || strings.Contains(lower, "nemrou") || strings.Contains(lower, "némrou") ||
			strings.Contains(lower, "terrain") ||
			(strings.Contains(lower, "land") && !strings.Contains(lower, "landlord")) {
			ctx.Intent = IntentSearchLand
			ctx.Type = "land"
		}
	}

	// Search triggers (Mauritanian: بدور = "I'm looking for", عقار = property; اندور = Hassaniya "looking for")
	if ctx.Intent == IntentUnknown &&
		hasAny(lower, msg, "أريد", "ابحث", "أبحث", "دور", "بدور", "اندور", "عقار", "عقارات", "cherche", "looking", "want", "need", "نحب", "نفضل") {
		ctx.Intent = IntentSearchAny
	}
	// English: "find a property", "looking for a house" without sale/rent keyword
	if ctx.Intent == IntentUnknown &&
		hasAny(lower, msg, "find a property", "find property", "looking for a property", "looking for property", "search property", "search for property") {
		ctx.Intent = IntentSearchAny
	}

	// City/zone detection — Nouakchott zones
	// Use pipe-separated OR patterns so Tevragh + الصحراوي in one message are not overwritten.
	var zoneOR []string
	if strings.Contains(msg, "تفرغ زينة") || strings.Contains(lower, "tevragh") {
		ctx.City = "nouakchott"
		zoneOR = append(zoneOR, "tevragh zeina", "tevragh", "تفرغ", "زينة")
	}
	if strings.Contains(msg, "الصحراوي") || strings.Contains(msg, "صحراوي") || strings.Contains(msg, "البوادي") || strings.Contains(lower, "station africa") {
		if ctx.City == "" {
			ctx.City = "nouakchott"
		}
		zoneOR = append(zoneOR, "صحراوي", "الصحراوي", "البوادي", "sahraoui", "sahrawi", "station africa", "africa")
	}
	if len(zoneOR) > 0 {
		ctx.Zone = strings.Join(zoneOR, "|")
	}
	// Old airport (distinct from generic "ksar")
	if strings.Contains(msg, "المطار القديم") || strings.Contains(msg, "مطار قديم") ||
		strings.Contains(lower, "old airport") {
		ctx.City = "nouakchott"
		ctx.Zone = "المطار القديم|مطار قديم|old airport|المطار|مطار"
	}
	// Ilô K / Hay Ilô — land-heavy zone in Nouakchott; match Latin and Arabic spellings.
	if strings.Contains(lower, "ilo k") || strings.Contains(lower, "ilô k") ||
		strings.Contains(msg, "إلو ك") || strings.Contains(msg, "ايلو ك") || strings.Contains(msg, "إلو") ||
		strings.Contains(msg, "ايلو") || strings.Contains(lower, " hay ilo") {
		ctx.City = "nouakchott"
		if ctx.Zone == "" {
			ctx.Zone = "ilo|ilô|إلو|ايلو|ilo k|ilô k|هيلو|يلو"
		} else {
			ctx.Zone = ctx.Zone + "|ilo|ilô|إلو|ايلو|ilo k|ilô k|هيلو|يلو"
		}
	}
	if hasAny(lower, msg, "كصر", "ksar") {
		ctx.City = "nouakchott"
		if ctx.Zone == "" {
			ctx.Zone = "ksar"
		}
	}
	if hasAny(lower, msg, "دار النعيم", "dar naim", "دار نعيم") {
		ctx.City = "nouakchott"
		ctx.Zone = "dar naim"
	}
	if hasAny(lower, msg, "السبخة", "sebkha", "سبخة") {
		ctx.City = "nouakchott"
		ctx.Zone = "sebkha"
	}
	if hasAny(lower, msg, "عرفات", "arafat") {
		ctx.City = "nouakchott"
		ctx.Zone = "arafat"
	}
	if hasAny(lower, msg, "الرياض", "riyad", "رياض") {
		ctx.City = "nouakchott"
		ctx.Zone = "riyad"
	}
	if hasAny(lower, msg, "الميناء", "el mina", "ميناء") {
		ctx.City = "nouakchott"
		ctx.Zone = "el mina"
	}
	if hasAny(lower, msg, "نواكشوط", "nouakchott") {
		if ctx.City == "" {
			ctx.City = "nouakchott"
		}
	}
	// "طريق انواذيبو" is usually a Nouakchott road reference, not city Nouadhibou.
	if strings.Contains(msg, "طريق انواذيبو") || strings.Contains(lower, "route nouadhibou") || strings.Contains(lower, "road to nouadhibou") {
		ctx.City = "nouakchott"
	}
	if hasAny(lower, msg, "نواذيبو", "nouadhibou") {
		// Keep Nouakchott if the user referenced the Nouadhibou road inside it.
		if ctx.City != "nouakchott" {
			ctx.City = "nouadhibou"
		}
	}
	if hasAny(lower, msg, "روصو", "rosso") {
		ctx.City = "rosso"
	}
	if hasAny(lower, msg, "كيهيدي", "kaedi", "كيهدي") {
		ctx.City = "kaedi"
	}
	if hasAny(lower, msg, "كيفة", "kiffa") {
		ctx.City = "kiffa"
	}

	// Iskan / الإسكان (Nouakchott) — treat Alnesim as part of Iskan for search expansion.
	if hasAny(lower, msg, "اسكان", "الإسكان", "سكان", "iskan", "el iskan", "al iskan") ||
		hasAny(lower, msg, "النسيم", "نسيم", "alnesim", "al nesim", "nesim") {
		if ctx.City == "" {
			ctx.City = "nouakchott"
		}
		iskanOR := []string{
			"اسكان", "الإسكان", "إسكان",
			"iskan", "Iskan", "EL ISKAN", "Al Iskan",
			"النسيم", "نسيم",
			"alnesim", "al nesim", "Alnesim", "Nesim",
		}
		if ctx.Zone == "" {
			ctx.Zone = strings.Join(iskanOR, "|")
		} else {
			ctx.Zone = ctx.Zone + "|" + strings.Join(iskanOR, "|")
		}
	}

	// Nouakchott numbered sectors (سكتور / secteur / common typos like سكتير)
	mergeNouakchottSecteurZone(&ctx, msg)

	// Land: do not auto-default city on bare type keywords ("terrain") — clarification first.
	if ctx.Intent == IntentSearchLand && ctx.City == "" && ctx.Zone != "" {
		ctx.City = "nouakchott"
	}
	if (ctx.Intent == IntentSearchRent || ctx.Intent == IntentSearchBuy || ctx.Intent == IntentSearchAny) && ctx.City == "" && ctx.Zone != "" {
		ctx.City = "nouakchott" // zone implies Nouakchott
	}

	// Budget
	if parsed := ParseCurrency(msg); parsed != nil {
		ctx.Budget = strconv.FormatInt(parsed.AmountMRU, 10)
		ctx.BudgetMRU = parsed.AmountMRU
		ctx.BudgetMin = parsed.RangeMin
		ctx.BudgetMax = parsed.RangeMax
	}

	// If the user is asking for a property in a specific area/budget, but didn't explicitly
	// mention "rent"/"buy", we still want DB-backed property search mode (no hallucinations).
	if ctx.Intent == IntentUnknown && ctx.Type != "" && (ctx.City != "" || ctx.Zone != "" || ctx.BudgetMRU != 0) {
		ctx.Intent = IntentSearchAny
	}
	// Single-word or very short type-only queries (e.g. "house", "دار", "villa")
	// should go through property-search flow, where we can clarify missing filters
	// deterministically instead of letting conversational LLM guess results.
	if ctx.Intent == IntentUnknown && ctx.Type != "" && looksLikeTypeOnlySearch(msg, lower) {
		ctx.Intent = IntentSearchAny
	}

	// Plot / parcel number (cadastre listings)
	if pn := extractPlotNumber(msg); pn != "" {
		ctx.PlotNumber = pn
	}

	return SanitizeBudgetContext(ctx, msg)
}

func extractPlotNumber(msg string) string {
	normalized := arabicDigitReplacer.Replace(msg)
	lower := strings.ToLower(normalized)
	patterns := []string{
		`(?i)plot\s*#?\s*(\d+[a-zA-Z]?)`,
		`(?i)parcel\s*#?\s*(\d+[a-zA-Z]?)`,
		`(?i)n[ií]m[eé]ro\s*(\d+[a-zA-Z]?)`,
		`(?i)lot\s*#?\s*(\d+[a-zA-Z]?)`,
	}
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			continue
		}
		if m := re.FindStringSubmatch(lower); len(m) > 1 {
			return strings.TrimSpace(m[1])
		}
	}
	// Arabic: نمرة / نmero / lot number patterns
	for _, kw := range []string{"نمرة", "نmero", "نمره", "رقم"} {
		if idx := strings.Index(msg, kw); idx >= 0 {
			rest := strings.TrimSpace(msg[idx+len(kw):])
			rest = strings.TrimLeft(rest, ":،.- ")
			fields := strings.Fields(rest)
			if len(fields) > 0 {
				token := arabicDigitReplacer.Replace(fields[0])
				token = strings.Trim(token, ".,،")
				if token != "" {
					return token
				}
			}
		}
	}
	return ""
}

func parsePickerTag(msg string) (city string, zone string, quartier string, budgetMin int64, budgetMax int64, ok bool) {
	const start = "[MESKENY_PICKER]"
	const end = "[/MESKENY_PICKER]"

	i := strings.Index(msg, start)
	j := strings.Index(msg, end)
	if i < 0 || j <= i {
		return "", "", "", 0, 0, false
	}

	block := msg[i+len(start) : j]
	lines := strings.Split(block, "\n")

	var cityName, zoneName, quartierName string
	var minSet, maxSet bool
	var minVal, maxVal int64

	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			continue
		}
		parts := strings.SplitN(ln, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch key {
		case "city_name":
			cityName = val
		case "zone_name":
			zoneName = val
		case "quartier_name":
			quartierName = val
		case "budget_min_mru":
			if v, err := strconv.ParseInt(val, 10, 64); err == nil {
				minVal = v
				minSet = true
			}
		case "budget_max_mru":
			if v, err := strconv.ParseInt(val, 10, 64); err == nil {
				maxVal = v
				maxSet = true
			}
		}
	}

	if strings.TrimSpace(cityName) == "" {
		return "", "", "", 0, 0, false
	}

	if !minSet {
		minVal = 0
	}
	if !maxSet {
		maxVal = 0
	}

	return cityName, strings.TrimSpace(zoneName), strings.TrimSpace(quartierName), minVal, maxVal, true
}

var arabicDigitReplacer = strings.NewReplacer(
	"٠", "0", "١", "1", "٢", "2", "٣", "3", "٤", "4",
	"٥", "5", "٦", "6", "٧", "7", "٨", "8", "٩", "9",
)

// mergeNouakchottSecteurZone fills ctx.Zone when the user names a sector by number
// (e.g. Secteur 1, سكتور 2, سكتير 1 typo). Implies Nouakchott if no other city.
func mergeNouakchottSecteurZone(ctx *MessageContext, msg string) {
	n, ok := extractNouakchottSecteurNumber(msg)
	if !ok {
		return
	}
	if ctx.City == "" {
		ctx.City = "nouakchott"
	}
	z := nouakchottSecteurZoneQuery(n)
	if z == "" {
		return
	}
	if ctx.Zone == "" {
		ctx.Zone = z
	} else if !strings.Contains(ctx.Zone, z) {
		ctx.Zone = ctx.Zone + "|" + z
	}
}

func extractNouakchottSecteurNumber(msg string) (int, bool) {
	compact := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, arabicDigitReplacer.Replace(msg))
	lowerCompact := strings.ToLower(compact)

	// "سكتير1", "secteur1", "sector12" after compaction (spaces removed).
	for _, key := range []string{
		"سكتور", "سكتر", "سكتير", "قطاع",
		"secteur", "sector",
	} {
		k := strings.ToLower(key)
		ix := strings.Index(lowerCompact, k)
		if ix < 0 {
			continue
		}
		rest := compact[ix+len(k):]
		if rest == "" {
			continue
		}
		var numStr strings.Builder
		for _, r := range rest {
			if r >= '0' && r <= '9' {
				numStr.WriteRune(r)
				continue
			}
			break
		}
		if numStr.Len() == 0 {
			continue
		}
		n, err := strconv.Atoi(numStr.String())
		if err != nil || n < 1 || n > 24 {
			continue
		}
		return n, true
	}
	return 0, false
}

func nouakchottSecteurZoneQuery(n int) string {
	if n < 1 || n > 24 {
		return ""
	}
	sn := strconv.Itoa(n)
	parts := []string{
		"secteur " + sn, "Secteur " + sn, "SECTEUR " + sn,
		"sector " + sn, "Sector " + sn,
		"سكتور " + sn, "سكتور" + sn, "سكتر " + sn, "سكتر" + sn,
		"سكتير " + sn, "سكتير" + sn,
		"قطاع " + sn, "القطاع " + sn,
		"nce secteur " + sn, "NCE Secteur " + sn,
	}
	return strings.Join(parts, "|")
}

func hasAny(lower, raw string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(lower, n) || strings.Contains(raw, n) {
			return true
		}
	}
	return false
}

func looksLikeTypeOnlySearch(raw, lower string) bool {
	tokens := strings.Fields(strings.TrimSpace(raw))
	if len(tokens) == 0 {
		return false
	}
	if len(tokens) > 3 {
		return false
	}
	// If message is very short and mostly contains type words, treat it as
	// property-search intent instead of generic chat.
	typeWords := map[string]bool{
		"house": true, "home": true, "villa": true, "maison": true, "appartement": true, "apartment": true, "flat": true, "studio": true,
		"land": true, "terrain": true, "property": true, "properties": true, "realestate": true,
		"دار": true, "بيت": true, "منزل": true, "شقة": true, "شقه": true, "أرض": true, "ارض": true, "تراب": true, "اتراب": true, "عقار": true, "عقارات": true,
	}
	matched := 0
	for _, tk := range tokens {
		norm := strings.ToLower(strings.TrimSpace(arabicDigitReplacer.Replace(tk)))
		norm = strings.Trim(norm, ".,!?،؛:()[]{}\"'")
		if norm == "" {
			continue
		}
		if typeWords[norm] {
			matched++
		}
	}
	return matched > 0 && matched == len(tokens)
}

// DetectLang performs a simple heuristic language detection.
// Previously, Latin script without accents defaulted to French, which mislabeled
// English queries like "I want a property for sale" as FR.
func DetectLang(msg string) Lang {
	trimmed := strings.TrimSpace(msg)
	if trimmed == "" {
		return LangEN
	}
	var han int
	for _, r := range trimmed {
		if r >= 0x4E00 && r <= 0x9FFF {
			han++
		}
	}
	if han >= 2 || (han > 0 && len([]rune(trimmed)) <= 12) {
		return LangZH
	}
	for _, r := range trimmed {
		if r >= 0x0600 && r <= 0x06FF {
			return LangAR
		}
	}
	// French diacritics → FR
	if strings.ContainsAny(trimmed, "éàèùçâêîôûïëüœæÉÀÈÙÇÂÊÎÔÛ") {
		return LangFR
	}

	lower := strings.ToLower(trimmed)
	padded := " " + lower + " "

	// Strong French cues (common in MAU real-estate chat)
	frCues := []string{
		" je ", " vous ", " une ", " des ", " les ", " pour ", " dans ", " avec ",
		" cherch", " louer", " vendre", " maison ", " appart", " merci", " bonjour",
		" salut ", " bonsoir", "à louer", "à vendre", "je veux", "je cherche",
		" j'ai ", " d'un ", " d'une ", " où ", " quartier ",
	}
	for _, w := range frCues {
		if strings.Contains(padded, w) {
			return LangFR
		}
	}

	// English cues (ASCII-heavy international users)
	enCues := []string{
		" i want", " i need", " i'm ", " looking for", " for sale", " to rent",
		" property ", " properties ", " home ", " house ", " apartment ",
		" the ", " and ", " can you", " help me", " near ", " budget ",
		" cheap ", " buy ", " rent ", " sale ", " real estate",
	}
	for _, w := range enCues {
		if strings.Contains(padded, w) {
			return LangEN
		}
	}
	if strings.HasPrefix(lower, "i ") || strings.HasPrefix(lower, "looking ") ||
		strings.HasPrefix(lower, "want ") || strings.HasPrefix(lower, "need ") {
		return LangEN
	}

	// Latin-only ambiguous short text: default to English (broader international default).
	if isMostlyLatinASCII(trimmed) {
		return LangEN
	}
	return LangFR
}

func isMostlyLatinASCII(s string) bool {
	var ascii int
	var total int
	for _, r := range s {
		total++
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == ' ' || r == '\'' || r == '-' || r == ',' || r == '.' || r == '?' || r == '!' {
			ascii++
		}
	}
	if total == 0 {
		return false
	}
	return float64(ascii)/float64(total) > 0.85
}

// IsExplicitPurposeIntent is true when rent vs buy is already known.
func IsExplicitPurposeIntent(intent Intent) bool {
	switch intent {
	case IntentSearchRent, IntentSearchBuy, IntentSearchLand, IntentSearchCommercial:
		return true
	default:
		return false
	}
}

// IsPropertySearchIntent tells if this message should trigger DB-backed
// property search instead of free-form LLM chat.
func IsPropertySearchIntent(intent Intent) bool {
	switch intent {
	case IntentSearchRent,
		IntentSearchBuy,
		IntentSearchAny,
		IntentSearchLand,
		IntentSearchCommercial,
		IntentSearchByBudget,
		IntentSearchByLocation,
		IntentSearchByRooms,
		IntentSearchByType:
		return true
	default:
		return false
	}
}

