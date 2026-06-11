package discoverpush

import (
	"apartments-clone-server/goldproperty"
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"
)

const discoveryEventSpotlight = "discovery_spotlight"
const (
	crossChannelMinGap   = 30 * time.Minute
	repeatListingBlockFor = 7 * 24 * time.Hour
)

// PushSender sends one Expo/FCM notification (implemented by services/push).
type PushSender func(token, title, body, image string, data map[string]string) error

// TrySendMarketingDiscovery builds a localized, deep-linked spotlight for one
// marketing device and delivers it via send. Returns true if a push was sent.
func TrySendMarketingDiscovery(device *models.MarketingDevice, now time.Time, send PushSender) bool {
	if device == nil || strings.TrimSpace(device.FCMToken) == "" {
		return false
	}
	if storage.DB == nil {
		return false
	}

	locale := normalizeDiscoveryLocale(device.Locale)
	if !canSendDiscoveryToDevice(device.DeviceID, now) {
		return false
	}

	var pref models.AnonymousUserPreference
	_ = storage.DB.Where("device_id = ?", device.DeviceID).First(&pref).Error

	choice, _ := pickDiscoveryTargetForProfile(profileFromAnonymousPref(&pref), now)
	if choice.sale == nil && choice.lm == nil {
		return false
	}

	if choice.sale != nil {
		sidCheck := choice.sale.ID
		if discoveryRecentlySentForListing(nil, device.DeviceID, &sidCheck, nil, now) {
			return false
		}
		title, body := buildSaleDiscoveryCopy(locale, choice.sale, rand.Intn(4))
		img := firstImageURL(choice.sale.Images)
		data := saleDeepLinkData(choice.sale.ID)
		if err := send(strings.TrimSpace(device.FCMToken), title, body, img, data); err != nil {
			log.Printf("discovery: marketing send failed device=%s: %v", device.DeviceID, err)
			return false
		}
		sid := choice.sale.ID
		_ = LogDiscoverySend(nil, device.DeviceID, &sid, nil)
		if choice.sale.IsGold {
			goldproperty.RecordNotificationSent(choice.sale.ID)
		}
		log.Printf("discovery: sent sale id=%d to marketing device=%s locale=%s", choice.sale.ID, device.DeviceID, locale)
		return true
	}

	if choice.lm != nil {
		if discoveryRecentlySentForListing(nil, device.DeviceID, nil, &choice.lm.ID, now) {
			return false
		}
		title, body := buildLandmarkDiscoveryCopy(locale, choice.lm, rand.Intn(4))
		img := landmarkFirstImageURL(choice.lm)
		data := landmarkDeepLinkData(choice.lm.ID)
		if err := send(strings.TrimSpace(device.FCMToken), title, body, img, data); err != nil {
			log.Printf("discovery: marketing landmark send failed device=%s: %v", device.DeviceID, err)
			return false
		}
		_ = LogDiscoverySend(nil, device.DeviceID, nil, &choice.lm.ID)
		log.Printf("discovery: sent landmark id=%d to marketing device=%s locale=%s", choice.lm.ID, device.DeviceID, locale)
		return true
	}

	return false
}

// PlanUserSpotlight prepares a localized discovery payload for a logged-in user.
// ok is false when rate limits, dedupe, or inventory prevent a send.
func PlanUserSpotlight(u *models.User, now time.Time) (title, body, img string, data map[string]string, saleID *uint, landmarkID *uint, ok bool) {
	if u == nil || storage.DB == nil {
		return
	}
	if !canSendDiscoveryToUser(u.ID, now) {
		return
	}
	locale := userDiscoveryLocale(u.ID)
	choice, _ := pickDiscoveryTargetForProfile(profileFromUser(u), now)
	if choice.sale == nil && choice.lm == nil {
		return
	}

	if choice.sale != nil {
		sidCheck := choice.sale.ID
		if discoveryRecentlySentForListing(&u.ID, "", &sidCheck, nil, now) {
			return
		}
		t, b := buildSaleDiscoveryCopy(locale, choice.sale, rand.Intn(4))
		sid := choice.sale.ID
		return t, b, firstImageURL(choice.sale.Images), saleDeepLinkData(choice.sale.ID), &sid, nil, true
	}

	if choice.lm != nil {
		if discoveryRecentlySentForListing(&u.ID, "", nil, &choice.lm.ID, now) {
			return
		}
		t, b := buildLandmarkDiscoveryCopy(locale, choice.lm, rand.Intn(4))
		lid := choice.lm.ID
		return t, b, landmarkFirstImageURL(choice.lm), landmarkDeepLinkData(choice.lm.ID), nil, &lid, true
	}
	return
}

