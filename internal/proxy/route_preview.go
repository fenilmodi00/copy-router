package proxy

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"workweave/router/internal/billing"
	"workweave/router/internal/observability"
	"workweave/router/internal/providers"
	"workweave/router/internal/router"
	"workweave/router/internal/router/policy"
	"workweave/router/internal/translate"
)

// openAIRoutingRequest parses an OpenAI Chat Completions body into a router.Request
// using the same normalization the dispatch path applies (strip routing marker,
// canonicalize context-window variant tags, strip feedback footer, resolve force-model
// from model field + x-weave-force-model header). Never writes a session pin.
func (s *Service) openAIRoutingRequest(ctx context.Context, body []byte, headers http.Header) (router.Request, error) {
	log := observability.FromContext(ctx)
	cleanBody, err := stripRoutingMarkerFromMessages(body)
	if err != nil {
		return router.Request{}, fmt.Errorf("strip routing marker: %w", err)
	}
	if withoutFooter, footerErr := translate.StripFeedbackFooterFromMessages(cleanBody); footerErr != nil {
		log.Error("Failed to strip feedback footer from OpenAI route preview", "err", footerErr)
	} else {
		cleanBody = withoutFooter
	}
	if canonical, _, modelErr := translate.CanonicalizeModelInBody(cleanBody); modelErr != nil {
		log.Error("Failed to canonicalize model for OpenAI route preview", "err", modelErr)
	} else {
		cleanBody = canonical
	}

	env, err := translate.ParseOpenAI(cleanBody)
	if err != nil {
		return router.Request{}, fmt.Errorf("parse request: %w", err)
	}
	previewForceModel, err := previewForceModelFromRequest(headers, env)
	if err != nil {
		return router.Request{}, err
	}
	embedOnlyUser := s.ResolveEmbedOnlyUserMessage(ctx)
	features := env.RoutingFeatures(embedOnlyUser)
	promptText := features.PromptText
	if embedOnlyUser && features.OnlyUserMessageText != "" {
		promptText = features.OnlyUserMessageText
	}

	enabledProviders := s.enabledProvidersForRequest(ctx, providers.ProviderOpenAI, headers)
	if billing.SubscriptionOnlyFromContext(ctx) {
		enabledProviders = restrictToSubscriptionProviders(ctx, headers, enabledProviders)
	}
	outputReserve := contextWindowOutputReserve
	if features.MaxTokens > outputReserve {
		outputReserve = features.MaxTokens
	}
	excluded := s.excludeCodexOAuthOnlyModels(ctx, headers, enabledProviders, s.excludedModelsForRequest(ctx))
	excluded, _ = excludeContextOverflowModels(
		env.ContextOverflowTokenEstimate(),
		env.SignatureTokenSavings(),
		outputReserve,
		enabledProviders,
		excluded,
		s.availableModels,
	)

	organizationID, _ := ctx.Value(ExternalIDContextKey{}).(string)
	installationID := ""
	if id := installationIDFromContext(ctx); id != uuid.Nil {
		installationID = id.String()
	}
	return router.Request{
		RequestedModel:               features.Model,
		ForceModel:                   previewForceModel,
		EstimatedInputTokens:         features.Tokens,
		HasTools:                     features.HasTools,
		HasImages:                    features.HasImages,
		TranslationRequirements:      env.TranslationRequirements(router.EndpointOpenAIChat),
		ReasoningConfigurationSHA256: env.ReasoningConfigurationSHA256(),
		ToolConfigurationSHA256:      env.ToolConfigurationSHA256(),
		PromptText:                   promptText,
		ConversationMessages:         conversationMessagesForRouting(env),
		AvailableTools:               availableToolsForRouting(env),
		OrganizationID:               organizationID,
		InstallationID:               installationID,
		ClientSessionID:              clientSessionIDForRequest(ctx, env),
		EnabledProviders:             enabledProviders,
		CustomBindings:               s.customBindingsForRequest(ctx),
		ExcludedModels:               excluded,
		AllowedModels:                allowedModelsForRequest(ctx),
		PreferredModels:              s.preferredModelsForRequest(ctx),
		RoutingKnobs:                 routingKnobsForRequest(ctx),
	}, nil
}

// PreviewOpenAIRoute parses an OpenAI Chat Completions body and returns the
// routing decision without dispatching (playground route preview).
func (s *Service) PreviewOpenAIRoute(ctx context.Context, body []byte, headers http.Header) (router.Decision, error) {
	req, err := s.openAIRoutingRequest(ctx, body, headers)
	if err != nil {
		return router.Decision{}, err
	}
	return s.Route(ctx, req)
}

