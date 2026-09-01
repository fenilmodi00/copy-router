package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

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
