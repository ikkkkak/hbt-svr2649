package lang

import "testing"

func TestProcedureIntentRouting(t *testing.T) {
	procedure := []string{
		"كيف يمكنني تبديل ملكية قطعة ارضية",       // the reported failure
		"ما هي الوثائق المطلوبة لتسجيل عقار",        // required documents to register
		"comment transférer un titre foncier",       // FR title transfer
		"how do i transfer ownership of a land plot", // EN
		"quels sont les frais d'enregistrement",      // FR fees + question
		"إجراءات تحفيظ الأرض",                        // registration procedure
		"لدي عقار ولدي ملكية على اسمي وأريد أن احول هذه الملكية إلى اسم آخر", // non-adjacent transfer (reported)
		"أريد تحويل ملكية عقار من اسم إلى اسم آخر",   // from-name-to-name (reported)
		"طلب دمج سندات العقارية",                     // merge title deeds (reported)
		"هل يمكن ان يكون عقار له عدة مالكين ؟",       // co-ownership info (reported)
	}
	for _, m := range procedure {
		got := AnalyzeMessage(m).Intent
		if got == IntentSearchLand || IsPropertySearchIntent(got) {
			t.Errorf("procedure message routed to SEARCH (intent=%d): %q", got, m)
		}
		if got != IntentInfoProcedure {
			t.Errorf("expected IntentInfoProcedure, got %d for %q", got, m)
		}
	}

	// These must STILL be property searches, not procedure.
	search := []string{
		"أريد شراء أرض في نواكشوط", // buy land
		"terrain à vendre à Tevragh Zeina",
		"looking for a house in Nouakchott",
	}
	for _, m := range search {
		got := AnalyzeMessage(m).Intent
		if got == IntentInfoProcedure {
			t.Errorf("search message wrongly routed to procedure: %q", m)
		}
	}
}
