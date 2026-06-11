package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/services"
	"apartments-clone-server/storage"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kataras/iris/v12"
	"gorm.io/datatypes"
)

type hostSuggestionItem struct {
	MatchID    uint     `json:"match_id"`
	UserID     uint     `json:"user_id"`
	Name       string   `json:"name"` // minimal label (first name only)
	MatchScore float64  `json:"match_score"`
	MatchTier  string   `json:"match_tier"`
	Reasons    []string `json:"reasons"`
	Engagement string   `json:"engagement_level,omitempty"`
}

var hostSuggestionRefreshLocks sync.Map
var hostSuggestionLastRefresh sync.Map
var hostSuggestionRefreshSem = make(chan struct{}, 2)

const (
	summaryMaxAgeForMatching = 48 * time.Hour
	enrichMemoryCacheTTL    = 5 * time.Minute
)

var (
	enrichUserMutex sync.Map // uint -> *sync.Mutex
	enrichMemCache  sync.Map // uint -> enrichMemEntry
	// If partial unique index isn't present yet, stop retrying ON CONFLICT batch path.
	batchUpsertDisabled atomic.Bool
)

type enrichMemEntry struct {
	user models.AIEnrichedUser
	at   time.Time
}

type scoredMatchRow struct {
	userID  uint
	score   float64
	tier    string
	reasons []string
}

func enrichMutexFor(uid uint) *sync.Mutex {
	v, _ := enrichUserMutex.LoadOrStore(uid, &sync.Mutex{})
	return v.(*sync.Mutex)
}

func GetHostSuggestions(ctx iris.Context) {
	hostID, ok := ctx.Values().Get("userID").(uint)
	if !ok || hostID == 0 {
		ctx.StopWithJSON(http.StatusUnauthorized, iris.Map{"error": "unauthorized"})
		return
	}

	propertyIDInt, err := ctx.URLParamInt("property_id")
	propertyID := uint(propertyIDInt)
	if err != nil || propertyID == 0 {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "property_id is required"})
		return
	}

	var property models.PropertySale
	if err := storage.DB.First(&property, propertyID).Error; err != nil {
		ctx.StopWithJSON(http.StatusNotFound, iris.Map{"error": "property not found"})
		return
	}
	if !ownsPropertySale(hostID, property) {
		ctx.StopWithJSON(http.StatusForbidden, iris.Map{"error": "you do not own this property"})
		return
	}

	refreshNow := strings.EqualFold(strings.TrimSpace(ctx.URLParamDefault("refresh", "0")), "1")
	cached, hasCached := getRecentSuggestionsFromMatches(propertyID, hostID, 20*time.Minute)

	// GET stays read-mostly: return stored rows immediately.
	if hasCached && !refreshNow {
		ctx.JSON(iris.Map{
			"success":       true,
			"property_id":   propertyID,
			"suggestions":   cached,
			"total_matches": len(cached),
			"cached":        true,
			"refreshing":    false,
		})
		return
	}
	scheduled := scheduleHostSuggestionRefresh(property, hostID)
	ctx.JSON(iris.Map{
		"success":       true,
		"property_id":   propertyID,
		"suggestions":   cached,
		"total_matches": len(cached),
		"cached":        true,
		"refreshing":    scheduled,
	})
}

func scheduleHostSuggestionRefresh(property models.PropertySale, hostID uint) bool {
	key := property.ID

	if lastRaw, ok := hostSuggestionLastRefresh.Load(key); ok {
		if last, ok2 := lastRaw.(time.Time); ok2 && time.Since(last) < 10*time.Minute {
			return false
		}
	}
	if _, loaded := hostSuggestionRefreshLocks.LoadOrStore(key, struct{}{}); loaded {
		return false
	}
	go func(p models.PropertySale, h uint) {
		defer hostSuggestionRefreshLocks.Delete(key)
		hostSuggestionRefreshSem <- struct{}{}
		defer func() { <-hostSuggestionRefreshSem }()
		if _, err := rebuildHostSuggestions(p, h); err != nil {
			log.Printf("host_suggestions: async refresh failed property=%d host=%d err=%v", p.ID, h, err)
			return
		}
		hostSuggestionLastRefresh.Store(key, time.Now())
	}(property, hostID)
	return true
}

