package proxy

import (
	"context"
	"errors"
	"testing"

	"workweave/router/internal/providers"
	"workweave/router/internal/router"

	"github.com/stretchr/testify/assert"
)

func compatibilityService(mode TranslationCompatibilityMode) *Service {
	return &Service{
		providers: map[string]providers.Client{
			providers.ProviderAiand: nil,
		},
		translationCompatibilityMode: mode,
	}
}

func TestTranslationPlan_OpenAIFamilyEligibilityInShadow(t *testing.T) {
	svc := compatibilityService(TranslationCompatibilityShadow)
	plan := svc.planTranslation(router.Request{
		EnabledProviders: map[string]struct{}{
			providers.ProviderAiand: {},
			"upstream-fallback":     {},
		},
		TranslationRequirements: router.TranslationRequirements{
			SourceFormat: router.WireFormatOpenAI,
			Endpoint:     router.EndpointOpenAIResponses,
			CustomTools:  true,
			NativeOnly:   true,
		},
	})

	// NativeOnly requirements are enforced even in shadow mode: eligibility is
	// family-based, aiand (FamilyOpenAICompat) stays enabled, and the
	// family-unknown fake upstream is hard-excluded with an enforced
	// exclusion instead of silently scoring as a sibling.
	assert.Equal(t, map[string]struct{}{providers.ProviderAiand: {}}, plan.EnabledProviders)
	requireExclusion(t, plan, "native_wire_family_required", "upstream-fallback", true)
}

func TestTranslationPlan_ImageConstraintShadowsBeforeEnforcement(t *testing.T) {
	req := router.Request{TranslationRequirements: router.TranslationRequirements{Images: true}}
	shadow := compatibilityService(TranslationCompatibilityShadow).planTranslation(req)
	enforce := compatibilityService(TranslationCompatibilityEnforce).planTranslation(req)

	assert.Empty(t, shadow.ExcludedModels, "shadow mode must preserve the pre-change candidate set")
	_, shadowReported := shadow.ExcludedModels["deepseek-ai/deepseek-v4-flash"]
	assert.False(t, shadowReported)
	_, enforced := enforce.ExcludedModels["deepseek-ai/deepseek-v4-flash"]
	assert.True(t, enforced, "known text-only models are hard excluded in enforce mode")
}

func TestTranslationPlan_OffRestoresFamilyEligibility(t *testing.T) {
	plan := compatibilityService(TranslationCompatibilityOff).planTranslation(router.Request{
		EnabledProviders: map[string]struct{}{
			providers.ProviderAiand: {},
			"upstream-fallback":     {},
		},
		TranslationRequirements: router.TranslationRequirements{
			SourceFormat: router.WireFormatOpenAI,
			Endpoint:     router.EndpointOpenAIResponses,
			NativeOnly:   true,
		},
	})

	// In off mode even native-only requirements stay diagnostic only: every
	// candidate remains eligible for scoring, with the ineligibility reported
	// as a non-enforced exclusion.
	assert.Equal(t, map[string]struct{}{
		providers.ProviderAiand: {},
		"upstream-fallback":     {},
	}, plan.EnabledProviders)
	requireExclusion(t, plan, "native_wire_family_required", "upstream-fallback", false)
}

func TestTranslationPlan_AnthropicFamilyEligibilityInShadow(t *testing.T) {
	svc := compatibilityService(TranslationCompatibilityShadow)
	// The Anthropic native arm defines a preserving target (the Anthropic
	// family) but no anthropic-family provider is registered: NativeOnly
	// requirements are enforced even in shadow mode, so the aiand candidate
	// (FamilyOpenAICompat) is hard-excluded and no provider remains eligible.
	plan := svc.planTranslation(router.Request{
		EnabledProviders: map[string]struct{}{
			providers.ProviderAiand: {},
		},
		TranslationRequirements: router.TranslationRequirements{
			SourceFormat: router.WireFormatAnthropic,
			Endpoint:     router.EndpointAnthropicMessages,
			NativeOnly:   true,
		},
	})

	assert.Equal(t, providers.FamilyAnthropic, plan.TargetFamily)
	assert.Empty(t, plan.EnabledProviders)
	requireExclusion(t, plan, "native_wire_family_required", providers.ProviderAiand, true)
}

