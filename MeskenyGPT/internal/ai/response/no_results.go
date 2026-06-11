// ─────────────────────────────────────────────────────────────────────────────
// no_results.go — "Nothing found" handler with actionable follow-up chips
// ─────────────────────────────────────────────────────────────────────────────

package response

import (
	"fmt"
	"strings"

	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
)

// SearchContext holds what the user was looking for.
type SearchContext struct {
	Lang         lang.Lang
	City         string
	Zone         string
	Quartier     string
	PropertyType string
	Purpose      string
	BudgetMRU    int64
	BudgetMin    int64
	BudgetMax    int64
}

// BuildNoResultsPayload generates a contextual "nothing found" response with
// actionable follow-up chips. Maps to response.Message and QuickReplies.
func BuildNoResultsPayload(ctx SearchContext) (msg string, chips []QuickReply) {
	msg = buildNoResultsMessage(ctx)
	chips = buildFollowUpChips(ctx)
	return msg, chips
}

func buildNoResultsMessage(ctx SearchContext) string {
	zone := displayZone(ctx)
	price := displayPrice(ctx)

	switch ctx.Lang {
	case lang.LangAR:
		return buildAR(ctx, zone, price)
	case lang.LangEN:
		return buildEN(ctx, zone, price)
	default:
		return buildFR(ctx, zone, price)
	}
}

func buildAR(ctx SearchContext, zone, price string) string {
	var sb strings.Builder
	sb.WriteString("لم نعثر على ")
	if ctx.PropertyType != "" {
		sb.WriteString(localiseType(ctx.PropertyType, lang.LangAR))
	} else if ctx.Purpose == "rent" {
		sb.WriteString("عقار للإيجار")
	} else if ctx.Purpose == "buy" {
		sb.WriteString("عقار للبيع")
	} else {
		sb.WriteString("عقار")
	}
	if zone != "" {
		sb.WriteString(" في " + zone)
	}
	if price != "" {
		sb.WriteString(" بسعر " + price)
	}
	sb.WriteString(".\n\n")
	sb.WriteString("إليك بعض الخيارات:\n")
	if price != "" {
		sb.WriteString("• رفع الميزانية قليلاً قد يفتح خيارات أكثر\n")
	}
	if strings.TrimSpace(ctx.Zone) != "" {
		sb.WriteString("• تجربة حي مجاور قد يناسب نفس الميزانية\n")
	}
	city := localiseZoneAR(ctx.City)
	if strings.TrimSpace(city) == "" {
		city = "نواكشوط"
	}
	sb.WriteString("• عرض جميع العقارات المتاحة في " + city)
	return sb.String()
}

func buildFR(ctx SearchContext, zone, price string) string {
	var sb strings.Builder
	sb.WriteString("Aucun ")
	if ctx.PropertyType != "" {
		sb.WriteString(strings.ToLower(localiseType(ctx.PropertyType, lang.LangFR)))
	} else if ctx.Purpose == "rent" {
		sb.WriteString("bien à louer")
	} else if ctx.Purpose == "buy" {
		sb.WriteString("bien à vendre")
	} else {
		sb.WriteString("bien")
	}
	if zone != "" {
		sb.WriteString(" à " + zone)
	}
	if price != "" {
		sb.WriteString(" autour de " + price)
	}
	sb.WriteString(" n'est disponible pour le moment.\n\n")
	sb.WriteString("Voici ce que je vous suggère :\n")
	if price != "" {
		sb.WriteString("• Élargir légèrement votre budget pour plus d'options\n")
	}
	if strings.TrimSpace(ctx.Zone) != "" {
		sb.WriteString("• Explorer un quartier voisin avec un prix similaire\n")
	}
	city := titleCase(ctx.City)
	if strings.TrimSpace(city) == "" {
		city = "Nouakchott"
	}
	sb.WriteString("• Voir tous les biens disponibles à " + city)
	return sb.String()
}

func buildEN(ctx SearchContext, zone, price string) string {
	var sb strings.Builder
	sb.WriteString("No ")
	if ctx.PropertyType != "" {
		sb.WriteString(strings.ToLower(localiseType(ctx.PropertyType, lang.LangEN)))
	} else if ctx.Purpose == "rent" {
		sb.WriteString("rental")
	} else if ctx.Purpose == "buy" {
		sb.WriteString("property for sale")
	} else {
		sb.WriteString("property")
	}
	if zone != "" {
		sb.WriteString(" in " + zone)
	}
	if price != "" {
		sb.WriteString(" around " + price)
	}
	sb.WriteString(" is available right now.\n\n")
	sb.WriteString("Here's what I suggest:\n")
	if price != "" {
		sb.WriteString("• Slightly increasing your budget may open more options\n")
	}
	if strings.TrimSpace(ctx.Zone) != "" {
		sb.WriteString("• Trying a nearby neighbourhood at a similar price\n")
	}
	city := titleCase(ctx.City)
	if strings.TrimSpace(city) == "" {
		city = "Nouakchott"
	}
	sb.WriteString("• Browsing all available properties in " + city)
	return sb.String()
}