func rebuildHostSuggestions(property models.PropertySale, hostID uint) (int, error) {
	dna, err := upsertPropertyDNA(property, hostID)
	if err != nil {
		return 0, err
	}
	candidateIDs, err := candidateUserIDs(hostID)
	if err != nil {
		return 0, err
	}
	if len(candidateIDs) == 0 {
		return 0, nil
	}

	sharePrefs, err := services.LoadSharePrefs(candidateIDs)
	if err != nil {
		return 0, err
	}

	var summaries []models.UserBehaviorSummary
	_ = storage.DB.Where("user_id IN ?", candidateIDs).Find(&summaries).Error
	sumByUser := make(map[uint]*models.UserBehaviorSummary, len(summaries))
	for i := range summaries {
		sumByUser[summaries[i].UserID] = &summaries[i]
	}

	toScore := make([]scoredMatchRow, 0, len(candidateIDs))
	for _, uid := range candidateIDs {
		pref, ok := sharePrefs[uid]
		if !ok || !services.UserConsentedToHostShare(&pref) {
			continue
		}
		if services.UserLockedToAnotherHost(&pref, hostID) {
			continue
		}
		enriched, err := enrichedProfileForHostMatching(uid, sumByUser[uid])
		if err != nil {
			continue
		}
		score, reasons := computeMatchScore(dna, enriched)
		if score < 60 {
			continue
		}
		tier := scoreTier(score)
		toScore = append(toScore, scoredMatchRow{userID: uid, score: score, tier: tier, reasons: reasons})
	}

	sort.Slice(toScore, func(i, j int) bool { return toScore[i].score > toScore[j].score })
	if len(toScore) > services.MaxPendingBuyerMatchesPerProperty {
		toScore = toScore[:services.MaxPendingBuyerMatchesPerProperty]
	}

	batchUpsertPropertyMatchesBestEffort(property.ID, hostID, toScore)
	_ = services.TrimPropertyPendingMatches(property.ID, hostID, services.MaxPendingBuyerMatchesPerProperty)
	for _, r := range toScore {
		_ = services.TryLockBuyerToHost(r.userID, hostID)
	}
	return len(toScore), nil
}

func getRecentSuggestionsFromMatches(propertyID, hostID uint, maxAge time.Duration) ([]hostSuggestionItem, bool) {
	var matches []models.PropertyMatch
	if err := storage.DB.
		Where("property_id = ? AND host_id = ? AND status = ?", propertyID, hostID, "pending").
		Order("updated_at DESC").
		Limit(120).
		Find(&matches).Error; err != nil || len(matches) == 0 {
		return nil, false
	}

	// If newest record is stale, don't trust cache.
	if time.Since(matches[0].UpdatedAt) > maxAge {
		return nil, false
	}

	userIDs := make([]uint, 0, len(matches))
	seenUsers := map[uint]struct{}{}
	for _, m := range matches {
		if _, ok := seenUsers[m.SuggestedUserID]; ok {
			continue
		}
		seenUsers[m.SuggestedUserID] = struct{}{}
		userIDs = append(userIDs, m.SuggestedUserID)
	}

	sharePrefs, err := services.LoadSharePrefs(userIDs)
	if err != nil {
		return nil, false
	}

	var enrichedRows []models.AIEnrichedUser
	_ = storage.DB.Where("user_id IN ?", userIDs).Find(&enrichedRows).Error
	enrichedMap := make(map[uint]models.AIEnrichedUser, len(enrichedRows))
	for _, e := range enrichedRows {
		enrichedMap[e.UserID] = e
	}

	out := make([]hostSuggestionItem, 0, len(matches))
	for _, m := range matches {
		u, ok := sharePrefs[m.SuggestedUserID]
		if !ok || !services.UserConsentedToHostShare(&u) {
			continue
		}
		if services.UserLockedToAnotherHost(&u, hostID) {
			continue
		}
		var reasons []string
		_ = json.Unmarshal(m.MatchReasons, &reasons)
		e := enrichedMap[m.SuggestedUserID]
		out = append(out, hostSuggestionItem{
			MatchID:    m.ID,
			UserID:     m.SuggestedUserID,
			Name:       services.MinimalBuyerLabel(u),
			MatchScore: m.MatchScore,
			MatchTier:  m.MatchTier,
			Reasons:    reasons,
			Engagement: e.EngagementLevel,
		})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].MatchScore > out[j].MatchScore })
	if len(out) > services.MaxPendingBuyerMatchesPerProperty {
		out = out[:services.MaxPendingBuyerMatchesPerProperty]
	}
	return out, len(out) > 0
}