// anthropicRoutingRequest parses an Anthropic Messages body into a router.Request
// using the same normalization the dispatch path applies (strip routing marker,
// strip feedback footer, canonicalize context-window variant tags, resolve force-model
// from model field + x-weave-force-model header). Never writes a session pin.
func (s *Service) anthropicRoutingRequest(ctx context.Context, body []byte, headers http.Header) (router.Request, error) {
	log := observability.FromContext(ctx)
	cleanBody, err := stripRoutingMarkerFromMessages(body)
	if err != nil {
		return router.Request{}, fmt.Errorf("strip routing marker: %w", err)
	}
	if withoutFooter, footerErr := translate.StripFeedbackFooterFromMessages(cleanBody); footerErr != nil {
		log.Error("Failed to strip feedback footer from route preview", "err", footerErr)
	} else {
		cleanBody = withoutFooter
	}
	if canonical, _, modelErr := translate.CanonicalizeModelInBody(cleanBody); modelErr != nil {
		log.Error("Failed to canonicalize model for route preview", "err", modelErr)
	} else {
		cleanBody = canonical
	}

	env, err := translate.ParseAnthropic(cleanBody)
	if err != nil {
		return router.Request{}, fmt.Errorf("parse request: %w", err)
	}
	previewForceModel, err := previewForceModelFromRequest(headers, env)
	if err != nil {
		return router.Request{}, err
	}
	embedOnlyUser := s.ResolveEmbedOnlyUserMessage(ctx)
	features := env.RoutingFeatures(embedOnlyUser)
	promptText := features.PromptText
	if embedOnlyUser && features.OnlyUserMessageText != "" {
		promptText = features.OnlyUserMessageText
	}

	enabledProviders := s.enabledProvidersForRequest(ctx, providers.ProviderAnthropic, headers)
	if billing.SubscriptionOnlyFromContext(ctx) {
		enabledProviders = restrictToSubscriptionProviders(ctx, headers, enabledProviders)
	}
	outputReserve := contextWindowOutputReserve
	if features.MaxTokens > outputReserve {
		outputReserve = features.MaxTokens
	}
	excluded := s.excludeCodexOAuthOnlyModels(ctx, headers, enabledProviders, s.excludedModelsForRequest(ctx))
	excluded, _ = excludeContextOverflowModels(
		env.ContextOverflowTokenEstimate(),
		env.SignatureTokenSavings(),
		outputReserve,
		enabledProviders,
		excluded,
		s.availableModels,
	)

	organizationID, _ := ctx.Value(ExternalIDContextKey{}).(string)
	installationID := ""
	if id := installationIDFromContext(ctx); id != uuid.Nil {
		installationID = id.String()
	}
	return router.Request{
		RequestedModel:               features.Model,
		ForceModel:                   previewForceModel,
		EstimatedInputTokens:         features.Tokens,
		HasTools:                     features.HasTools,
		HasImages:                    features.HasImages,
		TranslationRequirements:      env.TranslationRequirements(router.EndpointAnthropicMessages),
		ReasoningConfigurationSHA256: env.ReasoningConfigurationSHA256(),
		ToolConfigurationSHA256:      env.ToolConfigurationSHA256(),
		PromptText:                   promptText,
		ConversationMessages:         conversationMessagesForRouting(env),
		AvailableTools:               availableToolsForRouting(env),
		OrganizationID:               organizationID,
		InstallationID:               installationID,
		ClientSessionID:              clientSessionIDForRequest(ctx, env),
		EnabledProviders:             enabledProviders,
		CustomBindings:               s.customBindingsForRequest(ctx),
		ExcludedModels:               excluded,
		AllowedModels:                allowedModelsForRequest(ctx),
		PreferredModels:              s.preferredModelsForRequest(ctx),
		RoutingKnobs:                 routingKnobsForRequest(ctx),
	}, nil
}

// PreviewAnthropicRoute evaluates an Anthropic request with the registered
// policy preview contract without dispatching or invoking serving lifecycle state.
func (s *Service) PreviewAnthropicRoute(ctx context.Context, body []byte, headers http.Header) (policy.PreviewResult, error) {
	req, err := s.anthropicRoutingRequest(ctx, body, headers)
	if err != nil {
		return policy.PreviewResult{}, err
	}
	req, err = s.applyTranslationPlan(ctx, req)
	if err != nil {
		return policy.PreviewResult{}, err
	}
	req = s.withPolicyRequestContext(ctx, req)
	strategy := router.StrategyFromContext(ctx)
	registered, ok := s.strategies[strategy]
	if !ok || registered.router == nil {
		return policy.PreviewResult{}, fmt.Errorf("strategy %q requested but no router configured: %w", strategy, defaultStrategyUnavailable(strategy))
	}
	previewer, ok := registered.router.(policy.RoutePreviewer)
	if !ok {
		return policy.PreviewResult{}, fmt.Errorf("strategy %q has no route preview contract: %w", strategy, registered.unavailable)
	}
	return previewer.PreviewRoute(ctx, req)
}
