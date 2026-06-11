package lang

import "testing"

func TestShouldClarify_PropertyForSaleNoCity(t *testing.T) {
	ctx := AnalyzeMessage("Find a property for sale please")
	if ctx.Intent != IntentSearchBuy {
		t.Fatalf("expected buy intent, got %v", ctx.Intent)
	}
	if ShouldClarifyBeforeSearch(ctx) {
		return
	}
	t.Fatal("expected clarification before empty geo search")
}

func TestShouldSearch_NouakchottRent(t *testing.T) {
	ctx := AnalyzeMessage("I want to rent an apartment in Nouakchott under 100000 MRU")
	if ShouldClarifyBeforeSearch(ctx) {
		t.Fatal("expected DB search with city+budget")
	}
	r := EvaluateSearchReadiness(ctx)
	if !r.CanSearch {
		t.Fatal("should allow search")
	}
}

func TestShouldClarify_LandOnly(t *testing.T) {
	ctx := AnalyzeMessage("terrain")
	if !ShouldClarifyBeforeSearch(ctx) {
		t.Fatal("land without location should clarify")
	}
}