func ContactHostSuggestion(ctx iris.Context) {
	hostID, ok := ctx.Values().Get("userID").(uint)
	if !ok || hostID == 0 {
		ctx.StopWithJSON(http.StatusUnauthorized, iris.Map{"error": "unauthorized"})
		return
	}
	matchID, err := ctx.Params().GetUint("match_id")
	if err != nil || matchID == 0 {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid match id"})
		return
	}

	var match models.PropertyMatch
	if err := storage.DB.First(&match, matchID).Error; err != nil {
		ctx.StopWithJSON(http.StatusNotFound, iris.Map{"error": "match not found"})
		return
	}
	if match.HostID != hostID {
		ctx.StopWithJSON(http.StatusForbidden, iris.Map{"error": "forbidden"})
		return
	}

	var buyer models.User
	if err := storage.DB.Select(
		"id", "share_profile_with_hosts", "host_share_locked_host_id",
	).First(&buyer, match.SuggestedUserID).Error; err != nil {
		ctx.StopWithJSON(http.StatusNotFound, iris.Map{"error": "buyer not found"})
		return
	}
	if !services.UserConsentedToHostShare(&buyer) {
		ctx.StopWithJSON(http.StatusForbidden, iris.Map{"error": "buyer has not consented to host sharing"})
		return
	}
	if services.UserLockedToAnotherHost(&buyer, hostID) {
		ctx.StopWithJSON(http.StatusForbidden, iris.Map{"error": "buyer is already shared with another host"})
		return
	}
	_ = services.TryLockBuyerToHost(match.SuggestedUserID, hostID)

	var body struct {
		InitialMessage string `json:"initial_message"`
	}
	_ = ctx.ReadJSON(&body)
	initial := strings.TrimSpace(body.InitialMessage)
	recipientLang := resolveRecipientLanguage(match.SuggestedUserID)
	if initial == "" {
		initial = localizedHostSuggestionIntro(recipientLang)
	} else {
		// Host typed custom text: translate to recipient language when possible.
		if recipientLang != "" {
			if translated, err := services.TranslateOnceDirect(initial, recipientLang); err == nil && strings.TrimSpace(translated) != "" {
				initial = strings.TrimSpace(translated)
			}
		}
	}

	var listing models.PropertySale
	if err := storage.DB.First(&listing, match.PropertyID).Error; err != nil {
		ctx.StopWithJSON(http.StatusNotFound, iris.Map{"error": "property not found"})
		return
	}
	imgURL := ""
	if len(listing.Images) > 0 {
		imgURL = strings.TrimSpace(listing.Images[0])
	}
	card := struct {
		Caption      string  `json:"caption"`
		PropertyID   uint    `json:"property_id"`
		PropertyType string  `json:"property_type"`
		Title        string  `json:"title"`
		ListingPrice float64 `json:"listing_price"`
		Currency     string  `json:"currency"`
		ImageURL     string  `json:"image_url"`
	}{
		Caption:      initial,
		PropertyID:   listing.ID,
		PropertyType: "sale",
		Title:        listing.Title,
		ListingPrice: listing.ListingPrice,
		Currency:     listing.Currency,
		ImageURL:     imgURL,
	}
	cardJSON, err := json.Marshal(card)
	if err != nil {
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to build message"})
		return
	}

	pid := match.PropertyID
	message := models.DirectMessage{
		SenderID:   hostID,
		ReceiverID: match.SuggestedUserID,
		Content:    string(cardJSON),
		Type:       "property_card",
		RefType:    "property_sale",
		RefID:      &pid,
	}
	if err := storage.DB.Create(&message).Error; err != nil {
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to create conversation"})
		return
	}

	nowStatus := "contacted"
	if err := storage.DB.Model(&match).Updates(map[string]any{
		"status":          nowStatus,
		"conversation_id": message.ID,
	}).Error; err != nil {
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to update match"})
		return
	}

	ctx.JSON(iris.Map{
		"success":         true,
		"conversation_id": message.ID,
		"match_id":        match.ID,
		"status":          nowStatus,
	})
}

func resolveRecipientLanguage(userID uint) string {
	var u models.User
	if err := storage.DB.Select("id", "languages").First(&u, userID).Error; err != nil {
		return "fr"
	}
	var langs []string
	_ = json.Unmarshal(u.Languages, &langs)
	for _, raw := range langs {
		lang := strings.ToLower(strings.TrimSpace(raw))
		switch {
		case strings.HasPrefix(lang, "ar"):
			return "ar"
		case strings.HasPrefix(lang, "fr"):
			return "fr"
		case strings.HasPrefix(lang, "en"):
			return "en"
		}
	}
	return "fr"
}

