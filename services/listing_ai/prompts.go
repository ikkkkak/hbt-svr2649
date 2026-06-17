package listing_ai

import (
	"fmt"
	"strings"
)

func kindCopyRules(kind Kind, lang string) string {
	switch kind {
	case KindLand:
		return landCopyRules(lang)
	case KindRent:
		return rentCopyRules(lang)
	default:
		return saleCopyRules(lang)
	}
}

func saleCopyRules(lang string) string {
	switch lang {
	case "ar":
		return `- نوع الإعلان: عقار للبيع في موريتانيا.
- العنوان: يذكر النوع (شقة، فيلا، منزل…) + أبرز ميزة + الحي/المنطقة إن وُجدت.
- الوصف: فقرة افتتاحية جذابة، ثم المساحة والغرف والتجهيزات، ثم سياق الحي، ثم دعوة للتواصل.
- indoor_features / outdoor_features: وسوم قصيرة بالعربية (مكيف، موقف، حديقة…).`
	case "en":
		return `- Listing type: property for sale in Mauritania.
- Title: property type + standout feature + area/neighborhood when known.
- Description: opening hook, specs (area, beds, baths), features/amenities, neighborhood context, soft CTA.
- indoor_features / outdoor_features: short English tags (AC, parking, garden…).`
	default:
		return `- Type d'annonce : bien à vendre en Mauritanie.
- Titre : type de bien + atout principal + quartier/zone si connus.
- Description : accroche, caractéristiques (surface, chambres, SdB), équipements, contexte du quartier, appel à contact.
- indoor_features / outdoor_features : tags courts en français (clim, parking, jardin…).`
	}
}

func rentCopyRules(lang string) string {
	switch lang {
	case "ar":
		return `- نوع الإعلان: إيجار (شهري/طويل الأمد).
- ركّز على الراحة، الأثاث، القرب من الخدمات، وشروط الإيجار إن ذكرها المستخدم.
- لا تذكر «للبيع» أو سعر شراء.`
	case "en":
		return `- Listing type: rental (monthly/long-term).
- Emphasize comfort, furnishing, proximity to services, and lease terms only if the user stated them.
- Do NOT say "for sale" or purchase price.`
	default:
		return `- Type d'annonce : location (mensuelle / longue durée).
- Mettez l'accent sur le confort, le meublé, la proximité des services et les conditions si mentionnées.
- Ne pas écrire « à vendre » ni prix d'achat.`
	}
}

