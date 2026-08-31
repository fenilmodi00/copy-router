package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"workweave/router/internal/observability"
	"workweave/router/internal/router"
)

type respondRoutingMetadataContextKey struct{}

// WithRespondRoutingMetadata marks ctx so ProxyOpenAIChatCompletion emits a
// routing_metadata SSE event for the served decision (playground UI).
func WithRespondRoutingMetadata(ctx context.Context) context.Context {
	return context.WithValue(ctx, respondRoutingMetadataContextKey{}, true)
}

// RespondRoutingMetadata reports whether the OpenAI chat path should emit
// routing_metadata on the response stream.
func RespondRoutingMetadata(ctx context.Context) bool {
	v, _ := ctx.Value(respondRoutingMetadataContextKey{}).(bool)
	return v
}

// RoutingMetadataPayload is the playground routing_metadata SSE event body.
type RoutingMetadataPayload struct {
	Model            string  `json:"model"`
	Provider         string  `json:"provider"`
	Reason           string  `json:"reason"`
	RequestedCostUSD float64 `json:"requested_cost_usd"`
	ActualCostUSD    float64 `json:"actual_cost_usd"`
	CacheSavingsUSD  float64 `json:"cache_savings_usd"`
	ID               string  `json:"id"`
}

// emitOpenAIRoutingMetadataEvent writes a routing_metadata SSE frame.
func emitOpenAIRoutingMetadataEvent(w io.Writer, p RoutingMetadataPayload) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "event: routing_metadata\ndata: %s\n\n", data)
	return err
}

func routingMetadataPayload(
	s *Service,
	ctx context.Context,
	decision router.Decision,
	requestID string,
	body []byte,
	routingTextTokens int,
	maxTokens int,
) RoutingMetadataPayload {
	inputTokens, outputTokens := DecisionCostTokens(body, routingTextTokens, maxTokens)
	reqCost, actCost, err := s.CostForDecision(ctx, decision, inputTokens, outputTokens)
	if err != nil {
		reqCost, actCost = 0, 0
	}
	return RoutingMetadataPayload{
		Model:            CanonicalDecisionModel(decision.Model),
		Provider:         decision.Provider,
		Reason:           PlaygroundReasonShort(decision.Reason),
		RequestedCostUSD: reqCost,
		ActualCostUSD:    actCost,
		CacheSavingsUSD:  CacheInputSavings(decision, 0).Float64(),
		ID:               requestID,
	}
}

func maybeEmitOpenAIRoutingMetadata(
	ctx context.Context,
	s *Service,
	w http.ResponseWriter,
	streaming bool,
	responsesPassthrough bool,
	isResponses bool,
	decision router.Decision,
	requestID string,
	body []byte,
	routingTextTokens int,
	maxTokens int,
) {
	if !RespondRoutingMetadata(ctx) || !streaming || responsesPassthrough || isResponses {
		return
	}
	payload := routingMetadataPayload(s, ctx, decision, requestID, body, routingTextTokens, maxTokens)
	if err := emitOpenAIRoutingMetadataEvent(w, payload); err != nil {
		observability.FromContext(ctx).Debug("Failed to emit routing metadata", "err", err)
		return
	}
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}
