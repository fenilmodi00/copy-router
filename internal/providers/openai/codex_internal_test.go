package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"weave-os/router/internal/providers"
	"weave-os/router/internal/requestcontext"
	"weave-os/router/internal/router"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func codexCtx(token, accountID string) context.Context {
	return context.WithValue(context.Background(), requestcontext.CredentialsContextKey{}, &requestcontext.Credentials{
		APIKey:    []byte(token),
		AccountID: []byte(accountID),
		Source:    "codex_subscription",
		OAuth:     true,
	})
}

func TestProxy_CodexSubscriptionCredentialRejectsInfrastructureModel(t *testing.T) {
	c := NewClient("deployment-key", "https://api.openai.example")
	prep := providers.PreparedRequest{
		Body:     []byte(`{"model":"gpt-5.4-nano","input":"hi"}`),
		Endpoint: providers.EndpointResponses,
		Headers:  make(http.Header),
	}
	rec := httptest.NewRecorder()
	clientReq := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(""))

	err := c.Proxy(
		codexCtx("eyJhbGciOiJ-codex-jwt", "acct-12345"),
		router.Decision{Model: "gpt-5.4-nano", Provider: providers.ProviderOpenAI},
		prep,
		rec,
		clientReq,
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "refusing Codex subscription credential")
}

// TestProxy_CodexCredOnChatEndpointIsRejected guards the credential boundary:
// ChatGPT OAuth is accepted only by the Codex Responses endpoint.
func TestProxy_CodexCredOnChatEndpointIsRejected(t *testing.T) {
	requests := 0
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	c := NewClient("deployment-key", upstream.URL)
	c.codexBaseURL = "https://chatgpt.example.invalid"

	prep := providers.PreparedRequest{Body: []byte(`{"model":"gpt-5.6-sol","messages":[]}`), Headers: make(http.Header)}
	rec := httptest.NewRecorder()
	clientReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(""))

	ctx := codexCtx("eyJhbGciOiJ-codex-jwt", "acct-12345")
	err := c.Proxy(ctx, router.Decision{Model: "gpt-5.6-sol", Provider: providers.ProviderOpenAI}, prep, rec, clientReq)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Codex subscription credentials require a Responses endpoint")
	assert.Zero(t, requests, "Codex OAuth must never be sent to the public Chat Completions endpoint")
}

// TestProxy_NoCodexCredHitsOpenAI confirms the Codex switch is gated on the
// subscription credential: a normal (deployment-key) request still targets
// api.openai.com and sends no ChatGPT-Account-ID header.
func TestProxy_NoCodexCredHitsOpenAI(t *testing.T) {
	var gotPath, gotAccount string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAccount = r.Header.Get("ChatGPT-Account-ID")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()

	c := NewClient("deployment-key", upstream.URL)
	c.codexBaseURL = "https://chatgpt.example.invalid" // must NOT be used

	prep := providers.PreparedRequest{Body: []byte(`{"model":"deepseek-ai/deepseek-v4-pro"}`), Headers: make(http.Header)}
	rec := httptest.NewRecorder()
	clientReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(""))

	err := c.Proxy(context.Background(), router.Decision{Model: "deepseek-ai/deepseek-v4-pro", Provider: providers.ProviderOpenAI}, prep, rec, clientReq)
	require.NoError(t, err)

	assert.Equal(t, "/v1/chat/completions", gotPath)
	assert.Empty(t, gotAccount, "a non-subscription request must not send the Codex account-id header")
}

// TestProxy_ResponsesMaxEffortMatchesPublicModelMenu guards the per-model
// distinction: GPT-5.6 rejects max publicly, while GPT-6 Astra accepts it.
func TestProxy_ResponsesMaxEffortMatchesPublicModelMenu(t *testing.T) {
	for _, tc := range []struct {
		model      string
		wantEffort string
	}{
		{model: "gpt-5.6-sol", wantEffort: "xhigh"},
		{model: "gpt-6-astra", wantEffort: "max"},
	} {
		t.Run(tc.model, func(t *testing.T) {
			var gotPath string
			var gotBody []byte
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				gotBody, _ = io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				w.WriteHeader(http.StatusOK)
				_, _ = io.WriteString(w, "event: response.completed\ndata: {\"type\":\"response.completed\"}\n\n")
			}))
			defer upstream.Close()

			c := NewClient("deployment-key", upstream.URL)
			c.codexBaseURL = "https://chatgpt.example.invalid"
			body := []byte(`{"model":"` + tc.model + `","input":"hi","stream":true,"reasoning":{"effort":"max","summary":"detailed"}}`)
			prep := providers.PreparedRequest{Body: body, Endpoint: providers.EndpointResponses, Headers: make(http.Header)}
			err := c.Proxy(
				context.Background(),
				router.Decision{Model: tc.model, Provider: providers.ProviderOpenAI},
				prep,
				httptest.NewRecorder(),
				httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader("")),
			)
			require.NoError(t, err)

			assert.Equal(t, "/v1/responses", gotPath)
			assert.Equal(t, tc.wantEffort, gjson.GetBytes(gotBody, "reasoning.effort").String())
			assert.Equal(t, "detailed", gjson.GetBytes(gotBody, "reasoning.summary").String())
		})
	}
}
