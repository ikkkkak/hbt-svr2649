# 🚀 Redis Caching Infrastructure - Complete Implementation Guide

## Overview

This document provides a comprehensive guide to the production-grade Redis caching layer implemented for the apartments-clone application.

**Objective**: Ensure instant content loading for videos, property listings, and property details through intelligent caching strategies.

---

## 🏗️ Architecture

### Component Structure

```
┌─────────────────────────────────────────────────────────────────┐
│                     Application Layer                           │
│  (Routes / Controllers / API Handlers)                          │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│               Specialized Cache Services                        │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ VideoFeedCacheService     - Video feed caching           │  │
│  │ PropertySalesCacheService - Property list/detail cache   │  │
│  │ (Extensible for other domains)                           │  │
│  └──────────────────────────────────────────────────────────┘  │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│              Base Cache Service (CacheService)                  │
│  ┌──────────────────────────────────────────────────────────┐  │
│  │ Generic Operations                                       │  │
│  │ • Set/Get/Delete                                         │  │
│  │ • Increment/Decrement (counters)                         │  │
│  │ • Batch Operations (MGet/MSet)                           │  │
│  │ • Pattern Deletion & TTL Management                      │  │
│  └──────────────────────────────────────────────────────────┘  │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Redis Client                                 │
│  (go-redis/v9 library with connection pooling)                  │
└────────────────────────┬────────────────────────────────────────┘
                         │
                         ▼
┌─────────────────────────────────────────────────────────────────┐
│                    Redis Server                                 │
│  (Managed service or local instance)                            │
└─────────────────────────────────────────────────────────────────┘
```

### File Structure

```
apartmentscloneserver/
├── services/
│   ├── cacheService.go                    ← Base cache abstraction
│   ├── videoFeedCacheService.go           ← Video feed caching
│   ├── propertySalesCacheService.go       ← Property sales caching
│   └── [other services]
├── storage/
│   └── redis.go                           ← Redis initialization
├── routes/
│   ├── video.go                           ← Use VideoFeedCacheService
│   ├── propertySales.go                   ← Use PropertySalesCacheService
│   └── [other routes]
├── main.go                                ← Redis initialization
└── .env.redis                             ← Configuration template
```

---

## 📋 Cache Configuration

### Environment Variables

```bash
# Redis Connection
REDIS_HOST=localhost              # Redis server host
REDIS_PORT=6379                   # Redis server port
REDIS_PASSWORD=                   # Auth password (empty for dev)
REDIS_DB=0                        # Database number

# Cache TTLs (in minutes)
CACHE_TTL_VIDEO=15                # Video feed: 15 minutes
CACHE_TTL_PROPERTY_LIST=10        # Property list: 10 minutes
CACHE_TTL_PROPERTY_DETAILS=30     # Property details: 30 minutes
CACHE_TTL_DEFAULT=5               # Default: 5 minutes

# Advanced Settings
REDIS_MAX_RETRIES=3               # Retry failed operations
REDIS_POOL_SIZE=10                # Connection pool size
ENABLE_CACHE_PRELOAD=true         # Preload on startup
ENABLE_CACHE_MONITORING=true      # Log cache stats
```

### Loading Configuration

```go
// In main.go
config := services.CacheConfig{
    VideoFeedTTL:       15 * time.Minute,
    PropertyListTTL:    10 * time.Minute,
    PropertyDetailsTTL: 30 * time.Minute,
    DefaultTTL:         5 * time.Minute,
}
```

---

## 🎥 Video Feed Caching

### Cache Keys

| Key Template                          | Example                       | Purpose                      |
| ------------------------------------- | ----------------------------- | ---------------------------- |
| `feed:videos:page:{userId}:{pageNum}` | `feed:videos:page:user_123:1` | Cached video feed page       |
| `video:meta:{videoId}`                | `video:meta:456`              | Video metadata (lightweight) |
| `video:likes:{videoId}`               | `video:likes:456`             | Like count (counter)         |
| `video:saves:{videoId}`               | `video:saves:456`             | Save count (counter)         |

