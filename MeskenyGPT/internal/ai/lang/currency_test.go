package lang

import "testing"

type currencyTest struct {
	input       string
	wantMRU     int64
	wantApprox  bool
	wantRangeOK bool
}

var currencyTests = []currencyTest{
	{"4 million", 400_000, false, false},
	{"40 millions", 4_000_000, false, false},
	{"4 millions", 400_000, false, false},
	{"4 مليون", 400_000, false, false},
	{"4 million MRU", 4_000_000, false, false},
	{"4 million MRO", 400_000, false, false},
	{"4 million UM", 400_000, false, false},
	{"4 million ouguiya ancien", 400_000, false, false},
	{"400 ألف", 40_000, false, false},
	{"4000000", 4_000_000, false, false},
	{"حوالي 4 مليون", 400_000, true, true},
	{"environ 4 millions", 400_000, true, true},
	{"around 4 million", 400_000, true, true},
	{"مليونين", 200_000, false, false},
	{"نص مليون", 50_000, false, false},
	{"نصف مليون", 50_000, false, false},
	{"ربع مليون", 25_000, false, false},
	{"مليون ونصف", 150_000, false, false},
}

func TestParseCurrency(t *testing.T) {
	for _, tc := range currencyTests {
		t.Run(tc.input, func(t *testing.T) {
			result := ParseCurrency(tc.input)
			if tc.wantMRU == 0 {
				if result != nil {
					t.Errorf("expected nil, got %+v", result)
				}
				return
			}
			if result == nil {
				t.Fatalf("expected result, got nil for input %q", tc.input)
			}
			if result.AmountMRU != tc.wantMRU {
				t.Errorf("AmountMRU: got %d, want %d", result.AmountMRU, tc.wantMRU)
			}
			if result.IsApproximate != tc.wantApprox {
				t.Errorf("IsApproximate: got %v, want %v", result.IsApproximate, tc.wantApprox)
			}
			if tc.wantRangeOK {
				if result.RangeMin >= result.AmountMRU {
					t.Errorf("RangeMin (%d) should be < AmountMRU (%d)", result.RangeMin, result.AmountMRU)
				}
				if result.RangeMax <= result.AmountMRU {
					t.Errorf("RangeMax (%d) should be > AmountMRU (%d)", result.RangeMax, result.AmountMRU)
				}
			}
		})
	}
}

func TestParseCurrency_IgnoresSecteurNumber(t *testing.T) {
	for _, q := range []string{
		"دار للبيع في سكتير 1",
		"villa à vendre secteur 3",
		"سكتور 2 في نواكشوط",
		"dar for sale in sector 4",
	} {
		if r := ParseCurrency(q); r != nil {
			t.Errorf("%q: expected nil currency, got %+v", q, r)
		}
	}
}

func TestParseCurrency_SecteurThenMillionStillParsed(t *testing.T) {
	r := ParseCurrency("سكتور 1 ب 5 مليون")
	if r == nil {
		t.Fatal("expected budget from مليون")
	}
	if r.AmountMRU != 500_000 {
		t.Errorf("AmountMRU: want 500000, got %d", r.AmountMRU)
	}
}

func TestBudgetRange_LogReproduction(t *testing.T) {
	r := ParseCurrency("منزل للبيع في تفرغ زينة السعر حوالي 4 ملايين")
	if r == nil {
		t.Fatal("expected result, got nil")
	}
	if r.AmountMRU != 400_000 {
		t.Errorf("want 400000, got %d", r.AmountMRU)
	}
	if r.RangeMin != 240_000 {
		t.Errorf("RangeMin: want 240000, got %d", r.RangeMin)
	}
	if r.RangeMax != 560_000 {
		t.Errorf("RangeMax: want 560000, got %d", r.RangeMax)
	}
}

func TestFormatMRU(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{4_000_000, "4 مليون أوقية"},
		{2_500_000, "2 مليون و500 ألف أوقية"},
		{400_000, "400 ألف أوقية"},
	}
	for _, tc := range cases {
		got := FormatMRU(tc.in)
		if got != tc.want {
			t.Errorf("FormatMRU(%d): got %q, want %q", tc.in, got, tc.want)
		}
	}
}