// --- profiles ---

type discoveryProfile struct {
	favCityID   *uint
	investInter bool
}

func profileFromAnonymousPref(pref *models.AnonymousUserPreference) discoveryProfile {
	if pref == nil || pref.ID == 0 {
		return discoveryProfile{}
	}
	invest := false
	for _, it := range pref.Interests {
		if strings.EqualFold(strings.TrimSpace(it), "investment") {
			invest = true
			break
		}
	}
	return discoveryProfile{favCityID: pref.FavoriteCityID, investInter: invest}
}

func profileFromUser(u *models.User) discoveryProfile {
	if u == nil {
		return discoveryProfile{}
	}
	return discoveryProfile{favCityID: u.FavoriteCityID, investInter: false}
}

// --- picking ---

type discoveryChoice struct {
	sale *models.PropertySale
	lm   *models.Landmark
}

func pickDiscoveryTargetForProfile(p discoveryProfile, now time.Time) (discoveryChoice, string) {
	return pickDiscoveryTargetDepth(p, now, 0)
}

func pickDiscoveryTargetDepth(p discoveryProfile, now time.Time, depth int) (discoveryChoice, string) {
	if depth > 1 {
		return discoveryChoice{}, ""
	}
	// DISCOVERY_AI_RANK=1 reserved for future MeskenyGPT-assisted re-ranking of candidates.
	_ = os.Getenv("DISCOVERY_AI_RANK")

	var sales []models.PropertySale
	q := storage.DB.Where("status = ? AND is_published = ? AND is_deactivated = ? AND is_sold = ?", "published", true, false, false).
		Order("created_at DESC").
		Limit(40)
	if p.favCityID != nil {
		q = q.Where("city_id = ?", *p.favCityID)
	}
	_ = q.Find(&sales).Error

	var landmarks []models.Landmark
	lq := storage.DB.Where("is_published = ? AND is_verified = ? AND status = ?", true, true, "verified").
		Order("created_at DESC").
		Limit(24)
	_ = lq.Find(&landmarks).Error

	shuffleSales := func(list []models.PropertySale) {
		for i := range list {
			j := rand.Intn(i + 1)
			list[i], list[j] = list[j], list[i]
		}
	}
	shuffleLms := func(list []models.Landmark) {
		for i := range list {
			j := rand.Intn(i + 1)
			list[i], list[j] = list[j], list[i]
		}
	}
	shuffleSales(sales)
	shuffleLms(landmarks)

	bestSale := scorePickSale(sales, p, now)
	bestLm := scorePickLandmark(landmarks, p, now)

	if bestSale == nil && bestLm == nil {
		if p.favCityID != nil {
			wide := discoveryProfile{investInter: p.investInter, favCityID: nil}
			return pickDiscoveryTargetDepth(wide, now, depth+1)
		}
		return discoveryChoice{}, ""
	}

	// Random mix: prefer sale 65% when both exist, unless user is investment-focused and we have a flagged sale.
	if bestSale != nil && bestLm != nil {
		if bestSale.IsGold && rand.Intn(100) < 72 {
			return discoveryChoice{sale: bestSale}, "sale"
		}
		if p.investInter && bestSale.IsInvestmentOpportunity && rand.Intn(100) < 70 {
			return discoveryChoice{sale: bestSale}, "sale"
		}
		if rand.Intn(100) < 65 {
			return discoveryChoice{sale: bestSale}, "sale"
		}
		return discoveryChoice{lm: bestLm}, "landmark"
	}
	if bestSale != nil {
		return discoveryChoice{sale: bestSale}, "sale"
	}
	return discoveryChoice{lm: bestLm}, "landmark"
}

func scorePickSale(candidates []models.PropertySale, p discoveryProfile, now time.Time) *models.PropertySale {
	var best *models.PropertySale
	bestScore := -1.0
	for i := range candidates {
		s := &candidates[i]
		sc := 1.0
		if len(s.PaperTypes) > 0 {
			sc += 2.0 + 0.2*float64(minInt(len(s.PaperTypes), 5))
		}
		if s.IsInvestmentOpportunity {
			sc += 2.5
			if p.investInter {
				sc += 2.0
			}
		}
		if s.IsFeatured {
			sc += 0.8
		}
		if s.IsGold {
			sc += 3.2
		}
		if now.Sub(s.CreatedAt) < 72*time.Hour {
			sc += 1.2
		}
		if len(s.Images) > 0 {
			sc += 0.5
		}
		if sc > bestScore {
			bestScore = sc
			best = s
		}
	}
	return best
}

