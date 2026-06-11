package session

import (
	"testing"

	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
)

func TestMergeIntoContext_FillsMissingCityZone(t *testing.T) {
	ctx := lang.MessageContext{Intent: lang.IntentSearchBuy}
	fc := FilterContext{City: "Nouakchott", Zone: "Tevragh Zeina", MinPrice: 500000, MaxPrice: 1000000}
	out := MergeIntoContext(ctx, fc)
	if out.City != "Nouakchott" || out.Zone != "Tevragh Zeina" {
		t.Fatalf("expected merged location, got city=%q zone=%q", out.City, out.Zone)
	}
	if out.BudgetMin != 500000 || out.BudgetMax != 1000000 {
		t.Fatalf("expected budget merge")
	}
}
