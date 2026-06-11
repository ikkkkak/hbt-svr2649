package meskenyguide

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"apartments-clone-server/models"
	"apartments-clone-server/services"
)

var guideSystemPromptBase = `You are Meskeny Guide, a clinical real-estate performance analyst for hosts in Mauritania.
Output ONLY valid JSON with keys: diagnosis, root_cause, prescription, impact_forecast, category, tone.
Rules:
- Tone: clinical, never patronizing. Use "Listings with X perform Y% better" not "You should".
- root_cause must cite specific numbers from algorithm_signals only — never invent metrics.
- prescription: max 3 numbered steps, actionable.
- impact_forecast: ranges only e.g. "+15–25% inquiries within 48 hours".
- category: one of photo, price, seo, timing, engagement, competitive.`

type llmGuidePayload struct {
	Diagnosis      string `json:"diagnosis"`
	RootCause      string `json:"root_cause"`
	Prescription   string `json:"prescription"`
	ImpactForecast string `json:"impact_forecast"`
	Category       string `json:"category"`
	Tone           string `json:"tone"`
}

var (
	responseCache   = make(map[string]llmGuidePayload)
	responseCacheMu sync.RWMutex
)

func cacheKey(trigger, lang string, signals models.JSONMap) string {
	b, _ := json.Marshal(signals)
	return trigger + ":" + lang + ":" + string(b)
}

func generateCommentContent(trigger, severity, lang string, m ListingMetrics, signals models.JSONMap) (diagnosis, rootCause, prescription, impact, category, tone string) {
	lang = normalizeLocale(lang)
	if lang == "" {
		lang = "en"
	}
	key := cacheKey(trigger, lang, signals)
	responseCacheMu.RLock()
	if cached, ok := responseCache[key]; ok {
		responseCacheMu.RUnlock()
		return cached.Diagnosis, cached.RootCause, cached.Prescription, cached.ImpactForecast, cached.Category, cached.Tone
	}
	responseCacheMu.RUnlock()

	payload, err := callLLM(trigger, severity, lang, signals)
	if err != nil {
		payload = fallbackGuide(trigger, severity, m, lang)
	}
	payload.Diagnosis = strings.TrimSpace(payload.Diagnosis)
	payload.RootCause = strings.TrimSpace(payload.RootCause)
	payload.Prescription = strings.TrimSpace(payload.Prescription)
	payload.ImpactForecast = strings.TrimSpace(payload.ImpactForecast)
	if payload.Category == "" {
		payload.Category = categoryForTrigger(trigger)
	}
	if payload.Tone == "" {
		payload.Tone = "clinical"
		if severity == models.GuideSeverityUrgent {
			payload.Tone = "directive"
		} else if severity == models.GuideSeverityAction {
			payload.Tone = "supportive"
		}
	}

	responseCacheMu.Lock()
	responseCache[key] = payload
	responseCacheMu.Unlock()

	return payload.Diagnosis, payload.RootCause, payload.Prescription, payload.ImpactForecast, payload.Category, payload.Tone
}

func callLLM(trigger, severity, lang string, signals models.JSONMap) (llmGuidePayload, error) {
	sigJSON, _ := json.Marshal(signals)
	system := guideSystemPromptBase + "\n" + llmLanguageInstruction(lang)
	userPrompt := fmt.Sprintf(`trigger_event: %s
severity: %s
host_locale: %s
algorithm_signals: %s`, trigger, severity, lang, string(sigJSON))

	ai := services.NewAIService()
	raw, err := ai.CompleteJSON(system, userPrompt)
	if err != nil {
		return llmGuidePayload{}, err
	}
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)

	var out llmGuidePayload
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return llmGuidePayload{}, err
	}
	return out, nil
}

func categoryForTrigger(trigger string) string {
	switch trigger {
	case models.GuideTriggerFirstInquiry:
		return "photo"
	case models.GuideTriggerViewsDrop:
		return "engagement"
	case models.GuideTriggerActionImpact:
		return "engagement"
	default:
		return "engagement"
	}
}

func fallbackGuide(trigger, severity string, m ListingMetrics, lang string) llmGuidePayload {
	switch normalizeLocale(lang) {
	case "ar":
		return fallbackGuideAR(trigger, m)
	case "fr":
		return fallbackGuideFR(trigger, m)
	default:
		return fallbackGuideEN(trigger, m)
	}
}

