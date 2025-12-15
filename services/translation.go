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

type libreResponse struct {
	TranslatedText string `json:"translatedText"`
}

// translateOnce calls LibreTranslate for a single target language.
// It returns the translated text or an error if translation fails.
func translateOnce(text, target string) (string, error) {
	if text == "" {
		return "", nil
	}

	url := os.Getenv("LIBRETRANSLATE_URL")
	if url == "" {
		// Fallback – you can point this env var to your self-hosted instance
		url = "https://librerender.onrender.com/translate"
	}

	payload := map[string]string{
		"q":      text,
		"source": "auto",
		"target": target,
	}
	body, _ := json.Marshal(payload)

	// Debug: log the request (only for first call to avoid spam)
	if target == "en" {
		log.Printf("🔍 Translation request: POST %s, payload: %s", url, string(body))
	}

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call LibreTranslate: %w", err)
	}
	defer resp.Body.Close()

	// Read the full response body for debugging
	var responseBody bytes.Buffer
	responseBody.ReadFrom(resp.Body)
	responseStr := responseBody.String()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, responseStr)
	}

	var r libreResponse
	if err := json.Unmarshal(responseBody.Bytes(), &r); err != nil {
		// Log the actual response to help debug
		log.Printf("❌ Failed to decode response for target %s. Response: %s", target, responseStr)
		return "", fmt.Errorf("failed to decode response (got: %s): %w", responseStr, err)
	}

	if r.TranslatedText == "" {
		log.Printf("⚠️  Empty translation returned for target %s. Response: %s", target, responseStr)
		return "", fmt.Errorf("empty translation returned (response: %s)", responseStr)
	}

	// Debug: log if translation is same as original
	if r.TranslatedText == text {
		log.Printf("⚠️  Translation to %s returned same text: '%s' (source might already be %s)", target, text[:min(50, len(text))], target)
	} else {
		log.Printf("✅ Translation to %s: '%s' -> '%s'", target, text[:min(30, len(text))], r.TranslatedText[:min(30, len(r.TranslatedText))])
	}

	return r.TranslatedText, nil
}

// detectLanguage attempts to detect the source language of the text
func detectLanguage(text string) string {
	url := os.Getenv("LIBRETRANSLATE_URL")
	if url == "" {
		url = "https://librerender.onrender.com/translate"
	}

	// Use detect endpoint if available, otherwise use translate with auto-detect
	detectURL := strings.Replace(url, "/translate", "/detect", 1)

	payload := map[string]string{
		"q": text,
	}
	body, _ := json.Marshal(payload)

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", detectURL, bytes.NewBuffer(body))
	if err != nil {
		return "auto"
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "auto"
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "auto"
	}

	var detectResp struct {
		Language   string  `json:"language"`
		Confidence float64 `json:"confidence"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&detectResp); err != nil {
		return "auto"
	}

	// Normalize language code (e.g., "en-US" -> "en", "fr-FR" -> "fr", "ar-SA" -> "ar")
	detected := strings.ToLower(detectResp.Language)
	if strings.HasPrefix(detected, "en") {
		return "en"
	} else if strings.HasPrefix(detected, "fr") {
		return "fr"
	} else if strings.HasPrefix(detected, "ar") {
		return "ar"
	}
	return "auto"
}

// TranslateOnceDirect is a public wrapper for testing
func TranslateOnceDirect(text, target string) (string, error) {
	return translateOnce(text, target)
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

	// Translate to each target language
	// Note: If source is already in target language, LibreTranslate may return same text
	// This is expected behavior - we'll still save it
	for _, lang := range langs {
		translated, err := translateOnce(text, lang)
		if err != nil {
			// On error, use original text as fallback
			out[lang] = text
		} else if translated == "" {
			// Empty translation, use original
			out[lang] = text
		} else {
			// Use the translation (even if same - that's OK if source is already in that language)
			out[lang] = translated
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