func localizedHostSuggestionIntro(lang string) string {
	switch lang {
	case "ar":
		return "مرحبا، ملفك مناسب لهذا العقار. هل ترغب في مناقشته؟"
	case "en":
		return "Hi, your profile matches this property. Would you like to discuss it?"
	default:
		return "Bonjour, votre profil correspond a ce bien. Souhaitez-vous en discuter ?"
	}
}

func GetHostSuggestionPendingCount(ctx iris.Context) {
	hostID, ok := ctx.Values().Get("userID").(uint)
	if !ok || hostID == 0 {
		ctx.StopWithJSON(http.StatusUnauthorized, iris.Map{"error": "unauthorized"})
		return
	}
	var count int64
	if err := storage.DB.Model(&models.PropertyMatch{}).
		Where("host_id = ? AND status = ?", hostID, "pending").
		Count(&count).Error; err != nil {
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to count suggestions"})
		return
	}
	ctx.JSON(iris.Map{
		"success":       true,
		"pending_count": count,
	})
}

func DismissHostSuggestion(ctx iris.Context) {
	hostID, ok := ctx.Values().Get("userID").(uint)
	if !ok || hostID == 0 {
		ctx.StopWithJSON(http.StatusUnauthorized, iris.Map{"error": "unauthorized"})
		return
	}
	matchID, err := ctx.Params().GetUint("match_id")
	if err != nil || matchID == 0 {
		ctx.StopWithJSON(http.StatusBadRequest, iris.Map{"error": "invalid match id"})
		return
	}

	var match models.PropertyMatch
	if err := storage.DB.First(&match, matchID).Error; err != nil {
		ctx.StopWithJSON(http.StatusNotFound, iris.Map{"error": "match not found"})
		return
	}
	if match.HostID != hostID {
		ctx.StopWithJSON(http.StatusForbidden, iris.Map{"error": "forbidden"})
		return
	}

	if err := storage.DB.Model(&match).Update("status", "dismissed").Error; err != nil {
		ctx.StopWithJSON(http.StatusInternalServerError, iris.Map{"error": "failed to dismiss"})
		return
	}
	ctx.JSON(iris.Map{"success": true, "match_id": match.ID, "status": "dismissed"})
}

func ownsPropertySale(hostID uint, p models.PropertySale) bool {
	if p.OwnerID != nil && *p.OwnerID == hostID {
		return true
	}
	if p.OrganizationID != nil {
		var org models.Organization
		if err := storage.DB.Select("id", "owner_id").First(&org, *p.OrganizationID).Error; err == nil && org.OwnerID == hostID {
			return true
		}
	}
	return false
}

func upsertPropertyDNA(property models.PropertySale, hostID uint) (*models.PropertyDNA, error) {
	tags, personas, tier := inferPropertySignals(property.Title, property.Description, property.ListingPrice)
	tagsJSON, _ := json.Marshal(tags)
	personasJSON, _ := json.Marshal(personas)

	var dna models.PropertyDNA
	err := storage.DB.Where("property_id = ?", property.ID).First(&dna).Error
	if err == nil {
		// Property metadata changes slowly; avoid rewriting on each refresh.
		if time.Since(dna.UpdatedAt) < 6*time.Hour {
			return &dna, nil
		}
		dna.HostID = hostID
		dna.CityID = property.CityID
		dna.ZoneID = property.ZoneID
		dna.PropertyType = strings.ToLower(strings.TrimSpace(property.PropertyType))
		dna.Price = property.ListingPrice
		dna.Bedrooms = property.Bedrooms
		dna.AITags = datatypes.JSON(tagsJSON)
		dna.AIPriceTier = tier
		dna.AIPersonas = datatypes.JSON(personasJSON)
		return &dna, storage.DB.Save(&dna).Error
	}

	dna = models.PropertyDNA{
		PropertyID:   property.ID,
		HostID:       hostID,
		CityID:       property.CityID,
		ZoneID:       property.ZoneID,
		PropertyType: strings.ToLower(strings.TrimSpace(property.PropertyType)),
		Price:        property.ListingPrice,
		Bedrooms:     property.Bedrooms,
		AITags:       datatypes.JSON(tagsJSON),
		AIPriceTier:  tier,
		AIPersonas:   datatypes.JSON(personasJSON),
	}
	return &dna, storage.DB.Create(&dna).Error
}

