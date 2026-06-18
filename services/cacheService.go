package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

var (
	redisCircuitMu        sync.RWMutex
	redisCircuitOpenUntil time.Time
	lastRedisLimitLog     time.Time
)

func markRedisCircuit(err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	if !strings.Contains(msg, "max requests limit") &&
		!strings.Contains(msg, "ERR max requests") &&
		!strings.Contains(msg, "quota") {
		return
	}
	redisCircuitMu.Lock()
	redisCircuitOpenUntil = time.Now().Add(10 * time.Minute)
	if time.Since(lastRedisLimitLog) > time.Minute {
		lastRedisLimitLog = time.Now()
		log.Printf("[CacheService] Redis quota/limit hit — cache bypassed for 10m")
	}
	redisCircuitMu.Unlock()
}

func isRedisCircuitOpen() bool {
	redisCircuitMu.RLock()
	open := time.Now().Before(redisCircuitOpenUntil)
	redisCircuitMu.RUnlock()
	return open
}

// CacheService provides centralized Redis caching for the application
type CacheService struct {
	client *redis.Client
	logger *log.Logger
}

// CacheConfig holds configuration for cache TTLs
type CacheConfig struct {
	VideoFeedTTL        time.Duration
	PropertyListTTL     time.Duration
	PropertyDetailsTTL  time.Duration
	DefaultTTL          time.Duration
}

// Cache key constants (organized by domain)
const (
	// Video Feed Cache Keys
	VideoFeedKey            = "feed:videos:%s"             // feed:videos:{userId}
	VideoFeedPageKey        = "feed:videos:page:%s:%d"     // feed:videos:page:{userId}:{pageNum}
	VideoKey                = "video:%d"                   // video:{videoId}
	VideoMetadataKey        = "video:meta:%d"              // video:meta:{videoId}
	VideoLikeCountKey       = "video:likes:%d"             // video:likes:{videoId}
	VideoSaveCountKey       = "video:saves:%d"             // video:saves:{videoId}

	// Property Sales Cache Keys
	PropertySalesListKey    = "property:sales:list:%d"     // property:sales:list:{pageNum}
	PropertySaleKey         = "property:sale:%d"           // property:sale:{propertyId}
	PropertySalesSearchKey  = "property:sales:search:%s"   // property:sales:search:{searchHash}
	PropertySalesPageKey    = "property:sales:page:v3:%d:%d"  // v3 = full gallery (images + classified_photos)
	PropertySaleVideoFeedKey = "feed:property-sale-videos:anon:%d:%d" // page, limit
	PropertySalesSmartFeedAnonKey = "feed:property-sales:smart:anon:%d:%s" // limit, lang

	// Client mutation idempotency (mobile offline queue)
	ClientMutationKey = "mutation:client:%s" // clientMutationId

	// Property Details Cache Keys
	PropertyDetailsKey      = "property:details:%d"        // property:details:{propertyId}
	PropertyImagesKey       = "property:images:%d"         // property:images:{propertyId}
	PropertyMetadataKey     = "property:meta:%d"           // property:meta:{propertyId}
	PropertyLocationKey     = "property:location:%d"       // property:location:{propertyId}

	// Landmark Cache Keys
	LandmarkVideoKey        = "landmark:video:%d"          // landmark:video:{videoId}
	LandmarkVideosKey       = "landmark:videos:%d"         // landmark:videos:{pageNum}

	// General Cache Keys
	UserPreferencesKey      = "user:prefs:%d"              // user:prefs:{userId}
	SearchResultsKey        = "search:results:%s"          // search:results:{query}

	// Cache Invalidation Tags (for bulk operations)
	PropertySalesTag        = "tag:property-sales"
	VideoFeedTag            = "tag:video-feed"
	PropertyDetailsTag      = "tag:property-details"
)

// DefaultCacheConfig returns default cache configuration
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		VideoFeedTTL:       15 * time.Minute,
		PropertyListTTL:    10 * time.Minute,
		PropertyDetailsTTL: 30 * time.Minute,
		DefaultTTL:         5 * time.Minute,
	}
}

// NewCacheService creates a new cache service instance
func NewCacheService(client *redis.Client) *CacheService {
	return &CacheService{
		client: client,
		logger: log.New(log.Writer(), "[CacheService] ", log.LstdFlags|log.Lshortfile),
	}
}

