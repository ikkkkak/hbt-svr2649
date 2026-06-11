package lang

import "strings"

// SearchReadiness describes whether a DB property search should run.
type SearchReadiness struct {
	CanSearch       bool
	MissingLocation bool
	MissingPurpose  bool // rent vs buy unclear
	MissingType     bool // apartment / house / land / commercial
	MissingBudget   bool // optional hint only
	HasLocation     bool
	HasPurpose      bool
	HasType         bool
	HasBudget       bool
}

// EvaluateSearchReadiness decides if MeskenyGPT should query the database.
func EvaluateSearchReadiness(ctx MessageContext) SearchReadiness {
	out := SearchReadiness{}
	if !IsPropertySearchIntent(ctx.Intent) {
		out.CanSearch = false
		return out
	}

	city := strings.TrimSpace(ctx.City)
	zone := strings.TrimSpace(ctx.Zone)
	out.HasLocation = city != "" || zone != ""
	out.MissingLocation = !out.HasLocation

	out.HasPurpose = ctx.Intent == IntentSearchRent ||
		ctx.Intent == IntentSearchBuy ||
		ctx.Intent == IntentSearchLand ||
		ctx.Intent == IntentSearchCommercial
	if ctx.Intent == IntentSearchAny {
		out.MissingPurpose = true
	}
	out.HasType = strings.TrimSpace(ctx.Type) != "" || ctx.Intent == IntentSearchLand
	out.MissingType = !out.HasType

	out.HasBudget = ctx.BudgetMRU > 0 || ctx.BudgetMin > 0 || ctx.BudgetMax > 0
	out.MissingBudget = !out.HasBudget

	if ctx.Intent == IntentSearchLand && !out.HasLocation {
		out.CanSearch = false
		return out
	}

	if !out.HasLocation {
		out.CanSearch = false
		return out
	}

	if out.MissingPurpose {
		out.CanSearch = false
		return out
	}

	if out.MissingType {
		out.CanSearch = false
		return out
	}

	out.CanSearch = true
	return out
}

// ShouldClarifyBeforeSearch returns true when the user needs follow-up before any DB query.
func ShouldClarifyBeforeSearch(ctx MessageContext) bool {
	if !IsPropertySearchIntent(ctx.Intent) {
		return false
	}
	r := EvaluateSearchReadiness(ctx)
	if r.MissingLocation {
		return true
	}
	if r.MissingPurpose {
		return true
	}
	if r.MissingType {
		return true
	}
	return !r.CanSearch
}

// ProactiveClarificationMessage asks only for missing fields (AR/FR/EN/ZH).
func ProactiveClarificationMessage(ctx MessageContext) string {
	r := EvaluateSearchReadiness(ctx)
	loc := clarificationLocationLabel(ctx)
	var parts []string

	switch ctx.Lang {
	case LangZH:
		if r.MissingLocation {
			parts = append(parts, "您想在毛里塔尼亚哪个城市或区域找房？")
		}
		if r.MissingPurpose {
			parts = append(parts, "您需要租房还是买房？")
		}
		if r.MissingType {
			parts = append(parts, "什么类型（公寓、别墅、土地、商铺）？")
		}
		if r.MissingBudget && r.HasLocation && !r.MissingPurpose {
			parts = append(parts, "有大概预算（MRU）吗？")
		}
		if len(parts) == 0 {
			return "请补充租/售类型或物业类型，以便我搜索 Meskeny 真实房源。"
		}
		if loc != "" && !r.MissingLocation {
			return "好的，" + loc + "。请告诉我：" + strings.Join(parts, " ")
		}
		return "我可以帮您搜索真实房源。请告诉我：" + strings.Join(parts, " ")
	case LangAR:
		if r.MissingLocation {
			parts = append(parts, "في أي مدينة أو حي في موريتانيا؟")
		}
		if r.MissingPurpose {
			parts = append(parts, "هل تبحث عن كراء أم شراء؟")
		}
		if r.MissingType {
			parts = append(parts, "ما نوع العقار (شقة، فيلا، أرض، محل)؟")
		}
		if r.MissingBudget && r.HasLocation && !r.MissingPurpose {
			parts = append(parts, "ما ميزانيتك التقريبية بالأوقية؟")
		}
		if len(parts) == 0 {
			return "حدّد كراء أو بيع ونوع العقار لأعرض لك إعلانات حقيقية من Meskeny."
		}
		if loc != "" && !r.MissingLocation {
			return "تمام، " + loc + ". " + joinClarificationAR(r)
		}
		return joinClarificationAR(r)
	case LangEN:
		if r.MissingLocation {
			parts = append(parts, "Which city or neighbourhood in Mauritania?")
		}
		if r.MissingPurpose {
			parts = append(parts, "Are you looking to rent or buy?")
		}
		if r.MissingType {
			parts = append(parts, "What type — apartment, house, land, or commercial?")
		}
		if r.MissingBudget && r.HasLocation && !r.MissingPurpose {
			parts = append(parts, "What's your approximate budget in MRU?")
		}
		if len(parts) == 0 {
			return "Please share rent or buy and a property type so I can search real Meskeny listings."
		}
		if loc != "" && !r.MissingLocation {
			return "Got it — " + loc + ". " + joinClarificationEN(r)
		}
		return joinClarificationEN(r)
	default:
		if r.MissingLocation {
			parts = append(parts, "Dans quelle ville ou quel quartier en Mauritanie ?")
		}
		if r.MissingPurpose {
			parts = append(parts, "Cherchez-vous à louer ou à acheter ?")
		}
		if r.MissingType {
			parts = append(parts, "Quel type de bien (appartement, villa, terrain, local) ?")
		}
		if r.MissingBudget && r.HasLocation && !r.MissingPurpose {
			parts = append(parts, "Quel budget approximatif en MRU ?")
		}
		if len(parts) == 0 {
			return "Indique location ou achat et le type de bien pour lancer une recherche sur de vraies annonces Meskeny."
		}
		if loc != "" && !r.MissingLocation {
			return "Parfait — " + loc + ". " + joinClarificationFR(r)
		}
		return joinClarificationFR(r)
	}
}

