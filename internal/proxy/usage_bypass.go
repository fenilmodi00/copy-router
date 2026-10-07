package proxy

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"weave-os/router/internal/billing"
	"weave-os/router/internal/providers"
)

// claudeSubscriptionExhausted reports whether the caller's Claude subscription
// is exhausted or actively drawing billable overage. True only when a Claude
// token has an observed spent/billable snapshot and a paid fallback key exists.
// The token key is derived identically to withUsageObserver so this read agrees
// with what the observer recorded. When true the caller suppresses the
// subscription credential (withSuppressedClaudeSubscription) so the turn serves
// on the Weave / BYOK key rather than the customer's credits.
func (s *Service) claudeSubscriptionExhausted(ctx context.Context, headers http.Header) bool {
	return s.anthropicFallbackKeyAvailable(ctx) && s.anthropicSubscriptionObservedExhausted(ctx, headers)
}

// anthropicSubscriptionObservedExhausted reports whether the caller's present
// Claude subscription is exhausted or using paid overage per the observer,
// independent of whether a fallback key exists. claudeSubscriptionExhausted
// layers the fallback-key requirement on top for its suppress-and-serve-on-Weave
// -key path; subscription-only refusal uses this bare signal because paid
// fallback is disabled there — an exhausted sub can only 429, while using
// overage would charge the customer. The turn is refused with a controlled 402.
func (s *Service) anthropicSubscriptionObservedExhausted(ctx context.Context, headers http.Header) bool {
	if s.usageObserver == nil {
		return false
	}
	_, anthroTok := presentSubscriptionTokens(ctx, headers)
	if anthroTok == "" {
		return false
	}
	snap, ok := s.usageObserver.Snapshot(s.usageObserver.Key([]byte(anthroTok)))
	return ok && snap.BillableOrExhausted()
}

// anthropicFallbackKeyAvailable reports whether a non-subscription Anthropic
// credential is configured to serve a Claude turn when the caller's subscription
// is spent: a per-request BYOK Anthropic key, or the deployment's own
// ANTHROPIC_API_KEY (tracked in deploymentKeyedProviders). Without one, dropping
// the subscription token would leave the turn with no Anthropic credential and
// 400 — strictly worse than the 429 — so the caller keeps using the subscription.
func (s *Service) anthropicFallbackKeyAvailable(ctx context.Context) bool {
	if byok := BuildCredentialsMap(externalKeysFromContext(ctx)); byok != nil {
		if _, ok := byok[providers.ProviderAnthropic]; ok {
			return true
		}
	}
	if s.deploymentKeyedProviders != nil {
		if _, ok := s.deploymentKeyedProviders[providers.ProviderAnthropic]; ok {
			return true
		}
	}
	return false
}

// anthropicOAuthCredentialRejected reports whether err is a buffered Anthropic
// 401 authentication_error or 403 permission_error — a rejected subscription
// OAuth token that gates the failover onto the BYOK/deployment key. Narrow by
// design so an unrelated 403 (e.g. content policy) stays terminal.
func anthropicOAuthCredentialRejected(err error) bool {
	var buffered *providers.UpstreamErrorResponse
	if !errors.As(err, &buffered) {
		return false
	}
	if buffered.Status != http.StatusUnauthorized && buffered.Status != http.StatusForbidden {
		return false
	}
	var env struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if jsonErr := json.Unmarshal(buffered.Body, &env); jsonErr != nil {
		return false
	}
	return env.Error.Type == "authentication_error" || env.Error.Type == "permission_error"
}

// topUpURL is the customer-facing page where org admins buy router credits.
// Duplicated from middleware.TopUpURL (proxy can't import the middleware
// adapter without an import cycle) so subscription-only warnings and the
// credits-exhausted 402 can surface the CTA.
const topUpURL = "https://app.workweave.ai/organization/settings/weave-router"

// subscriptionOnlyWarningMarker is prepended to a subscription-only response so
// the customer sees why they're being served on their own plan and how to
// restore full routing. The marker is emitted only when terminal surfaces are
// enabled for the installation and request.
const subscriptionOnlyWarningMarker = routingMarkerPrefix +
	"your Weave router credits are depleted, so this turn is running on your own Anthropic subscription and paid model fallback is disabled. Add credits to restore full routing: " +
	topUpURL + "\n\n"

// subscriptionOnlyWarningMarkerCodex is the Codex/OpenAI-surface counterpart to
// subscriptionOnlyWarningMarker, prepended to a subscription-only turn served on
// the caller's own ChatGPT (Codex) subscription.
const subscriptionOnlyWarningMarkerCodex = routingMarkerPrefix +
	"your Weave router credits are depleted, so this turn is running on your own ChatGPT (Codex) subscription and paid model fallback is disabled. Add credits to restore full routing: " +
	topUpURL + "\n\n"

// subscriptionOnlyWarnsDepleted reports whether a subscription-only turn is
// standing in for capacity the organization could not fund, and so must carry
// the depleted-credits warning and its top-up CTA. A linked-first turn is the
// ordinary funded path — the caller's own plan paying first by design — and
// keeps its routing marker, so it no longer claims credits are gone.
func subscriptionOnlyWarnsDepleted(ctx context.Context) bool {
	reason, ok := billing.SubscriptionOnlyReasonFromContext(ctx)
	return ok && reason == billing.SubscriptionOnlyCreditsDepleted
}

// subscriptionOnlyWarningMarkerForRequest returns the depletion warning only
// when the turn is subscription-only because credits are depleted and the
// caller has not opted out of terminal routing surfaces.
func subscriptionOnlyWarningMarkerForRequest(ctx context.Context, headers http.Header, marker string) string {
	if !subscriptionOnlyWarnsDepleted(ctx) {
		return ""
	}
	return suppressMarkerIfRequested(ctx, headers, marker)
}

// ErrCreditsExhaustedSubscriptionUnavailable is returned by ProxyMessages and
// ProxyOpenAIChatCompletion when the org is in subscription-only mode but the
// turn cannot be served on the caller's own
// subscription (Claude or Codex) at all — routing resolved to a paid model, or
// the subscription is already rate-limit exhausted. Paid failover is disabled
// in this mode, so the turn is refused (HTTP 402) rather than billed against an
// already-negative balance. A runtime failure on a turn that DID resolve onto
// the subscription surfaces the raw upstream error instead — it's the caller's
// own plan failing, with nowhere to fail over to.
var ErrCreditsExhaustedSubscriptionUnavailable = errors.New("credits exhausted and subscription unavailable for this turn")
