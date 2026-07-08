package vector

import (
	"os"
	"strconv"
	"strings"
)

// Config holds Qdrant + embedding settings.
type Config struct {
	Enabled        bool
	QdrantURL      string
	Collection     string
	EmbeddingModel string
	EmbeddingDim   int
	OpenRouterKey  string
	ScoreThreshold float32
}

func ConfigFromEnv() Config {
	enabled := strings.EqualFold(strings.TrimSpace(os.Getenv("ENABLE_SEMANTIC_SEARCH")), "true") ||
		os.Getenv("ENABLE_SEMANTIC_SEARCH") == "1"

	host := strings.TrimSpace(os.Getenv("QDRANT_HOST"))
	if host == "" {
		host = "localhost"
	}
	port := 6333
	if v := strings.TrimSpace(os.Getenv("QDRANT_PORT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			port = n
		}
	}
	collection := strings.TrimSpace(os.Getenv("QDRANT_COLLECTION_PROPERTIES"))
	if collection == "" {
		collection = "meskeny_properties"
	}
	model := strings.TrimSpace(os.Getenv("EMBEDDING_MODEL"))
	if model == "" {
		model = "openai/text-embedding-3-small"
	}
	dim := 1536
	if v := strings.TrimSpace(os.Getenv("EMBEDDING_DIMENSION")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			dim = n
		}
	}
	threshold := float32(0.28)
	if v := strings.TrimSpace(os.Getenv("SEMANTIC_SCORE_THRESHOLD")); v != "" {
		if f, err := strconv.ParseFloat(v, 32); err == nil {
			threshold = float32(f)
		}
	}

	return Config{
		Enabled:        enabled,
		QdrantURL:      "http://" + host + ":" + strconv.Itoa(port),
		Collection:     collection,
		EmbeddingModel: model,
		EmbeddingDim:   dim,
		OpenRouterKey:  strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY")),
		ScoreThreshold: threshold,
	}
}
