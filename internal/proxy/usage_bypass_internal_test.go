package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"workweave/router/internal/providers"
	"workweave/router/internal/router"
	"workweave/router/internal/translate"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

// bypassFakeProvider is an internal-package fake providers.Client for
// bypassToAnthropic tests. It records the response writer it received so a
// test can assert no bytes were committed on a retryable error.
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

// newBypassService builds a minimal *Service wired only with the Anthropic
// provider for direct bypassToAnthropic tests. The full ProxyMessages path is
// not exercised here; only bypassToAnthropic is.
func newBypassService(p providers.Client) *Service {
	return &Service{
		providers: map[string]providers.Client{providers.ProviderAnthropic: p},
	}
}

func bypassAnthropicEnvelope(t *testing.T) *translate.RequestEnvelope {
	t.Helper()
	env, err := translate.ParseAnthropic([]byte(`{"model":"deepseek-ai/deepseek-v4-pro","messages":[{"role":"user","content":"hi"}]}`))
	require.NoError(t, err)
	return env
}

// subscriptionCtx returns a ctx carrying a Claude subscription token (as the
// auth middleware would stash it from X-Weave-Anthropic-Subscription) plus a
// non-empty installation id so resolveAndInjectCredentials takes the
// router-keyed subscription-first branch.
func subscriptionCtx() context.Context {
	ctx := context.WithValue(context.Background(), AnthropicSubscriptionContextKey{}, "sk-ant-oat01-test-subscription-token")
	return context.WithValue(ctx, InstallationIDContextKey{}, "11111111-1111-1111-1111-111111111111")
}

// TestSubscriptionFailover_EligibilityAndSuppression covers the three
// load-bearing predicates of the subscription-credit failover added for the
// 429/header-timeout bug: a subscription-served Anthropic turn is detected as
// such, a deployment Anthropic key counts as a fallback, and suppressing the
// subscription flips credential resolution onto the deployment key.
func TestSubscriptionFailover_EligibilityAndSuppression(t *testing.T) {
	// A request whose Anthropic credential resolves to the caller's subscription.
	ctx := resolveAndInjectCredentials(subscriptionCtx(), providers.ProviderAnthropic, "moonshotai/kimi-k3", http.Header{})
	require.True(t, servedOnSubscription(ctx), "a resolved subscription token must report servedOnSubscription")

	t.Run("no fallback key: not eligible", func(t *testing.T) {
		s := &Service{} // no deployment Anthropic key, no BYOK
		assert.False(t, s.anthropicFallbackKeyAvailable(ctx),
			"without a Weave/BYOK Anthropic key there is nothing to fail over to")
	})

	t.Run("deployment Anthropic key present: eligible", func(t *testing.T) {
		s := &Service{deploymentKeyedProviders: map[string]struct{}{providers.ProviderAnthropic: {}}}
		assert.True(t, s.anthropicFallbackKeyAvailable(ctx),
			"a deployment Anthropic key is a valid failover target for a throttled subscription")
	})

	t.Run("suppression flips resolution off the subscription", func(t *testing.T) {
		// After withSuppressedClaudeSubscription, resolution must NOT pick the
		// subscription token — so the retry dispatches on the deployment key and
		// servedOnSubscription reports false (billed at full cost, not sub rate).
		suppressed := withSuppressedClaudeSubscription(subscriptionCtx())
		suppressed = resolveAndInjectCredentials(suppressed, providers.ProviderAnthropic, "moonshotai/kimi-k3", http.Header{})
		assert.False(t, servedOnSubscription(suppressed),
			"a suppressed subscription must not resolve back as the served credential")
	})
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

func spanInt(t *testing.T, sp *tracev1.Span, key string) int64 {
	t.Helper()
	for _, kv := range sp.Attributes {
		if kv.Key == key {
			iv, ok := kv.Value.Value.(*commonv1.AnyValue_IntValue)
			require.True(t, ok, "attr %q must be an int", key)
			return iv.IntValue
		}
	}
	t.Fatalf("attr %q not present on span", key)
	return 0
}

func spanFloat(t *testing.T, sp *tracev1.Span, key string) float64 {
	t.Helper()
	for _, kv := range sp.Attributes {
		if kv.Key == key {
			dv, ok := kv.Value.Value.(*commonv1.AnyValue_DoubleValue)
			require.True(t, ok, "attr %q must be a double", key)
			return dv.DoubleValue
		}
	}
	t.Fatalf("attr %q not present on span", key)
	return 0
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
