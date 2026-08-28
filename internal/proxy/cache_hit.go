// Package proxy is the routing/dispatch service.
package proxy

import (
	"context"
	"net/http"
	"time"

	"workweave/router/internal/billing"
	"workweave/router/internal/observability"
	"workweave/router/internal/observability/otel"
	"workweave/router/internal/router"
	"workweave/router/internal/router/cache"
	"workweave/router/internal/translate"
)

// tryServeSemanticCacheHit extracts the duplicated cache-hit logic shared by
// ProxyMessages (FormatAnthropic) and ProxyOpenAIChatCompletion (FormatOpenAI).
//
// Eligibility: semantic cache enabled + non-nil, not streaming, decision has
// routing metadata, externalID present, not eval/override traffic, no compaction
// handover on the same turn (Anthropic) / not responses passthrough (OpenAI),
// not subscription-only, and subsidy factors empty (the cache key doesn't
// capture headroom-dependent model choice).
//
// On hit: writes the cached response (no feedback link — replaying would attribute
// a new client's rating to the wrong request_id), fires semantic-cache-hit
// telemetry, records a router.cache_hit OTel span, flushes the emitter, and logs.
//
// Returns true when the cached response was served (caller returns nil); false
// otherwise. The caller owns the format argument (FormatAnthropic / FormatOpenAI)
// so the two wire formats never share entries.
func (s *Service) tryServeSemanticCacheHit(
	ctx context.Context,
	w http.ResponseWriter,
	format cache.Format,
	feats translate.RoutingFeatures,
	decision router.Decision,
	routeRes turnLoopResult,
	sessionKey [16]byte,
	requestStart time.Time,
	routeMs int64,
	externalID string,
	stream bool,
	extraExclusion bool,
	r *http.Request,
	bypassEval bool,
	usageBypassEngaged bool,
) bool {
	log := observability.FromContext(ctx)
	installationID := installationIDFromContext(ctx)
	apiKeyID, _ := ctx.Value(APIKeyIDContextKey{}).(string)
	requestID := requestIDFor(ctx)

	cacheMeta := cacheMetadataFor(decision, routeRes)
	cacheEligible := s.semanticCacheAllowed(ctx) && s.semanticCache != nil && !usageBypassEngaged && !stream && cacheMeta != nil && externalID != "" && !bypassEval && !extraExclusion && !billing.SubscriptionOnlyFromContext(ctx) && len(s.subsidyFactors(ctx, r.Header)) == 0

	if !cacheEligible {
		return false
	}

	if resp, hit := s.semanticCache.Lookup(externalID, format, cacheMeta.Embedding, cacheMeta.ClusterIDs, cacheMeta.ClusterRouterVersion, cacheMeta.EffectiveKnobsHash); hit {
		s.writeCachedResponse(w, resp, decision)
		s.fireSemanticCacheHitTelemetry(ctx, installationID, apiKeyID, requestID, feats, decision, routeRes, sessionKey, requestStart, routeMs)
		otel.Record(ctx, otel.Span{
			Name:  "router.cache_hit",
			Start: requestStart,
			End:   time.Now(),
			Attrs: otel.NewAttrBuilder(7).
				String("request_id", requestID).
				String("external_id", externalID).
				String("decision.model", decision.Model).
				String("decision.provider", decision.Provider).
				Bool("cache.hit", true).
				String("cache.format", string(format)).
				Int64("latency.total_ms", time.Since(requestStart).Milliseconds()).
				Build(),
		})
		otel.Flush(ctx)
		log.Info("Semantic cache hit", "requested_model", feats.Model, "baseline_model", s.baselineFor(feats.Model), "decision_model", decision.Model, "decision_provider", decision.Provider, "external_id", externalID, "total_ms", time.Since(requestStart).Milliseconds())
		return true
	}

	return false
}