### Usage Example

```go
// Initialize service
vfCache := services.NewVideoFeedCacheService(cacheService, config)

// Get cached feed
ctx := context.Background()
cached, err := vfCache.GetVideoFeedFromCache(ctx, userID, pageNum)
if err == nil && cached != nil {
    // Return cached data
    return cached.Videos, nil
}

// If no cache, query database
videos := queryVideosFromDB(pageNum)

// Cache the results
vfCache.SetVideoFeedCache(ctx, userID, pageNum, videos, nextCursor)

// Return data
return videos, nil
```

### Engagement Counters

```go
// Increment like count
count, err := vfCache.IncrementVideoLikeCount(ctx, videoID)
// Result: count is updated in real-time

// Decrement on unlike
count, err := vfCache.DecrementVideoLikeCount(ctx, videoID)

// Get current count
count, err := vfCache.GetVideoLikeCount(ctx, videoID)
```

### Preloading Strategy

```go
// On app startup - preload first page of videos
if preloadEnabled {
    vfCache.PreloadVideoFeed(ctx, userID)
}
```

**TTL**: 15 minutes (refreshes when stale)

---

## 🏠 Property Sales List Caching

### Cache Keys

| Key Template                            | Example                              | Purpose        |
| --------------------------------------- | ------------------------------------ | -------------- |
| `property:sales:page:{pageNum}:{limit}` | `property:sales:page:1:10`           | Paginated list |
| `property:details:{propertyId}`         | `property:details:789`               | Full details   |
| `property:sales:search:{hash}`          | `property:sales:search:abc123def456` | Search results |

### Usage Example

```go
// Get property sales list from cache
pCache := services.NewPropertySalesCacheService(cacheService, config)

cached, err := pCache.GetPropertySalesListFromCache(ctx, pageNum, limit)
if cached != nil {
    return cached.Properties, nil
}

// Query database if not cached
properties := queryPropertiesFromDB(pageNum, limit)

// Cache results
pCache.SetPropertySalesListCache(ctx, pageNum, limit, properties, nextCursor, totalCount)

return properties, nil
```

### Property Details Caching

```go
// Get full property details from cache
details, err := pCache.GetPropertySalesDetailsFromCache(ctx, propertyID)
if details != nil {
    return details, nil
}

// Query and cache
property := queryPropertyFromDB(propertyID)
pCache.SetPropertySalesDetailsCache(ctx, property)

return property, nil
```

### Search Result Caching

```go
// Cache search results with query hashing
searchQuery := map[string]interface{}{
    "city": "New York",
    "min_price": 100000,
    "max_price": 500000,
}

cached, err := pCache.GetPropertySearchFromCache(ctx, searchQuery)
if cached != nil {
    return cached.Properties, nil
}

// Search and cache
results := searchProperties(searchQuery)
pCache.SetPropertySearchCache(ctx, searchQuery, results)

return results, nil
```

**TTL**: 10 minutes (property list), 30 minutes (details)

---

## 🔄 Cache Invalidation Strategy

### When to Invalidate

#### Video Feed

- **On video upload**: Invalidate `feed:videos:page:*`
- **On video deletion**: Invalidate relevant video keys
- **Manual refresh**: User pulls to refresh

#### Property Sales

- **On property created**: Invalidate `property:sales:page:*`
- **On property updated**: Invalidate `property:details:{id}`
- **On property deleted**: Invalidate `property:details:{id}`
- **On price change**: Invalidate `property:details:{id}`
- **On status change**: Invalidate `property:details:{id}`

### Invalidation Implementation

