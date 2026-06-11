package response

import (
	"fmt"
	"strings"
	"time"

	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
)

// ProactiveClarificationOutput when DB search must not run yet.
func ProactiveClarificationOutput(ctx lang.MessageContext) (out struct {
	Message      Message
	QuickReplies []QuickReply
}) {
	txt := lang.ProactiveClarificationMessage(ctx)
	out.Message = Message{
		ID:      fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		Role:    "assistant",
		Content: txt,
	}
	out.QuickReplies = clarificationQuickReplies(ctx)
	return
}

func clarificationQuickReplies(ctx lang.MessageContext) []QuickReply {
	qr := func(id, text, action string) QuickReply {
		return QuickReply{ID: id, Text: text, Action: action}
	}
	var chips []QuickReply
	r := lang.EvaluateSearchReadiness(ctx)
	loc := locationPhrase(ctx)

	switch ctx.Lang {
	case lang.LangAR:
		if r.MissingPurpose {
			chips = append(chips,
				qr("purpose_rent", "كراء", fmt.Sprintf("أبحث عن عقار للإيجار في %s", loc)),
				qr("purpose_buy", "شراء", fmt.Sprintf("أبحث عن عقار للبيع في %s", loc)),
			)
		}
		if r.MissingType {
			chips = append(chips, typeChipsAR(loc, ctx)...)
		}
		if r.MissingLocation {
			chips = append(chips,
				qr("loc_nkc", "نواكشوط", "عقارات في نواكشوط"),
				qr("loc_ndb", "نواذيبو", "عقارات في نواذيبو"),
			)
		}
	case lang.LangEN:
		if r.MissingPurpose {
			chips = append(chips,
				qr("purpose_rent", "Rent", fmt.Sprintf("I want to rent in %s", loc)),
				qr("purpose_buy", "Buy", fmt.Sprintf("I want to buy in %s", loc)),
			)
		}
		if r.MissingType {
			chips = append(chips, typeChipsEN(loc, ctx)...)
		}
		if r.MissingLocation {
			chips = append(chips,
				qr("loc_nkc", "Nouakchott", "Properties in Nouakchott"),
				qr("loc_ndb", "Nouadhibou", "Properties in Nouadhibou"),
			)
		}
	case lang.LangZH:
		if r.MissingPurpose {
			chips = append(chips,
				qr("purpose_rent", "租房", fmt.Sprintf("我想在%s租房", loc)),
				qr("purpose_buy", "买房", fmt.Sprintf("我想在%s买房", loc)),
			)
		}
		if r.MissingType {
			chips = append(chips, typeChipsZH(loc, ctx)...)
		}
		if r.MissingLocation {
			chips = append(chips,
				qr("loc_nkc", "努瓦克肖特", "努瓦克肖特的房源"),
				qr("loc_ndb", "努瓦迪布", "努瓦迪布的房源"),
			)
		}
	default:
		if r.MissingPurpose {
			chips = append(chips,
				qr("purpose_rent", "Louer", fmt.Sprintf("Je cherche à louer à %s", loc)),
				qr("purpose_buy", "Acheter", fmt.Sprintf("Je cherche à acheter à %s", loc)),
			)
		}
		if r.MissingType {
			chips = append(chips, typeChipsFR(loc, ctx)...)
		}
		if r.MissingLocation {
			chips = append(chips,
				qr("loc_nkc", "Nouakchott", "Biens à Nouakchott"),
				qr("loc_ndb", "Nouadhibou", "Biens à Nouadhibou"),
			)
		}
	}
	if !r.MissingLocation && strings.TrimSpace(ctx.Zone) != "" && strings.TrimSpace(ctx.Quartier) == "" {
		switch ctx.Lang {
		case lang.LangAR:
			chips = append(chips, qr("picker_location", "📍 تحديد الحي", "picker_city"))
		case lang.LangEN:
			chips = append(chips, qr("picker_location", "📍 Pick neighbourhood", "picker_city"))
		case lang.LangZH:
			chips = append(chips, qr("picker_location", "📍 选择街区", "picker_city"))
		default:
			chips = append(chips, qr("picker_location", "📍 Choisir le quartier", "picker_city"))
		}
	}
	return chips
}

func locationPhrase(ctx lang.MessageContext) string {
	q := strings.TrimSpace(ctx.Quartier)
	zone := strings.TrimSpace(ctx.Zone)
	city := strings.TrimSpace(ctx.City)
	if city == "" {
		city = "Nouakchott"
	}
	parts := make([]string, 0, 3)
	if q != "" {
		parts = append(parts, q)
	}
	if zone != "" {
		parts = append(parts, zone)
	}
	parts = append(parts, city)
	return strings.Join(parts, ", ")
}

func typeChipsEN(loc string, ctx lang.MessageContext) []QuickReply {
	return []QuickReply{
		{ID: "type_apartment", Text: "Apartment", Action: typeActionEN(ctx, loc, "an apartment", "apartment")},
		{ID: "type_house", Text: "House", Action: typeActionEN(ctx, loc, "a house", "house")},
		{ID: "type_villa", Text: "Villa", Action: typeActionEN(ctx, loc, "a villa", "villa")},
		{ID: "type_land", Text: "Land", Action: typeActionEN(ctx, loc, "land", "land")},
		{ID: "type_commercial", Text: "Commercial", Action: typeActionEN(ctx, loc, "commercial property", "commercial")},
	}
}

