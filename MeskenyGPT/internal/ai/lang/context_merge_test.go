package lang

import "testing"

func TestEnrichContextFromHistory_BuyAfterPicker(t *testing.T) {
	picker := `[MESKENY_PICKER]
city_name=Nouakchott
zone_name=Tevragh Zeina
quartier_name=
budget_min_mru=500000
budget_max_mru=1000000
[/MESKENY_PICKER]`
	hist := []HistoryTurn{{Role: "user", Content: picker}}
	ctx := AnalyzeMessage("I want to buy an apartment")
	ctx = EnrichContextFromHistory(ctx, hist, "I want to buy an apartment")
	if ctx.City == "" || ctx.Zone == "" {
		t.Fatalf("expected city/zone from picker history, got city=%q zone=%q", ctx.City, ctx.Zone)
	}
	if ShouldClarifyBeforeSearch(ctx) {
		t.Fatal("buy + apartment + picker location should search, not clarify again")
	}
}

func TestShouldClarify_PickerNeedsPurposeAndType(t *testing.T) {
	msg := `[MESKENY_PICKER]
city_name=Nouakchott
zone_name=Ksar
quartier_name=
budget_min_mru=0
budget_max_mru=0
[/MESKENY_PICKER]`
	ctx := AnalyzeMessage(msg)
	ctx.RawText = msg
	if !ShouldClarifyBeforeSearch(ctx) {
		t.Fatal("picker with city+zone but no rent/buy or type must clarify first")
	}
}

func TestShouldClarify_BuyAfterPickerNeedsType(t *testing.T) {
	picker := `[MESKENY_PICKER]
city_name=Nouakchott
zone_name=Ksar
quartier_name=
budget_min_mru=0
budget_max_mru=0
[/MESKENY_PICKER]`
	hist := []HistoryTurn{{Role: "user", Content: picker}}
	ctx := AnalyzeMessage("I want to buy")
	ctx = EnrichContextFromHistory(ctx, hist, "I want to buy")
	if !ShouldClarifyBeforeSearch(ctx) {
		t.Fatal("buy with location but no property type should clarify for type")
	}
}

func TestEnrichContextFromHistory_HouseAfterBuy(t *testing.T) {
	picker := `[MESKENY_PICKER]
city_name=Nouakchott
zone_name=Ksar
quartier_name=
budget_min_mru=0
budget_max_mru=0
[/MESKENY_PICKER]`
	hist := []HistoryTurn{
		{Role: "user", Content: picker},
		{Role: "user", Content: "I want to buy"},
	}
	ctx := AnalyzeMessage("house")
	ctx = EnrichContextFromHistory(ctx, hist, "house")
	if ctx.Intent != IntentSearchBuy {
		t.Fatalf("expected buy intent from history, got %v", ctx.Intent)
	}
	if ctx.Type != "house" {
		t.Fatalf("expected house type from house, got %q", ctx.Type)
	}
	if ShouldClarifyBeforeSearch(ctx) {
		t.Fatal("buy + house + picker location should search, not ask rent/buy again")
	}
}
