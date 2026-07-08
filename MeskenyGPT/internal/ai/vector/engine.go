package vector

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"apartments-clone-server/MeskenyGPT/internal/ai/property"
	"apartments-clone-server/models"

	"gorm.io/gorm"
)

// Engine orchestrates embeddings + Qdrant + DB hydration.
type Engine struct {
	cfg     Config
	db      *gorm.DB
	qdrant  *qdrantClient
	embed   *Embedder
	ready   bool
	initErr error
	once    sync.Once
}

func NewEngine(db *gorm.DB) *Engine {
	cfg := ConfigFromEnv()
	e := &Engine{
		cfg:    cfg,
		db:     db,
		qdrant: newQdrantClient(cfg.QdrantURL, cfg.Collection, cfg.EmbeddingDim),
		embed:  NewEmbedder(cfg.OpenRouterKey, cfg.EmbeddingModel),
	}
	return e
}

func (e *Engine) Enabled() bool {
	return e != nil && e.cfg.Enabled && e.cfg.OpenRouterKey != ""
}

func (e *Engine) init(ctx context.Context) {
	e.once.Do(func() {
		if !e.Enabled() {
			e.initErr = fmt.Errorf("semantic search disabled")
			return
		}
		if err := e.qdrant.EnsureCollection(ctx); err != nil {
			e.initErr = err
			log.Printf("⚠️ Qdrant init failed: %v", err)
			return
		}
		e.ready = true
		log.Printf("✅ Meskeny semantic search ready (collection=%s)", e.cfg.Collection)
	})
}

// Search runs semantic search and hydrates rows from Postgres.
func (e *Engine) Search(ctx context.Context, query string, f property.Filters, limit int) ([]property.Property, []float32, error) {
	if !e.Enabled() {
		return nil, nil, fmt.Errorf("semantic search disabled")
	}
	e.init(ctx)
	if !e.ready {
		if e.initErr != nil {
			return nil, nil, e.initErr
		}
		return nil, nil, fmt.Errorf("semantic search not ready")
	}
	if limit <= 0 {
		limit = 12
	}
	vec, err := e.embed.Embed(ctx, query)
	if err != nil {
		return nil, nil, err
	}
	filter := buildQdrantFilter(f)
	hits, err := e.qdrant.Search(ctx, vec, limit, filter, e.cfg.ScoreThreshold)
	if err != nil {
		return nil, nil, err
	}
	scores := make(map[string]float32, len(hits))
	saleIDs, rentIDs, landIDs := make([]uint, 0), make([]uint, 0), make([]uint, 0)
	for _, h := range hits {
		idStr := fmt.Sprint(h.ID)
		if src, id, ok := ParsePointID(idStr); ok {
			scores[idStr] = h.Score
			switch src {
			case SourceSale:
				saleIDs = append(saleIDs, id)
			case SourceRent:
				rentIDs = append(rentIDs, id)
			case SourceLand:
				landIDs = append(landIDs, id)
			}
		}
	}
	props, err := e.hydrate(ctx, saleIDs, rentIDs, landIDs)
	if err != nil {
		return nil, nil, err
	}
	// Preserve Qdrant ranking
	ordered := reorderByHits(props, hits)
	return ordered, scoresForHits(ordered, hits), nil
}

func (e *Engine) Similar(ctx context.Context, source string, listingID uint, limit int) ([]property.Property, error) {
	if !e.Enabled() || listingID == 0 {
		return nil, fmt.Errorf("semantic search disabled")
	}
	e.init(ctx)
	if !e.ready {
		return nil, e.initErr
	}
	pointID := PointID(source, listingID)
	// Recommend via search with same vector — fetch point vector first via scroll/filter
	// Simpler: rebuild document from DB and embed it
	doc, err := e.documentFor(ctx, source, listingID)
	if err != nil {
		return nil, err
	}
	vec, err := e.embed.Embed(ctx, doc)
	if err != nil {
		return nil, err
	}
	filter := map[string]any{
		"must_not": []map[string]any{
			{"key": "listing_id", "match": map[string]any{"value": listingID}},
		},
	}
	hits, err := e.qdrant.Search(ctx, vec, limit+1, filter, e.cfg.ScoreThreshold)
	if err != nil {
		return nil, err
	}
	saleIDs, rentIDs, landIDs := make([]uint, 0), make([]uint, 0), make([]uint, 0)
	for _, h := range hits {
		idStr := fmt.Sprint(h.ID)
		if idStr == pointID {
			continue
		}
		if src, id, ok := ParsePointID(idStr); ok {
			switch src {
			case SourceSale:
				saleIDs = append(saleIDs, id)
			case SourceRent:
				rentIDs = append(rentIDs, id)
			case SourceLand:
				landIDs = append(landIDs, id)
			}
		}
	}
	props, err := e.hydrate(ctx, saleIDs, rentIDs, landIDs)
	if err != nil {
		return nil, err
	}
	return reorderByHits(props, hits), nil
}

