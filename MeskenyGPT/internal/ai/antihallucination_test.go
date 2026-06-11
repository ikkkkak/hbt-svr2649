package ai

import (
	"strings"
	"testing"

	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
	"apartments-clone-server/MeskenyGPT/internal/ai/response"
)

// AH-001: vague search must clarify, not hit DB with empty geo.
func TestAH001_ClarifyBeforeEmptyGeoSearch(t *testing.T) {
	ctx := lang.AnalyzeMessage("Find a property for sale please")
	if !lang.ShouldClarifyBeforeSearch(ctx) {
		t.Fatal("expected clarification gate for vague sale query")
	}
}

// AH-003: zero-results copy must not claim fabricated listings.
func TestAH003_NoResultsDoesNotClaimFound(t *testing.T) {
	ctx := lang.AnalyzeMessage("appartement à louer à Nouakchott Tevragh Zeina")
	ctx.City = "nouakchott"
	ctx.Zone = "tevragh zeina"
	out := response.NoResultsOutput(ctx)
	if strings.Contains(strings.ToLower(out.Message.Content), "i found") {
		t.Fatal("no-results message must not claim discoveries")
	}
	if len(out.QuickReplies) == 0 {
		t.Fatal("expected actionable follow-up chips")
	}
}

// AH-006: conversational integrity blocks fake result claims without cards.
func TestAH006_EnforceNoCardsBlocksFakeFound(t *testing.T) {
	ctx := lang.MessageContext{Lang: lang.LangEN}
	got := enforceNoCardsResponseIntegrity(ctx, "I found 3 great apartments for you!")
	if strings.Contains(strings.ToLower(got), "i found") {
		t.Fatal("must replace hallucinated found-claims")
	}
}
