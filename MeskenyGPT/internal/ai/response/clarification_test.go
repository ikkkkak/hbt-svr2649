package response_test

import (
	"strings"
	"testing"

	"apartments-clone-server/MeskenyGPT/internal/ai/lang"
	"apartments-clone-server/MeskenyGPT/internal/ai/response"
)

func TestProactiveClarification_PropertyForSaleNoCity(t *testing.T) {
	ctx := lang.AnalyzeMessage("Find a property for sale please")
	if !lang.ShouldClarifyBeforeSearch(ctx) {
		t.Fatal("expected clarification before empty geo search")
	}
	out := response.ProactiveClarificationOutput(ctx)
	if strings.TrimSpace(out.Message.Content) == "" {
		t.Fatal("expected non-empty clarification message")
	}
	if len(out.QuickReplies) == 0 {
		t.Fatal("expected location quick replies")
	}
}

func TestProactiveClarification_NouakchottRentAllowed(t *testing.T) {
	ctx := lang.AnalyzeMessage("I want to rent an apartment in Nouakchott under 100000 MRU")
	if lang.ShouldClarifyBeforeSearch(ctx) {
		t.Fatal("complete query should not require clarification")
	}
}