func scorePickLandmark(candidates []models.Landmark, p discoveryProfile, now time.Time) *models.Landmark {
	var best *models.Landmark
	bestScore := -1.0
	for i := range candidates {
		l := &candidates[i]
		sc := 1.0
		if len(l.PaperTypes) > 0 {
			sc += 2.2 + 0.25*float64(minInt(len(l.PaperTypes), 5))
		}
		if l.IsInvestmentOpportunity {
			sc += 2.0
			if p.investInter {
				sc += 1.5
			}
		}
		if now.Sub(l.CreatedAt) < 96*time.Hour {
			sc += 1.0
		}
		if landmarkFirstImageURL(l) != "" {
			sc += 0.5
		}
		if sc > bestScore {
			bestScore = sc
			best = l
		}
	}
	return best
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// --- rate limits ---

func canSendDiscoveryToDevice(deviceID string, now time.Time) bool {
	if deviceID == "" {
		return false
	}
	var pref models.MarketingDevice
	if err := storage.DB.Where("device_id = ?", deviceID).First(&pref).Error; err != nil {
		return false
	}
	if quietDiscoveryForMarketingDevice(&pref, now) {
		return false
	}

	var n int64
	_ = storage.DB.Model(&models.DiscoveryEngagementLog{}).
		Where("device_id = ? AND created_at >= ?", deviceID, now.Add(-24*time.Hour)).
		Count(&n).Error
	if n >= 2 {
		return false
	}

	var last models.DiscoveryEngagementLog
	if err := storage.DB.Where("device_id = ?", deviceID).Order("created_at DESC").First(&last).Error; err == nil {
		if now.Sub(last.CreatedAt) < 8*time.Hour {
			return false
		}
	}

	// Cross-channel cooldown: if this marketing device is linked to a user account,
	// don't stack discovery with any smart_* notification inside the same 30 minutes.
	if pref.UserID != nil && *pref.UserID > 0 {
		var lastSmart models.NotificationDeliveryLog
		if err := storage.DB.Where("user_id = ? AND event_type LIKE ?", *pref.UserID, "smart_%").
			Order("created_at DESC").First(&lastSmart).Error; err == nil {
			if now.Sub(lastSmart.CreatedAt) < crossChannelMinGap {
				return false
			}
		}
	}
	return true
}

func quietDiscoveryForMarketingDevice(d *models.MarketingDevice, now time.Time) bool {
	if d == nil {
		return false
	}
	quietStart, quietEnd := 22, 7
	local := now
	if strings.TrimSpace(d.Timezone) != "" {
		if loc, err := time.LoadLocation(strings.TrimSpace(d.Timezone)); err == nil && loc != nil {
			local = now.In(loc)
		}
	}
	h := local.Hour()
	return h >= quietStart || h < quietEnd
}

func canSendDiscoveryToUser(userID uint, now time.Time) bool {
	var pref models.NotificationPreference
	_ = storage.DB.Where("user_id = ? AND enabled = ?", userID, true).Order("updated_at DESC").First(&pref).Error

	quietStart, quietEnd := 22, 7
	if pref.QuietStartHour >= 0 && pref.QuietStartHour <= 23 {
		quietStart = pref.QuietStartHour
	}
	if pref.QuietEndHour >= 0 && pref.QuietEndHour <= 23 {
		quietEnd = pref.QuietEndHour
	}
	local := now
	if strings.TrimSpace(pref.Timezone) != "" {
		if loc, err := time.LoadLocation(strings.TrimSpace(pref.Timezone)); err == nil && loc != nil {
			local = now.In(loc)
		}
	}
	h := local.Hour()
	if h >= quietStart || h < quietEnd {
		return false
	}

	var n int64
	_ = storage.DB.Model(&models.DiscoveryEngagementLog{}).
		Where("user_id = ? AND created_at >= ?", userID, now.Add(-24*time.Hour)).
		Count(&n).Error
	if n >= 2 {
		return false
	}

	var last models.DiscoveryEngagementLog
	if err := storage.DB.Where("user_id = ?", userID).Order("created_at DESC").First(&last).Error; err == nil {
		if now.Sub(last.CreatedAt) < 6*time.Hour {
			return false
		}
	}

	// Avoid stacking with generic smart push in the same hour.
	var lastSmart models.NotificationDeliveryLog
	if err := storage.DB.Where("user_id = ? AND event_type LIKE ?", userID, "smart_%").
		Order("created_at DESC").First(&lastSmart).Error; err == nil {
		if now.Sub(lastSmart.CreatedAt) < crossChannelMinGap {
			return false
		}
	}

	return true
}

func discoveryRecentlySentForListing(userID *uint, deviceID string, propertySaleID *uint, landmarkID *uint, now time.Time) bool {
	cut := now.Add(-72 * time.Hour)
	q := storage.DB.Model(&models.DiscoveryEngagementLog{}).Where("created_at >= ?", cut)
	if userID != nil {
		q = q.Where("user_id = ?", *userID)
	} else {
		q = q.Where("device_id = ?", deviceID)
	}
	if landmarkID != nil {
		q = q.Where("landmark_id = ?", *landmarkID)
	} else if propertySaleID != nil {
		q = q.Where("property_sale_id = ?", *propertySaleID)
	} else {
		return false
	}
	var c int64
	_ = q.Count(&c).Error
	if c > 0 {
		return true
	}

	// Cross-event dedupe: if the same sale was already sent by any smart_* flow,
	// block rediscovery for a longer window to avoid repetitive campaigns.
	if userID != nil && propertySaleID != nil {
		var cnt int64
		_ = storage.DB.Model(&models.NotificationDeliveryLog{}).
			Where("user_id = ? AND property_sale_id = ? AND created_at >= ?", *userID, *propertySaleID, now.Add(-repeatListingBlockFor)).
			Count(&cnt).Error
		if cnt > 0 {
			return true
		}
	}
	return false
}

// LogDiscoverySend records a delivered discovery spotlight for dedupe and caps.
func LogDiscoverySend(userID *uint, deviceID string, saleID *uint, landmarkID *uint) error {
	entry := models.DiscoveryEngagementLog{
		UserID:         userID,
		DeviceID:       strings.TrimSpace(deviceID),
		EventType:      discoveryEventSpotlight,
		PropertySaleID: saleID,
		LandmarkID:     landmarkID,
	}
	return storage.DB.Create(&entry).Error
}

// --- copy + i18n ---

func normalizeDiscoveryLocale(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case strings.HasPrefix(s, "fr"):
		return "fr"
	case strings.HasPrefix(s, "ar"):
		return "ar"
	default:
		return "en"
	}
}

