package services

import "testing"

func TestParseProceduresExport(t *testing.T) {
	payload := `{"source":"procedures.gov.mr","target_categories":[{"title_fr":"Domaines","title_ar":"الأملاك العقارية","procedures":[{"procedure_id":166,"french_details":{"url":"https://procedures.gov.mr/fr/procedure/166","title":"Morcellement d'un Titre Foncier","metadata_fields":{"Délai de traitement":"10 jours","Frais afférents":"Timbre 50 MRU"},"required_documents":["Titre foncier original","Photocopie de la CIN"]},"arabic_details":{"url":"https://procedures.gov.mr/ar/procedure/166","title":"تقسيم سند عقاري","required_documents":["أصل السند العقاري"]}}]}]}`
	items := parsePastedItems(payload)
	if len(items) != 2 {
		t.Fatalf("expected 2 records (ar+fr), got %d", len(items))
	}
	// Arabic first
	if items[0].Title != "تقسيم سند عقاري" || items[0].URL != "https://procedures.gov.mr/ar/procedure/166" {
		t.Errorf("bad AR record: %+v", items[0])
	}
	if items[1].Title != "Morcellement d'un Titre Foncier" {
		t.Errorf("bad FR title: %q", items[1].Title)
	}
	// FR description must carry metadata + documents
	d := items[1].Description
	if !contains(d, "50 MRU") || !contains(d, "Titre foncier original") {
		t.Errorf("FR description missing metadata/docs: %q", d)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
