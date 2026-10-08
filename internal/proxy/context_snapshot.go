package proxy

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"weave-os/router/internal/router/turntype"
)

// ContextEstimateKind names how a ContextSnapshot's estimate was derived.
type ContextEstimateKind string

// ContextEstimateApproximate is the only estimate kind in version 1: a
// conservative whole-request estimate from the capacity-filtering envelope.
const ContextEstimateApproximate ContextEstimateKind = "approximate"

// ContextSnapshotVersion is the companion estimate contract version.
const ContextSnapshotVersion = 1

// ContextSnapshotTTL bounds how long a snapshot may be surfaced after the
// request completed; older snapshots are treated as unavailable.
const ContextSnapshotTTL = 5 * time.Minute

// maxContextSnapshotTokens bounds every token field so a malformed or hostile
// value can never overflow an int or render nonsense.
const maxContextSnapshotTokens = 2_147_483_647

// ContextSnapshot describes a single Router request, never client-local usage.
type ContextSnapshot struct {
	Version             int                 `json:"version"`
	EstimateKind        ContextEstimateKind `json:"estimate_kind"`
	EstimateTokens      int                 `json:"estimate_tokens"`
	ContextWindow       int                 `json:"context_window"`
	OutputReserveTokens int                 `json:"output_reserve_tokens"`
	RequestedModel      string              `json:"requested_model,omitempty"`
	ServedModel         string              `json:"served_model"`
	RequestID           string              `json:"request_id"`
	RequestedAt         time.Time           `json:"requested_at"`
	RecordedAt          time.Time           `json:"recorded_at"`
}

// Fresh reports whether the snapshot is a well-formed version-1 approximate
// snapshot recorded no more than ContextSnapshotTTL before now.
func (snapshot ContextSnapshot) Fresh(now time.Time) bool {
	return snapshot.Version == ContextSnapshotVersion && snapshot.EstimateKind == ContextEstimateApproximate &&
		snapshot.EstimateTokens > 0 && snapshot.EstimateTokens <= maxContextSnapshotTokens &&
		snapshot.ContextWindow > 0 && snapshot.ContextWindow <= maxContextSnapshotTokens &&
		snapshot.OutputReserveTokens > 0 && snapshot.OutputReserveTokens <= maxContextSnapshotTokens &&
		snapshot.ServedModel != "" && len(snapshot.ServedModel) <= 128 && len(snapshot.RequestedModel) <= 128 &&
		snapshot.RequestID != "" && len(snapshot.RequestID) <= 128 &&
		!snapshot.RequestedAt.IsZero() && !snapshot.RecordedAt.Before(snapshot.RequestedAt) &&
		!snapshot.RecordedAt.After(now) && now.Sub(snapshot.RecordedAt) <= ContextSnapshotTTL
}

// ParseContextSnapshot decodes a stored snapshot, returning nil for empty,
// oversized, or malformed payloads. Callers must still check Fresh before use.
func ParseContextSnapshot(encoded []byte) *ContextSnapshot {
	if len(encoded) == 0 || len(encoded) > 2048 {
		return nil
	}
	var snapshot ContextSnapshot
	if json.Unmarshal(encoded, &snapshot) != nil {
		return nil
	}
	return &snapshot
}

// setContextEstimateHeaders writes the companion estimate contract headers
// next to x-router-context-window. Always clears the four names first so a
// replayed or failover response can never surface stale values, and omits the
// whole contract when the estimate is non-positive (unavailable, never zero).
func setContextEstimateHeaders(headers http.Header, estimate, reserve int) {
	for _, name := range []string{HeaderRouterContextEstimate, HeaderRouterContextReserve, HeaderRouterContextEstimateKind, HeaderRouterContextVersion} {
		headers.Del(name)
	}
	if estimate <= 0 {
		return
	}
	headers.Set(HeaderRouterContextEstimate, strconv.Itoa(estimate))
	headers.Set(HeaderRouterContextEstimateKind, string(ContextEstimateApproximate))
	headers.Set(HeaderRouterContextVersion, strconv.Itoa(ContextSnapshotVersion))
	if reserve > 0 {
		headers.Set(HeaderRouterContextReserve, strconv.Itoa(reserve))
	}
}

// contextSnapshotJSONForDecision encodes a snapshot for the decision that
// actually served the turn. servedModel/servedProvider are authoritative over
// the headers because a failover rewrites them after the headers may have been
// captured.
func contextSnapshotJSONForDecision(headers http.Header, requestID, requestedModel, servedModel, servedProvider string, requestedAt, recordedAt time.Time) []byte {
	estimate, _ := strconv.Atoi(headers.Get(HeaderRouterContextEstimate))
	reserve, _ := strconv.Atoi(headers.Get(HeaderRouterContextReserve))
	window := contextWindowForRequest(servedModel, servedProvider)
	snapshot := ContextSnapshot{
		Version: ContextSnapshotVersion, EstimateKind: ContextEstimateKind(headers.Get(HeaderRouterContextEstimateKind)),
		EstimateTokens: estimate, ContextWindow: window, OutputReserveTokens: reserve,
		RequestedModel: requestedModel, ServedModel: servedModel, RequestID: requestID, RequestedAt: requestedAt.UTC(), RecordedAt: recordedAt.UTC(),
	}
	if !snapshot.Fresh(recordedAt) {
		return nil
	}
	encoded, _ := json.Marshal(snapshot)
	return encoded
}

// conversationContextTurn reports whether a turn is a conversation request
// whose snapshot should replace the session's cached context display. Title
// generation, probes, classifiers, recaps, and subagent dispatches do not.
func conversationContextTurn(kind turntype.TurnType) bool {
	switch kind {
	case turntype.MainLoop, turntype.ToolResult, turntype.Compaction:
		return true
	default:
		return false
	}
}