func userDiscoveryLocale(userID uint) string {
	var pref models.NotificationPreference
	if err := storage.DB.Where("user_id = ?", userID).Order("updated_at DESC").First(&pref).Error; err != nil {
		return "en"
	}
	return normalizeDiscoveryLocale(pref.Language)
}

func paperLabel(locale, key string) string {
	k := strings.ToLower(strings.TrimSpace(key))
	aliases := map[string]string{
		"titre foncier": "titre_foncier",
		"titre_foncier": "titre_foncier",
		"quitane":       "quitane",
		"quittance":     "quitane",
		"lettre":        "lettre",
		"concession":    "concession",
		"bornage":       "bornage",
	}
	canonical, ok := aliases[k]
	if !ok {
		return strings.TrimSpace(key)
	}
	m := map[string]map[string]string{
		"titre_foncier": {"en": "land title (titre foncier)", "fr": "titre foncier", "ar": "تيتر فونصي"},
		"quitane":       {"en": "tax receipt (quitance)", "fr": "quittance", "ar": "وصل أداء الضريبة"},
		"lettre":        {"en": "administrative letter", "fr": "lettre administrative", "ar": "رسالة إدارية"},
		"concession":    {"en": "concession deed", "fr": "concession", "ar": "امتياز"},
		"bornage":       {"en": "boundary survey (bornage)", "fr": "bornage", "ar": "تحديد حدود"},
	}
	if row, ok2 := m[canonical]; ok2 {
		if v, ok3 := row[locale]; ok3 {
			return v
		}
		return row["en"]
	}
	return canonical
}

func formatPaperList(locale string, papers []string) string {
	if len(papers) == 0 {
		return ""
	}
	labels := make([]string, 0, len(papers))
	for _, p := range papers {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		labels = append(labels, paperLabel(locale, p))
	}
	if len(labels) == 0 {
		return ""
	}
	if len(labels) == 1 {
		return labels[0]
	}
	if locale == "ar" {
		return strings.Join(labels[:len(labels)-1], "، ") + " و" + labels[len(labels)-1]
	}
	if locale == "fr" {
		return strings.Join(labels[:len(labels)-1], ", ") + " et " + labels[len(labels)-1]
	}
	return strings.Join(labels[:len(labels)-1], ", ") + " and " + labels[len(labels)-1]
}

