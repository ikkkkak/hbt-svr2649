package routes

import (
	"apartments-clone-server/models"
	"apartments-clone-server/storage"
	"encoding/json"
	"strings"
	"time"

	"github.com/kataras/iris/v12"
)

type moderationPendingItem struct {
	Kind      string    `json:"kind"` // rent | sale | land
	ID        uint      `json:"id"`
	Title     string    `json:"title"`
	ImageURL  string    `json:"image_url"`
	City      string    `json:"city"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GET /admin/moderation/pending
// Returns counts + recent pending listings for admin alerts (dashboard toast, sidebar badges).
// Optional query: since=RFC3339 — only items created/updated after that time (for "new since last refresh").
func AdminModerationPending(ctx iris.Context) {
	var since *time.Time
	if raw := strings.TrimSpace(ctx.URLParam("since")); raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			since = &t
		}
	}

	var pendingRent int64
	storage.DB.Model(&models.Property{}).
		Where("LOWER(status) = ?", "pending").
		Count(&pendingRent)

	var pendingSale int64
	storage.DB.Model(&models.PropertySale{}).
		Where("is_verified = ? AND LOWER(status) IN ?", false, []string{"draft", "pending_verification"}).
		Count(&pendingSale)

	var pendingLand int64
	storage.DB.Model(&models.Landmark{}).
		Where("LOWER(status) = ?", "pending_verification").
		Count(&pendingLand)

	items := make([]moderationPendingItem, 0, 48)
	items = append(items, fetchPendingRentItems(since, 16)...)
	items = append(items, fetchPendingSaleItems(since, 16)...)
	items = append(items, fetchPendingLandItems(since, 16)...)

	// Sort merged list by updated_at desc (simple in-memory sort).
	for i := 0; i < len(items); i++ {
		for j := i + 1; j < len(items); j++ {
			if items[j].UpdatedAt.After(items[i].UpdatedAt) {
				items[i], items[j] = items[j], items[i]
			}
		}
	}
	if len(items) > 24 {
		items = items[:24]
	}

	ctx.JSON(iris.Map{
		"data": iris.Map{
			"counts": iris.Map{
				"rent": pendingRent,
				"sale": pendingSale,
				"land": pendingLand,
				"total": pendingRent + pendingSale + pendingLand,
			},
			"items": items,
		},
		"meta":  iris.Map{},
		"links": iris.Map{},
	})
}

func fetchPendingRentItems(since *time.Time, limit int) []moderationPendingItem {
	var rows []models.Property
	q := storage.DB.Where("LOWER(status) = ?", "pending").Order("updated_at DESC").Limit(limit)
	if since != nil {
		q = q.Where("updated_at > ?", *since)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil
	}
	out := make([]moderationPendingItem, 0, len(rows))
	for _, p := range rows {
		out = append(out, moderationPendingItem{
			Kind:      "rent",
			ID:        p.ID,
			Title:     strings.TrimSpace(p.Title),
			ImageURL:  firstPropertyImage(p.Images),
			City:      strings.TrimSpace(p.City),
			Status:    p.Status,
			CreatedAt: p.CreatedAt,
			UpdatedAt: p.UpdatedAt,
		})
	}
	return out
}

func fetchPendingSaleItems(since *time.Time, limit int) []moderationPendingItem {
	var rows []models.PropertySale
	q := storage.DB.
		Where("is_verified = ? AND LOWER(status) IN ?", false, []string{"draft", "pending_verification"}).
		Order("updated_at DESC").
		Limit(limit)
	if since != nil {
		q = q.Where("updated_at > ?", *since)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil
	}
	out := make([]moderationPendingItem, 0, len(rows))
	for _, p := range rows {
		img := ""
		if len(p.Images) > 0 {
			img = strings.TrimSpace(p.Images[0])
		}
		out = append(out, moderationPendingItem{
			Kind:      "sale",
			ID:        p.ID,
			Title:     strings.TrimSpace(p.Title),
			ImageURL:  img,
			City:      strings.TrimSpace(p.City),
			Status:    p.Status,
			CreatedAt: p.CreatedAt,
			UpdatedAt: p.UpdatedAt,
		})
	}
	return out
}

func fetchPendingLandItems(since *time.Time, limit int) []moderationPendingItem {
	var rows []models.Landmark
	q := storage.DB.Where("LOWER(status) = ?", "pending_verification").Order("updated_at DESC").Limit(limit)
	if since != nil {
		q = q.Where("updated_at > ?", *since)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil
	}
	out := make([]moderationPendingItem, 0, len(rows))
	for _, l := range rows {
		out = append(out, moderationPendingItem{
			Kind:      "land",
			ID:        l.ID,
			Title:     strings.TrimSpace(l.Title),
			ImageURL:  firstLandmarkImage(l.Images),
			City:      strings.TrimSpace(l.District),
			Status:    l.Status,
			CreatedAt: l.CreatedAt,
			UpdatedAt: l.UpdatedAt,
		})
	}
	return out
}

func firstPropertyImage(imagesJSON string) string {
	if imagesJSON == "" {
		return ""
	}
	var urls []string
	if err := json.Unmarshal([]byte(imagesJSON), &urls); err == nil && len(urls) > 0 {
		return strings.TrimSpace(urls[0])
	}
	return ""
}

func firstLandmarkImage(raw []byte) string {
	if len(raw) == 0 {
		return ""
	}
	var urls []string
	if err := json.Unmarshal(raw, &urls); err == nil && len(urls) > 0 {
		return strings.TrimSpace(urls[0])
	}
	return ""
}
