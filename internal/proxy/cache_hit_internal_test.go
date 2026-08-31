package proxy

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"workweave/router/internal/providers"
	"workweave/router/internal/router"
	"workweave/router/internal/router/cache"
	"workweave/router/internal/router/turntype"
	"workweave/router/internal/translate"
)

type cacheHitTelemetryRepo struct {
	rows []InsertTelemetryParams
}

func (r *cacheHitTelemetryRepo) InsertRequestTelemetry(_ context.Context, p InsertTelemetryParams) error {
	r.rows = append(r.rows, p)
	return nil
}

func (r *cacheHitTelemetryRepo) GetTelemetrySummary(context.Context, string, time.Time, time.Time) (TelemetrySummary, error) {
	return TelemetrySummary{}, nil
}
func (r *cacheHitTelemetryRepo) GetTelemetryTimeseries(context.Context, string, time.Time, time.Time, string) ([]TelemetryBucket, error) {
	return nil, nil
}
func (r *cacheHitTelemetryRepo) GetTelemetrySummaryAll(context.Context, time.Time, time.Time) (TelemetrySummary, error) {
	return TelemetrySummary{}, nil
}
func (r *cacheHitTelemetryRepo) GetTelemetryTimeseriesAll(context.Context, time.Time, time.Time, string) ([]TelemetryBucket, error) {
	return nil, nil
}
func (r *cacheHitTelemetryRepo) GetTelemetryRows(context.Context, string, time.Time, time.Time, int32) ([]TelemetryRow, error) {
	return nil, nil
}
func (r *cacheHitTelemetryRepo) GetTelemetryRowsAll(context.Context, time.Time, time.Time, int32) ([]TelemetryRow, error) {
	return nil, nil
}
func (r *cacheHitTelemetryRepo) GetTelemetryModelBreakdown(context.Context, string, time.Time, time.Time, string) ([]TelemetryModelBucket, error) {
	return nil, nil
}
func (r *cacheHitTelemetryRepo) GetTelemetryModelBreakdownAll(context.Context, time.Time, time.Time, string) ([]TelemetryModelBucket, error) {
	return nil, nil
}
func (r *cacheHitTelemetryRepo) GetTelemetryBySessionSequence(context.Context, uuid.UUID, []byte, string, int) (TelemetryTurnResult, error) {
	return TelemetryTurnResult{}, nil
}

func cacheHitEmbedding(seed float32) []float32 {
	v := []float32{seed, 1, 0, 0, 0, 0, 0, 0}
	var sum float32
	for _, x := range v {
		sum += x * x
	}
	norm := float32(math.Sqrt(float64(sum)))
	out := make([]float32, len(v))
	for i, x := range v {
		out[i] = x / norm
	}
	return out
}

func cacheHitDecision(emb []float32) router.Decision {
	return router.Decision{
		Provider: providers.ProviderAiand,
		Model:    "deepseek-ai/deepseek-v4-flash",
		Reason:   "test",
		Metadata: &router.RoutingMetadata{Embedding: emb, ClusterIDs: []int{0, 1}},
	}
}

func cacheHitRouteRes() turnLoopResult {
	return turnLoopResult{
		TurnType: turntype.MainLoop,
		PinRole:  "high",
	}
}

func cacheHitCtx(t *testing.T) context.Context {
	t.Helper()
	ctx := context.Background()
	ctx = context.WithValue(ctx, ExternalIDContextKey{}, "tenant-cache-hit")
	ctx = context.WithValue(ctx, InstallationIDContextKey{}, uuid.New().String())
	ctx = context.WithValue(ctx, APIKeyIDContextKey{}, "key-1")
	return ctx
}

func TestTryServeSemanticCacheHit_HitServesAndReturnsTrue(t *testing.T) {
	emb := cacheHitEmbedding(3)
	c := cache.New(cache.DefaultConfig())
	telem := &cacheHitTelemetryRepo{}
	svc := NewService(
		nil,
		nil,
		nil,
		false,
		c,
		nil,
		false,
		providers.ProviderAiand,
		"deepseek-ai/deepseek-v4-flash",
		telem,
	)
	c.Store("tenant-cache-hit", cache.FormatAnthropic, emb, 0, cache.CachedResponse{StatusCode: 200, Body: []byte(`{"ok":true}`)}, "", 0)

	ctx := cacheHitCtx(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	feats := translate.RoutingFeatures{Model: "moonshotai/kimi-k3", Tokens: 42}
	decision := cacheHitDecision(emb)
	start := time.Now()

	served := svc.tryServeSemanticCacheHit(ctx, rec, cache.FormatAnthropic, feats, decision, cacheHitRouteRes(), [16]byte{1}, start, 5, "tenant-cache-hit", false, false, req, false, false)
	require.True(t, served)
	assert.Equal(t, RouterCacheHit, rec.Header().Get(HeaderRouterCache))
	require.Eventually(t, func() bool { return len(telem.rows) == 1 }, time.Second, 10*time.Millisecond)
	require.NotNil(t, telem.rows[0].SemanticCacheHit)
	assert.True(t, *telem.rows[0].SemanticCacheHit)
}

func TestTryServeSemanticCacheHit_MissReturnsFalse(t *testing.T) {
	emb := cacheHitEmbedding(4)
	c := cache.New(cache.DefaultConfig())
	svc := NewService(nil, nil, nil, false, c, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil)

	ctx := cacheHitCtx(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	served := svc.tryServeSemanticCacheHit(ctx, rec, cache.FormatAnthropic, translate.RoutingFeatures{Model: "moonshotai/kimi-k3"}, cacheHitDecision(emb), cacheHitRouteRes(), [16]byte{1}, time.Now(), 1, "other-tenant", false, false, req, false, false)
	assert.False(t, served)
}

func TestTryServeSemanticCacheHit_ExcludedWhenStream(t *testing.T) {
	emb := cacheHitEmbedding(6)
	c := cache.New(cache.DefaultConfig())
	c.Store("tenant-cache-hit", cache.FormatAnthropic, emb, 0, cache.CachedResponse{StatusCode: 200, Body: []byte(`{"ok":true}`)}, "", 0)
	svc := NewService(nil, nil, nil, false, c, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil)

	ctx := cacheHitCtx(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	served := svc.tryServeSemanticCacheHit(ctx, rec, cache.FormatAnthropic, translate.RoutingFeatures{Model: "moonshotai/kimi-k3"}, cacheHitDecision(emb), cacheHitRouteRes(), [16]byte{1}, time.Now(), 1, "tenant-cache-hit", true, false, req, false, false)
	assert.False(t, served)
}

func TestTryServeSemanticCacheHit_ExcludedWhenNoMetadata(t *testing.T) {
	c := cache.New(cache.DefaultConfig())
	svc := NewService(nil, nil, nil, false, c, nil, false, providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil)

	ctx := cacheHitCtx(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	decision := router.Decision{Provider: providers.ProviderAiand, Model: "deepseek-ai/deepseek-v4-flash", Reason: "test"}
	served := svc.tryServeSemanticCacheHit(ctx, rec, cache.FormatAnthropic, translate.RoutingFeatures{Model: "moonshotai/kimi-k3"}, decision, cacheHitRouteRes(), [16]byte{1}, time.Now(), 1, "tenant-cache-hit", false, false, req, false, false)
	assert.False(t, served)
}