func buildSaleDiscoveryCopy(locale string, s *models.PropertySale, variant int) (string, string) {
	papers := formatPaperList(locale, s.PaperTypes)
	shortTitle := trimRunes(s.Title, 42)

	invest := ""
	if s.IsInvestmentOpportunity {
		switch locale {
		case "fr":
			invest = " — opportunité d’investissement signalée par l’équipe."
		case "ar":
			invest = " — فرصة استثمارية بإشارة من المنصة."
		default:
			invest = " — flagged as a strong investment opportunity."
		}
	}

	paperClause := ""
	if papers != "" {
		switch locale {
		case "fr":
			paperClause = fmt.Sprintf(" Documents déclarés : %s.", papers)
		case "ar":
			paperClause = fmt.Sprintf(" وثائق: %s.", papers)
		default:
			paperClause = fmt.Sprintf(" Declared papers include %s.", papers)
		}
	}

	v := variant % 4
	var title, body string
	switch locale {
	case "fr":
		switch v {
		case 0:
			title = "Nouveau bien sur Meskeny"
			body = fmt.Sprintf("Découvrez « %s ».%s%s Ouvrez l’app pour photos, prix et lieu.", shortTitle, paperClause, invest)
		case 1:
			title = "Documents déclarés"
			if papers == "" {
				body = fmt.Sprintf("Voir « %s » sur Meskeny.%s", shortTitle, invest)
			} else {
				body = fmt.Sprintf("Ce bien indique : %s — ouvrez « %s ».%s", papers, shortTitle, invest)
			}
		case 2:
			title = "Sélection du jour"
			body = fmt.Sprintf("« %s ».%s%s Touchez pour explorer.", shortTitle, paperClause, invest)
		default:
			title = "Coup de projecteur"
			body = fmt.Sprintf("« %s ».%s%s Fiche complète dans l’app Meskeny.", shortTitle, paperClause, invest)
		}
	case "ar":
		switch v {
		case 0:
			title = "عقار جديد على مسكني"
			body = fmt.Sprintf("شاهد «%s».%s%s افتح التطبيق للصور والسعر والموقع.", shortTitle, paperClause, invest)
		case 1:
			title = "أوراق معلنة"
			if papers == "" {
				body = fmt.Sprintf("راجع «%s» على مسكني.%s", shortTitle, invest)
			} else {
				body = fmt.Sprintf("هذا الإعلان يذكر: %s — «%s».%s", papers, shortTitle, invest)
			}
		case 2:
			title = "اختيار اليوم"
			body = fmt.Sprintf("«%s».%s%s اضغط للتفاصيل.", shortTitle, paperClause, invest)
		default:
			title = "فرصة مميزة"
			body = fmt.Sprintf("«%s».%s%s التفاصيل الكاملة داخل التطبيق.", shortTitle, paperClause, invest)
		}
	default:
		switch v {
		case 0:
			title = "New home on Meskeny"
			body = fmt.Sprintf("Check out “%s”.%s%s Open the app for photos, price, and location.", shortTitle, paperClause, invest)
		case 1:
			title = "Papers you can ask for"
			if papers == "" {
				body = fmt.Sprintf("Open “%s” on Meskeny.%s", shortTitle, invest)
			} else {
				body = fmt.Sprintf("This listing highlights: %s — see “%s”.%s", papers, shortTitle, invest)
			}
		case 2:
			title = "Fresh property pick"
			body = fmt.Sprintf("We picked “%s” for you.%s%s Tap to explore.", shortTitle, paperClause, invest)
		default:
			title = "Deal spotlight"
			body = fmt.Sprintf("“%s”.%s%s Rich details inside Meskeny.", shortTitle, paperClause, invest)
		}
	}
	return title, trimRunes(body, 178)
}

