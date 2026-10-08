package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"weave-os/router/internal/providers"
	"weave-os/router/internal/requestcontext"
	"weave-os/router/internal/router"
	"weave-os/router/internal/subscriptions/entitlement"
	"weave-os/router/internal/translate"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

// ctxWithCreds stashes explicit credentials on a fresh context.
func ctxWithCreds(creds *Credentials) context.Context {
	return context.WithValue(context.Background(), CredentialsContextKey{}, creds)
}

// testInstallationID is a fixed installation uuid for router-keyed test ctx.
const testInstallationID = "11111111-1111-1111-1111-111111111111"

// anthropicMessagesBody is a minimal Anthropic Messages body with tools.
func anthropicMessagesBody() []byte {
	return []byte(`{"model":"zai-org/glm-5.3","max_tokens":1024,"stream":true,"messages":[{"role":"user","content":"read main.go"}],"tools":[{"name":"read_file","input_schema":{"type":"object","properties":{"path":{"type":"string"}}}}]}`)
}

// openaiChatBody is a minimal OpenAI Chat Completions streaming body.
func openaiChatBody() []byte {
	return []byte(`{"model":"moonshotai/kimi-k3","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
}

// ctxWithAllowedModels stashes an installation model allowlist on a fresh context.
func ctxWithAllowedModels(models ...string) context.Context {
	return context.WithValue(context.Background(), InstallationAllowedModelsContextKey{}, models)
}

// planOwnedContext builds a product-scoped (Max) request context.
func planOwnedContext() context.Context {
	return planOwnedContextFor(entitlement.PlanMax)
}

func planOwnedContextFor(plan entitlement.Plan) context.Context {
	alpha := 0.9
	ctx := requestcontext.WithServingIdentity(context.Background(), requestcontext.ServingIdentity{Plan: string(plan)})
	ctx = entitlement.WithProductScope(ctx, plan)
	ctx = context.WithValue(ctx, InstallationAllowedModelsContextKey{}, []string{"customer-allowed"})
	ctx = context.WithValue(ctx, InstallationExcludedModelsContextKey{}, []string{"customer-excluded"})
	ctx = context.WithValue(ctx, InstallationExcludedProvidersContextKey{}, []string{"customer-provider"})
	ctx = context.WithValue(ctx, InstallationPreferredModelsContextKey{}, []string{"customer-preferred"})
	ctx = context.WithValue(ctx, InstallationFastModeModelsContextKey{}, []string{"customer-fast"})
	ctx = context.WithValue(ctx, ClusterModelListsContextKey{}, map[string][]string{"cluster": {"customer-arm"}})
	return router.WithRoutingKnobs(ctx, &router.Overrides{Alpha: &alpha})
}

// bypassFakeProvider is an internal-package fake providers.Client that records
// what it received, so a test can assert on the dispatched request and on
// whether any bytes were committed.
type bypassFakeProvider struct {
	proxyErr     error
	respBody     string
	dispatches   int
	capturedW    http.ResponseWriter
	capturedBody []byte
	capturedR    *http.Request
	capturedCtx  context.Context
	capturedDec  router.Decision
}

func (f *bypassFakeProvider) Proxy(ctx context.Context, decision router.Decision, prep providers.PreparedRequest, w http.ResponseWriter, r *http.Request) error {
	f.dispatches++
	f.capturedCtx = ctx
	f.capturedDec = decision
	f.capturedW = w
	f.capturedR = r
	f.capturedBody = append([]byte(nil), prep.Body...)
	if f.respBody != "" {
		_, _ = io.WriteString(w, f.respBody)
	}
	return f.proxyErr
}

func (f *bypassFakeProvider) Passthrough(context.Context, providers.PreparedRequest, http.ResponseWriter, *http.Request) error {
	return nil
}

func bypassAnthropicEnvelope(t *testing.T) *translate.RequestEnvelope {
	t.Helper()
	env, err := translate.ParseAnthropic([]byte(`{"model":"deepseek-ai/deepseek-v4.1-flash","messages":[{"role":"user","content":"hi"}]}`))
	require.NoError(t, err)
	return env
}

// bypassSpanCollector is an in-process OTLP endpoint that records spans by name for assertion.
type bypassSpanCollector struct {
	srv    *httptest.Server
	mu     sync.Mutex
	byName map[string][]*tracev1.Span
}

func newBypassSpanCollector(t *testing.T) *bypassSpanCollector {
	t.Helper()
	c := &bypassSpanCollector{byName: make(map[string][]*tracev1.Span)}
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		var req coltracepb.ExportTraceServiceRequest
		require.NoError(t, proto.Unmarshal(body, &req))
		c.mu.Lock()
		for _, rs := range req.ResourceSpans {
			for _, ss := range rs.ScopeSpans {
				for _, sp := range ss.Spans {
					c.byName[sp.Name] = append(c.byName[sp.Name], sp)
				}
			}
		}
		c.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(c.srv.Close)
	return c
}

func spanStr(t *testing.T, sp *tracev1.Span, key string) string {
	t.Helper()
	for _, kv := range sp.Attributes {
		if kv.Key == key {
			sv, ok := kv.Value.Value.(*commonv1.AnyValue_StringValue)
			require.True(t, ok, "attr %q must be a string", key)
			return sv.StringValue
		}
	}
	t.Fatalf("attr %q not present on span", key)
	return ""
}

func spanBool(t *testing.T, sp *tracev1.Span, key string) bool {
	t.Helper()
	for _, kv := range sp.Attributes {
		if kv.Key == key {
			bv, ok := kv.Value.Value.(*commonv1.AnyValue_BoolValue)
			require.True(t, ok, "attr %q must be a bool", key)
			return bv.BoolValue
		}
	}
	t.Fatalf("attr %q not present on span", key)
	return false
}

// bypassCaptureTelemetry records InsertTelemetryParams rows for assertions.
// Only InsertRequestTelemetry matters; the read methods satisfy the interface.
type bypassCaptureTelemetry struct {
	panicTelemetryRepo // inherit no-op reads
	mu                 sync.Mutex
	rows               []InsertTelemetryParams
	notify             chan struct{}
}

func newBypassCaptureTelemetry() *bypassCaptureTelemetry {
	return &bypassCaptureTelemetry{notify: make(chan struct{}, 4)}
}

func (c *bypassCaptureTelemetry) InsertRequestTelemetry(_ context.Context, p InsertTelemetryParams) error {
	c.mu.Lock()
	c.rows = append(c.rows, p)
	c.mu.Unlock()
	select {
	case c.notify <- struct{}{}:
	default:
	}
	return nil
}
