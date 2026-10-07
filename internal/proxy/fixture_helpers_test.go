package proxy_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"weave-os/router/internal/providers"
	"weave-os/router/internal/proxy"
	"weave-os/router/internal/router"
)

const (
	bypassRequestedMdl  = "deepseek-ai/deepseek-v4.1-flash"
	bypassScorerPickMdl = "zai-org/glm-5.3-flash"
)

// bypassFixture builds a service whose fake scorer routes to
// bypassScorerPickMdl with a fake Anthropic provider that returns a minimal
// valid Messages response so a routed turn completes. seedUtil is retained for
// callers that historically seeded a subscription observer; the observer lane
// is gone, so it is ignored.
func bypassFixture(t *testing.T, seedUtil float64) (*proxy.Service, *fakeRouter, *fakeProvider) {
	t.Helper()
	_ = seedUtil
	fr := &fakeRouter{decision: router.Decision{Provider: providers.ProviderAnthropic, Model: bypassScorerPickMdl}}
	p := &fakeProvider{proxyResponse: func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"msg_1","type":"message","role":"assistant","model":"` + bypassScorerPickMdl + `","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	}}
	svc := proxy.NewService(fr, map[string]providers.Client{providers.ProviderAnthropic: p}, nil, false, nil, nil, false, providers.ProviderAnthropic, bypassScorerPickMdl, nil)
	return svc, fr, p
}

// bypassCtx returns the request context for a bypassFixture turn.
func bypassCtx(threshold float64) context.Context {
	_ = threshold
	return context.Background()
}

func bypassRequest(t *testing.T) (*httptest.ResponseRecorder, *http.Request, []byte) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	body := []byte(`{"model":"` + bypassRequestedMdl + `","messages":[{"role":"user","content":"hi"}]}`)
	return rec, req, body
}

// A Codex (ChatGPT) subscription bearer is a JWT-shaped token (not sk-/rk_)
// paired with a ChatGPT-Account-ID header.
const (
	codexSubToken     = "eyJhbGciOi.codex.jwt"
	codexSubAccountID = "acct-codex-123"
)

// codexSubRequest builds an OpenAI chat-completions request carrying a Codex
// subscription in the inbound Authorization header.
func codexSubRequest(t *testing.T, body string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+codexSubToken)
	req.Header.Set("ChatGPT-Account-ID", codexSubAccountID)
	return httptest.NewRecorder(), req
}
