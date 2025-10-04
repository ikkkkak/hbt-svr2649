package storage

import (
	"log"
	"os"

	"github.com/go-redis/redis/v8"
)

var Redis *redis.Client

func InitializeRedis() {
	// Get Redis URL from environment, fallback to localhost for development
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "localhost:6379"
		log.Println("⚠️  REDIS_URL not set, using localhost:6379 (development mode)")
	}

	Redis = redis.NewClient(&redis.Options{
		Addr:     redisURL,
		Password: "", // No password for now
		DB:       0,
	})

	log.Println("🔧 Redis initialized with address:", redisURL)
}
