package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"weave-os/router/internal/providers"
	"weave-os/router/internal/router"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace"
)

type flowSpanRouter struct {
	spanID   trace.SpanID
	decision router.Decision
}

func (r *flowSpanRouter) Route(ctx context.Context, _ router.Request) (router.Decision, error) {
	r.spanID = trace.SpanContextFromContext(ctx).SpanID()
	if r.decision.Model != "" {
		return r.decision, nil
	}
	return router.Decision{
		Provider: providers.ProviderAnthropic,
		Model:    "zai-org/glm-5.3-flash",
		Reason:   "cluster",
	}, nil
}

type flowSpanProvider struct {
	spanID trace.SpanID
}

func (p *flowSpanProvider) Proxy(ctx context.Context, _ router.Decision, _ providers.PreparedRequest, _ http.ResponseWriter, _ *http.Request) error {
	p.spanID = trace.SpanContextFromContext(ctx).SpanID()
	return nil
}

func (p *flowSpanProvider) Passthrough(context.Context, providers.PreparedRequest, http.ResponseWriter, *http.Request) error {
	return nil
}

func TestProxyMessagesEmitsHighLevelFlowSpans(t *testing.T) {
	recorder := tracetest.NewSpanRecorder()
	provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	t.Cleanup(func() { require.NoError(t, provider.Shutdown(context.Background())) })

	previousTracer := proxyFlowTracer
	proxyFlowTracer = provider.Tracer("test")
	t.Cleanup(func() { proxyFlowTracer = previousTracer })

	routerImpl := &flowSpanRouter{}
	providerImpl := &flowSpanProvider{}
	svc := NewService(
		routerImpl,
		map[string]providers.Client{providers.ProviderAnthropic: providerImpl},
		nil,
		false,
		nil,
		nil,
		false,
		providers.ProviderAnthropic,
		"zai-org/glm-5.3-flash",
		nil,
	)

	ctx, requestSpan := provider.Tracer("test").Start(context.Background(), "request")
	ctx = context.WithValue(ctx, ClientIdentityContextKey{}, ClientIdentity{SessionID: "client-session-abc"})
	body := []byte(`{"model":"zai-org/glm-5.3","max_tokens":4096,"tools":[{"name":"Read","input_schema":{"type":"object"}}],"messages":[{"role":"user","content":"inspect the repository"}]}`)
	err := svc.ProxyMessages(ctx, body, httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/messages", nil))
	requestSpan.End()
	require.NoError(t, err)

	routing := proxyEndedSpanByName(t, recorder.Ended(), "router.routing")
	inference := proxyEndedSpanByName(t, recorder.Ended(), "router.inference")
	assert.Equal(t, requestSpan.SpanContext().SpanID(), routing.Parent().SpanID())
	assert.Equal(t, requestSpan.SpanContext().SpanID(), inference.Parent().SpanID())
	assert.Equal(t, routing.SpanContext().SpanID(), routerImpl.spanID)
	assert.Equal(t, inference.SpanContext().SpanID(), providerImpl.spanID)
	assert.Equal(t, codes.Ok, routing.Status().Code)
	assert.Equal(t, codes.Ok, inference.Status().Code)
	assert.Equal(t, "client-session-abc", proxySpanAttribute(t, routing.Attributes(), "client.session_id").AsString())
	assert.Equal(t, "client-session-abc", proxySpanAttribute(t, inference.Attributes(), "client.session_id").AsString())
	assert.Equal(t, "zai-org/glm-5.3", proxySpanAttribute(t, routing.Attributes(), "requested.model").AsString())
	assert.Equal(t, "zai-org/glm-5.3-flash", proxySpanAttribute(t, routing.Attributes(), "decision.model").AsString())
	assert.Equal(t, providers.ProviderAnthropic, proxySpanAttribute(t, inference.Attributes(), "served.provider").AsString())
}

func proxyEndedSpanByName(t *testing.T, spans []sdktrace.ReadOnlySpan, name string) sdktrace.ReadOnlySpan {
	t.Helper()
	for _, span := range spans {
		if span.Name() == name {
			return span
		}
	}
	require.FailNow(t, "span not found", "name=%s", name)
	return nil
}

func proxySpanAttribute(t *testing.T, attrs []attribute.KeyValue, key string) attribute.Value {
	t.Helper()
	for _, attr := range attrs {
		if string(attr.Key) == key {
			return attr.Value
		}
	}
	require.FailNow(t, "span attribute not found", "key=%s", key)
	return attribute.Value{}
}
