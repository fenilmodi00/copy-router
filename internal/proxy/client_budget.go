package proxy

import (
	"context"
	"net/http"
	"strings"

	"weave-os/router/internal/router"
	"weave-os/router/internal/translate"
)

const (
	// Verified against the shipped client's custom-base-URL window calculation.
	claudeCodeBudgetVersion        = "2.1.257"
	claudeCodeDefaultWindow        = 200_000
	claudeCodeCompactOutputReserve = 20_000
	claudeCodeAutoCompactBuffer    = 13_000
)

// resolveClientBudget carries the per-request client-budget hints the Claude
// Code harness exposes: whether the caller asked for the 1M context variant,
// whether the request carries the context-1m beta, and — for the known harness
// version — the default window the client compacts at. The window is a
// property of the shipped client, not of the served catalog row, so it applies
// on the AIand roster the same as it did on Anthropic-served claude-* models.
func resolveClientBudget(ctx context.Context, hadVariant bool, headers http.Header) router.ClientBudget {
	budget := router.ClientBudget{
		ModelVariant1M:   hadVariant,
		InboundContext1M: translate.HasContext1MBeta(headers),
	}
	identity := ClientIdentityFrom(ctx)
	if identity.ClientApp != ClientAppClaudeCode {
		return budget
	}
	versionAndMetadata, ok := strings.CutPrefix(identity.UserAgent, "claude-cli/")
	if !ok {
		return budget
	}
	budget.Version, _, _ = strings.Cut(versionAndMetadata, " ")
	if budget.Version != claudeCodeBudgetVersion {
		return budget
	}
	// ANTHROPIC_BETAS can add this capability without changing the local window.
	// The shipped client also strips a real [1m] suffix before sending it.
	if budget.InboundContext1M && !hadVariant {
		budget.Evidence = router.ClientBudgetAmbiguousLongContext
		return budget
	}
	budget.Evidence = router.ClientBudgetHarnessDefault
	budget.DefaultWindow = claudeCodeDefaultWindow
	if hadVariant {
		budget.DefaultWindow = 1_000_000
	}
	budget.DefaultCompactThreshold = budget.DefaultWindow - claudeCodeCompactOutputReserve - claudeCodeAutoCompactBuffer
	return budget
}
