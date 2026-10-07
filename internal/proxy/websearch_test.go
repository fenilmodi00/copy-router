package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"weave-os/router/internal/auth"
	"weave-os/router/internal/providers"
	"weave-os/router/internal/websearch"
)

type fakeSearch struct {
	got  websearch.Query
	resp websearch.Response
	err  error
	runs int
}

func (f *fakeSearch) Search(_ context.Context, q websearch.Query) (websearch.Response, error) {
	f.runs++
	f.got = q
	return f.resp, f.err
}

const searchTurnBody = `{
	"model":"claude-sonnet-5",
	"stream":false,
	"tools":[{"type":"web_search_20250305","name":"web_search"}],
	"messages":[{"role":"user","content":"Perform a web search for the query: cortex agents"}]
}`

func gatewayCtx() context.Context {
	return context.WithValue(context.Background(), CredentialsContextKey{}, &Credentials{
		APIKey:   []byte("WIF.GCP.token"),
		BaseURL:  "https://acct.snowflakecomputing.com/api/v2/cortex",
		AuthType: auth.AuthTypeWIF,
	})
}

// noNativeServerToolProviders is an enabled-provider set with no provider that
// runs Anthropic server tools natively, so the native web-search detector
// cannot defer to routing on that ground.
func noNativeServerToolProviders() map[string]struct{} {
	return map[string]struct{}{providers.ProviderAIAND: {}}
}

func TestServeNativeWebSearchDefersToNativelyCapableProvider(t *testing.T) {
	ex := &fakeSearch{}
	s := &Service{webSearch: ex}
	enabled := map[string]struct{}{providers.ProviderAnthropic: {}}

	if s.serveNativeWebSearch(gatewayCtx(), []byte(searchTurnBody), "claude-sonnet-5", false, 0, enabled, http.Header{}, httptest.NewRecorder()) {
		t.Fatal("vendor Anthropic runs the tool itself; the turn must stay on normal routing")
	}
	if ex.runs != 0 {
		t.Fatal("executor must not run when a capable provider is available")
	}
}

func TestServeNativeWebSearchIgnoresTurnsWithoutTheTool(t *testing.T) {
	ex := &fakeSearch{}
	s := &Service{webSearch: ex}
	body := []byte(`{"model":"claude-sonnet-5","messages":[{"role":"user","content":"Perform a web search for the query: x"}]}`)

	if s.serveNativeWebSearch(gatewayCtx(), body, "claude-sonnet-5", false, 0, noNativeServerToolProviders(), http.Header{}, httptest.NewRecorder()) {
		t.Fatal("no native tool declared; nothing to serve")
	}
	if ex.runs != 0 {
		t.Fatal("executor ran without a declared server tool")
	}
}

func TestServeNativeWebSearchDisabled(t *testing.T) {
	s := &Service{}
	if s.serveNativeWebSearch(gatewayCtx(), []byte(searchTurnBody), "claude-sonnet-5", false, 0, noNativeServerToolProviders(), http.Header{}, httptest.NewRecorder()) {
		t.Fatal("no executor wired; the turn must stay on normal routing")
	}
}
