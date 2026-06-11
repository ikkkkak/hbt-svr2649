// ─────────────────────────────────────────────────────────────────────────────
// currency.go — Mauritanian Currency Normalizer
// ─────────────────────────────────────────────────────────────────────────────
//
// THE MAURITANIAN CURRENCY SYSTEM (read this first):
//
//   MRU  = Ouguiya Mauritanien (NEW) — official since 2018
//   MRO  = Ouguiya (OLD)             — used before 2018, still in daily speech
//
//   Conversion: 1 MRU = 10 MRO
//   So:         1 000 000 MRO  =  100 000 MRU
//               4 000 000 MRO  =  400 000 MRU
//               4 000 000 MRU  =  40 000 000 MRO
//
// USER SPEECH PATTERNS AND WHAT THEY MEAN IN MRU:
//
//   "4 million" (no unit)             → 400 000 MRU     (spoken in old MRO)
//   "40 millions" (no unit)           → 4 000 000 MRU
//   "4 مليون" (no unit)               → 400 000 MRU
//   "4 million MRU"                   → 4 000 000 MRU   (explicit new currency)
//   "4 million MRO"                   → 400 000 MRU
//   "4000000" (plain digits)          → 4 000 000 MRU   (canonical DB amount)
//
// ─────────────────────────────────────────────────────────────────────────────

package lang

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// CurrencyHint tells us which currency the user likely meant.
type CurrencyHint int

const (
	CurrencyMRU     CurrencyHint = iota // New Ouguiya — default
	CurrencyMRO                         // Old Ouguiya — divide by 10 to get MRU
	CurrencyAmbiguous                   // No MRU/MRO qualifier → treat spoken amounts as old MRO
)

// ParsedAmount holds the normalized result.
type ParsedAmount struct {
	Raw           string
	AmountMRU     int64
	Currency      CurrencyHint
	IsApproximate bool
	RangeMin      int64
	RangeMax      int64
}

var arabicToLatin = strings.NewReplacer(
	"٠", "0", "١", "1", "٢", "2", "٣", "3", "٤", "4",
	"٥", "5", "٦", "6", "٧", "7", "٨", "8", "٩", "9",
	"،", ",", "٫", ".",
)

func normalizeDigits(s string) string {
	return arabicToLatin.Replace(s)
}

var mroSignals = []string{
	"mro", "um",
	"ancien", "ancienne", "vieux", "vieille", "anciens ouguiya", "ancienne monnaie",
	"old", "previous", "former",
	"قديم", "قديمة", "القديم", "القديمة", "العملة القديمة",
}

var mruSignals = []string{
	"mru",
	"nouveau", "nouvelle", "nouvel", "nouvelle monnaie",
	"new",
	"جديد", "جديدة", "الجديد", "الجديدة", "العملة الجديدة",
}

var approxSignals = []string{
	"حوالي", "تقريبا", "تقريباً", "نحو", "قرابة", "حول",
	"environ", "à peu près", "autour de", "approximativement", "vers",
	"around", "about", "approximately", "roughly", "nearly", "close to",
}

var millionWords = []string{
	"million", "millions", "مليون", "ملايين",
}

var thousandWords = []string{
	"mille", "thousand", "ألف", "آلاف",
}

var arabicSpecials = map[string]float64{
	"مليونين": 2_000_000,
	"نص مليون": 500_000,
	"نصف مليون": 500_000,
	"ربع مليون": 250_000,
	"مليون ونص": 1_500_000,
	"مليون ونصف": 1_500_000,
}

const BudgetTolerance = 0.40
const ExactBudgetTolerance = 0.10

var numberRe = regexp.MustCompile(
	`(\d[\d,.\s]*\d|\d)` +
		`(?:\s*` +
		`(million[s]?|مليون|ملايين|mille|thousand|ألف|آلاف|k)` +
		`)?`,
)

