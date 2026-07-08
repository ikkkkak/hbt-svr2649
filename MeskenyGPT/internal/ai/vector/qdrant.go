package vector

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type qdrantClient struct {
	baseURL    string
	collection string
	dim        int
	http       *http.Client
}

func newQdrantClient(baseURL, collection string, dim int) *qdrantClient {
	return &qdrantClient{
		baseURL:    stringsTrimRightSlash(baseURL),
		collection: collection,
		dim:        dim,
		http:       &http.Client{Timeout: 30 * time.Second},
	}
}

func stringsTrimRightSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}

func (q *qdrantClient) EnsureCollection(ctx context.Context) error {
	url := fmt.Sprintf("%s/collections/%s", q.baseURL, q.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	resp, err := q.http.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return nil
	}

	createBody := map[string]any{
		"vectors": map[string]any{
			"size":     q.dim,
			"distance": "Cosine",
		},
	}
	b, _ := json.Marshal(createBody)
	req, err = http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err = q.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create collection failed %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

type qdrantPoint struct {
	ID      string         `json:"id"`
	Vector  []float32      `json:"vector"`
	Payload map[string]any `json:"payload"`
}

func (q *qdrantClient) Upsert(ctx context.Context, points []qdrantPoint) error {
	if len(points) == 0 {
		return nil
	}
	body := map[string]any{"points": points}
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/collections/%s/points?wait=true", q.baseURL, q.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant upsert %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

func (q *qdrantClient) Delete(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	pointIDs := make([]map[string]string, 0, len(ids))
	for _, id := range ids {
		pointIDs = append(pointIDs, map[string]string{"id": id})
	}
	body := map[string]any{"points": pointIDs}
	b, _ := json.Marshal(body)
	url := fmt.Sprintf("%s/collections/%s/points/delete?wait=true", q.baseURL, q.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("qdrant delete %d: %s", resp.StatusCode, string(raw))
	}
	return nil
}

type searchHit struct {
	ID      any            `json:"id"`
	Score   float32        `json:"score"`
	Payload map[string]any `json:"payload"`
}

func (q *qdrantClient) Search(ctx context.Context, vector []float32, limit int, filter map[string]any, scoreThreshold float32) ([]searchHit, error) {
	if limit <= 0 {
		limit = 12
	}
	body := map[string]any{
		"vector":       vector,
		"limit":        limit,
		"with_payload": true,
	}
	if scoreThreshold > 0 {
		body["score_threshold"] = scoreThreshold
	}
	if filter != nil {
		body["filter"] = filter
	}
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s/collections/%s/points/search", q.baseURL, q.collection)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := q.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("qdrant search %d: %s", resp.StatusCode, string(raw))
	}
	var out struct {
		Result []searchHit `json:"result"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out.Result, nil
}
