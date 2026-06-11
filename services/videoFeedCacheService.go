package services

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"context"
	"fmt"
	"log"
	"time"
)

// VideoFeedCacheService handles caching for video feeds
type VideoFeedCacheService struct {
	cache  *CacheService
	config CacheConfig
	logger *log.Logger
}

// NewVideoFeedCacheService creates a new video feed cache service
func NewVideoFeedCacheService(cache *CacheService, config CacheConfig) *VideoFeedCacheService {
	return &VideoFeedCacheService{
		cache:  cache,
		config: config,
		logger: log.New(log.Writer(), "[VideoFeedCache] ", log.LstdFlags|log.Lshortfile),
	}
}

// ─────────────────────────────────────────────────────────────────
// Video Feed Caching
// ─────────────────────────────────────────────────────────────────

// CachedVideoFeed represents cached video feed data
type CachedVideoFeed struct {
	Videos    []models.Video `json:"videos"`
	NextCursor string         `json:"next_cursor"`
	CachedAt  time.Time      `json:"cached_at"`
}

// GetVideoFeedFromCache retrieves cached video feed
func (vfc *VideoFeedCacheService) GetVideoFeedFromCache(ctx context.Context, userID uint, pageNum int) (*CachedVideoFeed, error) {
	key := FormatKey(VideoFeedPageKey, fmt.Sprintf("user_%d", userID), pageNum)

	var cached CachedVideoFeed
	err := vfc.cache.Get(ctx, key, &cached)
	if err != nil {
		return nil, err
	}

	// Cache miss if no videos found
	if len(cached.Videos) == 0 {
		return nil, nil
	}

	vfc.logger.Printf("💾 Video feed cache hit: user=%d, page=%d, count=%d", userID, pageNum, len(cached.Videos))
	return &cached, nil
}

// SetVideoFeedCache caches video feed data
func (vfc *VideoFeedCacheService) SetVideoFeedCache(ctx context.Context, userID uint, pageNum int, videos []models.Video, nextCursor string) error {
	key := FormatKey(VideoFeedPageKey, fmt.Sprintf("user_%d", userID), pageNum)

	cached := CachedVideoFeed{
		Videos:    videos,
		NextCursor: nextCursor,
		CachedAt:  time.Now(),
	}

	err := vfc.cache.Set(ctx, key, cached, vfc.config.VideoFeedTTL)
	if err != nil {
		vfc.logger.Printf("⚠️ Failed to cache video feed: %v", err)
		return err
	}

	vfc.logger.Printf("✅ Cached video feed: user=%d, page=%d, videos=%d", userID, pageNum, len(videos))
	return nil
}

// ─────────────────────────────────────────────────────────────────
// Individual Video Caching (Metadata + Engagement)
// ─────────────────────────────────────────────────────────────────

// CachedVideoMetadata represents cached video metadata (lightweight)
type CachedVideoMetadata struct {
	ID            uint      `json:"id"`
	UserID        uint      `json:"user_id"`
	PropertyID    uint      `json:"property_id"`
	VideoURL      string    `json:"video_url"`
	ThumbnailURL  string    `json:"thumbnail_url"`
	Title         string    `json:"title"`
	Description   string    `json:"description"`
	Duration      int       `json:"duration"`
	LikeCount     int64     `json:"like_count"`
	SaveCount     int64     `json:"save_count"`
	ViewCount     int64     `json:"view_count"`
	CreatedAt     time.Time `json:"created_at"`
}

// GetVideoMetadataFromCache retrieves cached video metadata
func (vfc *VideoFeedCacheService) GetVideoMetadataFromCache(ctx context.Context, videoID uint) (*CachedVideoMetadata, error) {
	key := FormatKey(VideoMetadataKey, videoID)

	var cached CachedVideoMetadata
	err := vfc.cache.Get(ctx, key, &cached)
	if err != nil {
		return nil, err
	}

	if cached.ID == 0 {
		return nil, nil
	}

	vfc.logger.Printf("💾 Video metadata cache hit: video=%d", videoID)
	return &cached, nil
}

// SetVideoMetadataCache caches video metadata
func (vfc *VideoFeedCacheService) SetVideoMetadataCache(ctx context.Context, video *models.Video) error {
	key := FormatKey(VideoMetadataKey, video.ID)

	var propertyID uint
	if video.PropertyID != nil {
		propertyID = *video.PropertyID
	}

	metadata := CachedVideoMetadata{
		ID:           video.ID,
		UserID:       video.UserID,
		PropertyID:   propertyID,
		VideoURL:     video.VideoURL,
		ThumbnailURL: video.ThumbnailURL,
		Title:        video.Title,
		Description:  video.Description,
		Duration:     int(video.DurationSec),
		CreatedAt:    video.CreatedAt,
	}

	err := vfc.cache.Set(ctx, key, metadata, vfc.config.VideoFeedTTL)
	if err != nil {
		vfc.logger.Printf("⚠️ Failed to cache video metadata: %v", err)
		return err
	}

	vfc.logger.Printf("✅ Cached video metadata: video=%d", video.ID)
	return nil
}