func buildLandmarkDiscoveryCopy(locale string, l *models.Landmark, variant int) (string, string) {
	papers := formatPaperList(locale, l.PaperTypes)
	shortTitle := trimRunes(l.Title, 42)

	invest := ""
	if l.IsInvestmentOpportunity {
		switch locale {
		case "fr":
			invest = " — terrain mis en avant comme opportunité."
		case "ar":
			invest = " — أرض مميزة كفرصة."
		default:
			invest = " — highlighted as an investment-grade land deal."
		}
	}

	paperClause := ""
	if papers != "" {
		switch locale {
		case "fr":
			paperClause = fmt.Sprintf(" Documents déclarés : %s.", papers)
		case "ar":
			paperClause = fmt.Sprintf(" وثائق: %s.", papers)
		default:
			paperClause = fmt.Sprintf(" Declared papers include %s.", papers)
		}
	}

	v := variant % 4
	var title, body string
	switch locale {
	case "fr":
		switch v {
		case 0:
			title = "Terrain sur Meskeny"
			body = fmt.Sprintf("Explorez « %s ».%s%s Carte, documents et prix dans l’app.", shortTitle, paperClause, invest)
		case 1:
			title = "Terrain — papiers"
			if papers == "" {
				body = fmt.Sprintf("Voir « %s ».%s", shortTitle, invest)
			} else {
				body = fmt.Sprintf("Documents déclarés : %s — « %s ».%s", papers, shortTitle, invest)
			}
		case 2:
			title = "Parcelle du jour"
			body = fmt.Sprintf("« %s ».%s%s Touchez pour ouvrir la fiche.", shortTitle, paperClause, invest)
		default:
			title = "Opportunité foncière"
			body = fmt.Sprintf("« %s ».%s%s Transparence Meskeny.", shortTitle, paperClause, invest)
		}
	case "ar":
		switch v {
		case 0:
			title = "قطعة أرض على مسكني"
			body = fmt.Sprintf("استكشف «%s».%s%s الخريطة والوثائق والسعر في التطبيق.", shortTitle, paperClause, invest)
		case 1:
			title = "أرض — وثائق"
			if papers == "" {
				body = fmt.Sprintf("شاهد «%s».%s", shortTitle, invest)
			} else {
				body = fmt.Sprintf("وثائق معلنة: %s — «%s».%s", papers, shortTitle, invest)
			}
		case 2:
			title = "أرض مختارة"
			body = fmt.Sprintf("«%s».%s%s اضغط للفتح.", shortTitle, paperClause, invest)
		default:
			title = "فرصة أرضية"
			body = fmt.Sprintf("«%s».%s%s خرائط وشفافية على مسكني.", shortTitle, paperClause, invest)
		}
	default:
		switch v {
		case 0:
			title = "Land parcel on Meskeny"
			body = fmt.Sprintf("Explore “%s”.%s%s Full map, papers, and price in the app.", shortTitle, paperClause, invest)
		case 1:
			title = "Land — documents"
			if papers == "" {
				body = fmt.Sprintf("See “%s”.%s", shortTitle, invest)
			} else {
				body = fmt.Sprintf("Declared papers: %s — open “%s”.%s", papers, shortTitle, invest)
			}
		case 2:
			title = "Plot spotlight"
			body = fmt.Sprintf("“%s”.%s%s Tap to open the land sheet.", shortTitle, paperClause, invest)
		default:
			title = "New land opportunity"
			body = fmt.Sprintf("“%s”.%s%s Meskeny maps + transparency.", shortTitle, paperClause, invest)
		}
	}
	return title, trimRunes(body, 178)
}

func trimRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	// byte trim is ok for push; avoid importing utf8 heavy paths
	if len(s) > max {
		return s[:max-1] + "…"
	}
	return s
}

func saleDeepLinkData(id uint) map[string]string {
	return map[string]string{
		"type":       "discovery_spotlight",
		"id":         fmt.Sprintf("%d", id),
		"propertyId": fmt.Sprintf("%d", id),
		"screen":     "PropertySaleDetails",
		"params":     fmt.Sprintf(`{"propertyId": %d}`, id),
		"action":     "open_property_sale",
	}
}

func landmarkDeepLinkData(id uint) map[string]string {
	return map[string]string{
		"type":       "discovery_landmark",
		"id":         fmt.Sprintf("%d", id),
		"landmarkId": fmt.Sprintf("%d", id),
		"screen":     "LandmarkDetails",
		"params":     fmt.Sprintf(`{"landmarkId": %d}`, id),
		"action":     "open_landmark",
	}
}

func firstImageURL(images []string) string {
	if len(images) == 0 {
		return ""
	}
	return strings.TrimSpace(images[0])
}

func landmarkFirstImageURL(l *models.Landmark) string {
	if l == nil || len(l.Images) == 0 {
		return ""
	}
	var imgs []string
	if err := json.Unmarshal(l.Images, &imgs); err != nil {
		return ""
	}
	return firstImageURL(imgs)
}
