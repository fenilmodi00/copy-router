package proxy

import (
	"context"
	"testing"

	"workweave/router/internal/providers"
	"workweave/router/internal/router"
	"workweave/router/internal/router/bandswap"
	"workweave/router/internal/router/sessionpin"
	"workweave/router/internal/router/turntype"
)

// orderBandPair must put the stronger-tier model in `large` regardless of which
// half of the pin it is stored as.
func TestOrderBandPair_ByTier(t *testing.T) {
	want := func(large, small router.Decision, wantLarge, wantSmall string) {
		t.Helper()
		if large.Model != wantLarge || small.Model != wantSmall {
			t.Fatalf("large=%q small=%q, want large=%q small=%q", large.Model, small.Model, wantLarge, wantSmall)
		}
	}

	anchorLarge := sessionpin.Pin{
		Provider: providers.ProviderAiand, Model: "moonshotai/kimi-k3",
		PairedProvider: providers.ProviderAiand, PairedModel: "deepseek-ai/deepseek-v4-flash",
	}
	l, s := orderBandPair(anchorLarge)
	want(l, s, "moonshotai/kimi-k3", "deepseek-ai/deepseek-v4-flash")

	// Same pair, anchor and runner-up swapped -> identical large/small split.
	anchorSmall := sessionpin.Pin{
		Provider: providers.ProviderAiand, Model: "deepseek-ai/deepseek-v4-flash",
		PairedProvider: providers.ProviderAiand, PairedModel: "moonshotai/kimi-k3",
	}
	l, s = orderBandPair(anchorSmall)
	want(l, s, "moonshotai/kimi-k3", "deepseek-ai/deepseek-v4-flash")
}

// With the swap head disabled the sticky turn must serve the pin's anchor.
func TestBandSwapServed_DisabledServesAnchor(t *testing.T) {
	s := &Service{} // bandSwap nil
	pin := sessionpin.Pin{
		Provider: providers.ProviderAiand, Model: "moonshotai/kimi-k3",
		PairedProvider: providers.ProviderAiand, PairedModel: "deepseek-ai/deepseek-v4-flash", Reason: "cluster",
	}
	got := s.bandSwapServed(context.Background(), turntype.MainLoop, pin, router.Decision{}, false, nil, nil)
	if got.Model != "moonshotai/kimi-k3" {
		t.Fatalf("served %q, want anchor moonshotai/kimi-k3", got.Model)
	}
}

// A pin with no runner-up can never swap, even if the head were enabled.
func TestBandSwapServed_NoPairServesAnchor(t *testing.T) {
	s := &Service{}
	pin := sessionpin.Pin{Provider: providers.ProviderAiand, Model: "moonshotai/kimi-k3", Reason: "cluster"}
	got := s.bandSwapServed(context.Background(), turntype.MainLoop, pin, router.Decision{}, false, nil, nil)
	if got.Model != "moonshotai/kimi-k3" {
		t.Fatalf("served %q, want anchor", got.Model)
	}
}

// When the head picks the paired model but that model is unservable this turn
// — excluded by the context-window pre-filter, or bound to a provider the
// request can't use — the swap must fall back to the anchor rather than emit a
// decision that would fail downstream. Mirrors turnloop's sticky-pin guards.
func TestBandSwapServed_UnservableChoiceFallsBackToAnchor(t *testing.T) {
	clf, err := bandswap.New()
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	emb := make([]float32, bandswap.EmbedDim)
	for i := range emb {
		emb[i] = 0.01
	}
	_, band, ok := clf.PredictBand(emb)
	if !ok {
		t.Fatal("PredictBand not ok for valid-width embedding")
	}
	// Anchor the pin on the NOT-predicted member and pair it with a provider
	// that does not exist (prune: aiand is the only registered provider), so
	// honoring the head would be both a real swap and an ineligible dispatch.
	const large, small = "moonshotai/kimi-k3", "deepseek-ai/deepseek-v4-flash"
	served := large
	if band == bandswap.Small {
		served = small
	}
	anchor, paired := small, large
	if served == small {
		anchor, paired = large, small
	}

	s := &Service{
		embedOnlyUserMessage: true,
		bandSwap:             clf,
		availableModels:      map[string]struct{}{large: {}, small: {}},
		providers:            map[string]providers.Client{providers.ProviderAiand: nil},
	}
	pin := sessionpin.Pin{
		Provider: providers.ProviderAiand, Model: anchor,
		PairedProvider: "upstream-fallback", PairedModel: paired, Reason: "cluster",
	}
	fresh := router.Decision{Metadata: &router.RoutingMetadata{Embedding: emb}}

	// Chosen model is bound to a provider this deployment doesn't register ->
	// the swap must fall back to the anchor rather than emit a dead decision.
	if got := s.bandSwapServed(context.Background(), turntype.MainLoop, pin, fresh, false, nil, nil); got.Model != anchor {
		t.Fatalf("ineligible-provider swap served %q, want anchor %q", got.Model, anchor)
	}
}