// ParseCurrency extracts a monetary amount from a user message and returns
// the canonical MRU value plus a suggested DB search range.
func ParseCurrency(msg string) *ParsedAmount {
	if looksLikeRoomCount(msg) {
		return nil
	}
	norm := strings.ToLower(normalizeDigits(msg))

	for phrase, val := range arabicSpecials {
		if strings.Contains(norm, phrase) {
			cur, _ := detectCurrencyHint(norm)
			mruVal := applyConversion(int64(val), cur, true)
			tol := ExactBudgetTolerance
			if detectApproximate(norm) {
				tol = BudgetTolerance
			}
			return buildResult(msg, mruVal, cur, detectApproximate(norm), tol)
		}
	}

	raw, multiplier, ok := extractNumber(norm)
	if !ok {
		return nil
	}

	cur, _ := detectCurrencyHint(norm)
	isApprox := detectApproximate(norm)
	colloquial := multiplier > 1
	mruVal := applyConversion(int64(math.Round(raw*multiplier)), cur, colloquial)
	tol := ExactBudgetTolerance
	if isApprox {
		tol = BudgetTolerance
	}
	return buildResult(msg, mruVal, cur, isApprox, tol)
}

func extractNumber(norm string) (value float64, multiplier float64, ok bool) {
	plainRe := regexp.MustCompile(`\b(\d{4,})\b`)
	if m := plainRe.FindStringSubmatch(norm); m != nil {
		v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
		if err == nil && v > 0 {
			return v, 1, true
		}
	}

	// Walk all numeric matches so we can skip Nouakchott "secteur / سكتور N" labels
	// (e.g. "دار للبيع في سكتير 1" must not become 1 MRU).
	all := numberRe.FindAllStringSubmatchIndex(norm, -1)
	for _, loc := range all {
		if len(loc) < 4 {
			continue
		}
		numStart, numEnd := loc[2], loc[3]
		if numStart < 0 || numEnd <= numStart {
			continue
		}
		raw := strings.ReplaceAll(norm[numStart:numEnd], ",", "")
		raw = strings.ReplaceAll(raw, " ", "")
		v, err := strconv.ParseFloat(raw, 64)
		if err != nil || v == 0 {
			continue
		}

		mult := 1.0
		if len(loc) >= 6 && loc[4] >= 0 && loc[5] >= loc[4] {
			unit := strings.TrimSpace(norm[loc[4]:loc[5]])
			switch {
			case containsAny(unit, millionWords):
				mult = 1_000_000
			case containsAny(unit, thousandWords) || unit == "k":
				mult = 1_000
			}
		}

		if mult == 1 && v < 1000 && numberLooksLikeSecteurZoneIndex(norm, numStart, numEnd) {
			continue
		}
		if mult == 1 && v <= 12 && numberLooksLikeRoomCount(norm, numStart, numEnd) {
			continue
		}
		return v, mult, true
	}
	return 0, 0, false
}

// secteurPriceFalsePositiveKeys — user means "Secteur N" / "سكتور N", not a price.
var secteurPriceFalsePositiveKeys = []string{
	"سكتور", "سكتر", "سكتير", "قطاع", "secteur", "sector",
}

var roomCountKeys = []string{
	"غرف", "غرفة", "غرفه", "chambre", "chambres", "bedroom", "bedrooms", "room", "rooms",
	"pièce", "pièces", "piece", "pieces", "br ", "brs", "bed ", "beds",
}

func looksLikeRoomCount(msg string) bool {
	norm := strings.ToLower(normalizeDigits(msg))
	for _, k := range roomCountKeys {
		if strings.Contains(norm, k) {
			return true
		}
	}
	return false
}

func numberLooksLikeRoomCount(norm string, numStart, numEnd int) bool {
	const win = 28
	s := numStart - win
	if s < 0 {
		s = 0
	}
	before := norm[s:numStart]
	e := numEnd + win
	if e > len(norm) {
		e = len(norm)
	}
	after := norm[numEnd:e]
	for _, k := range roomCountKeys {
		if strings.Contains(before, k) || strings.Contains(after, k) {
			return true
		}
	}
	return false
}