```go
// In routes/propertySales.go

// When updating a property
err := db.Save(&property).Error
if err == nil {
    // Invalidate cache immediately
    pCache.InvalidatePropertyDetails(ctx, property.ID)

    // Also invalidate list caches (property lists include this item)
    pCache.InvalidatePropertySalesLists(ctx)
}

// When creating new property
err := db.Create(&newProperty).Error
if err == nil {
    // Invalidate first N pages of listings
    pCache.InvalidatePropertySalesLists(ctx)
}

// When deleting property
err := db.Delete(&property).Error
if err == nil {
    pCache.InvalidatePropertyDetails(ctx, property.ID)
    pCache.InvalidatePropertySalesLists(ctx)
}
```

---

## ⚙️ Redis Setup & Deployment

### Option A: Local Development

```bash
# Install Redis
# macOS
brew install redis

# Ubuntu
sudo apt-get install redis-server

# Start Redis
redis-server

# Test connection
redis-cli ping  # Should output: PONG
```

### Option B: Docker Compose

```yaml
# docker-compose.yml
version: "3.8"

services:
  redis:
    image: redis:7-alpine
    ports:
      - "6379:6379"
    volumes:
      - redis-data:/data
    command: redis-server --appendonly yes
    environment:
      - REDIS_PASSWORD=${REDIS_PASSWORD}

  app:
    build: .
    depends_on:
      - redis
    environment:
      REDIS_HOST: redis
      REDIS_PORT: 6379
    ports:
      - "8080:8080"

volumes:
  redis-data:
```

Start with:

```bash
docker-compose up
```

### Option C: Managed Redis (Production)

**Render.com**:

1. Create Redis instance from Render dashboard
2. Copy connection URL
3. Set environment variables:
   ```
   REDIS_HOST=redis-xxxxx.render.com
   REDIS_PORT=6379
   REDIS_PASSWORD=your_password
   ```

**AWS ElastiCache**:

1. Create Redis cluster
2. Set endpoint as `REDIS_HOST`
3. Enable in-transit encryption (TLS)

**Azure Cache for Redis**:

1. Create instance
2. Copy primary connection string
3. Parse host, port, password from connection string

---

## 📊 Performance Metrics & Monitoring

### Cache Hit Ratio Monitoring

```go
// In cache service
type CacheStats struct {
    Hits       int64
    Misses     int64
    HitRatio   float64
    TotalOps   int64
    Errors     int64
}

func (cs *CacheService) GetCacheStats(ctx context.Context) (*CacheStats, error) {
    info, err := cs.client.Info(ctx, "stats").Result()
    // Parse info and calculate hit ratio
    // hitRatio = hits / (hits + misses)
}
```

### Expected Performance Gains

| Metric                | Before Cache        | After Cache      | Improvement    |
| --------------------- | ------------------- | ---------------- | -------------- |
| Video Feed Load       | 800-1200ms          | 50-150ms         | **85-95%** ↓   |
| Property List Load    | 600-1000ms          | 40-100ms         | **85-95%** ↓   |
| Property Details Load | 400-800ms           | 20-60ms          | **85-95%** ↓   |
| DB Query Count        | ~10/request         | ~1/request       | **90%** ↓      |
| Memory Usage          | ~500MB              | ~600MB           | +100MB (Redis) |
| Network Cost          | High (many DB hits) | Low (cache hits) | **80%** ↓      |

---

## 🛠️ Integration with Routes

### Video Routes (routes/video.go)

```go
func GetVideoFeed(ctx iris.Context) {
    userID := ctx.Values().GetUint("userID")
    pageNum := ctx.Query("page", "1")

    // Try cache first
    vfCache := services.NewVideoFeedCacheService(cacheService, config)
    cached, _ := vfCache.GetVideoFeedFromCache(context.Background(), userID, pageNum)
    if cached != nil {
        ctx.JSON(iris.StatusOK, cached)
        return
    }

    // Query database if not cached
    videos := queryVideosFromDB(pageNum)

    // Cache results
    vfCache.SetVideoFeedCache(context.Background(), userID, pageNum, videos, nextCursor)

    ctx.JSON(iris.StatusOK, videos)
}
```

