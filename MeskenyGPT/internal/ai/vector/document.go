package vector

import (
	"fmt"
	"strings"

	"apartments-clone-server/models"
)

const (
	SourceSale = "sale"
	SourceRent = "rent"
	SourceLand = "land"
)

func PointID(source string, id uint) string {
	return fmt.Sprintf("%s:%d", source, id)
}

func ParsePointID(pointID string) (source string, id uint, ok bool) {
	parts := strings.SplitN(strings.TrimSpace(pointID), ":", 2)
	if len(parts) != 2 {
		return "", 0, false
	}
	var n uint
	if _, err := fmt.Sscanf(parts[1], "%d", &n); err != nil || n == 0 {
		return "", 0, false
	}
	return parts[0], n, true
}

func saleDocument(p *models.PropertySale) string {
	var b strings.Builder
	appendLine(&b, "title", p.Title)
	appendLine(&b, "description", p.Description)
	appendLine(&b, "type", p.PropertyType)
	appendLine(&b, "category", p.Category)
	appendLine(&b, "city", p.City)
	appendLine(&b, "address", p.Address)
	appendLine(&b, "state", p.State)
	appendLine(&b, "country", p.Country)
	if p.Bedrooms > 0 {
		appendLine(&b, "bedrooms", fmt.Sprintf("%d", p.Bedrooms))
	}
	if p.Bathrooms > 0 {
		appendLine(&b, "bathrooms", fmt.Sprintf("%d", p.Bathrooms))
	}
	if p.SquareFootage > 0 {
		appendLine(&b, "area_sqm", fmt.Sprintf("%d", p.SquareFootage))
	}
	if p.ListingPrice > 0 {
		appendLine(&b, "price", fmt.Sprintf("%.0f %s", p.ListingPrice, p.Currency))
	}
	if len(p.Features) > 0 {
		appendLine(&b, "features", strings.Join(p.Features, ", "))
	}
	if len(p.Amenities) > 0 {
		appendLine(&b, "amenities", strings.Join(p.Amenities, ", "))
	}
	if len(p.PaperTypes) > 0 {
		appendLine(&b, "papers", strings.Join(p.PaperTypes, ", "))
	}
	return b.String()
}

func rentDocument(p *models.Property) string {
	var b strings.Builder
	appendLine(&b, "title", p.Title)
	appendLine(&b, "description", p.Description)
	appendLine(&b, "type", p.PropertyType)
	appendLine(&b, "city", p.City)
	appendLine(&b, "address", p.AddressLine1)
	if p.AddressLine2 != "" {
		appendLine(&b, "address2", p.AddressLine2)
	}
	if p.Bedrooms > 0 {
		appendLine(&b, "bedrooms", fmt.Sprintf("%d", p.Bedrooms))
	}
	if p.Bathrooms > 0 {
		appendLine(&b, "bathrooms", fmt.Sprintf("%d", p.Bathrooms))
	}
	if p.NightlyPrice > 0 {
		appendLine(&b, "price", fmt.Sprintf("%.0f %s per night", p.NightlyPrice, p.Currency))
	}
	if p.NeighborhoodDescription != "" {
		appendLine(&b, "neighborhood", p.NeighborhoodDescription)
	}
	return b.String()
}

func landDocument(l *models.Landmark) string {
	var b strings.Builder
	appendLine(&b, "title", l.Title)
	appendLine(&b, "description", l.Description)
	appendLine(&b, "land_type", l.LandType)
	appendLine(&b, "district", l.District)
	appendLine(&b, "region", l.Region)
	appendLine(&b, "plot", l.PlotNumber)
	if l.Area > 0 {
		appendLine(&b, "area", fmt.Sprintf("%.0f %s", l.Area, l.AreaUnit))
	}
	if l.Price > 0 {
		appendLine(&b, "price", fmt.Sprintf("%.0f %s", l.Price, l.Currency))
	}
	if len(l.PaperTypes) > 0 {
		appendLine(&b, "papers", strings.Join(l.PaperTypes, ", "))
	}
	return b.String()
}

func salePayload(p *models.PropertySale) map[string]any {
	return map[string]any{
		"source":        SourceSale,
		"listing_id":    p.ID,
		"title":         p.Title,
		"city":          strings.ToLower(strings.TrimSpace(p.City)),
		"property_type": strings.ToLower(strings.TrimSpace(p.PropertyType)),
		"purpose":       "sale",
		"price":         p.ListingPrice,
		"currency":      p.Currency,
		"bedrooms":      p.Bedrooms,
		"bathrooms":     p.Bathrooms,
		"status":        strings.ToLower(strings.TrimSpace(p.Status)),
		"is_published":  p.IsPublished,
	}
}

func rentPayload(p *models.Property) map[string]any {
	return map[string]any{
		"source":        SourceRent,
		"listing_id":    p.ID,
		"title":         p.Title,
		"city":          strings.ToLower(strings.TrimSpace(p.City)),
		"property_type": strings.ToLower(strings.TrimSpace(p.PropertyType)),
		"purpose":       "rent",
		"price":         p.NightlyPrice,
		"currency":      p.Currency,
		"bedrooms":      p.Bedrooms,
		"bathrooms":     p.Bathrooms,
		"status":        strings.ToLower(strings.TrimSpace(p.Status)),
		"is_published":  true,
	}
}

func landPayload(l *models.Landmark) map[string]any {
	return map[string]any{
		"source":        SourceLand,
		"listing_id":    l.ID,
		"title":         l.Title,
		"city":          strings.ToLower(strings.TrimSpace(l.District)),
		"property_type": "land",
		"purpose":       "land",
		"price":         l.Price,
		"currency":      l.Currency,
		"status":        strings.ToLower(strings.TrimSpace(l.Status)),
		"is_published":  l.IsPublished,
	}
}

func appendLine(b *strings.Builder, key, val string) {
	val = strings.TrimSpace(val)
	if val == "" {
		return
	}
	if b.Len() > 0 {
		b.WriteString("\n")
	}
	fmt.Fprintf(b, "%s: %s", key, val)
}
