package lang

import (
	"strings"
	"testing"
)

func TestAnalyzeMessage_SecteurSetsZoneNotBudget(t *testing.T) {
	ctx := AnalyzeMessage("دار للبيع في سكتير 1")
	if ctx.BudgetMRU != 0 {
		t.Errorf("BudgetMRU: want 0, got %d", ctx.BudgetMRU)
	}
	if ctx.City != "nouakchott" {
		t.Errorf("City: want nouakchott, got %q", ctx.City)
	}
	if ctx.Zone == "" || !strings.Contains(ctx.Zone, "secteur 1") {
		t.Errorf("Zone should include secteur 1, got %q", ctx.Zone)
	}
	if ctx.Intent != IntentSearchBuy {
		t.Errorf("Intent: want buy, got %v", ctx.Intent)
	}
}

func TestAnalyzeMessage_SecteurFrench(t *testing.T) {
	ctx := AnalyzeMessage("maison à vendre secteur 2 nouakchott")
	if ctx.BudgetMRU != 0 {
		t.Errorf("BudgetMRU: want 0, got %d", ctx.BudgetMRU)
	}
	if !strings.Contains(ctx.Zone, "secteur 2") {
		t.Errorf("Zone: %q", ctx.Zone)
	}
}
