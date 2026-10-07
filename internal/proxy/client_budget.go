package proxy

import (
	"net/http"

	"weave-os/router/internal/router"
	"weave-os/router/internal/translate"
)

const (
	// Verified against the shipped client's custom-base-URL window calculation.
	claudeCodeDefaultWindow        = 200_000
	claudeCodeCompactOutputReserve = 20_000
	claudeCodeAutoCompactBuffer    = 13_000
)

// resolveClientBudget carries the per-request client-budget hints the Claude
// Code harness exposes: whether the caller asked for the 1M context variant and
// whether the request carries the context-1m beta. The harness default-window
// inference it used to derive is gone with the Anthropic catalog rows — no
// served model is a claude-* row, so no client default window is knowable here.
// Consumers that need a concrete window build the shape directly.
func resolveClientBudget(hadVariant bool, headers http.Header) router.ClientBudget {
	return router.ClientBudget{
		ModelVariant1M:   hadVariant,
		InboundContext1M: translate.HasContext1MBeta(headers),
	}
}
