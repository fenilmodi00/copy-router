package catalog

import (
	"testing"

	"weave-os/router/internal/providers"

	"github.com/stretchr/testify/assert"
)

// The AIand roster exposes no fast tier (no binding carries FastPrice), so
// fast-mode pricing resolves for nothing.

func TestFastPriceFor_NoFastTierOnRoster(t *testing.T) {
	cases := []struct{ name, provider, id string }{
		{"roster deepseek-pro has no fast tier", providers.ProviderAIAND, "deepseek-ai/deepseek-v4-pro"},
		{"roster glm-5.3 has no fast tier", providers.ProviderAIAND, "zai-org/glm-5.3"},
		{"unknown model", providers.ProviderAIAND, "no-such-model"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := FastPriceFor(tc.provider, tc.id)
			assert.False(t, ok)
		})
	}
}

func TestSupportsFastMode_NoRosterRowSupportsIt(t *testing.T) {
	for _, id := range aiandRoster {
		assert.Falsef(t, SupportsFastMode(id), "%s must not advertise a fast tier", id)
	}
	assert.False(t, SupportsFastMode("unknown"))
}

func TestCatalog_FastPriceOnlyOnFirstPartyBindingsAndAboveList(t *testing.T) {
	for _, m := range Models {
		for _, b := range m.Providers {
			fast, ok := b.FastPricing()
			if !ok {
				continue
			}
			assert.Greater(t, fast.OutputUSDPer1M, b.Price.OutputUSDPer1M, "%s/%s fast output must cost more than list", m.ID, b.Provider)
			assert.Equal(t, b.Price.CacheReadMultiplier, fast.CacheReadMultiplier, "%s/%s cache discount carries over", m.ID, b.Provider)
		}
	}
}