func candidateUserIDs(hostID uint) ([]uint, error) {
	type row struct{ UserID uint }
	rows := []row{}
	// Only buyers who opted in; exclusive lock must be unset or match this host.
	err := storage.DB.Raw(`
		SELECT ub.user_id
		FROM (
			SELECT DISTINCT user_id
			FROM user_behaviors
			WHERE user_id IS NOT NULL
			  AND user_id <> ?
			  AND property_type = 'sale'
			  AND interaction_type IN ('view','favorite','contact','click')
			  AND timestamp >= NOW() - INTERVAL '180 days'
			  AND deleted_at IS NULL
		) ub
		INNER JOIN users u ON u.id = ub.user_id AND u.deleted_at IS NULL
		  AND u.share_profile_with_hosts = TRUE
		  AND (u.host_share_locked_host_id IS NULL OR u.host_share_locked_host_id = 0 OR u.host_share_locked_host_id = ?)
		LEFT JOIN user_behavior_summary s ON s.user_id = ub.user_id
		ORDER BY
		  (COALESCE(s.favorites_90d,0)*8 + COALESCE(s.views_90d,0) + COALESCE(s.contacts_90d,0)*25) DESC NULLS LAST,
		  ub.user_id DESC
		LIMIT 100
	`, hostID, hostID).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]uint, 0, len(rows))
	for _, r := range rows {
		if r.UserID > 0 {
			out = append(out, r.UserID)
		}
	}
	return out, nil
}

// enrichedProfileForHostMatching uses pre-aggregated user_behavior_summary when fresh to avoid per-user CTE scans.
func enrichedProfileForHostMatching(userID uint, summary *models.UserBehaviorSummary) (*models.AIEnrichedUser, error) {
	if userID == 0 {
		return nil, fmt.Errorf("invalid user id")
	}
	if summary != nil && !summary.LastUpdated.IsZero() && time.Since(summary.LastUpdated) < summaryMaxAgeForMatching {
		return buildAIEnrichedUserFromSummary(summary), nil
	}
	return upsertEnrichedUser(userID)
}

func buildAIEnrichedUserFromSummary(s *models.UserBehaviorSummary) *models.AIEnrichedUser {
	avgPrice := s.AvgPrice180d
	if avgPrice <= 0 {
		avgPrice = 1
	}
	budgetMin := avgPrice * 0.7
	budgetMax := avgPrice * 1.3
	behaviorScore := float64(s.Views90d)*1.0 + float64(s.Favorites90d)*8.0 + float64(s.Contacts90d)*25.0
	engagement := "cold"
	if behaviorScore > 250 {
		engagement = "hot"
	} else if behaviorScore > 100 {
		engagement = "warm"
	}
	urgency := "browsing"
	if s.Contacts90d >= 3 {
		urgency = "ready_to_buy"
	} else if s.Views90d >= 20 {
		urgency = "researching"
	}
	personaTags := []string{}
	if s.Favorites90d >= 5 {
		personaTags = append(personaTags, "investor")
	}
	if avgPrice < 2000000 {
		personaTags = append(personaTags, "price_sensitive")
	}
	if s.Contacts90d > 0 && s.Views90d <= 10 {
		personaTags = append(personaTags, "quick_decider")
	}
	if len(personaTags) == 0 {
		personaTags = append(personaTags, "active_browser")
	}
	tagsJSON, _ := json.Marshal(personaTags)
	return &models.AIEnrichedUser{
		UserID:                s.UserID,
		TopCityID:             s.TopCityID,
		TopZoneID:             s.TopZoneID,
		BehaviorScore:         behaviorScore,
		EngagementLevel:       engagement,
		AIBudgetMin:           budgetMin,
		AIBudgetMax:           budgetMax,
		AIPersonaTags:         datatypes.JSON(tagsJSON),
		AIUrgency:             urgency,
		PreferredPropertyType: "sale",
	}
}

func persistAIEnrichedFromComputed(desired *models.AIEnrichedUser) (*models.AIEnrichedUser, error) {
	var cur models.AIEnrichedUser
	err := storage.DB.Where("user_id = ?", desired.UserID).First(&cur).Error
	if err == nil {
		if time.Since(cur.UpdatedAt) < 6*time.Hour {
			enrichMemCache.Store(desired.UserID, enrichMemEntry{user: cur, at: time.Now()})
			return &cur, nil
		}
		cur.TopCityID = desired.TopCityID
		cur.TopZoneID = desired.TopZoneID
		cur.BehaviorScore = desired.BehaviorScore
		cur.EngagementLevel = desired.EngagementLevel
		cur.AIBudgetMin = desired.AIBudgetMin
		cur.AIBudgetMax = desired.AIBudgetMax
		cur.AIPersonaTags = desired.AIPersonaTags
		cur.AIUrgency = desired.AIUrgency
		cur.PreferredPropertyType = desired.PreferredPropertyType
		if err := storage.DB.Save(&cur).Error; err != nil {
			return nil, err
		}
		enrichMemCache.Store(desired.UserID, enrichMemEntry{user: cur, at: time.Now()})
		return &cur, nil
	}
	created := *desired
	if err := storage.DB.Create(&created).Error; err != nil {
		return nil, err
	}
	enrichMemCache.Store(desired.UserID, enrichMemEntry{user: created, at: time.Now()})
	return &created, nil
}

