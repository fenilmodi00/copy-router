package proxy

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"weave-os/router/internal/providers"
	"weave-os/router/internal/router/sessionpin"
)

// Local roster: every model binds ProviderAIAND (5m cache TTL). The baseline
// and the served model just have to be two different priced catalog IDs.
const (
	baselineModelID = "zai-org/glm-5.3"
	servedModelID   = "deepseek-ai/deepseek-v4-pro"
)

func priorTurnEndedAt(d time.Duration) time.Time {
	return time.Now().Add(-d)
}

// priorTurn is a previous turn whose 100k-token prompt the baseline would hold warm.
func priorTurn(model string, ago time.Duration) turnLoopResult {
	return turnLoopResult{PriorServedModel: model, PriorServedEndedAt: priorTurnEndedAt(ago), PriorPromptTokens: 100_000}
}

func TestBaselineWarmPrefillTokens_SwitchWithinBaselineTTLIsCorrected(t *testing.T) {
	turnResult := priorTurn(baselineModelID, 2*time.Minute)

	assert.Equal(t, 100_000, turnResult.baselineWarmPrefillTokens(time.Now(), 100_000, 0, servedModelID, baselineModelID, false),
		"switching away from the baseline pays a cold prefill the baseline would have read warm")
}

func TestBaselineWarmPrefillTokens_SwitchBackToBaselineIsCorrected(t *testing.T) {
	turnResult := priorTurn(servedModelID, 2*time.Minute)

	assert.Equal(t, 100_000, turnResult.baselineWarmPrefillTokens(time.Now(), 100_000, 0, baselineModelID, baselineModelID, false),
		"returning to the baseline re-primes a cache the baseline never let go cold")
}

// Content appended since the previous turn is a cache write for the baseline
// too; only the previous prompt, less what the served model did read, was warm.
func TestBaselineWarmPrefillTokens_LimitedToPreviousPrompt(t *testing.T) {
	turnResult := priorTurn(baselineModelID, 2*time.Minute)

	assert.Equal(t, 100_000, turnResult.baselineWarmPrefillTokens(time.Now(), 110_000, 0, servedModelID, baselineModelID, false),
		"10k of new content stays a write")
	assert.Equal(t, 95_000, turnResult.baselineWarmPrefillTokens(time.Now(), 105_000, 5_000, servedModelID, baselineModelID, false),
		"a 5k system prefix the served model already read is not repriced again")
}

// An effort change keeps the base model, and a baseline that never switched
// carries the client's effort anyway, so it is not a router-caused prefill.
func TestBaselineWarmPrefillTokens_EffortOnlyChangeIsNotASwitch(t *testing.T) {
	turnResult := priorTurn(baselineModelID+":low", time.Minute)

	assert.Zero(t, turnResult.baselineWarmPrefillTokens(time.Now(), 100_000, 0, baselineModelID, baselineModelID, false))
}

// The TTL that matters is the baseline's: an AIand binding caches for minutes,
// so a gap past 5m means the baseline would have re-primed too.
func TestBaselineWarmPrefillTokens_UsesBaselineProviderTTL(t *testing.T) {
	turnResult := priorTurn(servedModelID, 2*time.Minute)
	assert.Equal(t, 100_000, turnResult.baselineWarmPrefillTokens(time.Now(), 100_000, 0, baselineModelID, baselineModelID, false),
		"a 2m gap is inside the 5m cache TTL")

	turnResult = priorTurn(servedModelID, 20*time.Minute)
	assert.Zero(t, turnResult.baselineWarmPrefillTokens(time.Now(), 100_000, 0, baselineModelID, baselineModelID, false),
		"a 20m gap expires the cache, so the baseline would have re-primed too")
}

// A fresh HMM switch has no thread pin; the prior served turn lives only on
// the HMM history pin, and its timing and size must come from that same pin.
func TestApplySwitchHistory_ReadsThePriorTurnFromTheSelectedPin(t *testing.T) {
	hmmHistory := sessionpin.Pin{
		LastServedModel:      baselineModelID,
		Provider:             providers.ProviderAnthropic,
		LastTurnEndedAt:      priorTurnEndedAt(2 * time.Minute),
		LastInputTokens:      2_000,
		LastCachedReadTokens: 98_000,
	}
	var turnResult turnLoopResult
	turnResult.applySwitchHistory(sessionpin.Pin{}, hmmHistory)

	assert.Equal(t, 100_000, turnResult.baselineWarmPrefillTokens(time.Now(), 110_000, 0, servedModelID, baselineModelID, false))
}

// Anthropic-family bindings report usage in the Anthropic shape, so their
// cache counters are disjoint from input_tokens like first-party. The local
// tree registers no gateway Anthropic provider, so register one to prove the
// branch keys on the wire family.
func TestPriorPromptTokens_AnthropicFamilyAddsDisjointCacheCounters(t *testing.T) {
	providers.ProviderFamilies["anthropic_gateway"] = providers.FamilyAnthropic
	t.Cleanup(func() { delete(providers.ProviderFamilies, "anthropic_gateway") })

	for _, provider := range []string{providers.ProviderAnthropic, "anthropic_gateway"} {
		pin := sessionpin.Pin{Provider: provider, LastInputTokens: 2_000, LastCachedReadTokens: 90_000, LastCachedWriteTokens: 8_000}
		assert.Equal(t, 100_000, priorPromptTokens(pin), provider)
	}
	openAI := sessionpin.Pin{Provider: providers.ProviderOpenAI, LastInputTokens: 100_000, LastCachedReadTokens: 90_000}
	assert.Equal(t, 100_000, priorPromptTokens(openAI), "OpenAI prompt_tokens already include cached tokens")
}

func TestBaselineWarmPrefillTokens_BaselineWouldAlsoBeCold(t *testing.T) {
	cases := map[string]struct {
		turnResult       turnLoopResult
		baseline         string
		historyTruncated bool
	}{
		"first turn":           {turnResult: turnLoopResult{PriorServedEndedAt: priorTurnEndedAt(time.Minute), PriorPromptTokens: 100_000}, baseline: baselineModelID},
		"no prior turn time":   {turnResult: turnLoopResult{PriorServedModel: baselineModelID, PriorPromptTokens: 100_000}, baseline: baselineModelID},
		"no prior prompt size": {turnResult: turnLoopResult{PriorServedModel: baselineModelID, PriorServedEndedAt: priorTurnEndedAt(time.Minute)}, baseline: baselineModelID},
		"client trimmed":       {turnResult: turnLoopResult{PriorServedModel: baselineModelID, PriorServedEndedAt: priorTurnEndedAt(time.Minute), PriorPromptTokens: 100_000, PrefixTrimmed: true}, baseline: baselineModelID},
		"ingress truncation":   {turnResult: priorTurn(baselineModelID, time.Minute), baseline: baselineModelID, historyTruncated: true},
		"unpriced baseline":    {turnResult: priorTurn(baselineModelID, time.Minute), baseline: "not-a-model"},
		"cache gap past TTL":   {turnResult: priorTurn(baselineModelID, 2*time.Hour), baseline: baselineModelID},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Zero(t, tc.turnResult.baselineWarmPrefillTokens(time.Now(), 100_000, 0, servedModelID, tc.baseline, tc.historyTruncated))
		})
	}
}
