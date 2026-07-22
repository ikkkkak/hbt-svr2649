package services

import "testing"

func TestParseMinistriesExport(t *testing.T) {
	payload := `{"source":"https://ijraati.gov.mr/procedures/","ministries":[{"ministry_ar":"وزارة العقارات","procedures":[{"title":"طلب تحويل رخصة حيازة","description":"نقل ملكية رخصة الإشغال","category":"العقارات","concerned_parties":{"مكلف بالإجراء":"المديرية العامة"},"required_documents":"الطلب، رخصة حيازة، بطاقة تعريف","duration_and_cost":{"المدة":"3 أيام","المبلغ":"50 أوقية + 2% من القيمة المساحية"}},{"title":"طلب نقل سند عقاري","description":"تسجيل نقل ملكية","required_documents":"طلب, سند عقاري","duration_and_cost":{"المدة":"10 أيام"}}]}]}`
	items := parsePastedItems(payload)
	if len(items) != 2 {
		t.Fatalf("expected 2 procedure records, got %d", len(items))
	}
	if items[0].Title != "طلب تحويل رخصة حيازة" {
		t.Errorf("bad title: %q", items[0].Title)
	}
	d := items[0].Description
	for _, must := range []string{"رخصة حيازة", "3 أيام", "2%", "50 أوقية"} {
		if indexOf(d, must) < 0 {
			t.Errorf("description missing %q: %q", must, d)
		}
	}
	if items[0].URL != "https://ijraati.gov.mr/procedures/" {
		t.Errorf("bad url: %q", items[0].URL)
	}
}