func upsertEnrichedUser(userID uint) (*models.AIEnrichedUser, error) {
	if userID == 0 {
		return nil, fmt.Errorf("invalid user id")
	}
	if v, ok := enrichMemCache.Load(userID); ok {
		e := v.(enrichMemEntry)
		if time.Since(e.at) < enrichMemoryCacheTTL {
			u := e.user
			return &u, nil
		}
	}
	mu := enrichMutexFor(userID)
	mu.Lock()
	defer mu.Unlock()
	if v, ok := enrichMemCache.Load(userID); ok {
		e := v.(enrichMemEntry)
		if time.Since(e.at) < enrichMemoryCacheTTL {
			u := e.user
			return &u, nil
		}
	}

	var sum models.UserBehaviorSummary
	if err := storage.DB.Where("user_id = ?", userID).First(&sum).Error; err == nil && !sum.LastUpdated.IsZero() && time.Since(sum.LastUpdated) < summaryMaxAgeForMatching {
		u := buildAIEnrichedUserFromSummary(&sum)
		enrichMemCache.Store(userID, enrichMemEntry{user: *u, at: time.Now()})
		return persistAIEnrichedFromComputed(u)
	}

	type behaviorAgg struct {
		TopCityID *uint
		TopZoneID *uint
		Views     int64
		Favorites int64
		Contacts  int64
	}
	var agg behaviorAgg
	_ = storage.DB.Raw(`
		WITH ranked_city AS (
			SELECT city_id, COUNT(*) c
			FROM user_behaviors
			WHERE user_id = ? AND property_type = 'sale' AND city_id IS NOT NULL AND deleted_at IS NULL
			GROUP BY city_id ORDER BY c DESC LIMIT 1
		), ranked_zone AS (
			SELECT zone_id, COUNT(*) c
			FROM user_behaviors
			WHERE user_id = ? AND property_type = 'sale' AND zone_id IS NOT NULL AND deleted_at IS NULL
			GROUP BY zone_id ORDER BY c DESC LIMIT 1
		)
		SELECT
			(SELECT city_id FROM ranked_city) AS top_city_id,
			(SELECT zone_id FROM ranked_zone) AS top_zone_id,
			COALESCE(SUM(CASE WHEN interaction_type = 'view' THEN 1 ELSE 0 END),0) AS views,
			COALESCE(SUM(CASE WHEN interaction_type = 'favorite' THEN 1 ELSE 0 END),0) AS favorites,
			COALESCE(SUM(CASE WHEN interaction_type = 'contact' THEN 1 ELSE 0 END),0) AS contacts
		FROM user_behaviors
		WHERE user_id = ? AND property_type = 'sale' AND timestamp >= NOW() - INTERVAL '90 days' AND deleted_at IS NULL
	`, userID, userID, userID).Scan(&agg).Error

	behaviorScore := float64(agg.Views)*1.0 + float64(agg.Favorites)*8.0 + float64(agg.Contacts)*25.0
	engagement := "cold"
	if behaviorScore > 250 {
		engagement = "hot"
	} else if behaviorScore > 100 {
		engagement = "warm"
	}
	urgency := "browsing"
	if agg.Contacts >= 3 {
		urgency = "ready_to_buy"
	} else if agg.Views >= 20 {
		urgency = "researching"
	}

	var avgPrice float64
	_ = storage.DB.Raw(`
		SELECT COALESCE(AVG(ps.listing_price),0)
		FROM user_behaviors ub
		JOIN property_sales ps ON ps.id = ub.property_id AND ps.deleted_at IS NULL
		WHERE ub.user_id = ?
		  AND ub.property_type = 'sale'
		  AND ub.interaction_type IN ('favorite','contact','view')
		  AND ps.listing_price > 0
		  AND ub.timestamp >= NOW() - INTERVAL '180 days'
		  AND ub.deleted_at IS NULL
	`, userID).Scan(&avgPrice).Error
	if avgPrice <= 0 {
		avgPrice = 1
	}
	budgetMin := avgPrice * 0.7
	budgetMax := avgPrice * 1.3

	personaTags := []string{}
	if agg.Favorites >= 5 {
		personaTags = append(personaTags, "investor")
	}
	if avgPrice < 2000000 {
		personaTags = append(personaTags, "price_sensitive")
	}
	if agg.Contacts > 0 && agg.Views <= 10 {
		personaTags = append(personaTags, "quick_decider")
	}
	if len(personaTags) == 0 {
		personaTags = append(personaTags, "active_browser")
	}
	tagsJSON, _ := json.Marshal(personaTags)

	desired := &models.AIEnrichedUser{
		UserID:                userID,
		TopCityID:             agg.TopCityID,
		TopZoneID:             agg.TopZoneID,
		BehaviorScore:         behaviorScore,
		EngagementLevel:       engagement,
		AIBudgetMin:           budgetMin,
		AIBudgetMax:           budgetMax,
		AIPersonaTags:         datatypes.JSON(tagsJSON),
		AIUrgency:             urgency,
		PreferredPropertyType: "sale",
	}
	return persistAIEnrichedFromComputed(desired)
}