func landCopyRules(lang string) string {
	switch lang {
	case "ar":
		return `- نوع الإعلان: أرض/قطعة أرض للبيع.
- اذكر المساحة بالم²، نوع التجزئة (سكني/تجاري…) إن وُجد، ووضع الملكية فقط إذا ذكره المستخدم.
- plot_number: رقم القطعة من النص فقط — لا تخترع.`
	case "en":
		return `- Listing type: land/plot for sale.
- State area in m², zoning if given, title/legal status ONLY if the user mentioned it.
- plot_number: copy from user text only — never invent.`
	default:
		return `- Type d'annonce : terrain / parcelle à vendre.
- Indiquez la surface en m², le zonage si précisé, le statut juridique UNIQUEMENT si l'utilisateur l'a mentionné.
- plot_number : reprendre le texte utilisateur — ne jamais inventer.`
	}
}
func buildListingSystemPrompt(kind Kind, lang string, schema string, plotRules string) string {
	return fmt.Sprintf(`You are Meskeny Listing Agent — a senior real-estate copywriter for Mauritania (Meskeny app).

Your job is to transform raw property information into professional, market-ready real-estate listings that feel trustworthy, attractive, and useful to buyers and renters.

NEVER paste or lightly edit the user's raw notes as the title or description. Always rewrite into fresh marketplace copy.

Return ONLY valid JSON (no markdown fences) matching this schema:
%s

COPY QUALITY (mandatory):
- title: max 90 characters, clear, professional, natural, no ALL CAPS, no emoji
- description: 3–5 short paragraphs (not one wall of text)
- Write naturally like a professional real-estate agent
- Focus on real selling points, not generic filler
- Never sound robotic or AI-generated
- Use persuasive but honest language
- Be concise, specific, and easy to read

TITLE GENERATION RULES:
- Highlight the strongest selling point first
- Prioritize location, area, luxury level, investment value, view, or standout features
- Avoid simply repeating the user input
- Make titles feel like real marketplace listings
- Never include information that was not provided

DESCRIPTION WRITING FORMULA:
1) Hook
   - Open with the strongest benefit or opportunity

2) Core Facts
   - Mention confirmed details only

3) Features
   - Integrate amenities and user-provided details naturally

4) Location
   - Mention location information when provided

5) Call To Action
   - End with a short professional invitation to contact or visit

TRUTHFULNESS RULES:
- Never invent facts
- Never invent bedrooms, bathrooms, area, price, amenities, ownership status, renovations, or nearby services
- If information is missing, simply omit it
- Never fabricate luxury features

AMENITY RULES:
- NEVER mention numeric amenity IDs
- NEVER expose internal database values
- Convert amenities into natural human language

LOCATION RULES:
- city_name, zone_name, quartier_name: copy EXACT catalog spellings when provided
- Never invent quartier names
- neighborhood_description: 1–2 sentences maximum

GOOD EXAMPLES:

Example 1

INPUT:
Type: Duplex
Purpose: Sale
Location: Tevragh Zeina
Area: 185 m²

OUTPUT STYLE:

Title:
دوبلكس للبيع في تفرغ زينة بمساحة 185 متر مربع

Description:
فرصة مميزة لاقتناء دوبلكس في حي تفرغ زينة، أحد أكثر الأحياء طلباً في نواكشوط.

يمتد العقار على مساحة 185 متر مربع ويوفر مساحة مناسبة للسكن العائلي أو الاستثمار العقاري.

يتميز بموقع جيد يسهل الوصول إلى مختلف الخدمات والمرافق داخل المنطقة.

للمزيد من المعلومات أو لترتيب زيارة للعقار يرجى التواصل معنا.

Example 2

INPUT:
Type: Villa
Purpose: Rent
Location: Tevragh Zeina
Bedrooms: 5
Bathrooms: 4
Pool: Yes

OUTPUT STYLE:

Title:
فيلا للإيجار مع مسبح في تفرغ زينة

Description:
إذا كنت تبحث عن سكن مريح في موقع مميز، فإن هذه الفيلا تمثل خياراً مناسباً للعائلات الباحثة عن المساحة والخصوصية.

تضم الفيلا 5 غرف نوم و4 حمامات، بالإضافة إلى مسبح يوفر أجواءً مثالية للاسترخاء.

تقع الفيلا في حي تفرغ زينة المعروف بموقعه الحيوي وقربه من العديد من الخدمات.

للمزيد من التفاصيل أو لتحديد موعد للمعاينة يرجى التواصل معنا.

Example 3

INPUT:
Type: Apartment
Purpose: Sale
Location: Nouakchott
Area: 120 m²
Bedrooms: 3
Bathrooms: 2
Parking: Yes

OUTPUT STYLE:

Title:
شقة للبيع بمساحة 120 متر مربع مع موقف سيارات

Description:
فرصة رائعة لامتلاك شقة عملية ومناسبة للسكن العائلي.

تبلغ مساحة الشقة 120 متر مربع وتتكون من 3 غرف نوم و2 حمام.

كما تتوفر على موقف سيارات يوفر مزيداً من الراحة للسكان.

للمزيد من المعلومات أو لترتيب زيارة يرجى التواصل معنا.

BAD EXAMPLES (NEVER DO THIS):

❌ شقة رائعة جداً جداً وفخمة للغاية وأفضل فرصة في موريتانيا
(Exaggerated marketing)

❌ شقة بالقرب من المدارس والمستشفيات والمطار
(When this information was not provided)

❌ العقار يحتوي على حديقة كبيرة ومسبح
(If these features were not provided)

❌ Amenities Selected: [1,5,7,8]
(Internal data leak)

WRITING STYLE:
- Professional real-estate marketing tone
- Natural Arabic, French, or English depending on requested language
- Readable and human
- Short paragraphs
- No hashtags
- No emojis
- No markdown
- No bullet points inside description
- Sound like a real property listing published on a marketplace

%s

%s
%s`, schema, kindCopyRules(kind, lang), plotRules, languagePromptBlock(lang))
}

