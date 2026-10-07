package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"weave-os/router/internal/providers"
	"weave-os/router/internal/router"
	"weave-os/router/internal/translate"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

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
