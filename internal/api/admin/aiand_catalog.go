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

	"workweave/router/internal/auth"
	"workweave/router/internal/observability"
	"workweave/router/internal/providers"
	"workweave/router/internal/proxy"
	"workweave/router/internal/server/middleware"

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

// aiandCatalogSlot is one cached upstream payload for a single credential.
type aiandCatalogSlot struct {
	payload   []byte
	count     int
	fetchedAt time.Time
}

// aiandCatalogCacheSet holds per-credential 60-second caches. Self-serve users
// each authenticate with their own aiand BYOK key, so a single global slot
// would either leak one user's catalog to another or thrash on every request.
type aiandCatalogCacheSet struct {
	mu    sync.Mutex
	slots map[string]*aiandCatalogSlot
	now   func() time.Time
}

// newAiandCatalogCacheSet constructs a cache set that compares time via now.
func newAiandCatalogCacheSet(now func() time.Time) *aiandCatalogCacheSet {
	return &aiandCatalogCacheSet{slots: make(map[string]*aiandCatalogSlot), now: now}
}

// Get returns the cached payload for cacheKey. When the slot is empty or stale
// (older than aiandCatalogTTL), it calls fetch and stores the result.
func (c *aiandCatalogCacheSet) Get(cacheKey string, ctx context.Context, fetch func(context.Context) ([]byte, error)) ([]byte, int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	slot := c.slots[cacheKey]
	if slot != nil && slot.payload != nil && c.now().Sub(slot.fetchedAt) < aiandCatalogTTL {
		return slot.payload, slot.count, nil
	}
	payload, err := fetch(ctx)
	if err != nil {
		if slot != nil && slot.payload != nil {
			return slot.payload, slot.count, nil
		}
		return nil, 0, err
	}
	var resp aiandCatalogResponse
	if err := json.Unmarshal(payload, &resp); err != nil {
		return nil, 0, err
	}
	if slot == nil {
		slot = &aiandCatalogSlot{}
		c.slots[cacheKey] = slot
	}
	slot.payload = payload
	slot.count = len(resp.Data)
	slot.fetchedAt = c.now()
	return payload, slot.count, nil
}

// resolveAiandCatalogCredential picks the aiand API key for one catalog
// request. Installation BYOK (stashed on ctx by dashboard auth middleware) wins
// over the optional deployment AIAND_API_KEY fallback.
func resolveAiandCatalogCredential(ctx context.Context, c *gin.Context, deploymentKey string) (apiKey, cacheKey string, ok bool) {
	if keys, _ := ctx.Value(proxy.ExternalAPIKeysContextKey{}).([]*auth.ExternalAPIKey); len(keys) > 0 {
		for _, k := range keys {
			if k != nil && k.Provider == providers.ProviderAiand && len(k.Plaintext) > 0 {
				if inst := middleware.InstallationFrom(c); inst != nil && inst.ID != "" {
					return string(k.Plaintext), inst.ID, true
				}
				return string(k.Plaintext), k.ID, true
			}
		}
	}
	if deploymentKey != "" {
		return deploymentKey, "deployment", true
	}
	return "", "", false
}

// AiandCatalogHandler builds the authenticated GET /admin/v1/aiand/models
// handler. deploymentKey is the optional boot-time AIAND_API_KEY fallback for
// self-hosted installs; self-serve callers authenticate with their own aiand
// BYOK key from the account session. baseURL is the full upstream base
// (defaults to openaicompat.AiandBaseURL in the composition root). It forwards
// ai&'s /v1/models response verbatim with a 5-second per-request upstream
// budget and a 60-second per-credential cache. 401 when no credential is
// available; 502 on upstream failure; empty data[] is 200.
func AiandCatalogHandler(deploymentKey, baseURL string, client *http.Client, now func() time.Time) gin.HandlerFunc {
	cache := newAiandCatalogCacheSet(now)
	if client == nil {
		client = http.DefaultClient
	}
	return func(c *gin.Context) {
		log := observability.FromGin(c)
		apiKey, cacheKey, ok := resolveAiandCatalogCredential(c.Request.Context(), c, deploymentKey)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "aiand_key_required"})
			return
		}
		payload, _, err := cache.Get(cacheKey, c.Request.Context(), func(ctx context.Context) ([]byte, error) {
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