func (e *Engine) IndexListing(ctx context.Context, source string, id uint) error {
	if !e.Enabled() || id == 0 {
		return nil
	}
	e.init(ctx)
	if !e.ready {
		return e.initErr
	}
	doc, payload, published, err := e.loadForIndex(ctx, source, id)
	if err != nil {
		return err
	}
	pointID := PointID(source, id)
	if !published || strings.TrimSpace(doc) == "" {
		return e.qdrant.Delete(ctx, []string{pointID})
	}
	vec, err := e.embed.Embed(ctx, doc)
	if err != nil {
		return err
	}
	return e.qdrant.Upsert(ctx, []qdrantPoint{{
		ID:      pointID,
		Vector:  vec,
		Payload: payload,
	}})
}

func (e *Engine) DeleteListing(ctx context.Context, source string, id uint) error {
	if !e.Enabled() || id == 0 {
		return nil
	}
	e.init(ctx)
	if !e.ready {
		return nil
	}
	return e.qdrant.Delete(ctx, []string{PointID(source, id)})
}

func (e *Engine) ReindexAll(ctx context.Context) (int, error) {
	if !e.Enabled() {
		return 0, fmt.Errorf("semantic search disabled")
	}
	e.init(ctx)
	if !e.ready {
		return 0, e.initErr
	}
	count := 0
	var sales []models.PropertySale
	e.db.WithContext(ctx).
		Where("is_published = ? AND COALESCE(is_deactivated,false) = ? AND COALESCE(is_sold,false) = ?", true, false, false).
		Find(&sales)
	for _, s := range sales {
		if err := e.IndexListing(ctx, SourceSale, s.ID); err == nil {
			count++
		}
	}
	var rents []models.Property
	e.db.WithContext(ctx).
		Where("COALESCE(is_active,true) = ? AND LOWER(status) IN ?", true, []string{"approved", "live", "published"}).
		Find(&rents)
	for _, r := range rents {
		if err := e.IndexListing(ctx, SourceRent, r.ID); err == nil {
			count++
		}
	}
	var lands []models.Landmark
	e.db.WithContext(ctx).
		Where("is_verified = ? AND is_published = ?", true, true).
		Find(&lands)
	for _, l := range lands {
		if err := e.IndexListing(ctx, SourceLand, l.ID); err == nil {
			count++
		}
	}
	return count, nil
}

func (e *Engine) loadForIndex(ctx context.Context, source string, id uint) (doc string, payload map[string]any, published bool, err error) {
	switch source {
	case SourceSale:
		var row models.PropertySale
		if err = e.db.WithContext(ctx).First(&row, id).Error; err != nil {
			return
		}
		published = row.IsPublished && !row.IsDeactivated && !row.IsSold
		return saleDocument(&row), salePayload(&row), published, nil
	case SourceRent:
		var row models.Property
		if err = e.db.WithContext(ctx).First(&row, id).Error; err != nil {
			return
		}
		st := strings.ToLower(strings.TrimSpace(row.Status))
		active := row.IsActive == nil || *row.IsActive
		published = active && (st == "approved" || st == "live" || st == "published")
		return rentDocument(&row), rentPayload(&row), published, nil
	case SourceLand:
		var row models.Landmark
		if err = e.db.WithContext(ctx).First(&row, id).Error; err != nil {
			return
		}
		published = row.IsVerified && row.IsPublished
		return landDocument(&row), landPayload(&row), published, nil
	default:
		return "", nil, false, fmt.Errorf("unknown source %s", source)
	}
}

func (e *Engine) documentFor(ctx context.Context, source string, id uint) (string, error) {
	doc, _, _, err := e.loadForIndex(ctx, source, id)
	return doc, err
}

func (e *Engine) hydrate(ctx context.Context, saleIDs, rentIDs, landIDs []uint) ([]property.Property, error) {
	out := make([]property.Property, 0)
	if len(saleIDs) > 0 {
		var rows []models.PropertySale
		e.db.WithContext(ctx).Where("id IN ?", saleIDs).Find(&rows)
		for _, r := range rows {
			out = append(out, propertyFromSale(r))
		}
	}
	if len(rentIDs) > 0 {
		var rows []models.Property
		e.db.WithContext(ctx).Where("id IN ?", rentIDs).Find(&rows)
		for _, r := range rows {
			img := firstImageFromJSON(r.Images)
			out = append(out, property.Property{
				ID:       r.ID,
				Title:    r.Title,
				Price:    float64(r.NightlyPrice),
				Currency: r.Currency,
				City:     r.City,
				Bedrooms: r.Bedrooms,
				Image:    img,
				Type:     "rent",
				Source:   "property",
			})
		}
	}
	if len(landIDs) > 0 {
		var rows []models.Landmark
		e.db.WithContext(ctx).Where("id IN ?", landIDs).Find(&rows)
		for _, l := range rows {
			out = append(out, propertyFromLandmark(l))
		}
	}
	return out, nil
}

func firstImageFromJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "[]" {
		return ""
	}
	var urls []string
	if err := json.Unmarshal([]byte(raw), &urls); err == nil && len(urls) > 0 {
		return urls[0]
	}
	return ""
}

func propertyFromLandmark(l models.Landmark) property.Property {
	img := ""
	if l.Images != nil {
		var urls []string
		_ = json.Unmarshal(l.Images, &urls)
		if len(urls) > 0 {
			img = urls[0]
		}
	}
	return property.Property{
		ID:            l.ID,
		Title:         l.Title,
		Price:         l.Price,
		Currency:      l.Currency,
		City:          l.District,
		Image:         img,
		Type:          "sale",
		Source:        "landmark",
		Area:          l.Area,
		LocationLabel: l.District,
		PlotNumber:    l.PlotNumber,
	}
}

func propertyFromSale(r models.PropertySale) property.Property {
	img := ""
	if len(r.Images) > 0 {
		img = r.Images[0]
	}
	return property.Property{
		ID:       r.ID,
		Title:    r.Title,
		Price:    r.ListingPrice,
		Currency: r.Currency,
		City:     r.City,
		Bedrooms: r.Bedrooms,
		Image:    img,
		Type:     "sale",
		Source:   "property_sale",
	}
}

func buildQdrantFilter(f property.Filters) map[string]any {
	must := []map[string]any{
		{"key": "is_published", "match": map[string]any{"value": true}},
	}
	purpose := strings.ToLower(strings.TrimSpace(f.Purpose))
	if purpose == "rent" {
		must = append(must, map[string]any{"key": "purpose", "match": map[string]any{"value": "rent"}})
	} else if purpose == "land" || strings.EqualFold(f.Type, "land") {
		must = append(must, map[string]any{"key": "purpose", "match": map[string]any{"value": "land"}})
	} else if purpose == "sale" || purpose == "buy" {
		must = append(must, map[string]any{"key": "purpose", "match": map[string]any{"value": "sale"}})
	}
	if city := strings.ToLower(strings.TrimSpace(f.City)); city != "" {
		must = append(must, map[string]any{"key": "city", "match": map[string]any{"text": city}})
	}
	if f.BudgetMax > f.BudgetMin && f.BudgetMax > 0 {
		must = append(must, map[string]any{
			"key": "price",
			"range": map[string]any{
				"gte": f.BudgetMin,
				"lte": f.BudgetMax,
			},
		})
	}
	if f.Bedrooms > 0 {
		must = append(must, map[string]any{
			"key": "bedrooms",
			"range": map[string]any{"gte": f.Bedrooms},
		})
	}
	if t := strings.ToLower(strings.TrimSpace(f.Type)); t != "" && t != "land" {
		must = append(must, map[string]any{"key": "property_type", "match": map[string]any{"value": t}})
	}
	return map[string]any{"must": must}
}

func reorderByHits(props []property.Property, hits []searchHit) []property.Property {
	if len(hits) == 0 {
		return props
	}
	byKey := map[string]property.Property{}
	for _, p := range props {
		src := SourceSale
		if p.Source == "landmark" {
			src = SourceLand
		} else if p.Type == "rent" {
			src = SourceRent
		}
		byKey[PointID(src, p.ID)] = p
	}
	out := make([]property.Property, 0, len(hits))
	for _, h := range hits {
		if p, ok := byKey[fmt.Sprint(h.ID)]; ok {
			out = append(out, p)
		}
	}
	return out
}

func scoresForHits(props []property.Property, hits []searchHit) []float32 {
	scoreByID := map[string]float32{}
	for _, h := range hits {
		scoreByID[fmt.Sprint(h.ID)] = h.Score
	}
	out := make([]float32, 0, len(props))
	for _, p := range props {
		src := SourceSale
		if p.Source == "landmark" {
			src = SourceLand
		} else if p.Type == "rent" {
			src = SourceRent
		}
		out = append(out, scoreByID[PointID(src, p.ID)])
	}
	return out
}

// QueueIndex runs indexing in the background (safe for HTTP handlers).
func QueueIndex(engine *Engine, source string, id uint) {
	if engine == nil || !engine.Enabled() || id == 0 {
		return
	}
	go func() {
		if err := engine.IndexListing(context.Background(), source, id); err != nil {
			log.Printf("⚠️ semantic index %s:%d failed: %v", source, id, err)
		}
	}()
}
