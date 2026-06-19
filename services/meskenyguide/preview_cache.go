package meskenyguide

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

type previewCacheEntry struct {
	data map[uint]ListingGuidePreview
	exp  time.Time
}

var (
	previewCacheMu sync.RWMutex
	previewCache   = map[string]previewCacheEntry{}
)

func previewCacheKey(hostID uint, listingIDs []uint) string {
	ids := append([]uint(nil), listingIDs...)
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return fmt.Sprintf("%d:%v", hostID, ids)
}

func getCachedListingPreviews(hostID uint, listingIDs []uint) (map[uint]ListingGuidePreview, bool) {
	key := previewCacheKey(hostID, listingIDs)
	previewCacheMu.RLock()
	entry, ok := previewCache[key]
	previewCacheMu.RUnlock()
	if !ok || time.Now().After(entry.exp) {
		return nil, false
	}
	out := make(map[uint]ListingGuidePreview, len(entry.data))
	for k, v := range entry.data {
		out[k] = v
	}
	return out, true
}

func setCachedListingPreviews(hostID uint, listingIDs []uint, data map[uint]ListingGuidePreview) {
	if len(data) == 0 {
		return
	}
	key := previewCacheKey(hostID, listingIDs)
	cp := make(map[uint]ListingGuidePreview, len(data))
	for k, v := range data {
		cp[k] = v
	}
	previewCacheMu.Lock()
	previewCache[key] = previewCacheEntry{data: cp, exp: time.Now().Add(45 * time.Second)}
	previewCacheMu.Unlock()
}