// GetVideoLikeCount retrieves cached like count for a video
func (vfc *VideoFeedCacheService) GetVideoLikeCount(ctx context.Context, videoID uint) (int64, error) {
	key := FormatKey(VideoLikeCountKey, videoID)

	count, err := vfc.cache.GetInt64(ctx, key)
	if err != nil {
		return 0, err
	}

	if count > 0 {
		vfc.logger.Printf("💾 Video like count cache hit: video=%d, likes=%d", videoID, count)
	}

	return count, nil
}

// IncrementVideoLikeCount increments video like counter
func (vfc *VideoFeedCacheService) IncrementVideoLikeCount(ctx context.Context, videoID uint) (int64, error) {
	key := FormatKey(VideoLikeCountKey, videoID)

	count, err := vfc.cache.Incr(ctx, key)
	if err != nil {
		return 0, err
	}

	// Set expiration if not already set
	vfc.cache.SetExpire(ctx, key, vfc.config.VideoFeedTTL)

	vfc.logger.Printf("👍 Video like count incremented: video=%d, total=%d", videoID, count)
	return count, nil
}

// DecrementVideoLikeCount decrements video like counter
func (vfc *VideoFeedCacheService) DecrementVideoLikeCount(ctx context.Context, videoID uint) (int64, error) {
	key := FormatKey(VideoLikeCountKey, videoID)

	count, err := vfc.cache.Decr(ctx, key)
	if err != nil {
		return 0, err
	}

	vfc.logger.Printf("👎 Video like count decremented: video=%d, total=%d", videoID, count)
	return count, nil
}

// GetVideoSaveCount retrieves cached save count for a video
func (vfc *VideoFeedCacheService) GetVideoSaveCount(ctx context.Context, videoID uint) (int64, error) {
	key := FormatKey(VideoSaveCountKey, videoID)
	count, err := vfc.cache.GetInt64(ctx, key)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// IncrementVideoSaveCount increments video save counter
func (vfc *VideoFeedCacheService) IncrementVideoSaveCount(ctx context.Context, videoID uint) (int64, error) {
	key := FormatKey(VideoSaveCountKey, videoID)
	count, err := vfc.cache.Incr(ctx, key)
	if err != nil {
		return 0, err
	}
	vfc.cache.SetExpire(ctx, key, vfc.config.VideoFeedTTL)
	return count, nil
}

// DecrementVideoSaveCount decrements video save counter
func (vfc *VideoFeedCacheService) DecrementVideoSaveCount(ctx context.Context, videoID uint) (int64, error) {
	key := FormatKey(VideoSaveCountKey, videoID)
	count, err := vfc.cache.Decr(ctx, key)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// ─────────────────────────────────────────────────────────────────
// Prefetching Strategy
// ─────────────────────────────────────────────────────────────────

// PreloadVideoFeed preloads the first page of video feed (called on app start)
func (vfc *VideoFeedCacheService) PreloadVideoFeed(ctx context.Context, userID uint) error {
	vfc.logger.Printf("🔄 Starting video feed preload for user=%d", userID)

	// Query first 10 videos from database
	var videos []models.Video
	db := storage.DB

	err := db.WithContext(ctx).
		Where("status = ?", "approved").
		Order("created_at DESC").
		Limit(10).
		Find(&videos).Error

	if err != nil {
		vfc.logger.Printf("❌ Failed to preload video feed: %v", err)
		return err
	}

	// Cache the videos
	err = vfc.SetVideoFeedCache(ctx, userID, 1, videos, "")
	if err != nil {
		vfc.logger.Printf("⚠️ Failed to cache preloaded videos: %v", err)
		return err
	}

	vfc.logger.Printf("✅ Video feed preloaded: %d videos cached", len(videos))
	return nil
}

// ─────────────────────────────────────────────────────────────────
// Cache Invalidation
// ─────────────────────────────────────────────────────────────────

// InvalidateVideoFeed invalidates all video feed caches
func (vfc *VideoFeedCacheService) InvalidateVideoFeed(ctx context.Context) error {
	return vfc.cache.InvalidateVideoFeedCaches(ctx)
}

// InvalidateVideoMetadata invalidates metadata for a specific video
func (vfc *VideoFeedCacheService) InvalidateVideoMetadata(ctx context.Context, videoID uint) error {
	keys := []string{
		FormatKey(VideoMetadataKey, videoID),
		FormatKey(VideoLikeCountKey, videoID),
		FormatKey(VideoSaveCountKey, videoID),
	}
	return vfc.cache.Delete(ctx, keys...)
}