func TestTranslationPlan_UnknownProviderNeverResponsesEligible(t *testing.T) {
	// A provider string outside the family registry (plain fake upstream key)
	// is FamilyUnknown, so the OpenAI wire family reports it as ineligible.
	plan := compatibilityService(TranslationCompatibilityShadow).planTranslation(router.Request{
		EnabledProviders: map[string]struct{}{
			"upstream-fallback": {},
		},
		TranslationRequirements: router.TranslationRequirements{
			SourceFormat: router.WireFormatOpenAI,
			Endpoint:     router.EndpointOpenAIChat,
			NativeOnly:   true,
		},
	})

	requireExclusion(t, plan, "native_wire_family_required", "upstream-fallback", true)
}

func TestTranslationPlan_AnthropicNativeConstraintReportsWithoutFiltering(t *testing.T) {
	req := router.Request{
		EnabledProviders: map[string]struct{}{
			providers.ProviderAiand: {},
		},
		TranslationRequirements: router.TranslationRequirements{
			SourceFormat:       router.WireFormatAnthropic,
			Endpoint:           router.EndpointAnthropicMessages,
			PromptCacheControl: true,
		},
	}
	shadow := compatibilityService(TranslationCompatibilityShadow).planTranslation(req)
	enforce := compatibilityService(TranslationCompatibilityEnforce).planTranslation(req)

	// The Anthropic native arm keeps prompt-cache-control diagnostic in shadow
	// mode (no anthropic-family provider is registered; /v1/messages serves
	// cross-format), and in enforce mode it hard-filters every candidate —
	// the aiand candidate is excluded, leaving no eligible provider.
	assert.Equal(t, req.EnabledProviders, shadow.EnabledProviders)
	requireExclusion(t, shadow, "prompt_cache_control_native_required", providers.ProviderAiand, false)
	assert.Empty(t, enforce.EnabledProviders)
	plan := enforce
	requireExclusion(t, plan, "prompt_cache_control_native_required", providers.ProviderAiand, true)
}

func TestApplyTranslationPlan_CompatibleFamilyUnavailable(t *testing.T) {
	// The Anthropic native arm defines a preserving target (the Anthropic
	// family) but no anthropic-family provider is registered, so every
	// candidate is filtered in enforce mode and the plan reports the
	// compatible-but-unavailable error instead of an intrinsic incompatibility.
	svc := compatibilityService(TranslationCompatibilityEnforce)
	_, err := svc.applyTranslationPlan(context.Background(), router.Request{
		EnabledProviders: map[string]struct{}{providers.ProviderAiand: {}},
		TranslationRequirements: router.TranslationRequirements{
			SourceFormat: router.WireFormatAnthropic,
			Endpoint:     router.EndpointAnthropicMessages,
			NativeOnly:   true,
		},
	})

	assert.ErrorIs(t, err, ErrTranslationCompatibleProviderUnavailable)
}

func TestApplyTranslationPlan_IntrinsicallyIncompatible(t *testing.T) {
	svc := compatibilityService(TranslationCompatibilityEnforce)
	_, err := svc.applyTranslationPlan(context.Background(), router.Request{
		TranslationRequirements: router.TranslationRequirements{NativeOnly: true},
	})

	assert.True(t, errors.Is(err, ErrTranslationIntrinsicallyIncompatible))
}

func requireExclusion(t *testing.T, plan TranslationPlan, code, provider string, enforced bool) {
	t.Helper()
	for _, exclusion := range plan.Exclusions {
		if exclusion.Code == code && exclusion.Provider == provider {
			assert.Equal(t, enforced, exclusion.Enforced)
			return
		}
	}
	t.Fatalf("missing exclusion code=%q provider=%q in %#v", code, provider, plan.Exclusions)
}