func batchUpsertPropertyMatchesBestEffort(propertyID, hostID uint, rows []scoredMatchRow) error {
	const chunk = 35
	for start := 0; start < len(rows); start += chunk {
		end := start + chunk
		if end > len(rows) {
			end = len(rows)
		}
		part := rows[start:end]
		if batchUpsertDisabled.Load() || batchUpsertPropertyMatchesChunk(propertyID, hostID, part) != nil {
			for _, r := range part {
				_, _ = upsertPropertyMatch(propertyID, hostID, r.userID, r.score, r.tier, r.reasons)
			}
		}
	}
	return nil
}

func batchUpsertPropertyMatchesChunk(propertyID, hostID uint, rows []scoredMatchRow) error {
	if len(rows) == 0 {
		return nil
	}
	var buf strings.Builder
	args := make([]interface{}, 0, len(rows)*7)
	for i, r := range rows {
		if i > 0 {
			buf.WriteString(", ")
		}
		buf.WriteString("(?,?,?,?,?,?,?,NOW(),NOW())")
		rj, _ := json.Marshal(r.reasons)
		args = append(args, propertyID, hostID, r.userID, r.score, r.tier, string(rj), "pending")
	}
	q := `INSERT INTO property_matches (property_id, host_id, suggested_user_id, match_score, match_tier, match_reasons, status, created_at, updated_at) VALUES ` +
		buf.String() + `
ON CONFLICT (property_id, host_id, suggested_user_id) WHERE deleted_at IS NULL
DO UPDATE SET
  match_score = CASE WHEN property_matches.status = 'pending' THEN EXCLUDED.match_score ELSE property_matches.match_score END,
  match_tier = CASE WHEN property_matches.status = 'pending' THEN EXCLUDED.match_tier ELSE property_matches.match_tier END,
  match_reasons = CASE WHEN property_matches.status = 'pending' THEN EXCLUDED.match_reasons ELSE property_matches.match_reasons END,
  updated_at = NOW()`
	err := storage.DB.Exec(q, args...).Error
	if err != nil && strings.Contains(err.Error(), "no unique or exclusion constraint matching the ON CONFLICT specification") {
		batchUpsertDisabled.Store(true)
	}
	return err
}

