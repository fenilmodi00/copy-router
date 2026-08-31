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

func TestTranslationPlan_NativeResponsesFiltersToOpenAIFamilyInShadow(t *testing.T) {
	svc := compatibilityService(TranslationCompatibilityShadow)
	plan := svc.planTranslation(router.Request{
		EnabledProviders: map[string]struct{}{
			providers.ProviderAiand: {},
		},
		TranslationRequirements: router.TranslationRequirements{
			SourceFormat: router.WireFormatOpenAI,
			Endpoint:     router.EndpointOpenAIResponses,
			CustomTools:  true,
			NativeOnly:   true,
		},
	})

	assert.Equal(t, map[string]struct{}{providers.ProviderAiand: {}}, plan.EnabledProviders)
	assert.Equal(t, providers.FamilyOpenAICompat, plan.TargetFamily)
	requireExclusion(t, plan, "native_wire_family_required", providers.ProviderAiand, true)
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

func TestTranslationPlan_OffRestoresNativeFamilyEligibility(t *testing.T) {
	plan := compatibilityService(TranslationCompatibilityOff).planTranslation(router.Request{
		EnabledProviders: map[string]struct{}{
			providers.ProviderAiand: {},
		},
		TranslationRequirements: router.TranslationRequirements{
			SourceFormat: router.WireFormatOpenAI,
			Endpoint:     router.EndpointOpenAIResponses,
			NativeOnly:   true,
		},
	})

	assert.Equal(t, map[string]struct{}{
		providers.ProviderAiand: {},
	}, plan.EnabledProviders)
	requireExclusion(t, plan, "native_wire_family_required", providers.ProviderAiand, false)
}

func TestTranslationPlan_NativeResponsesRequireOpenAIResponsesAdapter(t *testing.T) {
	svc := compatibilityService(TranslationCompatibilityShadow)
	plan := svc.planTranslation(router.Request{
		EnabledProviders: map[string]struct{}{
			providers.ProviderAiand:     {},
		},
		TranslationRequirements: router.TranslationRequirements{
			SourceFormat: router.WireFormatOpenAI,
			Endpoint:     router.EndpointOpenAIResponses,
			NativeOnly:   true,
		},
	})

	assert.Equal(t, map[string]struct{}{providers.ProviderAiand: {}}, plan.EnabledProviders)
	requireExclusion(t, plan, "native_wire_family_required", providers.ProviderAiand, true)
}

func TestTranslationPlan_BroadSemanticRequirementOnlyFiltersInEnforce(t *testing.T) {
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

	assert.Equal(t, req.EnabledProviders, shadow.EnabledProviders)
	requireExclusion(t, shadow, "prompt_cache_control_native_required", providers.ProviderAiand, false)
	assert.Equal(t, map[string]struct{}{providers.ProviderAiand: {}}, enforce.EnabledProviders)
	requireExclusion(t, enforce, "prompt_cache_control_native_required", providers.ProviderAiand, true)
}

func TestApplyTranslationPlan_CompatibleButUnavailable(t *testing.T) {
	svc := &Service{
		providers:                    map[string]providers.Client{providers.ProviderAiand: nil},
		translationCompatibilityMode: TranslationCompatibilityShadow,
	}
	_, err := svc.applyTranslationPlan(context.Background(), router.Request{
		EnabledProviders: map[string]struct{}{providers.ProviderAiand: {}},
		TranslationRequirements: router.TranslationRequirements{
			SourceFormat: router.WireFormatOpenAI,
			Endpoint:     router.EndpointOpenAIResponses,
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
