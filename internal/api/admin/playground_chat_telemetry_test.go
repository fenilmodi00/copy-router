package admin_test

import (
	"context"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"workweave/router/internal/api/admin"
	"workweave/router/internal/auth"
	"workweave/router/internal/providers"
	"workweave/router/internal/proxy"
	"workweave/router/internal/router"
)

type playgroundTelemetryCapture struct {
	mu    sync.Mutex
	rows  []proxy.InsertTelemetryParams
	notify chan struct{}
}

func newPlaygroundTelemetryCapture() *playgroundTelemetryCapture {
	return &playgroundTelemetryCapture{notify: make(chan struct{}, 1)}
}

func (c *playgroundTelemetryCapture) InsertRequestTelemetry(_ context.Context, p proxy.InsertTelemetryParams) error {
	c.mu.Lock()
	c.rows = append(c.rows, p)
	c.mu.Unlock()
	select {
	case c.notify <- struct{}{}:
	default:
	}
	return nil
}

func (c *playgroundTelemetryCapture) GetTelemetrySummary(context.Context, string, time.Time, time.Time) (proxy.TelemetrySummary, error) {
	return proxy.TelemetrySummary{}, nil
}
func (c *playgroundTelemetryCapture) GetTelemetryTimeseries(context.Context, string, time.Time, time.Time, string) ([]proxy.TelemetryBucket, error) {
	return nil, nil
}
func (c *playgroundTelemetryCapture) GetTelemetrySummaryAll(context.Context, time.Time, time.Time) (proxy.TelemetrySummary, error) {
	return proxy.TelemetrySummary{}, nil
}
func (c *playgroundTelemetryCapture) GetTelemetryTimeseriesAll(context.Context, time.Time, time.Time, string) ([]proxy.TelemetryBucket, error) {
	return nil, nil
}
func (c *playgroundTelemetryCapture) GetTelemetryRows(context.Context, string, time.Time, time.Time, int32) ([]proxy.TelemetryRow, error) {
	return nil, nil
}
func (c *playgroundTelemetryCapture) GetTelemetryRowsAll(context.Context, time.Time, time.Time, int32) ([]proxy.TelemetryRow, error) {
	return nil, nil
}
func (c *playgroundTelemetryCapture) GetTelemetryModelBreakdown(context.Context, string, time.Time, time.Time, string) ([]proxy.TelemetryModelBucket, error) {
	return nil, nil
}
func (c *playgroundTelemetryCapture) GetTelemetryModelBreakdownAll(context.Context, time.Time, time.Time, string) ([]proxy.TelemetryModelBucket, error) {
	return nil, nil
}
func (c *playgroundTelemetryCapture) GetTelemetryBySessionSequence(context.Context, uuid.UUID, []byte, string, int) (proxy.TelemetryTurnResult, error) {
	return proxy.TelemetryTurnResult{}, nil
}

func (c *playgroundTelemetryCapture) firstRow(t *testing.T) proxy.InsertTelemetryParams {
	t.Helper()
	select {
	case <-c.notify:
	case <-time.After(2 * time.Second):
		t.Fatal("expected playground chat telemetry within 2s")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	require.NotEmpty(t, c.rows)
	return c.rows[0]
}

type playgroundStreamProvider struct {
	stream func(w http.ResponseWriter)
}

func (p playgroundStreamProvider) Proxy(_ context.Context, _ router.Decision, _ providers.PreparedRequest, w http.ResponseWriter, _ *http.Request) error {
	if p.stream != nil {
		p.stream(w)
	}
	return nil
}

func (p playgroundStreamProvider) Passthrough(context.Context, providers.PreparedRequest, http.ResponseWriter, *http.Request) error {
	return nil
}

// TestPlaygroundChat_HandlerPersistsTelemetry asserts the dashboard playground
// chat path attributes telemetry to the caller's installation and playground key.
func TestPlaygroundChat_HandlerPersistsTelemetry(t *testing.T) {
	const installID = "11111111-1111-1111-1111-111111111111"
	inst := &auth.Installation{ID: installID, ExternalID: "acct-playground"}
	ext := &playgroundExternalKeyRepo{keys: []*auth.ExternalAPIKey{{
		ID: "ek-1", Provider: providers.ProviderAiand, Plaintext: []byte("sk-user"),
	}}}
	name := auth.PlaygroundAPIKeyName
	keyRepo := &playgroundAPIKeyRepo{keys: []*auth.APIKey{{
		ID:             "playground-key-id",
		InstallationID: installID,
		Name:           &name,
		Scope:          auth.ScopeRouting,
	}}}
	authSvc := auth.NewService(nil, keyRepo, ext, nil, auth.NoOpAPIKeyCache{}, nil, func() time.Time { return time.Now() })

	telem := newPlaygroundTelemetryCapture()
	svc := proxy.NewService(
		&playgroundFakeRouter{decision: router.Decision{
			Provider: providers.ProviderAiand,
			Model:    "deepseek-ai/deepseek-v4-flash",
			Reason:   "cluster:test",
		}},
		map[string]providers.Client{providers.ProviderAiand: playgroundStreamProvider{stream: func(w http.ResponseWriter) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"ok"}}]}` + "\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
		}}},
		nil, false, nil, nil, false,
		providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", telem,
	)

	rec := playgroundChatPOSTWithInst(
		admin.PlaygroundChatHandler(svc, authSvc),
		`{"model":"auto","messages":[{"role":"user","content":"hi"}],"stream":true}`,
		map[string]string{"X-Playground-Session": "pg-sess-1"},
		inst,
	)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "event: routing_metadata")

	row := telem.firstRow(t)
	assert.Equal(t, installID, row.InstallationID)
	assert.Equal(t, "playground-key-id", row.APIKeyID)
	assert.Equal(t, proxy.ClientAppPlayground, row.ClientApp)
	assert.Equal(t, "deepseek-ai/deepseek-v4-flash", row.DecisionModel)
	assert.NotEmpty(t, row.RequestID)
}
