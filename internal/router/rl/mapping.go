package rl

import "workweave/router/internal/router/catalog"

// rosterAliases maps a catalog model ID to the policy-artifact roster ID when
// the ai& upslot below would produce the wrong slug.
var rosterAliases = map[string]string{
	// The roster ID for kimi-k2.7 is ai&'s served upstream name, not the
	// canonical catalog ID.
	"moonshotai/kimi-k2.7": "moonshotai/kimi-k2.7-code",
}

// rosterIDFor returns the policy-artifact roster ID for a catalog model. The
// sidecar canonicalizes and intersects candidates against its own roster, so a
// best-effort slug is safe: a model the policy doesn't know is simply dropped.
func rosterIDFor(m catalog.Model) string {
	if alias, ok := rosterAliases[m.ID]; ok {
		return alias
	}
	return m.ID
}
