package listing_ai

import "testing"

func testCatalog() []LocationEntry {
	return []LocationEntry{
		{CityID: 1, CityName: "Nouakchott", CityNameAr: "نواكشوط", ZoneID: 10, ZoneName: "Tevragh Zeina", ZoneNameAr: "تفرغ زينة", QuartierID: 100, QuartierName: "Ksar", QuartierAr: "لكصر"},
		{CityID: 1, CityName: "Nouakchott", ZoneID: 10, ZoneName: "Tevragh Zeina", QuartierID: 101, QuartierName: "Ambassades", QuartierAr: "السفارات"},
		{CityID: 1, CityName: "Nouakchott", ZoneID: 11, ZoneName: "Arafat", QuartierID: 102, QuartierName: "Arafat Centre", QuartierAr: "عرفات"},
	}
}

func TestMatchLocation_QuartierHintAndDescription(t *testing.T) {
	entries := testCatalog()
	details := "شقة للبيع في حي لكصر تفرغ زينة نواكشوط"
	loc, conf := MatchLocation(entries, "Nouakchott", "Tevragh Zeina", "Ksar", details)
	if loc.QuartierID != 100 {
		t.Fatalf("expected quartier Ksar (100), got %+v conf=%s", loc, conf)
	}
	if loc.ZoneID != 10 {
		t.Fatalf("expected zone Tevragh Zeina (10), got %d", loc.ZoneID)
	}
}

func TestMatchLocation_QuartierOnlyInDescription(t *testing.T) {
	entries := testCatalog()
	details := "Beautiful apartment in Ambassades, Tevragh Zeina"
	loc, conf := MatchLocation(entries, "", "", "", details)
	if loc.QuartierID != 101 {
		t.Fatalf("expected Ambassades, got %+v conf=%s", loc, conf)
	}
}

func TestFindQuartierInText_LongestWins(t *testing.T) {
	entries := testCatalog()
	q := findQuartierInText(entries, "near Arafat Centre market")
	if q != "Arafat Centre" {
		t.Fatalf("expected Arafat Centre, got %q", q)
	}
}
