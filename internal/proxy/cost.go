package proxy

import (
	"context"
	"fmt"

	"workweave/router/internal/router"
	"workweave/router/internal/router/catalog"
	"workweave/router/internal/translate"
)

const (
	decisionCostMinInputTokens      = 1
	decisionCostDefaultOutputTokens = 1024
)

// CostForDecision computes requested and actual USD costs for a routing decision.
// requestedCostUSD uses catalog.PrimaryPriceFor(decision.Model); actualCostUSD
// uses catalog.PriceFor(decision.Provider, decision.Model), applying cache
// multipliers via catalog.EffectiveInputCost/EffectiveOutputCost. Unknown models
// yield (0, 0, nil). Tokens are total input/output tokens for the call.
func (s *Service) CostForDecision(ctx context.Context, decision router.Decision, inputTokens, outputTokens int) (requestedCostUSD, actualCostUSD float64, err error) {
	model := catalog.CanonicalModel(decision.Model)
	if model == "" {
		return 0, 0, fmt.Errorf("decision model is required")
	}
	primaryPrice, ok := catalog.PrimaryPriceFor(model)
	if !ok {
		// Unknown model: return zeros, no error.
		return 0, 0, nil
	}
	requestedCostUSD = catalog.EffectiveInputCost(inputTokens, 0, 0, primaryPrice.InputUSDPer1M, primaryPrice, "anthropic") + catalog.EffectiveOutputCost(outputTokens, primaryPrice.OutputUSDPer1M)

	actualPrice, ok := catalog.PriceFor(decision.Provider, model)
	if !ok {
		// Provider-specific pricing missing; fall back to primary.
		actualPrice = primaryPrice
	}
	actualCostUSD = catalog.EffectiveInputCost(inputTokens, 0, 0, actualPrice.InputUSDPer1M, actualPrice, decision.Provider) + catalog.EffectiveOutputCost(outputTokens, actualPrice.OutputUSDPer1M)

	return requestedCostUSD, actualCostUSD, nil
}

// DecisionCostTokens returns input/output token counts for pre-dispatch cost
// estimates (playground route preview, routing_metadata SSE). Short user text
// can round RoutingFeatures.Tokens down to zero; the body-byte estimate and a
// floor keep catalog-priced models from showing $0.00. Output defaults to
// max_tokens when set, otherwise decisionCostDefaultOutputTokens.
func DecisionCostTokens(body []byte, routingTextTokens, maxTokens int) (input, output int) {
	input = routingTextTokens
	if bodyEstimate := OpenAIRequestInputTokenEstimate(body); bodyEstimate > input {
		input = bodyEstimate
	}
	if input < decisionCostMinInputTokens && len(body) > 0 {
		input = decisionCostMinInputTokens
	}
	output = maxTokens
	if output <= 0 {
		output = decisionCostDefaultOutputTokens
	}
	return input, output
}

// CanonicalDecisionModel normalizes a served model id for dashboard surfaces.
func CanonicalDecisionModel(model string) string {
	return catalog.CanonicalModel(model)
}

// OpenAIRequestInputTokenEstimate returns the routing token estimate for an
// OpenAI Chat Completions body without running the scorer.
func OpenAIRequestInputTokenEstimate(body []byte) int {
	env, err := translate.ParseOpenAI(body)
	if err != nil {
		return 0
	}
	return env.ContextOverflowTokenEstimate()
}

// PlaygroundReasonShort maps internal routing reason strings to short
// user-facing labels for the dashboard playground.
func PlaygroundReasonShort(reason string) string {
	if reason == "" {
		return "Auto-routed"
	}
	switch reason {
	case translate.ReasonUserForceModel:
		return "Forced model"
	case translate.ReasonLoopEscalation:
		return "Loop escalation"
	case translate.ReasonStruggleEscalation:
		return "Quality escalation"
	}
	if len(reason) >= 8 && reason[:8] == "cluster:" {
		return "Auto-routed"
	}
	return reason
}
