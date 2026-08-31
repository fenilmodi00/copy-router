package rl

import (
	"testing"

	"workweave/router/internal/providers"
	"workweave/router/internal/router/catalog"

	"github.com/stretchr/testify/assert"
)

// rosterIDFor is now a pure alias map: ai& is the sole provider, so the
// per-provider switch and its vendor-prefix arms are gone. These tests pin
// the two behaviors that remain — the alias and the bare-ID passthrough.

func TestRosterIDForAppliesAlias(t *testing.T) {
	model := catalog.Model{ID: "moonshotai/kimi-k2.7"}
	assert.Equal(t, "moonshotai/kimi-k2.7-code", rosterIDFor(model))
}

func TestRosterIDForPassthroughUnaliased(t *testing.T) {
	for _, id := range []string{"zai-org/glm-5.3", "deepseek-ai/deepseek-v4-flash", "moonshotai/kimi-k3"} {
		assert.Equal(t, id, rosterIDFor(catalog.Model{ID: id}), "unaliased catalog ID must pass through")
	}
}

func TestRosterAliasesOnlyReferenceCatalogModels(t *testing.T) {
	for alias := range rosterAliases {
		_, ok := catalog.ByID(alias)
		assert.Truef(t, ok, "roster alias %q has no catalog model", alias)
		_ = providers.ProviderAiand // keep the import honest if catalog drops providers
	}
}
