package hmm

import (
	"strings"

	"workweave/router/internal/providers"
	"workweave/router/internal/router"
	"workweave/router/internal/router/catalog"
	"workweave/router/internal/translate"
)

var rosterAliases = map[string]string{
	// The roster ID for kimi-k2.7 is ai&'s served upstream name, not the
	// canonical catalog ID.
	"moonshotai/kimi-k2.7": "moonshotai/kimi-k2.7-code",
}

func rosterIDFor(m catalog.Model) string {
	if alias, ok := rosterAliases[m.ID]; ok {
		return alias
	}
	if strings.Contains(m.ID, "/") {
		return m.ID
	}
	switch m.PrimaryProvider() {
	case providers.ProviderAiand:
		// ai& serves each model under its catalog (upstream) ID, so no
		// vendor prefix applies.
		return m.ID
	}
	return ""
}

// SplitEffort splits a roster arm ID of the form "model:effort" into its base model and canonical effort level; bare arms return (armID, "").
func SplitEffort(armID string) (baseID string, effort string) {
	if idx := strings.LastIndex(armID, ":"); idx > 0 {
		suffix := armID[idx+1:]
		if !translate.IsValidEffort(suffix) {
			return armID, ""
		}
		return armID[:idx], router.NormalizeLegacyEffort(translate.CanonicalizeEffort(suffix))
	}
	return armID, ""
}

// EffortArm composes a roster arm ID from a base model ID and effort level.
func EffortArm(baseID, effort string) string {
	if effort == "" {
		return baseID
	}
	return baseID + ":" + effort
}

// CatalogIDForRoster maps a roster arm ID back to its catalog model ID via the
// same forward mapping the resolver uses. Returns the arm ID unchanged when no
// catalog model maps to it.
func CatalogIDForRoster(rosterID string) string {
	baseID, _ := SplitEffort(rosterID)
	for _, m := range catalog.Models {
		if rosterIDFor(m) == baseID {
			return m.ID
		}
	}
	return rosterID
}
