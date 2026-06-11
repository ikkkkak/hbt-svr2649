package agent

import (
	"testing"

	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
)

func TestBuildStepPlan_SearchWithCity(t *testing.T) {
	ctx := lang.MessageContext{Lang: lang.LangEN, Intent: lang.IntentSearchRent, City: "Nouakchott", Zone: "Tevragh Zeina"}
	plan := BuildStepPlan(RolePropertySearcher, ctx, PathSearch)
	if len(plan) < 5 {
		t.Fatalf("expected search plan, got %d steps", len(plan))
	}
	if plan[2].ID != "gather" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
}

func TestBuildStepPlan_Clarify(t *testing.T) {
	ctx := lang.AnalyzeMessage("Find a property for sale please")
	plan := BuildStepPlan(RolePropertySearcher, ctx, PathClarify)
	if len(plan) != 4 {
		t.Fatalf("expected 4 clarify steps, got %d", len(plan))
	}
}
