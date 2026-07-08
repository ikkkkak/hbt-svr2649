package semantic

import (
	"context"

	"apartments-clone-server/MeskenyGPT/internal/ai/property"
	"apartments-clone-server/MeskenyGPT/internal/ai/vector"

	"gorm.io/gorm"
)

// Filters re-exports property search filters for HTTP handlers.
type Filters = property.Filters

// Property re-exports a searchable property row.
type Property = property.Property

// Engine wraps the internal Qdrant semantic search engine.
type Engine = vector.Engine

// NewEngine creates a semantic search engine from env + DB.
func NewEngine(db *gorm.DB) *Engine {
	return vector.NewEngine(db)
}

// QueueIndex indexes a listing asynchronously.
func QueueIndex(engine *Engine, source string, id uint) {
	vector.QueueIndex(engine, source, id)
}

// Search runs semantic property search.
func Search(ctx context.Context, engine *Engine, query string, f property.Filters, limit int) ([]property.Property, []float32, error) {
	if engine == nil {
		return nil, nil, vector.ErrDisabled
	}
	return engine.Search(ctx, query, f, limit)
}

const (
	SourceSale = vector.SourceSale
	SourceRent = vector.SourceRent
	SourceLand = vector.SourceLand
)
