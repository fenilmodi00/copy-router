package proxy

import (
	"context"
	"testing"

	"weave-os/router/internal/requestcontext"
	"weave-os/router/internal/router"

	"github.com/stretchr/testify/assert"
)

const (
	// The AIand roster carries no claude-* row, so the Claude Code harness
	// window inference in resolveClientBudget can no longer fire for any model
	// the router serves. These tests build the ClientBudget shape explicitly.
	budgetTestModel     = "zai-org/glm-5.3"
	budgetTestUserAgent = "claude-cli/2.1.257 (external, sdk-ts)"
	budgetTestBeta      = "context-1m-2025-08-07"
)

// smallClientBudget is the harness budget a Claude Code client reports for a
// 200K-window model. resolveClientBudget cannot derive it once the catalog has
// no claude-* row, so the shape is built directly for the consumers that need
// a known (non-ambiguous) budget.
func smallClientBudget() router.ClientBudget {
	return router.ClientBudget{
		Evidence:                router.ClientBudgetHarnessDefault,
		DefaultWindow:           claudeCodeDefaultWindow,
		DefaultCompactThreshold: claudeCodeDefaultWindow - claudeCodeCompactOutputReserve - claudeCodeAutoCompactBuffer,
	}
}

// The budget a Claude Code client reports is carried on the request context
// unchanged, and a child context's budget overrides it without disturbing the
// parent's.
func TestClientBudgetRecomputedAcrossSameSessionAndAgents(t *testing.T) {
	var parent context.Context = context.Background()
	for _, tt := range []struct {
		budget router.ClientBudget
		window int
	}{
		{router.ClientBudget{Evidence: router.ClientBudgetHarnessDefault, DefaultWindow: 1_000_000}, 1_000_000},
		{smallClientBudget(), claudeCodeDefaultWindow},
		{router.ClientBudget{Evidence: router.ClientBudgetHarnessDefault, DefaultWindow: 200_000}, 200_000},
	} {
		parent = requestcontext.WithClientBudget(parent, tt.budget)
		svc := &Service{}
		req := svc.withPolicyRequestContext(parent, router.Request{})
		assert.Equal(t, tt.window, req.ClientBudget.DefaultWindow)
	}
	child := requestcontext.WithClientBudget(parent, router.ClientBudget{})
	assert.Zero(t, requestcontext.ClientBudgetFrom(child).DefaultWindow)
	assert.Equal(t, 200_000, requestcontext.ClientBudgetFrom(parent).DefaultWindow)
}
