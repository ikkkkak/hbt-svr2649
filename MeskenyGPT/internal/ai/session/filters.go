package session

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"apartments-clone-server/MeskenyGPT/internal/ai/lang"

	"github.com/go-redis/redis/v8"
)

const filterKeyPrefix = "meskenygpt:filters:"
const filterTTL = 30 * time.Minute

// FilterContext persists picker/search filters per session (Engineering Spec v2 §7).
type FilterContext struct {
	City      string    `json:"city,omitempty"`
	Zone      string    `json:"zone,omitempty"`
	Quartier  string    `json:"quartier,omitempty"`
	Type      string    `json:"type,omitempty"`
	Purpose   string    `json:"purpose,omitempty"` // rent | sale
	MinPrice  int64     `json:"min_price,omitempty"`
	MaxPrice  int64     `json:"max_price,omitempty"`
	Bedrooms  int       `json:"bedrooms,omitempty"`
	UpdatedAt time.Time `json:"updated_at,omitempty"`
}

func filterKey(sessionID string) string {
	return filterKeyPrefix + strings.TrimSpace(sessionID)
}

func FromMessageContext(ctx lang.MessageContext) FilterContext {
	fc := FilterContext{
		City:      strings.TrimSpace(ctx.City),
		Zone:      strings.TrimSpace(ctx.Zone),
		Quartier:  strings.TrimSpace(ctx.Quartier),
		Type:      strings.TrimSpace(ctx.Type),
		MinPrice:  ctx.BudgetMin,
		MaxPrice:  ctx.BudgetMax,
		UpdatedAt: time.Now(),
	}
	switch ctx.Intent {
	case lang.IntentSearchRent:
		fc.Purpose = "rent"
	case lang.IntentSearchBuy, lang.IntentSearchLand, lang.IntentSearchCommercial:
		fc.Purpose = "sale"
	}
	return fc
}

func MergeIntoContext(ctx lang.MessageContext, fc FilterContext) lang.MessageContext {
	if strings.TrimSpace(ctx.City) == "" && fc.City != "" {
		ctx.City = fc.City
	}
	if strings.TrimSpace(ctx.Zone) == "" && fc.Zone != "" {
		ctx.Zone = fc.Zone
	}
	if strings.TrimSpace(ctx.Quartier) == "" && fc.Quartier != "" {
		ctx.Quartier = fc.Quartier
	}
	if strings.TrimSpace(ctx.Type) == "" && fc.Type != "" {
		ctx.Type = fc.Type
	}
	ctx = ApplyPurposeFromFilter(ctx, fc.Purpose)
	if ctx.BudgetMin == 0 && fc.MinPrice > 0 {
		ctx.BudgetMin = fc.MinPrice
	}
	if ctx.BudgetMax == 0 && fc.MaxPrice > 0 {
		ctx.BudgetMax = fc.MaxPrice
	}
	return ctx
}

func SaveFilterContext(ctx context.Context, rdb *redis.Client, sessionID string, fc FilterContext) error {
	if rdb == nil || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	if fc.City == "" && fc.Zone == "" && fc.Quartier == "" && fc.Type == "" && fc.Purpose == "" && fc.MinPrice == 0 && fc.MaxPrice == 0 {
		return nil
	}
	fc.UpdatedAt = time.Now()
	b, err := json.Marshal(fc)
	if err != nil {
		return err
	}
	return rdb.Set(ctx, filterKey(sessionID), string(b), filterTTL).Err()
}

func LoadFilterContext(ctx context.Context, rdb *redis.Client, sessionID string) (FilterContext, bool) {
	if rdb == nil || strings.TrimSpace(sessionID) == "" {
		return FilterContext{}, false
	}
	raw, err := rdb.Get(ctx, filterKey(sessionID)).Result()
	if err != nil || strings.TrimSpace(raw) == "" {
		return FilterContext{}, false
	}
	var fc FilterContext
	if err := json.Unmarshal([]byte(raw), &fc); err != nil {
		return FilterContext{}, false
	}
	return fc, true
}

func UpdateFilterContext(ctx context.Context, rdb *redis.Client, sessionID string, patch FilterContext) (FilterContext, error) {
	fc, _ := LoadFilterContext(ctx, rdb, sessionID)
	if patch.City != "" {
		fc.City = patch.City
	}
	if patch.Zone != "" {
		fc.Zone = patch.Zone
	}
	if patch.Quartier != "" {
		fc.Quartier = patch.Quartier
	}
	if patch.Type != "" {
		fc.Type = patch.Type
	}
	if patch.Purpose != "" {
		fc.Purpose = patch.Purpose
	}
	if patch.MinPrice > 0 {
		fc.MinPrice = patch.MinPrice
	}
	if patch.MaxPrice > 0 {
		fc.MaxPrice = patch.MaxPrice
	}
	if patch.Bedrooms > 0 {
		fc.Bedrooms = patch.Bedrooms
	}
	return fc, SaveFilterContext(ctx, rdb, sessionID, fc)
}

// ApplyPurposeFromFilter restores rent/buy intent from a persisted session when the
// current message only mentions property type or location.
func ApplyPurposeFromFilter(ctx lang.MessageContext, purpose string) lang.MessageContext {
	purpose = strings.TrimSpace(strings.ToLower(purpose))
	if purpose == "" {
		return ctx
	}
	if lang.IsExplicitPurposeIntent(ctx.Intent) {
		return ctx
	}
	switch purpose {
	case "rent":
		ctx.Intent = lang.IntentSearchRent
	case "sale":
		t := strings.ToLower(strings.TrimSpace(ctx.Type))
		switch {
		case t == "land" || t == "terrain":
			ctx.Intent = lang.IntentSearchLand
		case t == "boutique" || t == "commercial" || strings.Contains(t, "commercial"):
			ctx.Intent = lang.IntentSearchCommercial
		default:
			ctx.Intent = lang.IntentSearchBuy
		}
	}
	return ctx
}
