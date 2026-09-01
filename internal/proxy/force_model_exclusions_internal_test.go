package proxy

import (
	"context"
	"net/http/httptest"
	"testing"

	"workweave/router/internal/providers"
	"workweave/router/internal/translate"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func keyed(names ...string) map[string]struct{} {
	out := make(map[string]struct{}, len(names))
	for _, n := range names {
		out[n] = struct{}{}
	}
	return out
}

func forceCommandEnv(t *testing.T) *translate.RequestEnvelope {
	t.Helper()
	env, err := translate.ParseAnthropic([]byte(`{
		"model":"moonshotai/kimi-k3",
		"messages":[{"role":"user","content":"hi"}]
	}`))
	require.NoError(t, err)
	return env
}

// TestForceModelCommand_SessionStrikeOutDoesNotReject keeps transient 529
// evidence from masquerading as operator policy: a provider struck out for
// overload must not refuse a force onto one of its models.
func TestForceModelCommand_SessionStrikeOutDoesNotReject(t *testing.T) {
	store := &recordingPinStore{}
	svc := NewService(nil, nil, nil, false, nil, store, false,
		providers.ProviderAiand, "deepseek-ai/deepseek-v4-flash", nil).
		WithDeploymentKeyedProviders(keyed(providers.ProviderAiand))

	ctx := context.WithValue(context.Background(),
		SessionDisabledProvidersContextKey{}, []string{providers.ProviderAiand})
	env := forceCommandEnv(t)
	rec := httptest.NewRecorder()
	require.NoError(t, svc.handleForceModelCommand(ctx, rec, env,
		translate.ForceModelResult{Model: "zai-org/glm-5.2"},
		uuid.New(), DeriveSessionKey(env, "key-1"), 10))

	require.Len(t, store.upserts, 1,
		"a provider struck out for overload is not an exclusion")
}