func typeActionEN(ctx lang.MessageContext, loc, phrase, kind string) string {
	switch ctx.Intent {
	case lang.IntentSearchRent:
		if kind == "land" {
			return fmt.Sprintf("I want to rent land in %s", loc)
		}
		return fmt.Sprintf("I want to rent %s in %s", phrase, loc)
	case lang.IntentSearchBuy, lang.IntentSearchLand:
		if kind == "land" {
			return fmt.Sprintf("I want to buy land in %s", loc)
		}
		return fmt.Sprintf("I want to buy %s in %s", phrase, loc)
	case lang.IntentSearchCommercial:
		return fmt.Sprintf("I want commercial property in %s", loc)
	default:
		return fmt.Sprintf("Show me %s in %s", phrase, loc)
	}
}

func typeChipsFR(loc string, ctx lang.MessageContext) []QuickReply {
	return []QuickReply{
		{ID: "type_apartment", Text: "Appartement", Action: typeActionFR(ctx, loc, "un appartement", "apartment")},
		{ID: "type_house", Text: "Maison", Action: typeActionFR(ctx, loc, "une maison", "house")},
		{ID: "type_villa", Text: "Villa", Action: typeActionFR(ctx, loc, "une villa", "villa")},
		{ID: "type_land", Text: "Terrain", Action: typeActionFR(ctx, loc, "un terrain", "land")},
		{ID: "type_commercial", Text: "Local", Action: typeActionFR(ctx, loc, "un local commercial", "commercial")},
	}
}

func typeActionFR(ctx lang.MessageContext, loc, phrase, kind string) string {
	switch ctx.Intent {
	case lang.IntentSearchRent:
		if kind == "land" {
			return fmt.Sprintf("Je cherche un terrain à louer à %s", loc)
		}
		return fmt.Sprintf("Je cherche à louer %s à %s", phrase, loc)
	case lang.IntentSearchBuy, lang.IntentSearchLand:
		if kind == "land" {
			return fmt.Sprintf("Je cherche un terrain à acheter à %s", loc)
		}
		return fmt.Sprintf("Je cherche à acheter %s à %s", phrase, loc)
	case lang.IntentSearchCommercial:
		return fmt.Sprintf("Je cherche un local commercial à %s", loc)
	default:
		return fmt.Sprintf("Montre-moi %s à %s", phrase, loc)
	}
}

func typeChipsAR(loc string, ctx lang.MessageContext) []QuickReply {
	return []QuickReply{
		{ID: "type_apartment", Text: "شقة", Action: typeActionAR(ctx, loc, "شقة", "apartment")},
		{ID: "type_house", Text: "منزل", Action: typeActionAR(ctx, loc, "منزل", "house")},
		{ID: "type_villa", Text: "فيلا", Action: typeActionAR(ctx, loc, "فيلا", "villa")},
		{ID: "type_land", Text: "أرض", Action: typeActionAR(ctx, loc, "أرض", "land")},
		{ID: "type_commercial", Text: "محل", Action: typeActionAR(ctx, loc, "محل تجاري", "commercial")},
	}
}

func typeActionAR(ctx lang.MessageContext, loc, phrase, kind string) string {
	switch ctx.Intent {
	case lang.IntentSearchRent:
		if kind == "land" {
			return fmt.Sprintf("أبحث عن أرض للإيجار في %s", loc)
		}
		return fmt.Sprintf("أبحث عن %s للإيجار في %s", phrase, loc)
	case lang.IntentSearchBuy, lang.IntentSearchLand:
		if kind == "land" {
			return fmt.Sprintf("أبحث عن أرض للبيع في %s", loc)
		}
		return fmt.Sprintf("أبحث عن %s للبيع في %s", phrase, loc)
	case lang.IntentSearchCommercial:
		return fmt.Sprintf("أبحث عن محل تجاري في %s", loc)
	default:
		return fmt.Sprintf("أبحث عن %s في %s", phrase, loc)
	}
}

func typeChipsZH(loc string, ctx lang.MessageContext) []QuickReply {
	return []QuickReply{
		{ID: "type_apartment", Text: "公寓", Action: typeActionZH(ctx, loc, "公寓", "apartment")},
		{ID: "type_house", Text: "住宅", Action: typeActionZH(ctx, loc, "住宅", "house")},
		{ID: "type_villa", Text: "别墅", Action: typeActionZH(ctx, loc, "别墅", "villa")},
		{ID: "type_land", Text: "土地", Action: typeActionZH(ctx, loc, "土地", "land")},
		{ID: "type_commercial", Text: "商铺", Action: typeActionZH(ctx, loc, "商铺", "commercial")},
	}
}

func typeActionZH(ctx lang.MessageContext, loc, phrase, kind string) string {
	switch ctx.Intent {
	case lang.IntentSearchRent:
		if kind == "land" {
			return fmt.Sprintf("我想在%s租土地", loc)
		}
		return fmt.Sprintf("我想在%s租%s", loc, phrase)
	case lang.IntentSearchBuy, lang.IntentSearchLand:
		if kind == "land" {
			return fmt.Sprintf("我想在%s买土地", loc)
		}
		return fmt.Sprintf("我想在%s买%s", loc, phrase)
	default:
		return fmt.Sprintf("我想在%s找%s", loc, phrase)
	}
}