func computeMatchScore(dna *models.PropertyDNA, user *models.AIEnrichedUser) (float64, []string) {
	score := 0.0
	reasons := []string{}

	zonePts := 0.0
	if dna.ZoneID != nil && user.TopZoneID != nil && *dna.ZoneID == *user.TopZoneID {
		zonePts = 30
		reasons = append(reasons, "same_zone")
	} else if dna.CityID != nil && user.TopCityID != nil && *dna.CityID == *user.TopCityID {
		zonePts = 15
		reasons = append(reasons, "same_city")
	}
	score += zonePts

	typePts := 0.0
	if strings.TrimSpace(dna.PropertyType) != "" && strings.Contains(strings.ToLower(user.PreferredPropertyType), "sale") {
		typePts = 20
		reasons = append(reasons, "type_match")
	}
	score += typePts

	pricePts := 0.0
	if dna.Price >= user.AIBudgetMin && dna.Price <= user.AIBudgetMax {
		pricePts = 20
		reasons = append(reasons, "price_match")
	} else if dna.Price > user.AIBudgetMax && user.AIBudgetMax > 0 && dna.Price <= user.AIBudgetMax*1.10 {
		pricePts = 14
		reasons = append(reasons, "price_near")
	}
	score += pricePts

	behaviorPts := 0.0
	switch strings.ToLower(user.EngagementLevel) {
	case "hot":
		behaviorPts = 15
	case "warm":
		behaviorPts = 10
	default:
		behaviorPts = 4
	}
	score += behaviorPts
	if behaviorPts >= 10 {
		reasons = append(reasons, "high_engagement")
	}

	personaPts := 0.0
	if hasPersonaOverlap(dna.AIPersonas, user.AIPersonaTags) {
		personaPts = 10
		reasons = append(reasons, "persona_overlap")
	}
	score += personaPts

	urgencyPts := 0.0
	switch strings.ToLower(user.AIUrgency) {
	case "ready_to_buy":
		urgencyPts = 5
		reasons = append(reasons, "ready_to_buy")
	case "researching":
		urgencyPts = 3
	}
	score += urgencyPts

	return math.Round(score*10) / 10, reasons
}

func hasPersonaOverlap(a, b datatypes.JSON) bool {
	var aa []string
	var bb []string
	_ = json.Unmarshal(a, &aa)
	_ = json.Unmarshal(b, &bb)
	set := map[string]bool{}
	for _, s := range aa {
		set[strings.ToLower(strings.TrimSpace(s))] = true
	}
	for _, s := range bb {
		if set[strings.ToLower(strings.TrimSpace(s))] {
			return true
		}
	}
	return false
}

func scoreTier(score float64) string {
	switch {
	case score >= 90:
		return "excellent"
	case score >= 75:
		return "strong"
	default:
		return "good"
	}
}

func upsertPropertyMatch(propertyID, hostID, userID uint, score float64, tier string, reasons []string) (*models.PropertyMatch, error) {
	reasonsJSON, _ := json.Marshal(reasons)
	var m models.PropertyMatch
	err := storage.DB.Where("property_id = ? AND host_id = ? AND suggested_user_id = ?", propertyID, hostID, userID).First(&m).Error
	if err == nil {
		// Keep recently computed pending matches stable; prevents constant rewrites per refresh.
		if m.Status == "pending" && time.Since(m.UpdatedAt) < 6*time.Hour {
			return &m, nil
		}
		m.MatchScore = score
		m.MatchTier = tier
		m.MatchReasons = datatypes.JSON(reasonsJSON)
		if m.Status == "" {
			m.Status = "pending"
		}
		return &m, storage.DB.Save(&m).Error
	}
	m = models.PropertyMatch{
		PropertyID:      propertyID,
		HostID:          hostID,
		SuggestedUserID: userID,
		MatchScore:      score,
		MatchTier:       tier,
		MatchReasons:    datatypes.JSON(reasonsJSON),
		Status:          "pending",
	}
	return &m, storage.DB.Create(&m).Error
}

func inferPropertySignals(title, description string, price float64) ([]string, []string, string) {
	txt := strings.ToLower(strings.TrimSpace(title + " " + description))
	tags := []string{}
	personas := []string{}

	addTag := func(t string) {
		for _, x := range tags {
			if x == t {
				return
			}
		}
		tags = append(tags, t)
	}
	addPersona := func(t string) {
		for _, x := range personas {
			if x == t {
				return
			}
		}
		personas = append(personas, t)
	}

	for _, k := range []string{"piscine", "pool", "garden", "jardin", "family", "famille", "luxury", "premium", "urgent"} {
		if strings.Contains(txt, k) {
			switch k {
			case "piscine", "pool":
				addTag("pool")
				addPersona("young_family")
			case "garden", "jardin", "family", "famille":
				addTag("family")
				addPersona("young_family")
			case "luxury", "premium":
				addTag("luxury")
				addPersona("diaspora")
			case "urgent":
				addTag("urgent_sale")
			}
		}
	}

	tier := "mid"
	switch {
	case price <= 1000000:
		tier = "budget"
		addPersona("price_sensitive")
	case price >= 6000000:
		tier = "luxury"
		addPersona("diaspora")
	case price >= 3000000:
		tier = "premium"
	}

	if len(tags) == 0 {
		tags = []string{"standard"}
	}
	if len(personas) == 0 {
		personas = []string{"general_buyer"}
	}
	return tags, personas, tier
}

