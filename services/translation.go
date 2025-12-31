package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// simple in-memory cache: original text -> lang -> translated
var translateCache = struct {
	sync.RWMutex
	data map[string]map[string]string
}{
	data: make(map[string]map[string]string),
}

type marianMTResponse struct {
	TranslatedText interface{} `json:"translated_text"` // Can be string or []string
}

// translateOnce calls MarianMT API for a single target language.
// It returns the translated text or an error if translation fails.
func translateOnce(text, target string) (string, error) {
	if text == "" {
		return "", nil
	}

	// Get MarianMT API URL from environment or use default
	url := os.Getenv("MARIANMT_URL")
	if url == "" {
		url = "https://librerender-1.onrender.com/translate"
	}

	// Detect source language (simple heuristic - can be improved)
	source := detectSourceLanguage(text)

	// If source and target are same, return original
	if source == target {
		return text, nil
	}

	payload := map[string]interface{}{
		"text":        text,
		"source_lang": source,
		"target_lang": target,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Debug: log the request (only for first call to avoid spam)
	if target == "en" {
		log.Printf("🔍 MarianMT translation request: POST %s, payload: %s", url, string(body))
	}

	// Use longer timeout for Render cold starts (up to 60 seconds)
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Retry logic for Render cold starts (up to 2 retries)
	var resp *http.Response
	maxRetries := 2
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retry (exponential backoff)
			waitTime := time.Duration(attempt) * 2 * time.Second
			log.Printf("⏳ Retrying MarianMT request (attempt %d/%d) after %v...", attempt+1, maxRetries+1, waitTime)
			time.Sleep(waitTime)
		}

		resp, err = client.Do(req)
		if err == nil {
			break
		}

		// Check if it's a timeout or connection error (likely cold start)
		if attempt < maxRetries && (strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "connection")) {
			log.Printf("⚠️  MarianMT request failed (attempt %d/%d): %v", attempt+1, maxRetries+1, err)
			continue
		}

		// Last attempt or non-retryable error
		return "", fmt.Errorf("failed to call MarianMT after %d attempts: %w", attempt+1, err)
	}

	if resp == nil {
		return "", fmt.Errorf("failed to get response from MarianMT")
	}
	defer resp.Body.Close()

	// Read the full response body for debugging
	var responseBody bytes.Buffer
	responseBody.ReadFrom(resp.Body)
	responseStr := responseBody.String()

	if resp.StatusCode != http.StatusOK {
		// Truncate response for logging (might be HTML error page)
		responsePreview := responseStr
		if len(responsePreview) > 500 {
			responsePreview = responsePreview[:500] + "..."
		}
		log.Printf("❌ MarianMT API error (HTTP %d): %s", resp.StatusCode, responsePreview)
		// Check for 502 Bad Gateway (common on Render cold starts)
		if resp.StatusCode == http.StatusBadGateway {
			return "", fmt.Errorf("HTTP 502 Bad Gateway - MarianMT service unavailable (likely cold start or service down)")
		}
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, responsePreview[:min(200, len(responsePreview))])
	}

	var r marianMTResponse
	if err := json.Unmarshal(responseBody.Bytes(), &r); err != nil {
		log.Printf("❌ Failed to decode MarianMT response for target %s. Response: %s", target, responseStr)
		return "", fmt.Errorf("failed to decode response (got: %s): %w", responseStr, err)
	}

	// Handle both string and array responses
	var translatedText string
	switch v := r.TranslatedText.(type) {
	case string:
		translatedText = v
	case []interface{}:
		if len(v) > 0 {
			if str, ok := v[0].(string); ok {
				translatedText = str
			} else {
				return "", fmt.Errorf("unexpected array element type in response")
			}
		} else {
			return "", fmt.Errorf("empty array in response")
		}
	case []string:
		if len(v) > 0 {
			translatedText = v[0]
		} else {
			return "", fmt.Errorf("empty array in response")
		}
	default:
		return "", fmt.Errorf("unexpected response type: %T", r.TranslatedText)
	}

	if translatedText == "" {
		log.Printf("⚠️  Empty translation returned for target %s. Response: %s", target, responseStr)
		return "", fmt.Errorf("empty translation returned (response: %s)", responseStr)
	}

	// Validate translation language matches target
	if target == "ar" && !containsArabic(translatedText) {
		// Check if we got French instead (common bug)
		if containsFrench(translatedText) {
			log.Printf("❌ CRITICAL BUG: Arabic translation returned French! Source: '%s', Got: '%s'", text[:min(50, len(text))], translatedText[:min(50, len(translatedText))])
			return "", fmt.Errorf("translation to Arabic returned French text - API model error")
		}
		// Check if we got English (might be OK if source was English)
		if containsEnglish(translatedText) && !containsArabic(text) {
			log.Printf("⚠️  Warning: Translation to Arabic returned English text: '%s'", translatedText[:min(50, len(translatedText))])
		} else {
			log.Printf("⚠️  Warning: Translation to Arabic doesn't contain Arabic characters: '%s'", translatedText[:min(50, len(translatedText))])
		}
	} else if target == "fr" && !containsFrench(translatedText) && !containsEnglish(translatedText) {
		// French translation should contain French characters or be English-like
		if containsArabic(translatedText) {
			log.Printf("❌ CRITICAL BUG: French translation returned Arabic! Source: '%s', Got: '%s'", text[:min(50, len(text))], translatedText[:min(50, len(translatedText))])
			return "", fmt.Errorf("translation to French returned Arabic text - API model error")
		}
	} else if target == "en" && containsFrench(translatedText) && !containsEnglish(translatedText) {
		// English translation should not be French (unless source was French and translation failed)
		// If source is French, we should get English, not French
		if containsFrench(text) {
			log.Printf("❌ CRITICAL BUG: English translation returned French (same as source)! Source: '%s', Got: '%s'", text[:min(50, len(text))], translatedText[:min(50, len(translatedText))])
			return "", fmt.Errorf("translation to English returned French text (same as source) - API translation failed")
		}
	}

	// Debug: log if translation is same as original
	if translatedText == text {
		log.Printf("⚠️  Translation to %s returned same text: '%s' (source might already be %s)", target, text[:min(50, len(text))], target)
	} else {
		log.Printf("✅ Translation to %s: '%s' -> '%s'", target, text[:min(30, len(text))], translatedText[:min(30, len(translatedText))])
	}

	return translatedText, nil
}