func buildFollowUpChips(ctx SearchContext) []QuickReply {
	qr := func(id, text, action string) QuickReply { return QuickReply{ID: id, Text: text, Action: action} }
	var chips []QuickReply
	i := 1

	if ctx.BudgetMRU >= 100_000 {
		higher := ctx.BudgetMRU + ctx.BudgetMRU/2
		lower := ctx.BudgetMRU - ctx.BudgetMRU/4
		switch ctx.Lang {
		case lang.LangAR:
			chips = append(chips, qr(fmt.Sprintf("%d", i), fmt.Sprintf("🔍 رفع الميزانية إلى %s", formatMRUShort(higher, lang.LangAR)), fmt.Sprintf("ارفع ميزانيتي إلى %s", formatMRUShort(higher, lang.LangAR))))
			i++
			if lower > 0 {
				chips = append(chips, qr(fmt.Sprintf("%d", i), fmt.Sprintf("💰 خفض الميزانية إلى %s", formatMRUShort(lower, lang.LangAR)), fmt.Sprintf("خفض ميزانيتي إلى %s", formatMRUShort(lower, lang.LangAR))))
				i++
			}
		case lang.LangEN:
			chips = append(chips, qr(fmt.Sprintf("%d", i), fmt.Sprintf("🔍 Increase budget to %s", formatMRUShort(higher, lang.LangEN)), fmt.Sprintf("Increase my budget to %s", formatMRUShort(higher, lang.LangEN))))
			i++
		default:
			chips = append(chips, qr(fmt.Sprintf("%d", i), fmt.Sprintf("🔍 Budget à %s", formatMRUShort(higher, lang.LangFR)), fmt.Sprintf("Augmenter mon budget à %s", formatMRUShort(higher, lang.LangFR))))
			i++
		}
	}

	if ctx.Zone != "" && strings.TrimSpace(ctx.Quartier) == "" {
		var label, action string
		switch ctx.Lang {
		case lang.LangAR:
			label = "📍 تحديد الحي/السكتور"
			action = "picker_city"
		case lang.LangEN:
			label = "📍 Pick neighbourhood"
			action = "picker_city"
		default:
			label = "📍 Choisir le quartier"
			action = "picker_city"
		}
		chips = append(chips, qr(fmt.Sprintf("%d", i), label, action))
		i++
	}

	if ctx.Zone != "" {
		nearby := nearbyZone(ctx.Zone)
		if nearby != "" {
			var label string
			switch ctx.Lang {
			case lang.LangAR:
				label = "📍 تجربة " + localiseZoneAR(nearby)
			case lang.LangEN:
				label = "📍 Try " + nearby
			default:
				label = "📍 Essayer " + nearby
			}
			var action string
			switch ctx.Lang {
			case lang.LangAR:
				action = "اعرض عقارات في " + localiseZoneAR(nearby)
			case lang.LangEN:
				action = "Show properties in " + nearby
			default:
				action = "Afficher les biens à " + nearby
			}
			chips = append(chips, qr(fmt.Sprintf("%d", i), label, action))
			i++
		}
	}

	switch ctx.Lang {
	case lang.LangAR:
		city := localiseZoneAR(ctx.City)
		if strings.TrimSpace(city) == "" {
			city = "نواكشوط"
		}
		chips = append(chips, qr(fmt.Sprintf("%d", i), "🏙️ كل عقارات "+city, "اعرض كل العقارات المتاحة في "+city))
		i++
		chips = append(chips, qr(fmt.Sprintf("%d", i), "🔔 تنبيه عند توفر عقار", "نبهني عند توفر عقار مناسب"))
	case lang.LangEN:
		city := titleCase(ctx.City)
		if strings.TrimSpace(city) == "" {
			city = "Nouakchott"
		}
		chips = append(chips, qr(fmt.Sprintf("%d", i), "🏙️ All "+city+" listings", "Show all available properties in "+city))
		i++
		chips = append(chips, qr(fmt.Sprintf("%d", i), "🔔 Alert me when available", "Notify me when a matching property is available"))
	default:
		city := titleCase(ctx.City)
		if strings.TrimSpace(city) == "" {
			city = "Nouakchott"
		}
		chips = append(chips, qr(fmt.Sprintf("%d", i), "🏙️ Tous les biens à "+city, "Afficher tous les biens disponibles à "+city))
		i++
		chips = append(chips, qr(fmt.Sprintf("%d", i), "🔔 M'alerter à la disponibilité", "Alerte-moi quand un bien correspondant est disponible"))
	}
	return chips
}

