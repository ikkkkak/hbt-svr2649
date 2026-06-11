package agent

import (
	"testing"

	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
)

func TestRouteRole_Searcher(t *testing.T) {
	ctx := lang.AnalyzeMessage("I want to rent in Nouakchott")
	if RouteRole(ctx, "I want to rent in Nouakchott") != RolePropertySearcher {
		t.Fatal("expected PropertySearcher")
	}
}

func TestRouteRole_Analyst(t *testing.T) {
	ctx := lang.AnalyzeMessage("What are market trends in Tevragh Zeina?")
	if RouteRole(ctx, "What are market trends in Tevragh Zeina?") != RoleMarketAnalyst {
		t.Fatal("expected MarketAnalyst")
	}
}