// detectSourceLanguage attempts to detect the source language using simple heuristics
func detectSourceLanguage(text string) string {
	// Simple heuristic: check for Arabic characters
	if containsArabic(text) {
		return "ar"
	}

	// Check for French-specific characters/patterns
	if containsFrench(text) {
		return "fr"
	}

	// Default to English
	return "en"
}

// containsArabic checks if text contains Arabic characters
func containsArabic(text string) bool {
	for _, r := range text {
		if r >= 0x0600 && r <= 0x06FF {
			return true
		}
	}
	return false
}

// containsFrench checks if text contains French-specific patterns
func containsFrench(text string) bool {
	frenchIndicators := []string{"é", "è", "ê", "ë", "à", "â", "ç", "ù", "û", "ü", "ô", "ö", "î", "ï"}
	textLower := strings.ToLower(text)
	for _, indicator := range frenchIndicators {
		if strings.Contains(textLower, indicator) {
			return true
		}
	}
	return false
}

// containsEnglish checks if text is likely English (basic heuristic)
func containsEnglish(text string) bool {
	// If it contains Arabic or French characters, it's not English
	if containsArabic(text) || containsFrench(text) {
		return false
	}
	// If it's mostly ASCII printable characters, likely English
	asciiCount := 0
	for _, r := range text {
		if r < 128 && (r >= 32 && r <= 126) {
			asciiCount++
		}
	}
	return asciiCount > len(text)*80/100
}


// TranslateOnceDirect is a public wrapper for testing
func TranslateOnceDirect(text, target string) (string, error) {
	return translateOnce(text, target)
}

// DetectSourceLanguageDirect is a public wrapper for language detection
func DetectSourceLanguageDirect(text string) string {
	return detectSourceLanguage(text)
}

// TranslateAllLanguages returns a map with permanent translations for en/fr/ar.
// It detects the source language and only translates to other languages.
func TranslateAllLanguages(text string) map[string]string {
	langs := []string{"en", "fr", "ar"}
	out := make(map[string]string, len(langs))
	if text == "" {
		return out
	}

	translateCache.RLock()
	if cached, ok := translateCache.data[text]; ok {
		translateCache.RUnlock()
		return cached
	}
	translateCache.RUnlock()

	// Detect source language first
	sourceLang := detectSourceLanguage(text)
	
	// Translate to each target language
	for _, lang := range langs {
		// If source language matches target, use original text (no translation needed)
		if sourceLang == lang {
			out[lang] = text
			continue
		}
		
		translated, err := translateOnce(text, lang)
		if err != nil {
			// On error, log and use original text as fallback
			log.Printf("⚠️  Translation to %s failed for text '%s': %v", lang, text[:min(50, len(text))], err)
			out[lang] = text
		} else if translated == "" {
			// Empty translation, use original
			log.Printf("⚠️  Empty translation returned for %s, using original text", lang)
			out[lang] = text
		} else {
			// CRITICAL VALIDATION: If source is French and we're translating to English/Arabic,
			// the result should NOT be French (this indicates API failure)
			if sourceLang == "fr" && lang != "fr" {
				if containsFrench(translated) && !containsEnglish(translated) && !containsArabic(translated) {
					// We got French when we should have gotten English or Arabic
					log.Printf("❌ CRITICAL: Translation from French to %s returned French text! API likely failed.", lang)
					log.Printf("   Source (fr): '%s'", text[:min(60, len(text))])
					log.Printf("   Got (should be %s): '%s'", lang, translated[:min(60, len(translated))])
					// Don't use the wrong translation - use original as fallback
					out[lang] = text
				} else {
					// Valid translation
					out[lang] = translated
				}
			} else if sourceLang == "ar" && lang != "ar" {
				// Similar check for Arabic source
				if containsArabic(translated) && !containsEnglish(translated) && !containsFrench(translated) {
					log.Printf("❌ CRITICAL: Translation from Arabic to %s returned Arabic text! API likely failed.", lang)
					out[lang] = text
				} else {
					out[lang] = translated
				}
			} else {
				// Use the translation
				out[lang] = translated
			}
		}
		// Delay to be gentle with the translation server
		time.Sleep(300 * time.Millisecond)
	}

	translateCache.Lock()
	translateCache.data[text] = out
	translateCache.Unlock()

	return out
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
