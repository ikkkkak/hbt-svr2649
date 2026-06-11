package lang

import "testing"

func TestDetectLang_EnglishPropertyQuery(t *testing.T) {
	if DetectLang("I want a property for sale") != LangEN {
		t.Fatalf("expected LangEN for English sale query, got %v", DetectLang("I want a property for sale"))
	}
}

func TestDetectLang_French(t *testing.T) {
	if DetectLang("Je cherche une maison à louer à Nouakchott") != LangFR {
		t.Fatalf("expected LangFR, got %v", DetectLang("Je cherche une maison à louer à Nouakchott"))
	}
}

func TestDetectLang_Arabic(t *testing.T) {
	if DetectLang("أريد شقة للبيع في نواكشوط") != LangAR {
		t.Fatalf("expected LangAR, got %v", DetectLang("أريد شقة للبيع في نواكشوط"))
	}
}

func TestDetectLang_Chinese(t *testing.T) {
	if DetectLang("我想在努瓦克肖特买房") != LangZH {
		t.Fatalf("expected LangZH, got %v", DetectLang("我想在努瓦克肖特买房"))
	}
}