func buildListingUserPrompt(in GenerateInput, userStory, lang, catalogSummary string) string {
	amenityLine := "not specified"
	if len(in.AmenityNames) > 0 {
		amenityLine = strings.Join(in.AmenityNames, ", ")
	} else if len(in.AmenityIDs) > 0 {
		amenityLine = "selected (mention generically by name — never numeric IDs)"
	}

	confirmedLoc := strings.TrimSpace(strings.Join([]string{
		formatLocHint("city", in.CityHint),
		formatLocHint("zone", in.ZoneHint),
		formatLocHint("quartier", in.QuartierHint),
	}, "\n"))

	var b strings.Builder
	b.WriteString(fmt.Sprintf("OUTPUT_LANGUAGE: %s (%s)\n", lang, languageHumanName(lang)))
	b.WriteString("Write title, description, and neighborhood_description ONLY in this language.\n\n")

	b.WriteString("CRITICAL: Transform the raw notes below into NEW professional listing copy.\n")
	b.WriteString("- Do NOT copy, paste, or lightly edit the user's sentences.\n")
	b.WriteString("- Rephrase completely while keeping only confirmed facts.\n")
	b.WriteString("- Title must be a polished headline, NOT the user's first line.\n\n")

	b.WriteString(fmt.Sprintf("Listing kind: %s\n", in.Kind))
	if confirmedLoc != "" {
		b.WriteString("User-confirmed location (use in copy + JSON location fields):\n")
		b.WriteString(confirmedLoc)
		b.WriteString("\n\n")
	}

	b.WriteString("Raw owner notes (facts only — rewrite professionally, do not copy verbatim):\n")
	story := strings.TrimSpace(userStory)
	if story == "" {
		story = strings.TrimSpace(extractUserStory(in.Details))
	}
	b.WriteString(story)
	b.WriteString("\n\n")

	b.WriteString("Structured facts (use accurately; omit lines marked not specified):\n")
	b.WriteString(fmt.Sprintf("- Price: %s\n", formatPrice(in.Price, in.Currency)))
	b.WriteString(fmt.Sprintf("- Bedrooms: %v\n", ptrInt(in.Bedrooms)))
	b.WriteString(fmt.Sprintf("- Bathrooms: %v\n", ptrInt(in.Bathrooms)))
	b.WriteString(fmt.Sprintf("- Area: %s\n", formatArea(in.Area, in.AreaUnit)))
	b.WriteString(fmt.Sprintf("- Property type: %s\n", emptyAs(in.PropertyType, "not specified")))
	b.WriteString(fmt.Sprintf("- Land type: %s\n", emptyAs(in.LandType, "not specified")))
	b.WriteString(fmt.Sprintf("- Amenities: %s\n", amenityLine))
	if strings.TrimSpace(in.PlotNumber) != "" {
		b.WriteString(fmt.Sprintf("- Plot number (form): %q\n", strings.TrimSpace(in.PlotNumber)))
	}
	b.WriteString(fmt.Sprintf("- Photos: %d | Videos: %d\n\n", len(in.ImageURLs), len(in.VideoURLs)))

	b.WriteString("Location catalog (city > zone > quartier — use exact spellings when matching):\n")
	b.WriteString(catalogSummary)

	return b.String()
}

func formatLocHint(label, value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	return fmt.Sprintf("- %s: %q", label, v)
}

func formatPrice(price float64, currency string) string {
	if price <= 0 {
		return "not specified"
	}
	cur := strings.TrimSpace(currency)
	if cur == "" {
		cur = "MRU"
	}
	return fmt.Sprintf("%.0f %s", price, cur)
}

func formatArea(area float64, unit string) string {
	if area <= 0 {
		return "not specified"
	}
	u := strings.TrimSpace(unit)
	if u == "" {
		u = "m²"
	}
	return fmt.Sprintf("%.0f %s", area, u)
}

func emptyAs(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return strings.TrimSpace(s)
}

func languageHumanName(lang string) string {
	switch lang {
	case "ar":
		return "Arabic / العربية"
	case "en":
		return "English"
	default:
		return "French / français"
	}
}
