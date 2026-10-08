package proxy

import (
	"context"
	"testing"

	"weave-os/router/internal/providers"
	"weave-os/router/internal/requestcontext"
	"weave-os/router/internal/router"
	"weave-os/router/internal/router/catalog"

	"github.com/stretchr/testify/assert"
)

const (
	cacheTestModelA = "deepseek-ai/deepseek-v4-pro"
	cacheTestModelB = "zai-org/glm-5.3"
)

func TestSemanticCacheStoreRejectsChangedDispatch(t *testing.T) {
	service := &Service{}
	decision := router.Decision{Model: cacheTestModelA, Provider: providers.ProviderAIAND}
	ctx := context.WithValue(context.Background(), APIKeyIDContextKey{}, "verified-key")
	ctx = requestcontext.WithCredentials(ctx, &Credentials{Source: credSourceBYOK, PrincipalID: "account"})
	provenance := service.semanticCacheProvenance(ctx, decision)
	assert.True(t, provenance.Valid())
	assert.True(t, service.semanticCacheStoreAllowed(ctx, provenance, decision, decision.Provider))
	changed := decision
	changed.Model = cacheTestModelB
	assert.False(t, service.semanticCacheStoreAllowed(ctx, provenance, changed, changed.Provider))
	assert.False(t, service.semanticCacheStoreAllowed(ctx, provenance, decision, providers.ProviderAnthropic))
}

func TestSemanticCacheManagedIdentityRequiresServingTuple(t *testing.T) {
	service := &Service{}
	decision := router.Decision{Model: cacheTestModelA, Provider: providers.ProviderAIAND}
	ctx := context.WithValue(context.Background(), APIKeyIDContextKey{}, "verified-key")
	ctx = requestcontext.WithCredentials(ctx, &Credentials{Source: credSourceBYOK, PrincipalID: "account"})
	identity := requestcontext.ServingIdentity{CredentialIdentity: "subject", ReleaseID: "release", BindingID: "binding"}
	assert.True(t, service.semanticCacheProvenance(requestcontext.WithServingIdentity(ctx, identity), decision).Valid())
	for name, remove := range map[string]func(*requestcontext.ServingIdentity){
		"release": func(i *requestcontext.ServingIdentity) { i.ReleaseID = "" },
		"binding": func(i *requestcontext.ServingIdentity) { i.BindingID = "" },
		"both":    func(i *requestcontext.ServingIdentity) { i.ReleaseID = ""; i.BindingID = "" },
	} {
		t.Run(name, func(t *testing.T) {
			partial := identity
			remove(&partial)
			assert.False(t, service.semanticCacheProvenance(requestcontext.WithServingIdentity(ctx, partial), decision).Valid())
		})
	}
}

func TestSemanticCacheOriginalRequirementsAndAutomaticExclusionsBypass(t *testing.T) {
	service := &Service{}
	assert.True(t, service.semanticCacheRequestAllowed(context.Background(), router.Request{}))
	for _, requirements := range []router.TranslationRequirements{{StructuredOutput: true}, {UsageDetail: true}, {FunctionTools: true}} {
		ctx := context.WithValue(context.Background(), responsesRequirementsContextKey{}, requirements)
		assert.False(t, service.semanticCacheRequestAllowed(ctx, router.Request{}))
	}
	service.WithGlobalAutomaticExclusions(&stubGlobalExclusionStore{byModel: map[string]string{catalog.ModelID(cacheTestModelB).String(): "disabled"}})
	assert.False(t, service.semanticCacheRequestAllowed(context.Background(), router.Request{}))
}
