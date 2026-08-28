package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"workweave/router/internal/api/admin"
	"workweave/router/internal/auth"
	"workweave/router/internal/providers"
	"workweave/router/internal/proxy"
	"workweave/router/internal/router/cache"
	"workweave/router/internal/router/sessionpin"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

// fakeTelemetryRepo stubs proxy.TelemetryRepository for metrics-handler tests.
// The embedded proxy.TelemetryRepository makes the type satisfy the interface
// even though we only implement the methods each handler test needs.
type fakeTelemetryRepo struct {
	proxy.TelemetryRepository
	buckets []proxy.TelemetryBucket
	summary proxy.TelemetrySummary
}

func (f *fakeTelemetryRepo) GetTelemetrySummary(ctx context.Context, installationID string, from, to time.Time) (proxy.TelemetrySummary, error) {
	return f.summary, nil
}

func (f *fakeTelemetryRepo) GetTelemetrySummaryAll(ctx context.Context, from, to time.Time) (proxy.TelemetrySummary, error) {
	return f.summary, nil
}

func (f *fakeTelemetryRepo) GetTelemetryTimeseries(ctx context.Context, installationID string, from, to time.Time, granularity string) ([]proxy.TelemetryBucket, error) {
	return f.buckets, nil
}

func (f *fakeTelemetryRepo) GetTelemetryTimeseriesAll(ctx context.Context, from, to time.Time, granularity string) ([]proxy.TelemetryBucket, error) {
	return f.buckets, nil
}

func metricsProxySvc(telemetry proxy.TelemetryRepository) *proxy.Service {
	return proxy.NewService(
		nil, // router unused by metrics handlers
		map[string]providers.Client{},
		nil, // emitter unused
		false,
		(*cache.Cache)(nil),
		sessionpin.Store(nil),
		false,
		"",
		"",
		telemetry,
	)
}

func metricsTimeseriesGET(handler gin.HandlerFunc) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/admin/v1/metrics/timeseries",
		func(c *gin.Context) {
			c.Set("router_installation", &auth.Installation{ID: "inst-1", Name: "test"})
		},
		handler,
	)
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/v1/metrics/timeseries?granularity=hour", nil))
	return rec
}

// The dashboard's per-bucket token sparkline must be fed the summed token
// counts per bucket, not a cost value. This is the regression test for the
// flat-sparkline bug: the handler previously dropped total_tokens and
// request_count from the timeseries JSON, so the Tokens and Requests cards
// both plotted dollars and rendered flat for tiny spend even when tokens
// varied across buckets.
// TestMetricsSummary_EmitsCacheInputSavingsUSD verifies that the metrics
// summary handler surfaces the additive cache_input_savings_usd field. The
// fake repo returns a TelemetrySummary with nonzero CacheInputSavingsUSD, and
// the handler must reflect it in the response JSON under the
// cache_input_savings_usd key.
func TestMetricsSummary_EmitsCacheInputSavingsUSD(t *testing.T) {
	summary := proxy.TelemetrySummary{
		RequestCount:          42,
		TotalTokens:           1_000_000,
		TotalRequestedCostUSD: 10.0,
		TotalActualCostUSD:    7.0,
		TotalSavingsUSD:       3.0,
		CacheWriteTokens:      0,
		CacheReadTokens:       500_000,
		CacheInputSavingsUSD:  proxy.CacheInputSavingsUSD(0.07),
		SemanticCacheHits:     5,
	}
	repo := &fakeTelemetryRepo{summary: summary}

	svc := metricsProxySvc(repo)

	gin.SetMode(gin.TestMode)
	handler := admin.MetricsSummaryHandler(svc, &auth.Service{})
	engine := gin.New()
	engine.GET("/admin/v1/metrics/summary",
		func(c *gin.Context) {
			c.Set("router_installation", &auth.Installation{ID: "inst-1", Name: "test"})
		},
		handler,
	)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/v1/metrics/summary?from=2026-08-27T00:00:00Z&to=2026-08-28T00:00:00Z", nil)
	engine.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		CacheInputSavingsUSD float64 `json:"cache_input_savings_usd"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	require.Equal(t, 0.07, body.CacheInputSavingsUSD)
}

func TestMetricsTimeseriesEmitsPerBucketTokens(t *testing.T) {
	telem := &fakeTelemetryRepo{buckets: []proxy.TelemetryBucket{
		{
			Bucket:           time.Date(2026, 8, 27, 0, 0, 0, 0, time.UTC),
			TotalTokens:      40_000,
			RequestCount:     8,
			RequestedCostUSD: 2.0,
			ActualCostUSD:    1.0,
		},
		{
			Bucket:           time.Date(2026, 8, 27, 1, 0, 0, 0, time.UTC),
			TotalTokens:      7_000,
			RequestCount:     2,
			RequestedCostUSD: 0.35,
			ActualCostUSD:    0.17,
		},
	}}

	rec := metricsTimeseriesGET(admin.MetricsTimeseriesHandler(metricsProxySvc(telem), &auth.Service{}))
	require.Equal(t, http.StatusOK, rec.Code)

	var body struct {
		Buckets []struct {
			TotalTokens   int64   `json:"total_tokens"`
			RequestCount  int64   `json:"request_count"`
			ActualCostUSD float64 `json:"actual_cost_usd"`
		} `json:"buckets"`
	}
	require.NoError(t, json.Unmarshal(bytes.TrimSpace(rec.Body.Bytes()), &body))
	require.Len(t, body.Buckets, 2)

	// Bucket totals round-trip summed (not flattened): a cost-only curve would
	// zero these out, which is exactly the flat-sparkline seam.
	require.Equal(t, int64(40_000), body.Buckets[0].TotalTokens)
	require.Equal(t, int64(8), body.Buckets[0].RequestCount)
	require.Equal(t, int64(7_000), body.Buckets[1].TotalTokens)
	require.Equal(t, int64(2), body.Buckets[1].RequestCount)
	// Cost fields keep flowing for the Actual-cost sparkline.
	require.Equal(t, 1.0, body.Buckets[0].ActualCostUSD)
}
