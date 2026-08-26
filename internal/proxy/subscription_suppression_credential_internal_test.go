package proxy

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"

	"workweave/router/internal/providers"
)

const routerKeyedInstallationID = "11111111-1111-1111-1111-111111111111"

// routerKeyedCtx mimics an authenticated managed request: the auth middleware
// stashed an installation ID, so installationIDFromContext != nil and
// resolveAndInjectCredentials treats it as router-keyed.
func routerKeyedCtx() context.Context {
	return context.WithValue(context.Background(), InstallationIDContextKey{}, routerKeyedInstallationID)
}

// subscriptionDisabledCtx returns a router-keyed ctx with subscription routing
// disabled (the "use my subscription first" toggle off).
func subscriptionDisabledCtx() context.Context {
	return context.WithValue(routerKeyedCtx(), InstallationSubscriptionRoutingDisabledContextKey{}, true)
}

// Regression guard: an inbound Codex subscription bearer on the router-key path
// must not slip back in via the client-credential branch when the toggle is off.
func TestResolveAndInjectCredentials_DisabledCodexNotReResolvedFromContext(t *testing.T) {
	spentCodex := &Credentials{APIKey: []byte("eyJhbGciOi.codex.jwt"), AccountID: []byte("acct-1"), Source: credSourceCodexSubscription, OAuth: true}
	ctx := context.WithValue(subscriptionDisabledCtx(), CredentialsContextKey{}, spentCodex)
	headers := http.Header{}
	headers.Set("Authorization", "Bearer eyJhbGciOi.codex.jwt")
	headers.Set("ChatGPT-Account-ID", "acct-1")

	out := resolveAndInjectCredentials(ctx, providers.ProviderOpenAI, "openai/gpt-oss-120b", headers)

	assert.Nil(t, CredentialsFromContext(out),
		"a disabled Codex subscription carried on ctx must be cleared, not re-resolved")
}
