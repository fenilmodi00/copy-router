package proxy_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"weave-os/router/internal/providers"
	"weave-os/router/internal/proxy"
	"weave-os/router/internal/proxy/usage"
	"weave-os/router/internal/router"
)

const (
	bypassSubToken      = "sk-ant-oat01-subscription-token"
	bypassRequestedMdl  = "deepseek-ai/deepseek-v4.1-flash"
	bypassScorerPickMdl = "zai-org/glm-5.3-flash"
)

// bypassFixture builds a service whose fake scorer would route to
// bypassScorerPickMdl, with the subscription usage observer wired and a fake
// Anthropic provider that returns a minimal valid Messages response so a routed
// turn completes. seedUtil >= 0 pre-records an observation at that utilization
// under the subscription token; seedUtil < 0 leaves the observer cold.
func bypassFixture(t *testing.T, seedUtil float64) (*proxy.Service, *fakeRouter, *fakeProvider) {
	t.Helper()
	fr := &fakeRouter{decision: router.Decision{Provider: providers.ProviderAnthropic, Model: bypassScorerPickMdl}}
	p := &fakeProvider{proxyResponse: func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"` + bypassScorerPickMdl + `","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}}
	obs := usage.NewObserver([]byte("salt"), 10*time.Minute, time.Now)
	if seedUtil >= 0 {
		obs.Record(obs.Key([]byte(bypassSubToken)), usage.Snapshot{
			Primary: usage.Window{UsedPercent: seedUtil, WindowMinutes: 300},
		})
	}
	svc := proxy.NewService(fr, map[string]providers.Client{providers.ProviderAnthropic: p}, nil, false, nil, nil, false, providers.ProviderAnthropic, bypassScorerPickMdl, nil).
		WithSubscriptionAwareRouting(obs, 0.05, 2.0)
	return svc, fr, p
}

// bypassCtx returns a ctx carrying a Claude subscription token plus a
// per-installation usage-bypass config at the given threshold.
func bypassCtx(threshold float64) context.Context {
	ctx := context.WithValue(context.Background(), proxy.AnthropicSubscriptionContextKey{}, bypassSubToken)
	return context.WithValue(ctx, proxy.InstallationUsageBypassContextKey{}, proxy.UsageBypassConfig{
		Enabled:   true,
		Threshold: &threshold,
	})
}

func bypassRequest(t *testing.T) (*httptest.ResponseRecorder, *http.Request, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	body := []byte(`{"model":"` + bypassRequestedMdl + `","messages":[{"role":"user","content":"hi"}]}`)
	return rec, req, body
}

// bypassStreamResponse writes a minimal valid Anthropic SSE stream so the
// subscription-only warning marker (which only injects on a streaming response)
// has a stream to prepend to.
func bypassStreamResponse(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_up\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"" + bypassRequestedMdl + "\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":1,\"output_tokens\":0}}}\n\n"))
	_, _ = w.Write([]byte("event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n"))
	_, _ = w.Write([]byte("event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n\n"))
	_, _ = w.Write([]byte("event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n"))
	_, _ = w.Write([]byte("event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\n"))
	_, _ = w.Write([]byte("event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"))
}