// ─────────────────────────────────────────────────────────────────
// Generic Cache Operations
// ─────────────────────────────────────────────────────────────────

// Set stores a value in Redis with TTL
func (cs *CacheService) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if isRedisCircuitOpen() {
		return nil
	}
	jsonData, err := json.Marshal(value)
	if err != nil {
		cs.logger.Printf("❌ JSON marshal error for key %s: %v", key, err)
		return err
	}

	err = cs.client.Set(ctx, key, jsonData, ttl).Err()
	if err != nil {
		markRedisCircuit(err)
		if !isRedisCircuitOpen() {
			cs.logger.Printf("⚠️ Failed to cache %s: %v", key, err)
		}
		return err
	}

	cs.logger.Printf("✅ Cached %s (TTL: %v)", key, ttl)
	return nil
}

// Get retrieves a value from Redis
func (cs *CacheService) Get(ctx context.Context, key string, dest interface{}) error {
	if isRedisCircuitOpen() {
		return nil // Treat as cache miss — serve from DB
	}
	val, err := cs.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil // Cache miss (not an error)
	}
	if err != nil {
		markRedisCircuit(err)
		if !isRedisCircuitOpen() {
			cs.logger.Printf("⚠️ Redis get error for key %s: %v", key, err)
		}
		return err
	}

	err = json.Unmarshal([]byte(val), dest)
	if err != nil {
		cs.logger.Printf("❌ JSON unmarshal error for key %s: %v", key, err)
		return err
	}

	cs.logger.Printf("💾 Cache hit for %s", key)
	return nil
}

// Delete removes a value from Redis
func (cs *CacheService) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}

	err := cs.client.Del(ctx, keys...).Err()
	if err != nil && err != redis.Nil {
		cs.logger.Printf("⚠️ Failed to delete keys: %v", err)
		return err
	}

	cs.logger.Printf("🗑️  Deleted %d cache keys", len(keys))
	return nil
}

// Exists checks if a key exists in Redis
func (cs *CacheService) Exists(ctx context.Context, keys ...string) (int64, error) {
	if len(keys) == 0 {
		return 0, nil
	}

	count, err := cs.client.Exists(ctx, keys...).Result()
	if err != nil {
		cs.logger.Printf("⚠️ Failed to check key existence: %v", err)
		return 0, err
	}

	return count, nil
}

// Incr increments a counter (useful for likes, saves, views)
func (cs *CacheService) Incr(ctx context.Context, key string) (int64, error) {
	val, err := cs.client.Incr(ctx, key).Result()
	if err != nil {
		cs.logger.Printf("⚠️ Failed to increment key %s: %v", key, err)
		return 0, err
	}
	return val, nil
}

// Decr decrements a counter
func (cs *CacheService) Decr(ctx context.Context, key string) (int64, error) {
	val, err := cs.client.Decr(ctx, key).Result()
	if err != nil {
		cs.logger.Printf("⚠️ Failed to decrement key %s: %v", key, err)
		return 0, err
	}
	return val, nil
}

// GetInt64 retrieves an integer value from Redis
func (cs *CacheService) GetInt64(ctx context.Context, key string) (int64, error) {
	val, err := cs.client.Get(ctx, key).Int64()
	if err == redis.Nil {
		return 0, nil
	}
	if err != nil {
		cs.logger.Printf("⚠️ Failed to get int64 for key %s: %v", key, err)
		return 0, err
	}
	return val, nil
}

// ─────────────────────────────────────────────────────────────────
// Batch Operations (Pipelining)
// ─────────────────────────────────────────────────────────────────

// MGet retrieves multiple values from Redis
func (cs *CacheService) MGet(ctx context.Context, keys []string) ([]interface{}, error) {
	if len(keys) == 0 {
		return []interface{}{}, nil
	}

	vals, err := cs.client.MGet(ctx, keys...).Result()
	if err != nil {
		cs.logger.Printf("⚠️ Failed to get multiple keys: %v", err)
		return nil, err
	}

	return vals, nil
}

// MSet sets multiple key-value pairs
func (cs *CacheService) MSet(ctx context.Context, keyValues ...interface{}) error {
	if len(keyValues) == 0 {
		return nil
	}

	err := cs.client.MSet(ctx, keyValues...).Err()
	if err != nil {
		cs.logger.Printf("⚠️ Failed to set multiple keys: %v", err)
		return err
	}

	return nil
}