### Property Routes (routes/propertySales.go)

```go
func GetPropertySalesList(ctx iris.Context) {
    pageNum := ctx.Query("page", "1")
    limit := ctx.Query("limit", "10")

    pCache := services.NewPropertySalesCacheService(cacheService, config)

    // Try cache
    cached, _ := pCache.GetPropertySalesListFromCache(
        context.Background(),
        pageNum,
        limit,
    )
    if cached != nil {
        ctx.JSON(iris.StatusOK, cached)
        return
    }

    // Query and cache
    properties := queryPropertiesFromDB(pageNum, limit)
    pCache.SetPropertySalesListCache(
        context.Background(),
        pageNum,
        limit,
        properties,
        nextCursor,
        totalCount,
    )

    ctx.JSON(iris.StatusOK, properties)
}

func UpdatePropertySale(ctx iris.Context) {
    propertyID := ctx.Params().GetUint("id")

    // Update database
    err := db.Model(&property).Updates(updates).Error
    if err != nil {
        ctx.StopWithStatus(iris.StatusInternalServerError)
        return
    }

    // ✅ CRITICAL: Invalidate cache immediately
    pCache := services.NewPropertySalesCacheService(cacheService, config)
    pCache.InvalidatePropertyDetails(context.Background(), propertyID)
    pCache.InvalidatePropertySalesLists(context.Background())

    ctx.JSON(iris.StatusOK, property)
}
```

---

## 🔍 Debugging & Troubleshooting

### Check Redis Connection

```bash
# Test Redis ping
redis-cli ping
# Output: PONG

# Check memory usage
redis-cli info memory
# Output: used_memory, used_memory_human, etc.

# List all keys
redis-cli keys "*"

# Get specific key
redis-cli get "feed:videos:page:user_123:1"

# Monitor in real-time
redis-cli monitor
```

### Common Issues

| Issue                    | Cause                       | Solution                         |
| ------------------------ | --------------------------- | -------------------------------- |
| Redis Connection Refused | Redis not running           | `redis-server` or check Docker   |
| Cache Always Empty       | TTL expired too quickly     | Increase TTL in config           |
| Stale Data               | Not invalidating on updates | Add invalidation to update route |
| Memory Full              | Too much cached data        | Increase memory or reduce TTL    |
| Slow Performance         | Connection pool exhausted   | Increase REDIS_POOL_SIZE         |

---

## 🚀 Deployment Checklist

- [ ] Redis instance is running and accessible
- [ ] Environment variables configured correctly
- [ ] Connection pooling enabled (REDIS_POOL_SIZE > 5)
- [ ] TTLs configured appropriately
- [ ] Cache invalidation added to update/delete routes
- [ ] Monitoring enabled (ENABLE_CACHE_MONITORING=true)
- [ ] Preloading enabled (ENABLE_CACHE_PRELOAD=true)
- [ ] Error handling implemented (fallback to DB on cache miss)
- [ ] Load testing completed
- [ ] Memory monitoring in place

---

## 📈 Future Enhancements

1. **Cache Warming**: Periodically refresh caches before TTL expires
2. **Distributed Caching**: Support Redis Cluster for scale
3. **Cache Compression**: Compress large cached objects
4. **Bloom Filters**: Prevent cache stampede on misses
5. **Analytics Dashboard**: Real-time cache metrics visualization
6. **Adaptive TTL**: Auto-adjust TTL based on hit ratio
7. **Cache Replication**: Redis Sentinel for HA

---

## 📚 References

- [go-redis/v9 Documentation](https://redis.uptrace.dev/)
- [Redis Commands](https://redis.io/commands/)
- [Redis Best Practices](https://redis.io/docs/management/persistence/)

---

**Last Updated**: February 21, 2026  
**Version**: 1.0 Production  
**Status**: Ready for Deployment ✅
