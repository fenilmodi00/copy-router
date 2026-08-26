package proxy

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"workweave/router/internal/providers"
	"workweave/router/internal/router"
	"workweave/router/internal/router/hmm"
	"workweave/router/internal/router/policy"
	"workweave/router/internal/router/sessionpin"
	"workweave/router/internal/translate"
)

// policyDeadlineTestErr mirrors the real chain: policyclient wraps context.DeadlineExceeded with %w;
// SidecarRouter wraps that with hmm.ErrHMMUnavailable via %w:%w. Both must survive for errors.Is.
var policyDeadlineTestErr = fmt.Errorf(
	"hmm_embedding: sidecar decide: policy sidecar retries exhausted: %w: %w",
	context.DeadlineExceeded,
	hmm.ErrHMMUnavailable,
)

// policyContractViolationTestErr mirrors a contract violation (sidecar_router.go:367/370): wraps
// ErrHMMUnavailable without a deadline/cancel, so isPolicyDeadlineErr must return false.
var policyContractViolationTestErr = fmt.Errorf(
	"hmm_embedding: sidecar returned unknown arm %q or model %q: %w",
	"bogus-arm", "bogus-model",
	hmm.ErrHMMUnavailable,
)

// erroringTestRouter always returns a fixed error, simulating a policy
// sidecar deadline/transport failure or contract violation.
type erroringTestRouter struct {
	err error
}

func (r *erroringTestRouter) Route(_ context.Context, _ router.Request) (router.Decision, error) {
	return router.Decision{}, r.err
}

func TestIsPolicyDeadlineErr(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "deadline exceeded wrapping ErrHMMUnavailable",
			err:  policyDeadlineTestErr,
			want: true,
		},
		{
			name: "context canceled wrapping ErrHMMUnavailable",
			err: fmt.Errorf("hmm_embedding: sidecar decide: %w: %w",
				context.Canceled, hmm.ErrHMMUnavailable),
			want: true,
		},
		{
			name: "contract violation (unknown arm) must still fail closed",
			err:  policyContractViolationTestErr,
			want: false,
		},
		{
			name: "contract violation (provider mismatch) must still fail closed",
			err: fmt.Errorf("hmm_embedding: sidecar returned provider %q for %q, expected %q: %w",
				"openai", "moonshotai/kimi-k3", "anthropic", hmm.ErrHMMUnavailable),
			want: false,
		},
		{
			name: "ErrHMMUnavailable without a deadline/cancel is not a deadline error",
			err:  fmt.Errorf("hmm_embedding: sidecar unavailable: %w", hmm.ErrHMMUnavailable),
			want: false,
		},
		{
			name: "unrelated error",
			err:  errors.New("boom"),
			want: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := isPolicyDeadlineErr(test.err)
			assert.Equal(t, test.want, got)
		})
	}
}

// buildPolicyDeadlineFallbackService constructs a *Service wired with an
// erroring policy strategy so runTurnLoop's routeFor call fails with err.
func buildPolicyDeadlineFallbackService(
	t *testing.T,
	strategy router.Strategy,
	err error,
	store sessionpin.Store,
	fallbackEnabled bool,
	defaultModel string,
) *Service {
	t.Helper()
	return NewService(
		nil,
		map[string]providers.Client{providers.ProviderAnthropic: nil},
		nil,
		false,
		nil,
		store,
		false,
		providers.ProviderAiand,
		"deepseek-ai/deepseek-v4-flash",
		nil,
	).WithPolicyDeadlineFallback(fallbackEnabled).
		WithPolicyDeadlineDefaultModel(defaultModel).
		WithPolicyStrategy(policy.StrategySpec{
			Strategy: strategy,
			Router:   &erroringTestRouter{err: err},
			Capabilities: policy.Capabilities{
				SchemaVersion: policy.SchemaVersionV1,
			},
		})
}

func runPolicyDeadlineFallbackTurnLoop(
	t *testing.T,
	svc *Service,
	strategy router.Strategy,
	excludedModels ...string,
) (turnLoopResult, error) {
	t.Helper()
	env, err := translate.ParseAnthropic(
		[]byte(`{"model":"moonshotai/kimi-k3","messages":[{"role":"user","content":"continue"}]}`),
	)
	require.NoError(t, err)
	features := env.RoutingFeatures(false)
	ctx := router.WithStrategy(context.Background(), strategy)
	var excluded map[string]struct{}
	if len(excludedModels) > 0 {
		excluded = make(map[string]struct{}, len(excludedModels))
		for _, model := range excludedModels {
			excluded[model] = struct{}{}
		}
	}
	req := router.Request{
		RequestedModel:       features.Model,
		EstimatedInputTokens: features.Tokens,
		HasTools:             features.HasTools,
		ConversationMessages: conversationMessagesForRouting(env),
		ExcludedModels:       excluded,
	}
	return svc.runTurnLoop(ctx, env, features, "api-key", uuid.New(), "", http.Header{}, req)
}