// ─────────────────────────────────────────────────────────────────
// Cache Pattern Deletion (for invalidation)
// ─────────────────────────────────────────────────────────────────

// DeletePattern deletes all keys matching a pattern (expensive in production)
// Use with caution - only in cleanup operations
func (cs *CacheService) DeletePattern(ctx context.Context, pattern string) error {
	iter := cs.client.Scan(ctx, 0, pattern, 0).Iterator()
	var keys []string

	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}

	if err := iter.Err(); err != nil {
		cs.logger.Printf("⚠️ Scan error: %v", err)
		return err
	}

	if len(keys) > 0 {
		return cs.Delete(ctx, keys...)
	}

	return nil
}

// ─────────────────────────────────────────────────────────────────
// Monitoring & Statistics
// ─────────────────────────────────────────────────────────────────

// GetStats returns Redis statistics
func (cs *CacheService) GetStats(ctx context.Context) (map[string]interface{}, error) {
	info, err := cs.client.Info(ctx, "stats", "memory").Result()
	if err != nil {
		cs.logger.Printf("⚠️ Failed to get Redis stats: %v", err)
		return nil, err
	}

	stats := map[string]interface{}{
		"info": info,
		"timestamp": time.Now(),
	}

	return stats, nil
}

// Ping checks if Redis is healthy
func (cs *CacheService) Ping(ctx context.Context) error {
	pong, err := cs.client.Ping(ctx).Result()
	if err != nil {
		cs.logger.Printf("❌ Redis ping failed: %v", err)
		return err
	}

	cs.logger.Printf("✅ Redis ping: %s", pong)
	return nil
}

// ─────────────────────────────────────────────────────────────────
// TTL Management
// ─────────────────────────────────────────────────────────────────

// SetExpire sets expiration on an existing key
func (cs *CacheService) SetExpire(ctx context.Context, key string, ttl time.Duration) error {
	err := cs.client.Expire(ctx, key, ttl).Err()
	if err != nil {
		cs.logger.Printf("⚠️ Failed to set expiration for key %s: %v", key, err)
		return err
	}
	return nil
}

// GetTTL gets remaining TTL for a key
func (cs *CacheService) GetTTL(ctx context.Context, key string) (time.Duration, error) {
	ttl, err := cs.client.TTL(ctx, key).Result()
	if err != nil {
		return 0, err
	}
	return ttl, nil
}

// PersistKey removes expiration from a key (makes it permanent)
func (cs *CacheService) PersistKey(ctx context.Context, key string) error {
	err := cs.client.Persist(ctx, key).Err()
	if err != nil {
		cs.logger.Printf("⚠️ Failed to persist key %s: %v", key, err)
		return err
	}
	return nil
}

// ─────────────────────────────────────────────────────────────────
// Utility Helpers
// ─────────────────────────────────────────────────────────────────

// FormatKey formats a cache key with parameters
func FormatKey(template string, args ...interface{}) string {
	return fmt.Sprintf(template, args...)
}

// GetCacheKeyPrefix returns the prefix for bulk operations
func GetCacheKeyPrefix(domain string) string {
	return fmt.Sprintf("*%s*", domain)
}

// InvalidateCacheForProperty invalidates all caches related to a property
func (cs *CacheService) InvalidateCacheForProperty(ctx context.Context, propertyID uint) error {
	keysToDelete := []string{
		FormatKey(PropertyDetailsKey, propertyID),
		FormatKey(PropertyImagesKey, propertyID),
		FormatKey(PropertyMetadataKey, propertyID),
		FormatKey(PropertyLocationKey, propertyID),
		FormatKey(PropertySaleKey, propertyID),
	}

	// Also invalidate pagination caches (pattern match would be expensive)
	// Client should handle this separately with DeletePattern if needed

	return cs.Delete(ctx, keysToDelete...)
}

// InvalidateVideoFeedCaches invalidates video feed caches
func (cs *CacheService) InvalidateVideoFeedCaches(ctx context.Context) error {
	return cs.DeletePattern(ctx, "feed:videos:*")
}

// InvalidatePropertySalesCaches invalidates property sales caches
func (cs *CacheService) InvalidatePropertySalesCaches(ctx context.Context) error {
	return cs.DeletePattern(ctx, "property:sales:*")
}

// Close closes the Redis connection
func (cs *CacheService) Close() error {
	return cs.client.Close()
}
