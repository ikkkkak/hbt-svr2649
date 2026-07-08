package lang

import "testing"

func TestMessageSignalsRent_KsarFamilyRental(t *testing.T) {
	msg := "اريد منزل للإجاره في لكصر يكون عايلي"
	if !MessageSignalsRent(msg) {
		t.Fatal("expected rent signal for للإجاره")
	}
	if MessageSignalsBuy(msg) {
		t.Fatal("rent message must not signal buy")
	}
	ctx := AnalyzeMessage(msg)
	if ctx.Intent != IntentSearchRent {
		t.Fatalf("intent=%v want IntentSearchRent", ctx.Intent)
	}
	if ctx.Type != "house" {
		t.Fatalf("type=%q want house", ctx.Type)
	}
	if ctx.City != "nouakchott" {
		t.Fatalf("city=%q want nouakchott", ctx.City)
	}
}

func TestReconcileTransactionOverridesSessionSale(t *testing.T) {
	ctx := MessageContext{
		RawText: "اريد منزل للإجاره في لكصر يكون عايلي",
		Intent:  IntentSearchBuy,
		Type:    "house",
		City:    "nouakchott",
		Zone:    "ksar",
	}
	ctx = ReconcileTransactionFromMessage(ctx)
	if ctx.Intent != IntentSearchRent {
		t.Fatalf("intent=%v want rent override", ctx.Intent)
	}
}