func fallbackGuideEN(trigger string, m ListingMetrics) llmGuidePayload {
	switch trigger {
	case models.GuideTriggerViewsDrop:
		return llmGuidePayload{
			Diagnosis: fmt.Sprintf("Views declined %.0f%% versus the prior 6-hour window (%d → %d views).",
				-m.ViewsDeltaPct, m.ViewsPrev6h, m.ViewsLast6h),
			RootCause: fmt.Sprintf("Engagement velocity dropped while photo count remains %d; listings with ≥8 photos retain steadier view curves.",
				m.PhotoCount),
			Prescription:   "1. Add 2–3 high-resolution exterior and living-area photos.\n2. Refresh the cover image to the brightest daylight shot.\n3. Republish after edits to reset feed freshness.",
			ImpactForecast: "Expected stabilization: +15–35% views within 48 hours after media refresh.",
			Category:       "photo",
			Tone:           "clinical",
		}
	case models.GuideTriggerFirstInquiry:
		return llmGuidePayload{
			Diagnosis: fmt.Sprintf("No inquiries after %.0f hours live; %d platform views recorded.", m.HoursSincePublish, m.ViewCount),
			RootCause: fmt.Sprintf("Photo count is %d; listings with ≥6 photos convert inquiries at materially higher rates in your city.",
				m.PhotoCount),
			Prescription:   "1. Upload at least 4 additional photos (exterior, kitchen, primary bedroom).\n2. Add a one-line location hook in the first sentence.\n3. Verify price is within 10% of comparable active listings.",
			ImpactForecast: "Expected first inquiries: within 24–72 hours after photo expansion.",
			Category:       "photo",
			Tone:           "directive",
		}
	case models.GuideTriggerActionImpact:
		return llmGuidePayload{
			Diagnosis:      "48-hour post-implementation review complete.",
			RootCause:      fmt.Sprintf("Views last 6h: %d vs prior baseline %d.", m.ViewsLast6h, m.ViewsPrev6h),
			Prescription:   "Continue monitoring; no further action unless inquiry rate stays below 2%.",
			ImpactForecast: "Sustained lift typically compounds over 5–7 days when photo count stays ≥8.",
			Category:       "engagement",
			Tone:           "clinical",
		}
	default:
		return llmGuidePayload{
			Diagnosis:      "Performance signal detected on this listing.",
			RootCause:      "Measured deltas are in algorithm_signals.",
			Prescription:   "1. Review listing media and pricing.\n2. Align description with top comparables.",
			ImpactForecast: "Expected improvement: +10–20% engagement within 48 hours.",
			Category:       categoryForTrigger(trigger),
			Tone:           "clinical",
		}
	}
}

func fallbackGuideFR(trigger string, m ListingMetrics) llmGuidePayload {
	switch trigger {
	case models.GuideTriggerViewsDrop:
		return llmGuidePayload{
			Diagnosis: fmt.Sprintf("Les vues ont baissé de %.0f %% sur les 6 dernières heures (%d → %d).",
				-m.ViewsDeltaPct, m.ViewsPrev6h, m.ViewsLast6h),
			RootCause: fmt.Sprintf("La dynamique d'engagement faiblit avec seulement %d photos ; les annonces avec ≥8 photos restent plus stables.",
				m.PhotoCount),
			Prescription:   "1. Ajoutez 2–3 photos HD (extérieur, séjour).\n2. Changez la photo de couverture (lumière du jour).\n3. Republiez après modification.",
			ImpactForecast: "Stabilisation attendue : +15–35 % de vues sous 48 h.",
			Category:       "photo",
			Tone:           "clinical",
		}
	case models.GuideTriggerFirstInquiry:
		return llmGuidePayload{
			Diagnosis: fmt.Sprintf("Aucune demande après %.0f h en ligne ; %d vues enregistrées.", m.HoursSincePublish, m.ViewCount),
			RootCause: fmt.Sprintf("Seulement %d photos ; les annonces avec ≥6 photos convertissent mieux dans votre ville.",
				m.PhotoCount),
			Prescription:   "1. Ajoutez au moins 4 photos (extérieur, cuisine, chambre).\n2. Accrochez la localisation dès la première phrase.\n3. Vérifiez le prix vs le marché (±10 %).",
			ImpactForecast: "Premières demandes attendues : 24–72 h après enrichissement photo.",
			Category:       "photo",
			Tone:           "directive",
		}
	case models.GuideTriggerActionImpact:
		return llmGuidePayload{
			Diagnosis:      "Bilan 48 h après vos modifications.",
			RootCause:      fmt.Sprintf("Vues sur 6 h : %d vs référence %d.", m.ViewsLast6h, m.ViewsPrev6h),
			Prescription:   "Poursuivez le suivi ; aucune action si le taux de demandes reste correct.",
			ImpactForecast: "L'effet se consolide souvent sur 5–7 jours avec ≥8 photos.",
			Category:       "engagement",
			Tone:           "clinical",
		}
	default:
		return llmGuidePayload{
			Diagnosis:      "Signal de performance détecté sur cette annonce.",
			RootCause:      "Voir les métriques mesurées dans algorithm_signals.",
			Prescription:   "1. Revoyez médias et prix.\n2. Alignez la description sur les meilleures annonces.",
			ImpactForecast: "Amélioration attendue : +10–20 % d'engagement sous 48 h.",
			Category:       categoryForTrigger(trigger),
			Tone:           "clinical",
		}
	}
}

