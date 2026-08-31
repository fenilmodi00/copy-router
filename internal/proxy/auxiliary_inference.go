package proxy

import (
	"context"
	"time"

	"github.com/google/uuid"

	"workweave/router/internal/auth"
	"workweave/router/internal/router/catalog"
	"workweave/router/internal/router/handover"
)

// SpanTypeAuxiliaryInference marks a telemetry row for a router-originated
// provider call that is not the client's turn: the switch-handover summary,
// the pre-compaction summary, and the compaction-handover summary. These are
// part of the session's total cost, so a session's cost total is wrong without
// them.
//
// Deliberately distinct from "router.upstream": every dashboard, analytics
// export, and offline-policy query pins span_type = 'router.upstream' and
// would misread an auxiliary call as a served turn (inflating request counts,
// polluting decision-model breakdowns, and feeding the policy trainer rows no
// policy produced). Consumers that want the true session total opt in by
// naming this span type — see the Weave public session-cost endpoint.
const SpanTypeAuxiliaryInference = "router.auxiliary_inference"

// Auxiliary request-id suffixes. They mirror the historical credit-ledger's
// router_request_id suffixes exactly, so a legacy ledger row and its
// telemetry row join on request_id without a second convention to keep in
// sync.
const (
	auxSuffixHandoverSummary          = "_summary"
	auxSuffixPrecompactionSummary     = "_precompaction_summary"
	auxSuffixCompactionHandoverSummry = "_compaction_summary"
)

// emitAuxiliaryInferenceTelemetry records one router-originated auxiliary
// provider call as a session-tagged telemetry row (what the session-cost
// endpoint sums), priced from the same catalog pricing as the served turn.
//
// No-ops when the usage carries no model or no tokens — a skipped or failed
// summarizer costs nothing and must not fabricate a zero-token row.
//
// requestIDSuffix distinguishes the auxiliary call from the turn that
// triggered it: the turn's own row keeps the bare request id, so the
// (installation_id, request_id, span_type) unique index admits both, and two
// different auxiliary calls on one turn stay distinct rows.
func (s *Service) emitAuxiliaryInferenceTelemetry(ctx context.Context, requestID, requestIDSuffix, externalID string, usage handover.Usage) {
	if usage.Model == "" || (usage.InputTokens == 0 && usage.OutputTokens == 0) {
		return
	}
	pricing, _ := catalog.PrimaryPriceFor(usage.Model)
	apiKeyID := apiKeyIDFromContext(ctx)
	auxRequestID := requestID + requestIDSuffix

	installationID := installationIDFromContext(ctx)
	if installationID == uuid.Nil {
		return
	}
	clientID := ClientIdentityFrom(ctx)
	inputCost := catalog.EffectiveInputCost(usage.InputTokens, usage.CacheCreation, usage.CacheRead,
		pricing.InputUSDPer1M, pricing, usage.Provider)
	outputCost := catalog.EffectiveOutputCost(usage.OutputTokens, pricing.OutputUSDPer1M)

	s.fireTelemetry(InsertTelemetryParams{
		InstallationID:   installationID.String(),
		APIKeyID:         apiKeyID,
		RequestID:        auxRequestID,
		SpanType:         SpanTypeAuxiliaryInference,
		TraceID:          requestID,
		Timestamp:        time.Now(),
		DecisionModel:    usage.Model,
		DecisionProvider: usage.Provider,
		// The client never asked for this call, so there is no requested model
		// to price it against. Requested cost mirrors actual cost so a
		// savings calculation (requested - actual) contributes zero rather
		// than crediting the router for a call it added.
		RequestedInputCostUSD:  inputCost,
		RequestedOutputCostUSD: outputCost,
		ActualInputCostUSD:     inputCost,
		ActualOutputCostUSD:    outputCost,
		InputTokens:            int32(usage.InputTokens),
		OutputTokens:           int32(usage.OutputTokens),
		CacheCreationTokens:    cacheTokenPtr(usage.CacheCreation),
		CacheReadTokens:        cacheTokenPtr(usage.CacheRead),
		DeviceID:               clientID.DeviceID,
		SessionID:              clientID.SessionID,
		RouterUserID:           auth.UserIDFrom(ctx),
		ClientApp:              clientID.ClientApp,
		RolloutID:              clientID.RolloutID,
	})
}