var zoneNeighbours = map[string]string{
	"tevragh zeina": "ksar", "ksar": "tevragh zeina",
	"el mina": "sebkha", "sebkha": "el mina",
	"arafat": "toujounine", "toujounine": "arafat",
	"dar naim": "riyad", "riyad": "dar naim",
	"teyarett": "ksar", "pk5": "ksar", "pk6": "el mina",
}

func nearbyZone(zone string) string { return zoneNeighbours[strings.ToLower(primaryZoneToken(zone))] }

func displayZone(ctx SearchContext) string {
	s := primaryZoneToken(ctx.Zone)
	if s == "" {
		s = ctx.City
	}
	if s == "" {
		return ""
	}
	if ctx.Lang == lang.LangAR {
		return localiseZoneAR(s)
	}
	if containsArabic(s) {
		return s // titleCase corrupts UTF-8 multi-byte runes (e.g. صحراوي → حراوي)
	}
	return titleCase(s)
}

func displayPrice(ctx SearchContext) string {
	if ctx.BudgetMRU < 100_000 {
		return ""
	}
	return formatMRUShort(ctx.BudgetMRU, ctx.Lang)
}

func formatMRUShort(v int64, l lang.Lang) string {
	switch {
	case v >= 1_000_000 && v%1_000_000 == 0:
		n := v / 1_000_000
		if l == lang.LangAR {
			return fmt.Sprintf("%d مليون أوقية", n)
		}
		return fmt.Sprintf("%d M MRU", n)
	case v >= 1_000:
		n := v / 1_000
		if l == lang.LangAR {
			return fmt.Sprintf("%d ألف أوقية", n)
		}
		return fmt.Sprintf("%d 000 MRU", n)
	default:
		return fmt.Sprintf("%d MRU", v)
	}
}

func localiseType(t string, l lang.Lang) string {
	types := map[string][3]string{
		"Appartement":      {"Appartement", "شقة", "Apartment"},
		"appartement":      {"Appartement", "شقة", "Apartment"},
		"apartment":        {"Appartement", "شقة", "Apartment"},
		"Studio":           {"Studio", "ستوديو", "Studio"},
		"studio":           {"Studio", "ستوديو", "Studio"},
		"Maison":           {"Maison", "منزل", "House"},
		"maison":           {"Maison", "منزل", "House"},
		"house":            {"Maison", "منزل", "House"},
		"home":             {"Maison", "منزل", "House"},
		"Villa":            {"Villa", "فيلا", "Villa"},
		"villa":            {"Villa", "فيلا", "Villa"},
		"Terrain":          {"Terrain", "أرض", "Land"},
		"land":             {"Terrain", "أرض", "Land"},
		"terrain":          {"Terrain", "أرض", "Land"},
		"Boutique":         {"Boutique", "محل", "Shop"},
		"boutique":         {"Boutique", "محل", "Shop"},
		"Local commercial": {"Local commercial", "محل تجاري", "Commercial space"},
		"Bureau":           {"Bureau", "مكتب", "Office"},
	}
	if v, ok := types[t]; ok {
		switch l {
		case lang.LangAR:
			return v[1]
		case lang.LangEN:
			return v[2]
		default:
			return v[0]
		}
	}
	return t
}

func localiseZoneAR(zone string) string {
	zones := map[string]string{
		"nouakchott": "نواكشوط", "nouadhibou": "نواذيبو",
		"tevragh zeina": "تفرغ زينة", "ksar": "كصر",
		"el mina": "الميناء", "sebkha": "السبخة",
		"arafat": "عرفات", "toujounine": "توجنين",
		"dar naim": "دار النعيم", "riyad": "الرياض", "teyarett": "تيارت",
		"صحراوي": "الصحراوي", "البوادي": "الصحراوي", "station africa": "الصحراوي", "el foug": "حي الفوز الصحراوي",
	}
	if v, ok := zones[strings.ToLower(zone)]; ok {
		return v
	}
	if zone != "" && containsArabic(zone) {
		return zone // Don't titleCase Arabic — corrupts multi-byte runes
	}
	return zone
}

func primaryZoneToken(zone string) string {
	zone = strings.TrimSpace(zone)
	if zone == "" {
		return ""
	}
	if !strings.Contains(zone, "|") {
		return zone
	}
	parts := strings.Split(zone, "|")
	for _, p := range parts {
		pp := strings.TrimSpace(p)
		if pp == "" {
			continue
		}
		if containsArabic(pp) {
			return pp
		}
	}
	for _, p := range parts {
		pp := strings.TrimSpace(p)
		if pp != "" {
			return pp
		}
	}
	return ""
}

func containsArabic(s string) bool {
	for _, r := range s {
		if r >= 0x0600 && r <= 0x06FF {
			return true
		}
	}
	return false
}

func titleCase(s string) string {
	words := strings.Fields(s)
	for i, w := range words {
		if len(w) > 0 {
			words[i] = strings.ToUpper(w[:1]) + strings.ToLower(w[1:])
		}
	}
	return strings.Join(words, " ")
}
