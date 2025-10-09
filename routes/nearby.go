package routes

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/kataras/iris/v12"
)

// Removed unused overpassResp struct

type poi struct {
	Name     string  `json:"name"`
	Lat      float64 `json:"lat"`
	Lng      float64 `json:"lng"`
	Distance int     `json:"distance_m"`
	Image    string  `json:"image,omitempty"`
	Rating   float64 `json:"rating,omitempty"`
}

type nearbyResponse struct {
	Schools     []poi `json:"schools"`
	Hospitals   []poi `json:"hospitals"`
	Restaurants []poi `json:"restaurants"`
}

func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6371000.0
	dLat := (lat2 - lat1) * math.Pi / 180.0
	dLon := (lon2 - lon1) * math.Pi / 180.0
	la1 := lat1 * math.Pi / 180.0
	la2 := lat2 * math.Pi / 180.0
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Sin(dLon/2)*math.Sin(dLon/2)*math.Cos(la1)*math.Cos(la2)
	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
	return R * c
}

func fetchOverpass(category string, lat, lng float64, radius int) ([]poi, error) {
	// Map our categories to Overpass amenity values
	amenity := category
	// Overpass QL query: search nodes with amenity within radius of lat/lng
	ql := fmt.Sprintf(`[out:json][timeout:15];(node["amenity"="%s"](around:%d,%f,%f););out body;`, amenity, radius, lat, lng)
	resp, err := http.Post("https://overpass-api.de/api/interpreter", "application/x-www-form-urlencoded", strings.NewReader("data="+ql))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("overpass status %d", resp.StatusCode)
	}
	var parsed struct {
		Elements []struct {
			Lat  float64 `json:"lat"`
			Lon  float64 `json:"lon"`
			Tags struct {
				Name string `json:"name"`
			} `json:"tags"`
		} `json:"elements"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, err
	}
	items := make([]poi, 0, len(parsed.Elements))
	for _, el := range parsed.Elements {
		d := int(haversine(lat, lng, el.Lat, el.Lon))
		name := el.Tags.Name
		if name == "" {
			name = strings.Title(category)
		}

		// For restaurants and schools, try to get image from Google Places
		image := ""
		if category == "restaurant" || category == "school" {
			image = getPlaceImage(name, el.Lat, el.Lon)
		}

		items = append(items, poi{
			Name:     name,
			Lat:      el.Lat,
			Lng:      el.Lon,
			Distance: d,
			Image:    image,
		})
	}
	// sort by distance and limit to 10
	sort.Slice(items, func(i, j int) bool { return items[i].Distance < items[j].Distance })
	if len(items) > 10 {
		items = items[:10]
	}
	return items, nil
}

func getPlaceImage(name string, lat, lng float64) string {
	// Use Google Places API to get photo reference
	// For now, return a placeholder or use a generic image service
	// You can implement Google Places API integration here
	return fmt.Sprintf("https://via.placeholder.com/100x100/4A90E2/FFFFFF?text=%s", strings.ReplaceAll(name, " ", "+"))
}

func NearbyHandler(ctx iris.Context) {
	lat, _ := strconv.ParseFloat(ctx.URLParam("lat"), 64)
	lng, _ := strconv.ParseFloat(ctx.URLParam("lng"), 64)
	if lat == 0 && lng == 0 {
		ctx.StatusCode(http.StatusBadRequest)
		ctx.JSON(iris.Map{"error": "lat and lng are required"})
		return
	}
	radius := 3000
	if v := ctx.URLParam("radius"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			radius = n
		}
	}

	schools, _ := fetchOverpass("school", lat, lng, radius)
	hospitals, _ := fetchOverpass("hospital", lat, lng, radius)
	restaurants, _ := fetchOverpass("restaurant", lat, lng, radius)

	ctx.ContentType("application/json")
	ctx.JSON(nearbyResponse{Schools: schools, Hospitals: hospitals, Restaurants: restaurants})
}
