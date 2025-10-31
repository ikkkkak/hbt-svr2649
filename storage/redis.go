package storage

import (
	"crypto/tls"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/go-redis/redis/v8"
)

var Redis *redis.Client

func InitializeRedis() {
	// Get Redis URL from environment, fallback to localhost for development
	raw := os.Getenv("REDIS_URL")
	addr := "localhost:6379"
	pass := ""
	user := ""
	useTLS := false
	if raw == "" {
		log.Println("⚠️  REDIS_URL not set, using localhost:6379 (development mode)")
	} else {
		// Support formats like: host:port OR redis://user:pass@host:port
		if strings.HasPrefix(raw, "redis://") || strings.HasPrefix(raw, "rediss://") {
			if u, err := url.Parse(raw); err == nil {
				addr = u.Host
				if u.User != nil {
					user = u.User.Username()
					p, _ := u.User.Password()
					pass = p
				}
				if u.Scheme == "rediss" {
					useTLS = true
				}
			} else {
				log.Printf("⚠️  Failed to parse REDIS_URL, falling back to raw host: %v", err)
				addr = strings.TrimPrefix(raw, "redis://")
				addr = strings.TrimPrefix(addr, "rediss://")
			}
		} else {
			addr = raw
		}
	}

	opts := &redis.Options{
		Addr:     addr,
		Username: user,
		Password: pass,
		DB:       0,
	}
	if useTLS {
		opts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	Redis = redis.NewClient(opts)

	log.Printf("🔧 Redis initialized with addr=%s user=%s tls=%v", addr, user, useTLS)
}
