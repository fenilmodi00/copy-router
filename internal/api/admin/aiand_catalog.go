// Package admin exposes the dashboard's operational surface.
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sync"
	"time"

	"workweave/router/internal/observability"

	"github.com/gin-gonic/gin"
)

// AiandCatalogRequestBudget bounds a single upstream /v1/models fetch.
const AiandCatalogRequestBudget = 5 * time.Second

// AiandModelRow is one entry in ai&'s GET /v1/models data array. The field
// names and JSON tags mirror ai&'s wire shape 1:1 so the frontend's TS type is
// a copy of this struct.
type AiandModelRow struct {
	ID                     string   `json:"id"`
	Provider               string   `json:"provider"`
	ContextWindow          int      `json:"context_window"`
	Capabilities           []string `json:"capabilities"`
	ReasoningEfforts       []string `json:"reasoning_efforts"`
	ReasoningEffortDefault string   `json:"reasoning_effort_default"`
	InputPer1M             string   `json:"input_per_1m"`
	OutputPer1M            string   `json:"output_per_1m"`
	CachedInputPer1M       string   `json:"cached_input_per_1m"`
	Currency               string   `json:"currency"`
}

// aiandCatalogResponse is ai&'s verbatim envelope; data is forwarded
// unchanged.
type aiandCatalogResponse struct {
	Data []AiandModelRow `json:"data"`
}

// aiandCatalogTTL bounds how long a cached upstream payload is served.
const aiandCatalogTTL = 60 * time.Second

// aiandCatalogCache is the in-process 60-second single-slot cache. A second
// call inside the window is served from payload without a new upstream
// request. Callers never observe a partial refresh: the fetch happens under
// mu, so concurrent callers block until the single refresh completes.
type aiandCatalogCache struct {
	mu        sync.Mutex
	payload   []byte
	count     int
	fetchedAt time.Time
	now       func() time.Time
}

// newAiandCatalogCache constructs a cache that compares time via now.
func newAiandCatalogCache(now func() time.Time) *aiandCatalogCache {
	return &aiandCatalogCache{now: now}
}

// Get returns the cached payload. When the cache is empty or stale (older
// than aiandCatalogTTL), it calls fetch (which is given a bounded context) and
// stores the result. The int return is the data[] count so the handler can
// distinguish legitimately-empty (data:[] -> 200 with payload) from a failed
// fetch (error -> 502). A fetch error leaves the cache intact; on a stale
// entry the prior payload is still served so a transient upstream blip does
// not blank the dashboard.
func (c *aiandCatalogCache) Get(ctx context.Context, fetch func(context.Context) ([]byte, error)) ([]byte, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.payload != nil && c.now().Sub(c.fetchedAt) < aiandCatalogTTL {
		return c.payload, c.count, nil
	}
	payload, err := fetch(ctx)
	if err != nil {
		if c.payload != nil {
			return c.payload, c.count, nil
		}
		return nil, 0, err
	}
	c.payload = payload
	var resp aiandCatalogResponse
	if err := json.Unmarshal(payload, &resp); err != nil {
		return nil, 0, err
	}
	c.count = len(resp.Data)
	c.fetchedAt = c.now()
	return payload, c.count, nil
}

// AiandCatalogHandler builds the authenticated GET /admin/v1/aiand/models
// handler for a selfhosted dashboard. apiKey is the deployment's AIAND_API_KEY;
// baseURL is the full upstream base (defaults to openaicompat.AiandBaseURL in
// the composition root). It forwards ai&'s /v1/models response verbatim with a
// 5-second per-request upstream budget, serving an in-process 60-second
// single-slot cache. 502 on upstream failure; empty data[] is 200.
func AiandCatalogHandler(apiKey, baseURL string, client *http.Client, now func() time.Time) gin.HandlerFunc {
	cache := newAiandCatalogCache(now)
	if client == nil {
		client = http.DefaultClient
	}
	return func(c *gin.Context) {
		log := observability.FromGin(c)
		payload, _, err := cache.Get(c.Request.Context(), func(ctx context.Context) ([]byte, error) {
			ctx, cancel := context.WithTimeout(ctx, AiandCatalogRequestBudget)
			defer cancel()
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/models", nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("Authorization", "Bearer "+apiKey)
			resp, err := client.Do(req)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				return nil, errors.New("aiand catalog upstream returned " + resp.Status)
			}
			payload, err := io.ReadAll(resp.Body)
			if err != nil {
				return nil, err
			}
			return payload, nil
		})
		if err != nil {
			log.Error("aiand catalog upstream failed", "err", err, "base_url", baseURL)
			c.AbortWithStatusJSON(http.StatusBadGateway, gin.H{"error": "The ai& catalog is currently unavailable."})
			return
		}
		c.Data(http.StatusOK, "application/json", payload)
	}
}