func numberLooksLikeSecteurZoneIndex(norm string, numStart, numEnd int) bool {
	const win = 36
	s := numStart - win
	if s < 0 {
		s = 0
	}
	before := norm[s:numStart]
	e := numEnd + win
	if e > len(norm) {
		e = len(norm)
	}
	after := norm[numEnd:e]
	for _, k := range secteurPriceFalsePositiveKeys {
		if strings.Contains(before, k) || strings.Contains(after, k) {
			return true
		}
	}
	return false
}

func detectCurrencyHint(norm string) (CurrencyHint, CurrencyHint) {
	for _, sig := range mroSignals {
		if strings.Contains(norm, sig) {
			return CurrencyMRO, CurrencyMRO
		}
	}
	for _, sig := range mruSignals {
		if strings.Contains(norm, sig) {
			return CurrencyMRU, CurrencyMRU
		}
	}
	return CurrencyAmbiguous, CurrencyAmbiguous
}

func detectApproximate(norm string) bool {
	for _, sig := range approxSignals {
		if strings.Contains(norm, sig) {
			return true
		}
	}
	return false
}

func applyConversion(raw int64, cur CurrencyHint, colloquialAmount bool) int64 {
	switch cur {
	case CurrencyMRU:
		return raw
	case CurrencyMRO:
		return raw / 10
	case CurrencyAmbiguous:
		if colloquialAmount {
			// Daily speech in Mauritania often quotes old ouguiya (MRO) without saying so.
			return raw / 10
		}
		// Plain digit amounts (e.g. 4000000) are already in MRU for DB listings.
		return raw
	default:
		return raw
	}
}

func buildResult(raw string, mru int64, hint CurrencyHint, approx bool, tol float64) *ParsedAmount {
	min := int64(math.Round(float64(mru) * (1 - tol)))
	max := int64(math.Round(float64(mru) * (1 + tol)))
	if min < 0 {
		min = 0
	}
	return &ParsedAmount{
		Raw:           raw,
		AmountMRU:     mru,
		Currency:      hint,
		IsApproximate: approx,
		RangeMin:      min,
		RangeMax:      max,
	}
}

func containsAny(s string, list []string) bool {
	for _, w := range list {
		if strings.Contains(s, w) {
			return true
		}
	}
	return false
}

// FormatMRU formats an MRU amount for display in Arabic context.
func FormatMRU(v int64) string {
	switch {
	case v >= 1_000_000 && v%1_000_000 == 0:
		return strconv.FormatInt(v/1_000_000, 10) + " مليون أوقية"
	case v >= 1_000_000:
		major := v / 1_000_000
		minor := (v % 1_000_000) / 1_000
		if minor > 0 {
			return strconv.FormatInt(major, 10) + " مليون و" +
				strconv.FormatInt(minor, 10) + " ألف أوقية"
		}
		return strconv.FormatInt(major, 10) + " مليون أوقية"
	case v >= 1_000 && v%1_000 == 0:
		return strconv.FormatInt(v/1_000, 10) + " ألف أوقية"
	default:
		return strconv.FormatInt(v, 10) + " أوقية"
	}
}

// FormatMRUFR formats for French context.
func FormatMRUFR(v int64) string {
	switch {
	case v >= 1_000_000 && v%1_000_000 == 0:
		return strconv.FormatInt(v/1_000_000, 10) + " millions MRU"
	case v >= 1_000_000:
		major := v / 1_000_000
		minor := (v % 1_000_000) / 1_000
		if minor > 0 {
			return strconv.FormatInt(major, 10) + "." +
				strconv.FormatInt(minor, 10) + " millions MRU"
		}
		return strconv.FormatInt(major, 10) + " millions MRU"
	case v >= 1_000:
		return strconv.FormatInt(v/1_000, 10) + " 000 MRU"
	default:
		return strconv.FormatInt(v, 10) + " MRU"
	}
}
