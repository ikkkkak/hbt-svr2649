package ai

import (
	"log"
	"os"
	"strconv"
)

// Config holds model + runtime configuration for MeskenyGPT.
type Config struct {
	Model          string
	APIKey         string
	TimeoutSeconds int
}

// DefaultConfigFromEnv loads configuration from environment variables
// with safe fallbacks.
func DefaultConfigFromEnv() Config {
	model := os.Getenv("OPENROUTER_MODEL")
	if model == "" {
		model = "openai/gpt-oss-120b"
	}

	apiKey := os.Getenv("OPENROUTER_API_KEY")
	if apiKey == "" {
		log.Println("⚠️  OPENROUTER_API_KEY not set — MeskenyGPT will not be able to call OpenRouter")
	}

	timeout := 60
	if v := os.Getenv("MESKENY_GPT_TIMEOUT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			timeout = n
		}
	}

	return Config{
		Model:          model,
		APIKey:         apiKey,
		TimeoutSeconds: timeout,
	}
}