func fallbackGuideAR(trigger string, m ListingMetrics) llmGuidePayload {
	switch trigger {
	case models.GuideTriggerViewsDrop:
		return llmGuidePayload{
			Diagnosis: fmt.Sprintf("انخفضت المشاهدات بنسبة %.0f%% مقارنة بالنافذة السابقة (6 ساعات) (%d ← %d).",
				-m.ViewsDeltaPct, m.ViewsPrev6h, m.ViewsLast6h),
			RootCause: fmt.Sprintf("تراجعت سرعة التفاعل مع %d صور فقط؛ الإعلانات ذات 8+ صور تحافظ على منحنى مشاهدات أكثر استقراراً.",
				m.PhotoCount),
			Prescription:   "1. أضف 2–3 صور عالية الدقة (واجهة، صالة).\n2. حدّث صورة الغلاف بأفضل إضاءة نهارية.\n3. أعد النشر بعد التعديل.",
			ImpactForecast: "استقرار متوقع: +15–35% مشاهدات خلال 48 ساعة.",
			Category:       "photo",
			Tone:           "clinical",
		}
	case models.GuideTriggerFirstInquiry:
		return llmGuidePayload{
			Diagnosis: fmt.Sprintf("لا استفسارات بعد %.0f ساعة من النشر؛ %d مشاهدة مسجّلة.", m.HoursSincePublish, m.ViewCount),
			RootCause: fmt.Sprintf("عدد الصور %d؛ الإعلانات بـ 6+ صور تحقق استفسارات أعلى في مدينتك.",
				m.PhotoCount),
			Prescription:   "1. أضف 4 صور على الأقل (خارجية، مطبخ، غرفة).\n2. اذكر الموقع في الجملة الأولى.\n3. راجع السعر مقارنة بالسوق (±10%).",
			ImpactForecast: "أول استفسارات متوقعة: خلال 24–72 ساعة بعد إضافة الصور.",
			Category:       "photo",
			Tone:           "directive",
		}
	case models.GuideTriggerActionImpact:
		return llmGuidePayload{
			Diagnosis:      "مراجعة بعد 48 ساعة من تنفيذ توصياتك.",
			RootCause:      fmt.Sprintf("مشاهدات آخر 6 س: %d مقابل %d سابقاً.", m.ViewsLast6h, m.ViewsPrev6h),
			Prescription:   "تابع المؤشرات؛ لا إجراء إضافي إذا بقي معدل الاستفسارات جيداً.",
			ImpactForecast: "التحسن يتعزز غالباً خلال 5–7 أيام مع 8+ صور.",
			Category:       "engagement",
			Tone:           "clinical",
		}
	default:
		return llmGuidePayload{
			Diagnosis:      "إشارة أداء على هذا الإعلان.",
			RootCause:      "الأرقام المقاسة في algorithm_signals.",
			Prescription:   "1. راجع الصور والسعر.\n2. قارن الوصف بأفضل الإعلانات المشابهة.",
			ImpactForecast: "تحسن متوقع: +10–20% تفاعل خلال 48 ساعة.",
			Category:       categoryForTrigger(trigger),
			Tone:           "clinical",
		}
	}
}
