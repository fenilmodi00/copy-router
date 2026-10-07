package proxy

import (
	"context"

	"weave-os/router/internal/router"
)

// fixedRouter routes every request to one decision.
type fixedRouter struct{ decision router.Decision }

func (f fixedRouter) Route(context.Context, router.Request) (router.Decision, error) {
	return f.decision, nil
}

// TestProxyOpenAIChatCompletion_ManagedPoolFailureIsRenderedOnce was deleted
// with the AIand-only cut: the managed Codex pool is only leased for models
// codexSubscriptionCanAttemptModel accepts (managedSubscriptionProviderFromUpstream),
// and the roster holds no ChatGPT/Codex-subscription model. Without pool
// engagement the throttle surfaces raw instead of as ErrSubscriptionPoolExhausted,
// so the rendered-once guarantee can no longer be exercised here.
