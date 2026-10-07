package proxy_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"weave-os/router/internal/providers"
	"weave-os/router/internal/providers/openai"
	"weave-os/router/internal/proxy"
	"weave-os/router/internal/router"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeOpenAIResponsesSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	for _, c := range []string{
		`data: {"type":"response.created","response":{"id":"resp_1","status":"in_progress"}}` + "\n\n",
		`data: {"type":"response.output_text.delta","delta":"hi"}` + "\n\n",
		`data: {"type":"response.completed","response":{"id":"resp_1","status":"completed","usage":{"input_tokens":5,"output_tokens":1}}}` + "\n\n",
	} {
		_, _ = w.Write([]byte(c))
		if flusher != nil {
			flusher.Flush()
		}
	}
}

func writeOpenAIChatSSE(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	flusher, _ := w.(http.Flusher)
	chunks := []string{
		`data: {"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-5.4-mini","choices":[{"index":0,"delta":{"role":"assistant","content":"hi"},"finish_reason":null}]}` + "\n\n",
		`data: {"id":"c1","object":"chat.completion.chunk","created":1,"model":"gpt-5.4-mini","choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":1}}` + "\n\n",
		"data: [DONE]\n\n",
	}
	for _, c := range chunks {
		_, _ = w.Write([]byte(c))
		if flusher != nil {
			flusher.Flush()
		}
	}
}

const anthropicToollessTurn = `{"model":"gpt-5.4-mini","stream":true,"max_tokens":1024,` +
	`"messages":[{"role":"user","content":"summarize this repo"}]}`

func directOpenAIService(t *testing.T, baseURL string, broad bool) *proxy.Service {
	t.Helper()
	return proxy.NewService(
		&fakeRouter{decision: router.Decision{Provider: providers.ProviderOpenAI, Model: "gpt-5.4-mini"}},
		map[string]providers.Client{
			providers.ProviderOpenAI: openai.NewClient("test-key", baseURL),
		},
		nil, false, nil, nil, false, providers.ProviderAnthropic, "claude-haiku-4-5", nil,
	).WithDeploymentKeyedProviders(map[string]struct{}{providers.ProviderOpenAI: {}}).
		WithOpenAIResponsesBroad(broad)
}

// Under the broad rollout, direct-OpenAI serves every expressible turn on
// Responses; with it off a toolless turn keeps the chat projection.
func TestProxyMessages_DirectOpenAIToollessTurnFollowsRollout(t *testing.T) {
	for _, tc := range []struct {
		name      string
		broad     bool
		wantPaths []string
	}{
		{name: "rollout on", broad: true, wantPaths: []string{"/v1/responses"}},
		{name: "rollout off", broad: false, wantPaths: []string{"/v1/chat/completions"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var (
				mu    sync.Mutex
				paths []string
			)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				paths = append(paths, r.URL.Path)
				mu.Unlock()
				if strings.HasSuffix(r.URL.Path, "/responses") {
					writeOpenAIResponsesSSE(w)
					return
				}
				writeOpenAIChatSSE(w)
			}))
			defer upstream.Close()

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
			require.NoError(t, directOpenAIService(t, upstream.URL, tc.broad).
				ProxyMessages(context.Background(), []byte(anthropicToollessTurn), rec, req))

			mu.Lock()
			defer mu.Unlock()
			assert.Equal(t, tc.wantPaths, paths)
			assert.Contains(t, rec.Body.String(), "event: message_start")
		})
	}
}

// Stop sequences have no Responses equivalent, so the turn stays on
// chat/completions instead of silently dropping them.
func TestProxyMessages_DirectOpenAIStopSequencesStayOnChat(t *testing.T) {
	var (
		mu    sync.Mutex
		paths []string
	)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()
		writeOpenAIChatSSE(w)
	}))
	defer upstream.Close()

	body := `{"model":"gpt-4.1","stream":true,"max_tokens":1024,"stop_sequences":["\n\nHuman:"],` +
		`"messages":[{"role":"user","content":"finish the sentence"}]}`
	svc := proxy.NewService(
		&fakeRouter{decision: router.Decision{Provider: providers.ProviderOpenAI, Model: "gpt-4.1"}},
		map[string]providers.Client{
			providers.ProviderOpenAI: openai.NewClient("test-key", upstream.URL),
		},
		nil, false, nil, nil, false, providers.ProviderAnthropic, "claude-haiku-4-5", nil,
	).WithDeploymentKeyedProviders(map[string]struct{}{providers.ProviderOpenAI: {}})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(""))
	require.NoError(t, svc.ProxyMessages(context.Background(), []byte(body), rec, req))

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, []string{"/v1/chat/completions"}, paths)
}
