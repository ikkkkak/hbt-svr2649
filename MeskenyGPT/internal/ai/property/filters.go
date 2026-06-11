package property

import (
	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
	"strings"
)

// FiltersFromContext converts a MessageContext into DB filters.
// Uses ParseCurrency results: BudgetMin/BudgetMax (MRU).
func FiltersFromContext(ctx lang.MessageContext) Filters {
	ctx = lang.SanitizeBudgetContext(ctx, ctx.RawText)
	f := Filters{}

	f.City = ctx.City
	f.Zone = ctx.Zone
	f.Quartier = ctx.Quartier
	f.PlotNumber = ctx.PlotNumber
	f.Type = ctx.Type

	switch ctx.Intent {
	case lang.IntentSearchRent:
		f.Purpose = "rent"
	case lang.IntentSearchBuy, lang.IntentSearchAny, lang.IntentSearchLand:
		f.Purpose = "sale"
	}

	if ctx.BudgetMin > 0 && ctx.BudgetMax > ctx.BudgetMin {
		f.BudgetMin = float64(ctx.BudgetMin)
		f.BudgetMax = float64(ctx.BudgetMax)
	}

	return f
}

// ToCards maps raw properties into card DTOs for the frontend.
func ToCards(props []Property) []Card {
	out := make([]Card, 0, len(props))
	for _, p := range props {
		src := p.Source
		if src == "" {
			src = "property_sale"
		}
		out = append(out, Card{
			ID:            p.ID,
			Title:         p.Title,
			Price:         p.Price,
			Currency:      normalizeCurrency(p.Currency),
			City:          p.City,
			Bedrooms:      p.Bedrooms,
			Image:         p.Image,
			Type:          p.Type,
			Source:        src,
			SizeM2:        p.Area,
			LocationLabel: p.LocationLabel,
			Lat:           p.Lat,
			Lng:           p.Lng,
			PlotNumber:     p.PlotNumber,
			PlotCorners:    p.PlotCorners,
			CadastreLinked: p.CadastreLinked,
			QuartierLabel:  p.QuartierLabel,
		})
	}
	return out
}

func normalizeCurrency(cur string) string {
	c := strings.ToUpper(strings.TrimSpace(cur))
	if c == "" || c == "USD" || c == "US$" || c == "$" {
		return "MRU"
	}
	return c
}