func clarificationLocationLabel(ctx MessageContext) string {
	q := strings.TrimSpace(ctx.Quartier)
	zone := strings.TrimSpace(ctx.Zone)
	city := strings.TrimSpace(ctx.City)
	parts := make([]string, 0, 3)
	if q != "" {
		parts = append(parts, q)
	}
	if zone != "" {
		parts = append(parts, zone)
	}
	if city != "" {
		parts = append(parts, city)
	}
	return strings.Join(parts, ", ")
}

func joinClarificationEN(r SearchReadiness) string {
	if r.MissingPurpose && r.MissingType {
		return "Are you looking to rent or buy, and what type — apartment, house, villa, land, or commercial?"
	}
	parts := clarificationPartsEN(r)
	return joinQuestionsEN(parts)
}

func joinClarificationFR(r SearchReadiness) string {
	if r.MissingPurpose && r.MissingType {
		return "Cherchez-vous à louer ou à acheter, et quel type — appartement, maison, villa, terrain ou local ?"
	}
	parts := clarificationPartsFR(r)
	return joinQuestionsFR(parts)
}

func joinClarificationAR(r SearchReadiness) string {
	if r.MissingPurpose && r.MissingType {
		return "هل تبحث عن كراء أم شراء؟ وما نوع العقار — شقة، منزل، فيلا، أرض أم محل؟"
	}
	parts := clarificationPartsAR(r)
	if len(parts) == 0 {
		return "حدّد كراء أو بيع ونوع العقار لأعرض لك إعلانات حقيقية من Meskeny."
	}
	return "بكل سرور. " + strings.Join(parts, " ")
}

func clarificationPartsEN(r SearchReadiness) []string {
	var parts []string
	if r.MissingLocation {
		parts = append(parts, "Which city or neighbourhood in Mauritania?")
	}
	if r.MissingPurpose {
		parts = append(parts, "Are you looking to rent or buy?")
	}
	if r.MissingType {
		parts = append(parts, "What type — apartment, house, land, or commercial?")
	}
	if r.MissingBudget && r.HasLocation && !r.MissingPurpose {
		parts = append(parts, "What's your approximate budget in MRU?")
	}
	return parts
}

func clarificationPartsFR(r SearchReadiness) []string {
	var parts []string
	if r.MissingLocation {
		parts = append(parts, "Dans quelle ville ou quel quartier en Mauritanie ?")
	}
	if r.MissingPurpose {
		parts = append(parts, "Cherchez-vous à louer ou à acheter ?")
	}
	if r.MissingType {
		parts = append(parts, "Quel type de bien (appartement, villa, terrain, local) ?")
	}
	if r.MissingBudget && r.HasLocation && !r.MissingPurpose {
		parts = append(parts, "Quel budget approximatif en MRU ?")
	}
	return parts
}

func clarificationPartsAR(r SearchReadiness) []string {
	var parts []string
	if r.MissingLocation {
		parts = append(parts, "في أي مدينة أو حي في موريتانيا؟")
	}
	if r.MissingPurpose {
		parts = append(parts, "هل تبحث عن كراء أم شراء؟")
	}
	if r.MissingType {
		parts = append(parts, "ما نوع العقار (شقة، فيلا، أرض، محل)؟")
	}
	if r.MissingBudget && r.HasLocation && !r.MissingPurpose {
		parts = append(parts, "ما ميزانيتك التقريبية بالأوقية؟")
	}
	return parts
}

func joinQuestionsEN(parts []string) string {
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func joinQuestionsFR(parts []string) string {
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}
