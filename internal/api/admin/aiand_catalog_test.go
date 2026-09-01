package admin_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"workweave/router/internal/api/admin"
	"workweave/router/internal/auth"
	"workweave/router/internal/providers"
	"workweave/router/internal/proxy"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const aiandModelsFixture = `{"object":"list","data":[` +
	`{"id":"deepseek-ai/deepseek-v4-flash","object":"model","created":1725784500,"owned_by":"ai&","provider":"deepseek-ai","context_window":1000000,"capabilities":["reasoning","tool_calling"],"reasoning_efforts":["none","high","max"],"reasoning_effort_default":"none","currency":"usd","input_per_1m":"0.15","output_per_1m":"0.25","cached_input_per_1m":"0.08"},` +
	`{"id":"google/gemma-4-31b-it","object":"model","created":1725784600,"owned_by":"ai&","provider":"google","context_window":262144,"capabilities":["reasoning","tool_calling","vision","video","document"],"reasoning_efforts":["none","high"],"reasoning_effort_default":"none","currency":"usd","input_per_1m":"0.20","output_per_1m":"0.50","cached_input_per_1m":"0.05"}` +
	`]}`

func aiandCatalogEngine(t *testing.T, upstream http.Handler) (*gin.Engine, *atomic.Int64) {
	t.Helper()
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-test" {
			t.Errorf("expected Bearer sk-test, got %q", got)
		}
		upstream.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/v1/aiand/models",
		func(c *gin.Context) {
			c.Set("router_installation", &auth.Installation{ID: "inst-1", Name: "test"})
		},
		// Mirror production wiring: AIAND_API_URL ends in /v1, the handler
		// appends /models to produce GET <base>/models with Authorization
		// Bearer sk-<key>. srv.URL has no /v1 suffix, so prepend it here so
		// the upstream mock sees the spec's /v1/models path.
		admin.AiandCatalogHandler("sk-test", srv.URL+"/v1",
			srv.Client(), func() time.Time { return time.Unix(1_700_000_000, 0) }),
	)
	return engine, &calls
}

func aiandCatalogGET(engine *gin.Engine) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/aiand/models", nil))
	return rec
}

func TestAiandCatalogHandler_ForwardsVerbatim200(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, aiandModelsFixture)
	})
	engine, calls := aiandCatalogEngine(t, upstream)

	rec := aiandCatalogGET(engine)
	require.Equal(t, http.StatusOK, rec.Code)
	var body struct {
		Data []admin.AiandModelRow `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Len(t, body.Data, 2)
	assert.Equal(t, "deepseek-ai/deepseek-v4-flash", body.Data[0].ID)
	assert.Equal(t, "0.15", body.Data[0].InputPer1M)
	assert.Equal(t, 262144, body.Data[1].ContextWindow)
	assert.Equal(t, "google/gemma-4-31b-it", body.Data[1].ID)
	// The header comment in metrics.go's response uses a dash; our response is
	// forwarded verbatim, so assert the raw JSON slice we forwarded (all extras
	// preserved — object/created/owned_by survive).
	assert.Contains(t, rec.Body.String(), `"object":"model"`)
	assert.Equal(t, int64(1), calls.Load(), "exactly one upstream call for the first request")
}

func TestAiandCatalogHandler_SecondCallWithinTTLDoesNotRehitUpstream(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, aiandModelsFixture)
	})
	engine, calls := aiandCatalogEngine(t, upstream)

	first := aiandCatalogGET(engine)
	require.Equal(t, http.StatusOK, first.Code)
	second := aiandCatalogGET(engine)
	require.Equal(t, http.StatusOK, second.Code)
	assert.JSONEq(t, first.Body.String(), second.Body.String())
	assert.Equal(t, int64(1), calls.Load(),
		"a second call inside the 60s window must be served from cache without a new upstream request")
}

func TestAiandCatalogHandler_UpstreamFailureIs502(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream exploded", http.StatusInternalServerError)
	})
	engine, calls := aiandCatalogEngine(t, upstream)

	rec := aiandCatalogGET(engine)
	assert.Equal(t, http.StatusBadGateway, rec.Code)
	assert.Contains(t, rec.Body.String(), "error")
	assert.Equal(t, int64(1), calls.Load())
}

func TestAiandCatalogHandler_EmptyDataIs200Not502(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"object":"list","data":[]}`)
	})
	engine, calls := aiandCatalogEngine(t, upstream)

	rec := aiandCatalogGET(engine)
	assert.Equal(t, http.StatusOK, rec.Code, "an empty catalog must render 200, not 502")
	var body struct {
		Data []admin.AiandModelRow `json:"data"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Empty(t, body.Data)
	assert.Equal(t, int64(1), calls.Load())
}

func TestAiandCatalogHandler_ConcurrentCallsSharedCache(t *testing.T) {
	upstream := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, aiandModelsFixture)
	})
	engine, calls := aiandCatalogEngine(t, upstream)

	for i := 0; i < 8; i++ {
		go func() {
			aiandCatalogGET(engine)
		}()
	}
	assert.Eventually(t, func() bool {
		return calls.Load() == 1
	}, 2*time.Second, 10*time.Millisecond)
}

func TestAiandCatalogHandler_UsesInstallationBYOKWithoutDeploymentKey(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if got := r.Header.Get("Authorization"); got != "Bearer sk-user-byok" {
			t.Errorf("expected Bearer sk-user-byok, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, aiandModelsFixture)
	}))
	t.Cleanup(srv.Close)

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	inst := &auth.Installation{ID: "inst-user-1"}
	engine.GET("/v1/aiand/models",
		func(c *gin.Context) {
			c.Set("router_installation", inst)
			keys := []*auth.ExternalAPIKey{{
				ID: "ek-1", Provider: providers.ProviderAiand, Plaintext: []byte("sk-user-byok"),
			}}
			ctx := context.WithValue(c.Request.Context(), proxy.ExternalAPIKeysContextKey{}, keys)
			c.Request = c.Request.WithContext(ctx)
		},
		admin.AiandCatalogHandler("", srv.URL+"/v1", srv.Client(), func() time.Time { return time.Unix(1_700_000_000, 0) }),
	)

	rec := aiandCatalogGET(engine)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, int64(1), calls.Load())
}

func TestAiandCatalogHandler_NoCredentialIs401(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/v1/aiand/models",
		admin.AiandCatalogHandler("", "https://example.invalid/v1", nil, time.Now),
	)
	rec := aiandCatalogGET(engine)
	assert.Equal(t, http.StatusUnauthorized, rec.Code)
	assert.Contains(t, rec.Body.String(), "aiand_key_required")
}
