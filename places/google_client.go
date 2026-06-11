package places

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

const (
	radiusMeters = 3000
	photoMaxWidth = 400
	requestTimeout = 15 * time.Second
	maxRetries     = 3
	retryDelay     = 1 * time.Second
)

// GooglePlacesClient calls Google Maps Platform APIs with rate limiting.
type GooglePlacesClient struct {
	apiKey string
	client *http.Client
	limiter *rate.Limiter // 5 req/sec
}

// NewGooglePlacesClient creates a client. apiKey from GOOGLE_PLACES_API_KEY.
func NewGooglePlacesClient(apiKey string) *GooglePlacesClient {
	if apiKey == "" {
		return nil
	}
	return &GooglePlacesClient{
		apiKey: apiKey,
		client: &http.Client{Timeout: requestTimeout},
		limiter: rate.NewLimiter(rate.Limit(5), 1),
	}
}

// nearbySearchResponse matches the Nearby Search JSON (subset we use).
type nearbySearchResponse struct {
	Results []struct {
		PlaceID  string `json:"place_id"`
		Name     string `json:"name"`
		Vicinity string `json:"vicinity"`
		Rating   float64 `json:"rating"`
		UserRatingsTotal int `json:"user_ratings_total"`
		Geometry struct {
			Location struct {
				Lat float64 `json:"lat"`
				Lng float64 `json:"lng"`
			} `json:"location"`
		} `json:"geometry"`
		Photos []struct {
			PhotoReference string `json:"photo_reference"`
		} `json:"photos"`
	} `json:"results"`
	Status string `json:"status"`
}

// detailsResponse matches Place Details JSON (subset).
type detailsResponse struct {
	Result struct {
		FormattedPhoneNumber string `json:"formatted_phone_number"`
		Website              string `json:"website"`
	} `json:"result"`
	Status string `json:"status"`
}

// RawPlace is a single place from Nearby Search, optionally with details.
type RawPlace struct {
	PlaceID   string
	Name      string
	Address   string
	Rating    float64
	Reviews   int
	Lat       float64
	Lng       float64
	PhotoRef  string
	Phone     string
	Website   string
}

// FetchNearby runs Nearby Search for one type and returns raw places.
func (c *GooglePlacesClient) FetchNearby(lat, lng float64, placeType string) ([]RawPlace, error) {
	if c == nil || c.apiKey == "" {
		return nil, nil
	}
	if err := c.limiter.Wait(context.Background()); err != nil {
		return nil, err
	}

	u := "https://maps.googleapis.com/maps/api/place/nearbysearch/json"
	reqURL := fmt.Sprintf("%s?location=%.6f,%.6f&radius=%d&type=%s&key=%s",
		u, lat, lng, radiusMeters, url.QueryEscape(placeType), url.QueryEscape(c.apiKey))

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err := c.client.Get(reqURL)
		if err != nil {
			lastErr = err
			time.Sleep(retryDelay)
			continue
		}
		var body nearbySearchResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			resp.Body.Close()
			lastErr = err
			time.Sleep(retryDelay)
			continue
		}
		resp.Body.Close()

		if body.Status != "OK" && body.Status != "ZERO_RESULTS" {
			lastErr = fmt.Errorf("google places status: %s", body.Status)
			time.Sleep(retryDelay)
			continue
		}

		out := make([]RawPlace, 0, len(body.Results))
		for _, r := range body.Results {
			photoRef := ""
			if len(r.Photos) > 0 {
				photoRef = r.Photos[0].PhotoReference
			}
			out = append(out, RawPlace{
				PlaceID:  r.PlaceID,
				Name:    r.Name,
				Address: r.Vicinity,
				Rating:  r.Rating,
				Reviews: r.UserRatingsTotal,
				Lat:     r.Geometry.Location.Lat,
				Lng:     r.Geometry.Location.Lng,
				PhotoRef: photoRef,
			})
		}
		return out, nil
	}
	return nil, lastErr
}

// FetchDetails returns phone and website for a place_id.
func (c *GooglePlacesClient) FetchDetails(placeID string) (phone, website string, err error) {
	if c == nil || c.apiKey == "" {
		return "", "", nil
	}
	if err := c.limiter.Wait(context.Background()); err != nil {
		return "", "", err
	}

	u := "https://maps.googleapis.com/maps/api/place/details/json"
	reqURL := fmt.Sprintf("%s?place_id=%s&fields=formatted_phone_number,website&key=%s",
		u, url.QueryEscape(placeID), url.QueryEscape(c.apiKey))

	for attempt := 0; attempt < maxRetries; attempt++ {
		resp, err := c.client.Get(reqURL)
		if err != nil {
			time.Sleep(retryDelay)
			continue
		}
		var body detailsResponse
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			resp.Body.Close()
			time.Sleep(retryDelay)
			continue
		}
		resp.Body.Close()
		if body.Status != "OK" {
			continue
		}
		return body.Result.FormattedPhoneNumber, body.Result.Website, nil
	}
	return "", "", nil
}

// PhotoURL returns the URL for a photo reference.
func (c *GooglePlacesClient) PhotoURL(photoRef string) string {
	if c == nil || c.apiKey == "" || photoRef == "" {
		return ""
	}
	return fmt.Sprintf("https://maps.googleapis.com/maps/api/place/photo?maxwidth=%d&photo_reference=%s&key=%s",
		photoMaxWidth, url.QueryEscape(photoRef), url.QueryEscape(c.apiKey))
}

// EnrichNearby fetches nearby places for all types and optionally enriches with details.
func (c *GooglePlacesClient) EnrichNearby(lat, lng float64, fetchDetails bool) (byType map[string][]RawPlace) {
	byType = make(map[string][]RawPlace)
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, t := range PlaceTypes {
		t := t
		wg.Add(1)
		go func() {
			defer wg.Done()
			list, err := c.FetchNearby(lat, lng, t)
			if err != nil {
				log.Printf("places: FetchNearby type=%s err=%v", t, err)
				return
			}
			if fetchDetails && len(list) > 0 {
				for i := range list {
					phone, web, _ := c.FetchDetails(list[i].PlaceID)
					list[i].Phone = phone
					list[i].Website = web
				}
			}
			mu.Lock()
			byType[t] = list
			mu.Unlock()
		}()
	}
	wg.Wait()
	return byType
}
